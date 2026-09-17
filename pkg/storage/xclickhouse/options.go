package xclickhouse

import (
	"context"
	"time"

	"github.com/omeyang/xkit/internal/storageopt"
	"github.com/omeyang/xkit/pkg/observability/xmetrics"
)

// =============================================================================
// 慢查询钩子
// =============================================================================

// SlowQueryInfo 包含慢查询的详细信息。
type SlowQueryInfo struct {
	// Query 是执行的查询语句。
	Query string

	// Args 是查询参数。
	Args []any

	// Duration 是查询耗时。
	Duration time.Duration
}

// SlowQueryHook 是慢查询同步回调函数类型。
// 当查询耗时超过 SlowQueryThreshold 时触发。
//
// ⚠️  警告：此钩子在请求路径上同步执行！
//
// 钩子函数的执行时间会直接增加请求延迟。以下操作应避免在同步钩子中执行：
//   - 网络 IO（如发送告警、写入远程日志）
//   - 磁盘 IO（如写入文件日志）
//   - 任何可能阻塞的操作
//
// 推荐做法：
//   - 简单场景：仅记录到内存计数器或 channel
//   - 复杂场景：使用 AsyncSlowQueryHook 代替
//
// 如果必须使用同步钩子，请确保执行时间在微秒级别。
type SlowQueryHook func(ctx context.Context, info SlowQueryInfo)

// AsyncSlowQueryHook 是慢查询异步回调函数类型。
// 通过内部 worker pool 异步执行，不阻塞请求路径。
//
// 注意：此钩子不接收 context 参数，因为异步执行时原始 context 可能已取消。
// 当 AsyncSlowQueryHook 和 SlowQueryHook 同时设置时，两者都会被调用。
type AsyncSlowQueryHook func(info SlowQueryInfo)

// =============================================================================
// 选项模式
// =============================================================================

// options 包含 ClickHouse 包装器的配置选项。
// 设计决策: 不导出 options 类型，用户通过 WithXxx 函数配置，避免绕过校验逻辑直接赋值。
type options struct {
	// HealthTimeout 是健康检查的超时时间。
	// 默认 5 秒。通过 WithHealthTimeout 设置，仅接受正值；0 或负值被忽略。
	// 仅在 context 未设置 deadline 时生效；已有 deadline 时取两者较短值。
	HealthTimeout time.Duration

	// SlowQueryThreshold 是慢查询阈值。
	// 如果为 0，则禁用慢查询检测。
	SlowQueryThreshold time.Duration

	// SlowQueryHook 是慢查询同步回调函数。
	// 当查询耗时超过 SlowQueryThreshold 时触发。
	// 在请求路径上同步执行。
	SlowQueryHook SlowQueryHook

	// AsyncSlowQueryHook 是慢查询异步回调函数。
	// 通过内部 worker pool 异步执行，不阻塞请求路径。
	// 当与 SlowQueryHook 同时设置时，两者都会被调用。
	AsyncSlowQueryHook AsyncSlowQueryHook

	// AsyncSlowQueryWorkers 是异步慢查询 worker pool 大小。
	// 仅当设置 AsyncSlowQueryHook 时生效。
	// 默认为 10。
	AsyncSlowQueryWorkers int

	// AsyncSlowQueryQueueSize 是异步慢查询任务队列大小。
	// 仅当设置 AsyncSlowQueryHook 时生效。
	// 默认为 1000。当队列满时，新任务将被静默丢弃（不记录日志），属于可接受的降级行为。
	AsyncSlowQueryQueueSize int

	// Observer 是统一观测接口（metrics/tracing）。
	Observer xmetrics.Observer
}

// Option 是用于配置 options 的函数类型。
type Option func(*options)

// 默认值常量（复用 storageopt 定义）。
const (
	// DefaultAsyncSlowQueryWorkers 默认异步慢查询 worker 数量。
	DefaultAsyncSlowQueryWorkers = storageopt.DefaultAsyncWorkerPoolSize

	// DefaultAsyncSlowQueryQueueSize 默认异步慢查询队列大小。
	DefaultAsyncSlowQueryQueueSize = storageopt.DefaultAsyncQueueSize
)

// defaultOptions 返回默认选项。
// SlowQueryThreshold 零值表示禁用慢查询检测。
func defaultOptions() *options {
	return &options{
		HealthTimeout:           storageopt.DefaultHealthTimeout,
		AsyncSlowQueryWorkers:   DefaultAsyncSlowQueryWorkers,
		AsyncSlowQueryQueueSize: DefaultAsyncSlowQueryQueueSize,
		Observer:                xmetrics.NoopObserver{},
	}
}

// WithHealthTimeout 设置健康检查超时时间。
// 仅正值生效；0 或负值被忽略，保持默认值（5 秒）。
func WithHealthTimeout(timeout time.Duration) Option {
	return func(o *options) {
		if timeout > 0 {
			o.HealthTimeout = timeout
		}
	}
}

// WithSlowQueryThreshold 设置慢查询阈值。
// 设置为 0 禁用慢查询检测。负值被忽略（保持默认值）。
func WithSlowQueryThreshold(threshold time.Duration) Option {
	return func(o *options) {
		if threshold >= 0 {
			o.SlowQueryThreshold = threshold
		}
	}
}

// WithSlowQueryHook 设置慢查询同步回调函数。
func WithSlowQueryHook(hook SlowQueryHook) Option {
	return func(o *options) {
		o.SlowQueryHook = hook
	}
}

// WithAsyncSlowQueryHook 设置慢查询异步回调函数。
// 通过内部 worker pool 异步执行，不阻塞请求路径。
func WithAsyncSlowQueryHook(hook AsyncSlowQueryHook) Option {
	return func(o *options) {
		o.AsyncSlowQueryHook = hook
	}
}

// WithAsyncSlowQueryWorkers 设置异步慢查询 worker pool 大小。
// 默认为 10。
func WithAsyncSlowQueryWorkers(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.AsyncSlowQueryWorkers = n
		}
	}
}

// WithAsyncSlowQueryQueueSize 设置异步慢查询任务队列大小。
// 默认为 1000。
func WithAsyncSlowQueryQueueSize(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.AsyncSlowQueryQueueSize = n
		}
	}
}

// WithObserver 设置统一观测接口。
func WithObserver(observer xmetrics.Observer) Option {
	return func(o *options) {
		if observer != nil {
			o.Observer = observer
		}
	}
}
