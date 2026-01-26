package xpool

import (
	"log/slog"
	"sync"
)

// WorkerPool 是一个泛型 worker pool 实现。
// 用于异步执行任务，支持优雅关闭和 panic 恢复。
type WorkerPool[T any] struct {
	workers   int
	queueSize int
	handler   func(T)
	queue     chan T
	wg        sync.WaitGroup
	stopOnce  sync.Once
	stopped   chan struct{}
	started   bool       // 是否已启动
	startMu   sync.Mutex // 保护 started 字段
}

// NewWorkerPool 创建 worker pool。
//
// 参数：
//   - workers: worker 数量，最小为 1
//   - queueSize: 任务队列大小，最小为 1（默认 100）
//   - handler: 任务处理函数，不能为 nil
//
// 如果 handler 为 nil，会 panic。
func NewWorkerPool[T any](workers, queueSize int, handler func(T)) *WorkerPool[T] {
	if handler == nil {
		panic("xpool: handler cannot be nil")
	}
	if workers < 1 {
		workers = 1
	}
	if queueSize < 1 {
		queueSize = 100
	}
	return &WorkerPool[T]{
		workers:   workers,
		queueSize: queueSize,
		handler:   handler,
		queue:     make(chan T, queueSize),
		stopped:   make(chan struct{}),
	}
}

// Start 启动 worker pool。
// 该方法是幂等的：多次调用只会启动一次 worker。
func (p *WorkerPool[T]) Start() {
	p.startMu.Lock()
	defer p.startMu.Unlock()

	if p.started {
		return // 幂等：已启动则直接返回
	}
	p.started = true

	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

// worker 是工作协程。
// 只从 queue 中读取任务，不检查 stopped 信号。
// 这确保在 Stop() 时能处理完队列中的剩余任务（优雅关闭）。
func (p *WorkerPool[T]) worker() {
	defer p.wg.Done()
	// 从 queue 读取直到 channel 关闭
	for task := range p.queue {
		// 安全执行 handler，捕获 panic
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("xpool: worker panic recovered", "panic", r)
				}
			}()
			p.handler(task)
		}()
	}
}

// Submit 提交任务到 worker pool。
// 如果队列满，任务将被丢弃并记录日志。
// 如果 pool 已停止，返回 false。
func (p *WorkerPool[T]) Submit(task T) (ok bool) {
	// 使用 recover 捕获 Stop() 和 Submit() 并发时可能的 send on closed channel panic。
	// 这种情况发生在 Stop() 关闭 p.stopped 后、关闭 p.queue 前的极短时间窗口内，
	// Submit 的 select 恰好选中了 p.queue <- task 分支。
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()

	select {
	case <-p.stopped:
		return false
	case p.queue <- task:
		return true
	default:
		// 队列满，丢弃任务
		slog.Warn("xpool: async queue full, task dropped")
		return false
	}
}

// Stop 停止 worker pool。
// 会等待队列中所有剩余任务处理完成后再退出（优雅关闭）。
func (p *WorkerPool[T]) Stop() {
	p.stopOnce.Do(func() {
		// 1. 先标记为已停止，拒绝新任务提交
		close(p.stopped)
		// 2. 关闭队列，让 worker 退出循环
		close(p.queue)
		// 3. 等待所有 worker 处理完剩余任务后退出
		p.wg.Wait()
	})
}

// Workers 返回 worker 数量。
func (p *WorkerPool[T]) Workers() int {
	return p.workers
}

// QueueSize 返回队列大小。
func (p *WorkerPool[T]) QueueSize() int {
	return p.queueSize
}
