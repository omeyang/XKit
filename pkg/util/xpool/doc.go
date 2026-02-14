// Package xpool 提供通用的 worker pool 实现。
//
// WorkerPool 是一个轻量级的泛型 worker pool，用于异步执行任务。
// 支持以下特性：
//   - 泛型任务类型
//   - 可配置的 worker 数量和队列大小
//   - 优雅关闭（处理完队列中的任务后退出）
//   - panic 恢复（单个任务失败不影响 pool）
//   - 队列满时丢弃任务并返回 ErrQueueFull
//   - 可注入自定义日志记录器
//
// # 注意事项
//
//   - Submit 是非阻塞的，队列满时返回 ErrQueueFull
//   - Stop 会等待所有队列中的任务处理完成
//   - 任务处理器应该是幂等的或可安全重试的
//   - NewWorkerPool 创建后自动启动 worker，无需手动 Start
//   - handler 参数不能为 nil，否则返回 ErrNilHandler
//
// # 设计选择说明
//
// Submit 队列满时丢弃任务：
//   - 这是有意设计，确保 Submit 永不阻塞
//   - 适用于日志、metrics 等可丢弃场景
//   - 如需阻塞语义，请使用 channel 或其他库
//
// Stop 等待所有任务完成：
//   - 这是优雅关闭的标准行为
//   - 如需超时，调用方应使用 context 或 select
package xpool
