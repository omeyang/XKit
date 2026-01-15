package xmongo

import (
	"context"
	"time"

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
	Filter any

	// Duration 操作耗时。
	Duration time.Duration
}

// SlowQueryHook 慢查询回调钩子。
// 当操作耗时超过阈值时调用。
//
// 注意：此钩子在请求路径上同步执行，
// 任何耗时操作（如网络 IO、重日志）都会增加请求延迟。
// 如需异步处理，请在钩子内部使用 goroutine：
//
//	WithSlowQueryHook(func(ctx context.Context, info SlowQueryInfo) {
//	    go func() {
//	        // 异步处理：发送告警、写入日志等
//	    }()
//	})
type SlowQueryHook func(ctx context.Context, info SlowQueryInfo)

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

	// SlowQueryHook 慢查询回调钩子。
	// 当操作耗时超过 SlowQueryThreshold 时调用。
	SlowQueryHook SlowQueryHook

	// Observer 是统一观测接口（metrics/tracing）。
	Observer xmetrics.Observer
}

// Option 定义配置 MongoDB 包装器的函数类型。
type Option func(*Options)

// defaultOptions 返回默认配置。
func defaultOptions() *Options {
	return &Options{
		HealthTimeout:      5 * time.Second,
		SlowQueryThreshold: 0,
		SlowQueryHook:      nil,
		Observer:           xmetrics.NoopObserver{},
	}
}

// WithHealthTimeout 设置健康检查超时时间。
func WithHealthTimeout(timeout time.Duration) Option {
	return func(o *Options) {
		o.HealthTimeout = timeout
	}
}

// WithSlowQueryThreshold 设置慢查询阈值。
// 当操作耗时超过此阈值时，如果设置了 SlowQueryHook，将触发回调。
// 设置为 0 禁用慢查询检测。
func WithSlowQueryThreshold(threshold time.Duration) Option {
	return func(o *Options) {
		o.SlowQueryThreshold = threshold
	}
}

// WithSlowQueryHook 设置慢查询回调钩子。
func WithSlowQueryHook(hook SlowQueryHook) Option {
	return func(o *Options) {
		o.SlowQueryHook = hook
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
