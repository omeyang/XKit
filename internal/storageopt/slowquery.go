package storageopt

import (
	"context"
	"sync"
	"time"

	"github.com/omeyang/xkit/pkg/util/xpool"
)

// SlowQueryHook 慢查询同步回调钩子。
// 在请求路径上同步执行。
//
// 注意：任何耗时操作（如网络 IO、重日志）都会增加请求延迟。
// 如需避免阻塞，请使用 AsyncSlowQueryHook。
type SlowQueryHook[T any] func(ctx context.Context, info T)

// AsyncSlowQueryHook 慢查询异步回调钩子。
// 通过 worker pool 异步执行，不阻塞请求路径。
//
// 注意：此钩子不接收 context 参数，因为异步执行时原始 context 可能已取消。
type AsyncSlowQueryHook[T any] func(info T)

// SlowQueryOptions 慢查询检测配置。
type SlowQueryOptions[T any] struct {
	// Threshold 慢查询阈值。
	// 为 0 时禁用慢查询检测。
	Threshold time.Duration

	// SyncHook 同步回调钩子。
	// 在请求路径上同步执行。
	// 注意：任何耗时操作（如网络 IO、重日志）都会增加请求延迟。
	SyncHook SlowQueryHook[T]

	// AsyncHook 异步回调钩子。
	// 通过内部 worker pool 异步执行，不阻塞请求路径。
	// 当 AsyncHook 和 SyncHook 同时设置时，两者都会被调用。
	AsyncHook AsyncSlowQueryHook[T]

	// AsyncWorkerPoolSize 异步 worker pool 大小。
	// 仅当设置 AsyncHook 时生效。
	// 默认为 10。
	AsyncWorkerPoolSize int

	// AsyncQueueSize 异步任务队列大小。
	// 仅当设置 AsyncHook 时生效。
	// 默认为 1000。当队列满时，新任务将被丢弃并记录日志。
	AsyncQueueSize int
}

// 默认值常量。
const (
	DefaultAsyncWorkerPoolSize = 10
	DefaultAsyncQueueSize      = 1000
)

// SlowQueryDetector 慢查询检测器。
// 封装了同步/异步钩子的调用逻辑。
type SlowQueryDetector[T any] struct {
	options SlowQueryOptions[T]
	pool    *xpool.WorkerPool[T]
	mu      sync.RWMutex
	closed  bool
}

// NewSlowQueryDetector 创建慢查询检测器。
func NewSlowQueryDetector[T any](opts SlowQueryOptions[T]) *SlowQueryDetector[T] {
	// 应用默认值
	if opts.AsyncWorkerPoolSize <= 0 {
		opts.AsyncWorkerPoolSize = DefaultAsyncWorkerPoolSize
	}
	if opts.AsyncQueueSize <= 0 {
		opts.AsyncQueueSize = DefaultAsyncQueueSize
	}

	return &SlowQueryDetector[T]{
		options: opts,
	}
}

// MaybeSlowQuery 检测并可能触发慢查询钩子。
// 返回是否触发了慢查询检测。
//
// 参数：
//   - ctx: 上下文，仅用于同步钩子
//   - info: 慢查询信息
//   - duration: 操作耗时
func (d *SlowQueryDetector[T]) MaybeSlowQuery(ctx context.Context, info T, duration time.Duration) bool {
	// 如果阈值为 0，禁用慢查询检测
	if d.options.Threshold == 0 {
		return false
	}

	// 如果耗时未超过阈值，不触发（使用 < 比较，即 duration >= threshold 时触发）
	if duration < d.options.Threshold {
		return false
	}

	// 触发同步钩子
	if d.options.SyncHook != nil {
		d.options.SyncHook(ctx, info)
	}

	// 触发异步钩子
	if d.options.AsyncHook != nil {
		d.ensurePoolStarted()
		d.mu.RLock()
		if !d.closed && d.pool != nil {
			d.pool.Submit(info)
		}
		d.mu.RUnlock()
	}

	return true
}

// ensurePoolStarted 确保 worker pool 已启动。
// 使用双重检查锁定模式，避免每次调用都获取写锁。
func (d *SlowQueryDetector[T]) ensurePoolStarted() {
	// 快速路径：读锁检查 pool 是否已创建
	d.mu.RLock()
	poolStarted := d.pool != nil
	d.mu.RUnlock()
	if poolStarted {
		return
	}

	// 慢速路径：获取写锁创建 pool
	d.mu.Lock()
	defer d.mu.Unlock()

	// 双重检查：获取写锁后再次检查
	if d.pool != nil || d.closed {
		return
	}

	if d.options.AsyncHook != nil {
		d.pool = xpool.NewWorkerPool(
			d.options.AsyncWorkerPoolSize,
			d.options.AsyncQueueSize,
			d.options.AsyncHook,
		)
		d.pool.Start()
	}
}

// Close 关闭检测器，释放资源。
func (d *SlowQueryDetector[T]) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return
	}
	d.closed = true

	if d.pool != nil {
		d.pool.Stop()
		d.pool = nil
	}
}
