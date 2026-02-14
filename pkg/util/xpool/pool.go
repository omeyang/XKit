package xpool

import (
	"log/slog"
	"sync"
)

// Option 定义 WorkerPool 可选配置函数类型。
type Option[T any] func(*WorkerPool[T])

// WithLogger 设置自定义日志记录器。
// 默认使用 slog.Default()。
func WithLogger[T any](logger *slog.Logger) Option[T] {
	return func(p *WorkerPool[T]) {
		if logger != nil {
			p.logger = logger
		}
	}
}

// WorkerPool 是一个泛型 worker pool 实现。
// 用于异步执行任务，支持优雅关闭和 panic 恢复。
type WorkerPool[T any] struct {
	workers   int
	queueSize int
	handler   func(T)
	queue     chan T
	wg        sync.WaitGroup
	stopOnce  sync.Once
	submitMu  sync.RWMutex // 保护 closed 字段和 queue 发送操作
	closed    bool
	logger    *slog.Logger
}

// NewWorkerPool 创建并启动 worker pool。
//
// 参数：
//   - workers: worker 数量，最小为 1
//   - queueSize: 任务队列大小，最小为 1（默认 100）
//   - handler: 任务处理函数，不能为 nil
//
// 如果 handler 为 nil，返回 ErrNilHandler。
func NewWorkerPool[T any](workers, queueSize int, handler func(T), opts ...Option[T]) (*WorkerPool[T], error) {
	if handler == nil {
		return nil, ErrNilHandler
	}
	if workers < 1 {
		workers = 1
	}
	if queueSize < 1 {
		queueSize = 100
	}

	p := &WorkerPool[T]{
		workers:   workers,
		queueSize: queueSize,
		handler:   handler,
		queue:     make(chan T, queueSize),
		logger:    slog.Default(),
	}

	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}

	// 自动启动 worker
	for range p.workers {
		p.wg.Add(1)
		go p.worker()
	}

	return p, nil
}

// worker 是工作协程。
// 从 queue 中读取任务直到 channel 关闭（优雅关闭）。
func (p *WorkerPool[T]) worker() {
	defer p.wg.Done()
	for task := range p.queue {
		p.safeHandle(task)
	}
}

// safeHandle 安全执行 handler，捕获 panic。
func (p *WorkerPool[T]) safeHandle(task T) {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("xpool: worker panic recovered", "panic", r)
		}
	}()
	p.handler(task)
}

// Submit 提交任务到 worker pool。
// 如果队列满，返回 ErrQueueFull。
// 如果 pool 已停止，返回 ErrPoolStopped。
func (p *WorkerPool[T]) Submit(task T) error {
	p.submitMu.RLock()
	defer p.submitMu.RUnlock()

	if p.closed {
		return ErrPoolStopped
	}

	select {
	case p.queue <- task:
		return nil
	default:
		p.logger.Warn("xpool: async queue full, task dropped")
		return ErrQueueFull
	}
}

// Stop 停止 worker pool。
// 会等待队列中所有剩余任务处理完成后再退出（优雅关闭）。
// 该方法是幂等的：多次调用只会执行一次关闭。
func (p *WorkerPool[T]) Stop() {
	p.stopOnce.Do(func() {
		// 1. 获取写锁，标记为已关闭
		// 写锁等待所有 Submit 的读锁释放后才能获取，
		// 确保所有进行中的 Submit 完成后才关闭 queue。
		p.submitMu.Lock()
		p.closed = true
		p.submitMu.Unlock()

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
