package xclickhouse

import (
	"context"
	"time"

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

// SlowQueryHook 是慢查询回调函数类型。
// 当查询耗时超过 SlowQueryThreshold 时触发。
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
// 选项模式
// =============================================================================

// Options 包含 ClickHouse 包装器的配置选项。
type Options struct {
	// HealthTimeout 是健康检查的超时时间。
	// 如果为 0，则使用 context 的超时。
	HealthTimeout time.Duration

	// SlowQueryThreshold 是慢查询阈值。
	// 如果为 0，则禁用慢查询检测。
	SlowQueryThreshold time.Duration

	// SlowQueryHook 是慢查询回调函数。
	// 当查询耗时超过 SlowQueryThreshold 时触发。
	SlowQueryHook SlowQueryHook

	// Observer 是统一观测接口（metrics/tracing）。
	Observer xmetrics.Observer
}

// Option 是用于配置 Options 的函数类型。
type Option func(*Options)

// defaultOptions 返回默认选项。
func defaultOptions() *Options {
	return &Options{
		HealthTimeout:      5 * time.Second,
		SlowQueryThreshold: 0, // 默认禁用慢查询检测
		SlowQueryHook:      nil,
		Observer:           xmetrics.NoopObserver{},
	}
}

// WithHealthTimeout 设置健康检查超时时间。
func WithHealthTimeout(timeout time.Duration) Option {
	return func(o *Options) {
		if timeout > 0 {
			o.HealthTimeout = timeout
		}
	}
}

// WithSlowQueryThreshold 设置慢查询阈值。
// 如果为 0，则禁用慢查询检测。
func WithSlowQueryThreshold(threshold time.Duration) Option {
	return func(o *Options) {
		o.SlowQueryThreshold = threshold
	}
}

// WithSlowQueryHook 设置慢查询回调函数。
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
