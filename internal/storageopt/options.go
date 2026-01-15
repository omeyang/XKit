package storageopt

import (
	"context"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"
)

// =============================================================================
// 通用钩子类型
// =============================================================================

// SyncHook 同步钩子函数类型。
// T 是慢查询信息的具体类型（如 xmongo.SlowQueryInfo 或 xclickhouse.SlowQueryInfo）。
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
//   - 复杂场景：使用 AsyncHook 代替
//
// 如果必须使用同步钩子，请确保执行时间在微秒级别。
type SyncHook[T any] func(ctx context.Context, info T)

// AsyncHook 异步钩子函数类型。
// 通过内部 worker pool 异步执行，不阻塞请求路径。
//
// 注意：此钩子不接收 context 参数，因为异步执行时原始 context 可能已取消。
// 当 AsyncHook 和 SyncHook 同时设置时，两者都会被调用。
type AsyncHook[T any] func(info T)

// =============================================================================
// 通用配置选项
// =============================================================================

// BaseOptions 定义存储包装器的通用配置选项。
// T 是慢查询信息的具体类型。
type BaseOptions[T any] struct {
	// HealthTimeout 健康检查超时时间。
	// 默认为 5 秒。
	HealthTimeout time.Duration

	// SlowQueryThreshold 慢查询阈值。
	// 为 0 时禁用慢查询检测。
	SlowQueryThreshold time.Duration

	// SlowQueryHook 慢查询同步回调钩子。
	// 当操作耗时超过 SlowQueryThreshold 时调用。
	// 在请求路径上同步执行。
	SlowQueryHook SyncHook[T]

	// AsyncSlowQueryHook 慢查询异步回调钩子。
	// 通过内部 worker pool 异步执行，不阻塞请求路径。
	// 当与 SlowQueryHook 同时设置时，两者都会被调用。
	AsyncSlowQueryHook AsyncHook[T]

	// AsyncSlowQueryWorkers 异步慢查询 worker pool 大小。
	// 仅当设置 AsyncSlowQueryHook 时生效。
	// 默认为 10。
	AsyncSlowQueryWorkers int

	// AsyncSlowQueryQueueSize 异步慢查询任务队列大小。
	// 仅当设置 AsyncSlowQueryHook 时生效。
	// 默认为 1000。当队列满时，新任务将被丢弃并记录日志。
	AsyncSlowQueryQueueSize int

	// Observer 是统一观测接口（metrics/tracing）。
	Observer xmetrics.Observer
}

// OptionFunc 定义配置 BaseOptions 的函数类型。
type OptionFunc[T any] func(*BaseOptions[T])

// DefaultBaseOptions 返回默认配置。
func DefaultBaseOptions[T any]() BaseOptions[T] {
	return BaseOptions[T]{
		HealthTimeout:           DefaultHealthTimeout,
		SlowQueryThreshold:      0,
		SlowQueryHook:           nil,
		AsyncSlowQueryHook:      nil,
		AsyncSlowQueryWorkers:   DefaultAsyncWorkerPoolSize,
		AsyncSlowQueryQueueSize: DefaultAsyncQueueSize,
		Observer:                xmetrics.NoopObserver{},
	}
}

// =============================================================================
// 通用 With* 配置函数
// =============================================================================

// WithHealthTimeout 设置健康检查超时时间。
func WithHealthTimeout[T any](timeout time.Duration) OptionFunc[T] {
	return func(o *BaseOptions[T]) {
		if timeout > 0 {
			o.HealthTimeout = timeout
		}
	}
}

// WithSlowQueryThreshold 设置慢查询阈值。
// 当操作耗时超过此阈值时，如果设置了 SlowQueryHook，将触发回调。
// 设置为 0 禁用慢查询检测。
func WithSlowQueryThreshold[T any](threshold time.Duration) OptionFunc[T] {
	return func(o *BaseOptions[T]) {
		o.SlowQueryThreshold = threshold
	}
}

// WithSlowQueryHook 设置慢查询同步回调钩子。
func WithSlowQueryHook[T any](hook SyncHook[T]) OptionFunc[T] {
	return func(o *BaseOptions[T]) {
		o.SlowQueryHook = hook
	}
}

// WithAsyncSlowQueryHook 设置慢查询异步回调钩子。
// 通过内部 worker pool 异步执行，不阻塞请求路径。
func WithAsyncSlowQueryHook[T any](hook AsyncHook[T]) OptionFunc[T] {
	return func(o *BaseOptions[T]) {
		o.AsyncSlowQueryHook = hook
	}
}

// WithAsyncSlowQueryWorkers 设置异步慢查询 worker pool 大小。
// 默认为 10。
func WithAsyncSlowQueryWorkers[T any](n int) OptionFunc[T] {
	return func(o *BaseOptions[T]) {
		if n > 0 {
			o.AsyncSlowQueryWorkers = n
		}
	}
}

// WithAsyncSlowQueryQueueSize 设置异步慢查询任务队列大小。
// 默认为 1000。
func WithAsyncSlowQueryQueueSize[T any](n int) OptionFunc[T] {
	return func(o *BaseOptions[T]) {
		if n > 0 {
			o.AsyncSlowQueryQueueSize = n
		}
	}
}

// WithObserver 设置统一观测接口。
func WithObserver[T any](observer xmetrics.Observer) OptionFunc[T] {
	return func(o *BaseOptions[T]) {
		if observer != nil {
			o.Observer = observer
		}
	}
}
