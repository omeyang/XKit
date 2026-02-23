// Package xrun 提供基于 errgroup + context 的进程生命周期管理。
//
// # 概述
//
// xrun 基于 Go 官方扩展库 [errgroup] 构建，提供：
//   - 多服务并发运行和协调关闭
//   - 信号处理（SIGINT、SIGTERM 等）
//   - 结构化日志记录
//   - HTTP Server 等常见服务的封装
//
// # 核心概念
//
// 基于 context 的协调：当任一服务返回错误或收到终止信号时，
// context 会被取消，所有服务应该监听 ctx.Done() 并优雅退出。
//
// # 与 Kubernetes 配合
//
// Kubernetes 会在 Pod 终止前发送 SIGTERM 信号。
// xrun.Run 默认监听此信号并触发优雅关闭。
//
// 建议：
//   - 服务的关闭逻辑应该在 terminationGracePeriodSeconds（默认 30s）内完成
//   - 长时间运行的任务应该定期检查 ctx.Done()
//
// # 错误处理
//
// Wait() 的错误处理遵循以下规则：
//   - 如果服务返回非 nil、非 context.Canceled 的错误，Wait() 直接返回该错误
//   - 如果错误是 context.Canceled，Wait() 通过 causeCtx 判断取消来源：
//   - 如果 Group 被主动取消（Cancel() 或父 context 取消），检查 cause 并过滤
//   - 如果 context.Canceled 来自服务内部（causeCtx 未被取消），不过滤，直接返回
//   - 当 Group 被取消且有显式 cause 时（如 SignalError），Wait() 返回该 cause
//   - 当 Group 被取消且无显式 cause 时，Wait() 返回 nil
//   - 即使所有服务返回 nil，Cancel(cause) 设置的显式 cause 仍会被返回
//
// # 设计决策
//
//  1. 基于 context 的协调：所有服务通过 context 感知取消信号，
//     符合 Go 的惯用并发模式。使用 context.WithCancelCause 保留取消原因。
//
//  2. errgroup 单错误语义：errgroup.Wait() 仅返回第一个非 nil 错误。
//     这是有意为之——当第一个服务失败时，其他服务会通过 context 取消收到通知。
//     如果需要收集所有错误，应在服务内部使用日志记录。
//
//  3. 无全局关闭钩子：xrun 不提供 OnShutdown 等全局回调注册机制。
//     关闭逻辑应内聚在各服务的 ctx.Done() 处理中，而非分散在外部回调。
//     这避免了回调排序、错误传播等复杂性，保持架构清晰。
//
//  4. 信号处理：Run/RunServices/RunWithOptions/RunServicesWithOptions 自动注册信号监听
//     （默认 SIGHUP、SIGINT、SIGTERM、SIGQUIT），收到信号时通过
//     Cancel(&SignalError{Signal: sig}) 传播退出原因。
//     可通过 WithSignals 自定义监听的信号列表，
//     或通过 WithoutSignalHandler 完全禁用自动信号处理。
//     直接使用 NewGroup 时不包含信号处理，需要自行管理。
//
//  5. DefaultSignals 为函数：DefaultSignals() 返回新切片而非暴露全局变量，
//     防止外部代码意外修改默认信号列表。
//
//  6. YAGNI 原则：不预定义未使用的错误类型（如 ShutdownTimeoutError）。
//     关闭超时由调用方通过 context.WithTimeout 自行管理。
//
//  7. context.Canceled 过滤策略：Wait() 使用 causeCtx（独立于 errgroup context）
//     判断 context.Canceled 的来源。当 causeCtx 未被取消时，说明
//     context.Canceled 来自服务内部逻辑（如 gRPC 调用），不应被过滤。
//     当 causeCtx 被取消时（通过 Cancel() 或父 context），按原逻辑过滤。
//
//  8. HTTPServer 关闭错误传播：HTTPServer 辅助函数通过 buffered channel
//     传递 Shutdown() 的返回值。当 ListenAndServe 返回 http.ErrServerClosed
//     （正常关闭）时，通过三路 select 区分关闭来源：ctx 驱动的关闭等待
//     shutdown 结果，外部直接调用 server.Shutdown/Close 时立即返回 nil
//     并通知 shutdown goroutine 退出，防止 goroutine 泄漏。
//     这确保关闭超时等错误不会被静默吞掉，同时保证函数始终能返回。
//
//  9. Ticker 输入校验：Ticker 的 interval 参数必须为正数，
//     否则返回的服务函数会返回 ErrInvalidInterval（fail-fast）。
//     这防止 time.NewTicker 在运行时 panic。
//
//  10. Ticker 慢执行监控：当前版本的 Ticker 不包含慢执行告警机制。
//     如需监控 tick 函数的执行时长，建议在 fn 内部使用
//     xmetrics.Timer 或类似工具自行记录。这符合 YAGNI 原则——
//     监控策略因业务而异，不宜内置通用方案。
//
//  11. Timer 输入校验：Timer 的 delay 参数不能为负数，
//     否则返回 ErrInvalidDelay（与 Ticker 的 ErrInvalidInterval 对齐）。
//     delay == 0 是有效用例（立即执行），而 Ticker 的 interval == 0
//     会导致 time.NewTicker panic，因此边界值不同。
//
//  12. 空信号列表回退：WithSignals([]os.Signal{}) 等同于 nil，
//     使用默认信号列表。这避免 signal.Notify(ch) 无参调用订阅所有信号的语义陷阱。
//     如需禁用信号处理，应使用 WithoutSignalHandler()。
//
//  13. WithSignals 防御性拷贝：WithSignals 在创建时拷贝输入切片，
//     避免调用方后续修改切片导致配置漂移或并发数据竞争。
//
// [errgroup]: https://pkg.go.dev/golang.org/x/sync/errgroup
package xrun
