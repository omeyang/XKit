package xmongo

import (
	"context"
	"time"

	"github.com/omeyang/xkit/internal/storageopt"
	"github.com/omeyang/xkit/pkg/observability/xmetrics"
)

// =============================================================================
// 慢查询信息
// =============================================================================

// SlowQueryInfo 慢查询详细信息。
type SlowQueryInfo struct {
	// Database 数据库名称。
	Database string

	// Collection 集合名称。
	Collection string

	// Operation 操作类型（find、insert、update、delete 等）。
	Operation string

	// Filter 查询过滤条件。
	//
	// ⚠️ 安全提示：Filter 包含原始查询条件，可能含有敏感信息（如密码查询条件）。
	// 在 SlowQueryHook 实现中写入日志时，请注意脱敏处理。
	Filter any

	// Duration 操作耗时。
	Duration time.Duration
}

// SlowQueryHook 慢查询同步回调钩子。
// 当操作耗时超过阈值时调用。
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

// AsyncSlowQueryHook 慢查询异步回调钩子。
// 通过内部 worker pool 异步执行，不阻塞请求路径。
//
// 注意：此钩子不接收 context 参数，因为异步执行时原始 context 可能已取消。
// 当 AsyncSlowQueryHook 和 SlowQueryHook 同时设置时，两者都会被调用。
type AsyncSlowQueryHook func(info SlowQueryInfo)

// =============================================================================
// 配置选项
// =============================================================================

// Options 定义 MongoDB 包装器的配置选项。
type Options struct {
	// HealthTimeout 健康检查超时时间。
	// 默认为 5 秒。
	HealthTimeout time.Duration

	// SlowQueryThreshold 慢查询阈值。
	// 为 0 时禁用慢查询检测。
	SlowQueryThreshold time.Duration

	// SlowQueryHook 慢查询同步回调钩子。
	// 当操作耗时超过 SlowQueryThreshold 时调用。
	// 在请求路径上同步执行。
	SlowQueryHook SlowQueryHook

	// AsyncSlowQueryHook 慢查询异步回调钩子。
	// 通过内部 worker pool 异步执行，不阻塞请求路径。
	// 当与 SlowQueryHook 同时设置时，两者都会被调用。
	AsyncSlowQueryHook AsyncSlowQueryHook

	// AsyncSlowQueryWorkers 异步慢查询 worker pool 大小。
	// 仅当设置 AsyncSlowQueryHook 时生效。
	// 默认为 10。
	AsyncSlowQueryWorkers int

	// AsyncSlowQueryQueueSize 异步慢查询任务队列大小。
	// 仅当设置 AsyncSlowQueryHook 时生效。
	// 默认为 1000。当队列满时，新任务将被静默丢弃（不记录日志），属于可接受的降级行为。
	AsyncSlowQueryQueueSize int

	// QueryTimeout 查询操作兜底超时时间。
	// 当调用方传入的 context 没有 deadline 时，FindPage 会使用此超时作为兜底。
	// 默认为 DefaultQueryTimeout（30 秒），防止无 deadline 的 context
	// 导致查询长时间悬挂，进而引发连接池耗尽。
	//
	// 通过 WithQueryTimeout(0) 可显式禁用兜底超时（完全依赖调用方 context）。
	// 已有 deadline 的 context 不受此设置影响。
	QueryTimeout time.Duration

	// WriteTimeout 写入操作兜底超时时间。
	// 当调用方传入的 context 没有 deadline 时，BulkInsert 会使用此超时作为兜底。
	// 默认为 DefaultWriteTimeout（60 秒），防止无 deadline 的 context
	// 导致写入长时间悬挂，进而引发连接池耗尽。
	//
	// 通过 WithWriteTimeout(0) 可显式禁用兜底超时（完全依赖调用方 context）。
	// 已有 deadline 的 context 不受此设置影响。
	WriteTimeout time.Duration

	// Observer 是统一观测接口（metrics/tracing）。
	Observer xmetrics.Observer
}

// Option 定义配置 MongoDB 包装器的函数类型。
type Option func(*Options)

// 默认值常量（复用 storageopt 定义）。
const (
	// DefaultAsyncSlowQueryWorkers 默认异步慢查询 worker 数量。
	DefaultAsyncSlowQueryWorkers = storageopt.DefaultAsyncWorkerPoolSize

	// DefaultAsyncSlowQueryQueueSize 默认异步慢查询队列大小。
	DefaultAsyncSlowQueryQueueSize = storageopt.DefaultAsyncQueueSize

	// DefaultQueryTimeout 查询操作默认兜底超时时间。
	// 当调用方 context 没有 deadline 时，FindPage 使用此超时防止无限阻塞。
	// 可通过 WithQueryTimeout(0) 显式禁用。
	DefaultQueryTimeout = 30 * time.Second

	// DefaultWriteTimeout 写入操作默认兜底超时时间。
	// 当调用方 context 没有 deadline 时，BulkInsert 使用此超时防止无限阻塞。
	// 可通过 WithWriteTimeout(0) 显式禁用。
	DefaultWriteTimeout = 60 * time.Second
)

// defaultOptions 返回默认配置。
func defaultOptions() *Options {
	return &Options{
		HealthTimeout:           storageopt.DefaultHealthTimeout,
		SlowQueryThreshold:      0,
		SlowQueryHook:           nil,
		AsyncSlowQueryHook:      nil,
		AsyncSlowQueryWorkers:   DefaultAsyncSlowQueryWorkers,
		AsyncSlowQueryQueueSize: DefaultAsyncSlowQueryQueueSize,
		QueryTimeout:            DefaultQueryTimeout,
		WriteTimeout:            DefaultWriteTimeout,
		Observer:                xmetrics.NoopObserver{},
	}
}

// WithHealthTimeout 设置健康检查超时时间。
// 非正值被忽略（保持默认值），与 storageopt.WithHealthTimeout 行为一致。
func WithHealthTimeout(timeout time.Duration) Option {
	return func(o *Options) {
		if timeout > 0 {
			o.HealthTimeout = timeout
		}
	}
}

// WithSlowQueryThreshold 设置慢查询阈值。
// 当操作耗时超过此阈值时，如果设置了 SlowQueryHook，将触发回调。
// 设置为 0 禁用慢查询检测。负值被忽略（保持默认值）。
func WithSlowQueryThreshold(threshold time.Duration) Option {
	return func(o *Options) {
		if threshold >= 0 {
			o.SlowQueryThreshold = threshold
		}
	}
}

// WithSlowQueryHook 设置慢查询同步回调钩子。
func WithSlowQueryHook(hook SlowQueryHook) Option {
	return func(o *Options) {
		o.SlowQueryHook = hook
	}
}

// WithAsyncSlowQueryHook 设置慢查询异步回调钩子。
// 通过内部 worker pool 异步执行，不阻塞请求路径。
func WithAsyncSlowQueryHook(hook AsyncSlowQueryHook) Option {
	return func(o *Options) {
		o.AsyncSlowQueryHook = hook
	}
}

// WithAsyncSlowQueryWorkers 设置异步慢查询 worker pool 大小。
// 默认为 10。
func WithAsyncSlowQueryWorkers(n int) Option {
	return func(o *Options) {
		if n > 0 {
			o.AsyncSlowQueryWorkers = n
		}
	}
}

// WithAsyncSlowQueryQueueSize 设置异步慢查询任务队列大小。
// 默认为 1000。
func WithAsyncSlowQueryQueueSize(n int) Option {
	return func(o *Options) {
		if n > 0 {
			o.AsyncSlowQueryQueueSize = n
		}
	}
}

// WithQueryTimeout 设置查询操作兜底超时时间。
// 仅在调用方 context 没有 deadline 时生效。
// 传入 0 可显式禁用兜底超时（完全依赖调用方 context）。
// 负值被忽略（保持默认值）。
func WithQueryTimeout(timeout time.Duration) Option {
	return func(o *Options) {
		if timeout >= 0 {
			o.QueryTimeout = timeout
		}
	}
}

// WithWriteTimeout 设置写入操作兜底超时时间。
// 仅在调用方 context 没有 deadline 时生效。
// 传入 0 可显式禁用兜底超时（完全依赖调用方 context）。
// 负值被忽略（保持默认值）。
func WithWriteTimeout(timeout time.Duration) Option {
	return func(o *Options) {
		if timeout >= 0 {
			o.WriteTimeout = timeout
		}
	}
}

// WithObserver 设置统一观测接口。
func WithObserver(observer xmetrics.Observer) Option {
	return func(o *Options) {
		if observer != nil {
			o.Observer = observer
		}
	}
}
