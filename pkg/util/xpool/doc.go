// Package xpool 提供通用的 worker pool 实现。
//
// WorkerPool 是一个轻量级的泛型 worker pool，用于异步执行任务。
// 支持以下特性：
//   - 泛型任务类型
//   - 可配置的 worker 数量和队列大小
//   - 优雅关闭（处理完队列中的任务后退出）
//   - panic 恢复（单个任务失败不影响 pool）
//   - 队列满时丢弃任务并记录日志
//
// # 基本用法
//
//	pool := xpool.NewWorkerPool(4, 100, func(task Task) {
//	    // 处理任务
//	})
//	pool.Start()
//	defer pool.Stop()
//
//	pool.Submit(task)
//
// # 注意事项
//
//   - Submit 是非阻塞的，队列满时会丢弃任务
//   - Stop 会等待所有队列中的任务处理完成
//   - 任务处理器应该是幂等的或可安全重试的
//   - Start 是幂等的，多次调用只会启动一次
//   - handler 参数不能为 nil，否则会 panic
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
