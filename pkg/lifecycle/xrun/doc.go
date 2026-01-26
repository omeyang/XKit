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
// # 快速开始
//
// 最简单的用法：
//
//	err := xrun.Run(context.Background(),
//	    func(ctx context.Context) error {
//	        server := &http.Server{Addr: ":8080"}
//	        go func() {
//	            <-ctx.Done()
//	            server.Shutdown(context.Background())
//	        }()
//	        return server.ListenAndServe()
//	    },
//	)
//
// 使用 HTTPServer 辅助函数：
//
//	server := &http.Server{Addr: ":8080", Handler: mux}
//	err := xrun.Run(ctx, xrun.HTTPServer(server, 10*time.Second))
//
// 使用 Group 管理多个服务：
//
//	g, ctx := xrun.NewGroup(ctx, xrun.WithName("my-service"))
//
//	// 添加 HTTP 服务器
//	g.Go(xrun.HTTPServer(httpServer, 10*time.Second))
//
//	// 添加自定义服务
//	g.Go(func(ctx context.Context) error {
//	    for {
//	        select {
//	        case <-ctx.Done():
//	            return ctx.Err()
//	        case msg := <-msgChan:
//	            process(msg)
//	        }
//	    }
//	})
//
//	// 等待所有服务退出
//	if err := g.Wait(); err != nil {
//	    log.Fatal(err)
//	}
//
// # Service 接口
//
// 实现 Service 接口的服务可以使用 RunServices 统一管理：
//
//	type MyService struct{}
//
//	func (s *MyService) Run(ctx context.Context) error {
//	    // 监听 ctx.Done() 以响应关闭
//	    <-ctx.Done()
//	    return ctx.Err()
//	}
//
//	err := xrun.RunServices(ctx, &MyService{}, httpServer, grpcServer)
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
//
// 信号退出示例：
//
//	err := xrun.Run(ctx, myService)
//	var sigErr *xrun.SignalError
//	if errors.As(err, &sigErr) {
//	    log.Printf("received signal: %v", sigErr.Signal)
//	}
//
// # 设计决策
//
// 1. 基于 context 的协调：所有服务通过 context 感知取消信号，
//    符合 Go 的惯用并发模式。使用 context.WithCancelCause 保留取消原因。
//
// 2. errgroup 单错误语义：errgroup.Wait() 仅返回第一个非 nil 错误。
//    这是有意为之——当第一个服务失败时，其他服务会通过 context 取消收到通知。
//    如果需要收集所有错误，应在服务内部使用日志记录。
//
// 3. 无全局关闭钩子：xrun 不提供 OnShutdown 等全局回调注册机制。
//    关闭逻辑应内聚在各服务的 ctx.Done() 处理中，而非分散在外部回调。
//    这避免了回调排序、错误传播等复杂性，保持架构清晰。
//
// 4. 信号处理：Run/RunServices/RunWithOptions/RunServicesWithOptions 自动注册信号监听
//    （默认 SIGHUP、SIGINT、SIGTERM、SIGQUIT），收到信号时通过
//    Cancel(&SignalError{Signal: sig}) 传播退出原因。
//    可通过 WithSignals 自定义监听的信号列表，
//    或通过 WithoutSignalHandler 完全禁用自动信号处理。
//    直接使用 NewGroup 时不包含信号处理，需要自行管理。
//
// 5. DefaultSignals 为函数：DefaultSignals() 返回新切片而非暴露全局变量，
//    防止外部代码意外修改默认信号列表。
//
// 6. YAGNI 原则：不预定义未使用的错误类型（如 ShutdownTimeoutError）。
//    关闭超时由调用方通过 context.WithTimeout 自行管理。
//
// 7. context.Canceled 过滤策略：Wait() 使用 causeCtx（独立于 errgroup context）
//    判断 context.Canceled 的来源。当 causeCtx 未被取消时，说明
//    context.Canceled 来自服务内部逻辑（如 gRPC 调用），不应被过滤。
//    当 causeCtx 被取消时（通过 Cancel() 或父 context），按原逻辑过滤。
//
// 8. HTTPServer 关闭错误传播：HTTPServer 辅助函数通过 buffered channel
//    传递 Shutdown() 的返回值。当 ListenAndServe 返回 http.ErrServerClosed
//    （正常关闭）时，等待 Shutdown 完成并返回其错误（如有）。
//    这确保关闭超时等错误不会被静默吞掉。
//
// 9. Ticker 慢执行监控：当前版本的 Ticker 不包含慢执行告警机制。
//    如需监控 tick 函数的执行时长，建议在 fn 内部使用
//    xmetrics.Timer 或类似工具自行记录。这符合 YAGNI 原则——
//    监控策略因业务而异，不宜内置通用方案。
//
// [errgroup]: https://pkg.go.dev/golang.org/x/sync/errgroup
package xrun
