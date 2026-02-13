package xlimit

import (
	"context"

	"go.opentelemetry.io/otel/metric"

	"github.com/omeyang/xkit/pkg/observability/xlog"
	"github.com/omeyang/xkit/pkg/observability/xmetrics"
)

// FallbackFunc 自定义降级函数类型
// 当分布式限流器不可用时调用
// 参数：
//   - ctx: 上下文
//   - key: 限流键
//   - n: 请求数量
//   - originalErr: 触发降级的原始错误
//
// 返回：
//   - *Result: 限流结果
//   - error: 如果需要返回错误
type FallbackFunc func(ctx context.Context, key Key, n int, originalErr error) (*Result, error)

// options 内部配置结构
type options struct {
	config           Config
	logger           xlog.Logger
	observer         xmetrics.Observer
	meterProvider    metric.MeterProvider
	metrics          *Metrics
	onAllow          func(key Key, result *Result)
	onDeny           func(key Key, result *Result)
	onFallback       func(key Key, strategy FallbackStrategy, err error)
	customFallback   FallbackFunc
	podCountProvider PodCountProvider
	initErr          error // 配置加载阶段的错误，延迟到 New/NewLocal 时返回
}

// validate 验证选项并返回初始化阶段收集的错误
// 设计决策: Option 函数签名不支持返回错误，因此将配置加载错误
// 暂存在 initErr 中，在 New/NewLocal 构造时统一检查。
func (o *options) validate() error {
	if o.initErr != nil {
		return o.initErr
	}
	return o.config.Validate()
}

// Option 配置选项函数
type Option func(*options)

// defaultOptions 返回默认配置
func defaultOptions() *options {
	return &options{
		config: DefaultConfig(),
	}
}

// WithRules 设置限流规则
func WithRules(rules ...Rule) Option {
	return func(o *options) {
		o.config.Rules = append(o.config.Rules, rules...)
	}
}

// WithKeyPrefix 设置 Redis 键前缀
// 默认为 "ratelimit:"
func WithKeyPrefix(prefix string) Option {
	return func(o *options) {
		o.config.KeyPrefix = prefix
	}
}

// WithFallback 设置 Redis 不可用时的降级策略
// 可选值：FallbackLocal, FallbackOpen, FallbackClose
func WithFallback(strategy FallbackStrategy) Option {
	return func(o *options) {
		o.config.Fallback = strategy
	}
}

// WithPodCount 设置预期 Pod 数量
// 用于计算本地降级时的配额：本地配额 = 分布式配额 / PodCount
func WithPodCount(count int) Option {
	return func(o *options) {
		o.config.LocalPodCount = count
	}
}

// WithMetrics 设置是否启用指标收集
func WithMetrics(enabled bool) Option {
	return func(o *options) {
		o.config.EnableMetrics = enabled
	}
}

// WithHeaders 设置 Config.EnableHeaders 字段（用于配置序列化/反序列化）。
//
// 设计决策: 此选项仅影响 Config 结构体，不直接控制 HTTP 中间件行为。
// HTTP 中间件使用独立的 WithMiddlewareHeaders 选项，因为中间件可能
// 由不同的团队/模块创建，需要独立于 limiter 配置控制响应头。
func WithHeaders(enabled bool) Option {
	return func(o *options) {
		o.config.EnableHeaders = enabled
	}
}

// WithConfig 使用完整配置覆盖
func WithConfig(config Config) Option {
	return func(o *options) {
		o.config = config
	}
}

// WithLogger 设置日志记录器
// 使用 xlog 进行结构化日志记录
func WithLogger(logger xlog.Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}

// WithObserver 设置可观测性观察者
// 使用 xmetrics 进行指标收集和追踪
func WithObserver(observer xmetrics.Observer) Option {
	return func(o *options) {
		o.observer = observer
	}
}

// WithOnAllow 设置请求通过时的回调
// 用于自定义日志记录、指标上报等
func WithOnAllow(fn func(key Key, result *Result)) Option {
	return func(o *options) {
		o.onAllow = fn
	}
}

// WithOnDeny 设置请求被拒绝时的回调
// 用于自定义日志记录、指标上报等
func WithOnDeny(fn func(key Key, result *Result)) Option {
	return func(o *options) {
		o.onDeny = fn
	}
}

// WithOnFallback 设置降级时的回调
// 当 Redis 不可用触发降级时调用
func WithOnFallback(fn func(key Key, strategy FallbackStrategy, err error)) Option {
	return func(o *options) {
		o.onFallback = fn
	}
}

// WithMeterProvider 设置 OpenTelemetry MeterProvider
// 用于收集 Counter/Histogram 类型的指标
// 如果不设置，不会收集指标
func WithMeterProvider(mp metric.MeterProvider) Option {
	return func(o *options) {
		o.meterProvider = mp
	}
}

// WithCustomFallback 设置自定义降级函数
// 当分布式限流器（Redis）不可用时调用
// 如果设置了自定义降级函数，将优先于 WithFallback 设置的策略
func WithCustomFallback(fn FallbackFunc) Option {
	return func(o *options) {
		o.customFallback = fn
	}
}

// WithPodCountProvider 设置动态 Pod 数量提供器
// 用于计算本地降级时的配额：本地配额 = 分布式配额 / PodCount
// 如果设置了此选项，将优先于 WithPodCount 设置的静态值
func WithPodCountProvider(provider PodCountProvider) Option {
	return func(o *options) {
		o.podCountProvider = provider
	}
}
