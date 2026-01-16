package xlimit

// options 内部配置结构
type options struct {
	config  Config
	onAllow func(key Key, result *Result)
	onDeny  func(key Key, result *Result)
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

// WithMetrics 设置是否启用 Prometheus 指标
func WithMetrics(enabled bool) Option {
	return func(o *options) {
		o.config.EnableMetrics = enabled
	}
}

// WithHeaders 设置是否在响应中添加限流头
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

// WithOnAllow 设置请求通过时的回调
// 用于日志记录、指标上报等
func WithOnAllow(fn func(key Key, result *Result)) Option {
	return func(o *options) {
		o.onAllow = fn
	}
}

// WithOnDeny 设置请求被拒绝时的回调
// 用于日志记录、指标上报等
func WithOnDeny(fn func(key Key, result *Result)) Option {
	return func(o *options) {
		o.onDeny = fn
	}
}
