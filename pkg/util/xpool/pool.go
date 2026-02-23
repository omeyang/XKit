package xpool

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

const (
	maxWorkers   = 1 << 16 // 65536
	maxQueueSize = 1 << 24 // 16777216
)

// closedCh 是一个预关闭的 channel，用于零值 Pool 的 Done() 返回值。
var closedCh = func() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}()

// Pool 是一个泛型 worker pool 实现。
// 用于异步执行任务，支持优雅关闭和 panic 恢复。
type Pool[T any] struct {
	workers     int
	queueSize   int
	handler     func(T)
	queue       chan T
	wg          sync.WaitGroup
	submitMu    sync.RWMutex // 保护 queue 发送操作，防止 send-on-closed-channel
	closed      atomic.Bool
	workersDone chan struct{} // 所有 worker 退出后关闭
	opts        options
}

// New 创建并启动 worker pool。
//
// 参数：
//   - workers: worker 数量，必须在 [1, 65536] 范围内，否则返回 [ErrInvalidWorkers]
//   - queueSize: 任务队列大小，必须在 [1, 16777216] 范围内，否则返回 [ErrInvalidQueueSize]
//   - handler: 任务处理函数，不能为 nil，否则返回 [ErrNilHandler]
func New[T any](workers, queueSize int, handler func(T), opts ...Option) (*Pool[T], error) {
	if handler == nil {
		return nil, ErrNilHandler
	}
	if workers < 1 || workers > maxWorkers {
		return nil, fmt.Errorf("%w: got %d, must be in [1, %d]", ErrInvalidWorkers, workers, maxWorkers)
	}
	if queueSize < 1 || queueSize > maxQueueSize {
		return nil, fmt.Errorf("%w: got %d, must be in [1, %d]", ErrInvalidQueueSize, queueSize, maxQueueSize)
	}

	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	p := &Pool[T]{
		workers:     workers,
		queueSize:   queueSize,
		handler:     handler,
		queue:       make(chan T, queueSize),
		workersDone: make(chan struct{}),
		opts:        o,
	}

	for range p.workers {
		p.wg.Add(1)
		go p.worker()
	}

	return p, nil
}

// worker 是工作协程。
// 从 queue 中读取任务直到 channel 关闭（优雅关闭）。
func (p *Pool[T]) worker() {
	defer p.wg.Done()
	for task := range p.queue {
		p.safeHandle(task)
	}
}

// safeHandle 安全执行 handler，捕获 panic 并记录堆栈。
//
// 设计决策: panic 恢复日志默认仅记录 task 类型（如 "int"、"*MyStruct"），
// 避免泛型 T 可能包含的敏感信息（密码、Token 等）泄露到日志系统。
// 如需在 panic 日志中包含完整 task 值以便调试，可通过 WithLogTaskValue() 显式启用。
func (p *Pool[T]) safeHandle(task T) {
	defer func() {
		if r := recover(); r != nil {
			attrs := make([]slog.Attr, 0, 4) // 预分配：panic + stack + task/task_type + pool（可选）
			attrs = append(attrs,
				slog.Any("panic", r),
				slog.String("stack", string(debug.Stack())),
			)
			if p.opts.logTaskValue {
				attrs = append(attrs, slog.Any("task", task))
			} else {
				attrs = append(attrs, slog.String("task_type", fmt.Sprintf("%T", task)))
			}
			if p.opts.name != "" {
				attrs = append(attrs, slog.String("pool", p.opts.name))
			}
			p.opts.logger.LogAttrs(context.Background(), slog.LevelError,
				"xpool: worker panic recovered", attrs...)
		}
	}()
	p.handler(task)
}

// Submit 提交任务到 worker pool。
// 如果队列满，返回 [ErrQueueFull]（非阻塞，不记录日志——由调用方决定处理方式）。
// 如果 pool 已关闭或未初始化，返回 [ErrPoolStopped]。
func (p *Pool[T]) Submit(task T) error {
	p.submitMu.RLock()
	defer p.submitMu.RUnlock()

	if p.closed.Load() || p.queue == nil {
		return ErrPoolStopped
	}

	select {
	case p.queue <- task:
		return nil
	default:
		return ErrQueueFull
	}
}

// Shutdown 优雅关闭 worker pool，支持 context 超时/取消控制。
// 会等待队列中所有剩余任务处理完成，或 ctx 到期后返回对应错误。
// 首次调用返回 nil（或 ctx 错误），后续调用返回 [ErrPoolStopped]。
// ctx 不得为 nil，否则返回 [ErrNilContext]。
//
// 注意事项：
//   - 不可在 handler 内调用，否则会死锁
//   - handler 不应无限阻塞；若 handler 因外部依赖永久阻塞，
//     对应的 worker goroutine 将无法退出，导致资源泄漏。
//     如需支持取消，可在 T 中嵌入 context.Context 或使用闭包捕获取消信号
//   - ctx 超时返回后，残留的 worker goroutine 仍在后台运行，
//     会继续处理队列中的剩余任务直到耗尽后自行退出；
//     可通过 [Pool.Done] 等待所有 worker 最终完成
func (p *Pool[T]) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}
	if !p.closed.CompareAndSwap(false, true) {
		return ErrPoolStopped
	}
	// 零值 Pool（未通过 New 创建）无 queue、无 worker，直接返回。
	if p.queue == nil {
		return nil
	}
	// 写锁等待所有进行中的 Submit（持有读锁）完成后才能获取，
	// 确保 close(p.queue) 时不会出现 send-on-closed-channel。
	p.submitMu.Lock()
	close(p.queue)
	p.submitMu.Unlock()

	// 设计决策: 监听 goroutine 的生命周期与 worker 一致——当所有 worker 退出后自动结束。
	// 若 handler 永久阻塞导致 worker 无法退出，此 goroutine 也会阻塞，
	// 但这是 handler 违反契约（不应永久阻塞）的结果，而非 pool 的设计缺陷。
	go func() {
		p.wg.Wait()
		close(p.workersDone)
	}()

	select {
	case <-p.workersDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close 关闭 worker pool 并等待所有排队任务完成。
// 等价于 Shutdown(context.Background())。
// 首次调用返回 nil，后续调用返回 [ErrPoolStopped]。
//
// 注意：不可在 handler 内调用 Close，否则会死锁。
func (p *Pool[T]) Close() error {
	return p.Shutdown(context.Background())
}

// Done 返回一个 channel，在所有 worker goroutine 退出后关闭。
// 用于在 Shutdown 超时返回后等待残留 worker 最终完成。
// 必须先调用 Shutdown 或 Close，否则返回的 channel 永远不会关闭。
// 未通过 New 创建的零值 Pool 返回一个已关闭的 channel（无 worker 需等待）。
func (p *Pool[T]) Done() <-chan struct{} {
	if p.workersDone == nil {
		return closedCh
	}
	return p.workersDone
}

// Workers 返回 worker 数量。
func (p *Pool[T]) Workers() int {
	return p.workers
}

// QueueSize 返回队列大小。
func (p *Pool[T]) QueueSize() int {
	return p.queueSize
}

// QueueLen 返回当前队列中等待处理的任务数量。
// 适用于运行时监控和指标采集。
// 零值 Pool 返回 0（Go 规范保证 len(nil channel) == 0）。
func (p *Pool[T]) QueueLen() int {
	return len(p.queue)
}

// 编译期接口检查。
var _ io.Closer = (*Pool[int])(nil)
