// Package xpool 提供通用的 worker pool 实现。
//
// Pool 是一个轻量级的泛型 worker pool，用于异步执行任务。
// 支持以下特性：
//   - 泛型任务类型
//   - 可配置的 worker 数量（[1, 65536]）和队列大小（[1, 16777216]）
//   - 优雅关闭（处理完队列中的任务后退出）
//   - 超时关闭（Shutdown(ctx) 支持 context 超时/取消）
//   - panic 恢复（单个任务失败不影响 pool，含堆栈跟踪日志）
//   - 队列满时返回 ErrQueueFull
//   - Done() channel：Shutdown 超时返回后可等待残留 worker 最终完成
//   - 可注入自定义日志记录器（WithLogger）
//   - 多实例场景下可设置名称以区分日志来源（WithName）
//   - panic 日志默认安全（仅记录 task 类型），可通过 WithLogTaskValue 启用完整值输出
//
// # 注意事项
//
//   - Submit 是非阻塞的，队列满时返回 ErrQueueFull
//   - Close 会等待所有队列中的任务处理完成
//   - Close/Shutdown 不可在 handler 内调用，否则会死锁
//   - 任务处理器应设计为幂等的，因为同一逻辑任务可能被多次提交
//   - panic 的任务不会被重试——仅记录日志后丢弃；
//     panic 恢复日志默认仅记录 task 类型（避免敏感信息泄露），
//     如需记录完整 task 值以便调试，可通过 WithLogTaskValue() 显式启用
//   - New 创建后自动启动 worker，无需手动 Start
//   - handler 参数不能为 nil，否则返回 ErrNilHandler
//   - workers 和 queueSize 超出有效范围时返回错误（而非 panic）
//   - Shutdown(nil) 返回 ErrNilContext（而非 panic），与项目其他包一致
//
// # 关闭策略
//
// Close 等价于 Shutdown(context.Background())，无限等待所有任务完成。
// Shutdown(ctx) 支持超时控制：ctx 到期后立即返回 context 错误，
// 残留的 worker goroutine 仍在后台运行，会继续处理剩余任务直到耗尽后退出。
// 调用方可通过 Done() 返回的 channel 等待所有 worker 最终完成。
//
// # 设计选择说明
//
// 设计决策: New 返回 *Pool[T] 而非接口：
//   - 虽然 Go 支持参数化接口（如 type Pool[T any] interface{...}），
//     但 xpool 作为轻量级工具包，不需要多实现替换，返回具体类型更简洁
//   - 编译期通过 io.Closer 断言确保关闭契约
//
// Submit 队列满时返回 ErrQueueFull：
//   - 这是有意设计，确保 Submit 永不阻塞
//   - 适用于日志、metrics 等可丢弃场景
//   - 如需阻塞语义，请使用 channel 或其他库
package xpool
