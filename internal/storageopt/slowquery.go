package storageopt

import (
	"context"
	"fmt"
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
	// 默认为 1000。当队列满时，新任务将被静默丢弃。
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
	pool    *xpool.Pool[T]
	mu      sync.RWMutex
	closed  bool
}

// NewSlowQueryDetector 创建慢查询检测器。
//
// 当 AsyncHook 不为 nil 时，会立即创建 worker pool。
// 如果 pool 参数超出 xpool 允许范围，返回错误（fail-fast）。
func NewSlowQueryDetector[T any](opts SlowQueryOptions[T]) (*SlowQueryDetector[T], error) {
	// 应用默认值
	if opts.AsyncWorkerPoolSize <= 0 {
		opts.AsyncWorkerPoolSize = DefaultAsyncWorkerPoolSize
	}
	if opts.AsyncQueueSize <= 0 {
		opts.AsyncQueueSize = DefaultAsyncQueueSize
	}

	d := &SlowQueryDetector[T]{
		options: opts,
	}

	// 设计决策: AsyncHook 非 nil 时立即创建 pool（eager init），将参数校验错误
	// 暴露给调用方，避免运行时静默失效。Threshold 为 0 时 pool 不会被使用，
	// 少量 goroutine 开销可接受，换取 fail-fast 语义。
	if opts.AsyncHook != nil {
		pool, err := xpool.New(
			opts.AsyncWorkerPoolSize,
			opts.AsyncQueueSize,
			opts.AsyncHook,
		)
		if err != nil {
			return nil, fmt.Errorf("storageopt: create async pool: %w", err)
		}
		d.pool = pool
	}

	return d, nil
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
	d.mu.RLock()
	if !d.closed && d.pool != nil {
		d.pool.Submit(info) //nolint:errcheck,gosec // 队列满时丢弃慢查询通知，可接受的降级行为（见 01-exceptions.md EX-02）
	}
	d.mu.RUnlock()

	return true
}

// Close 关闭检测器，释放资源。
//
// 设计决策: pool 引用在锁内取出后立即释放锁，pool.Close() 在锁外执行。
// 这避免了 pool 排空期间占用写锁导致并发 MaybeSlowQuery 阻塞（尾延迟）。
// 并发的 MaybeSlowQuery 会快速看到 closed == true 并跳过提交。
func (d *SlowQueryDetector[T]) Close() {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	pool := d.pool
	d.pool = nil
	d.mu.Unlock()

	if pool != nil {
		pool.Close() //nolint:errcheck,gosec // 内部清理，忽略重复关闭错误（见 01-exceptions.md EX-02）
	}
}
