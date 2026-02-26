package xlimit

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// =============================================================================
// 核心接口定义
// =============================================================================

// Limiter 限流器核心接口
//
// 提供限流检查和资源清理的基本能力。
// 实现应该是并发安全的。
//
// Allow/AllowN 的返回契约：
//   - err == nil 时，返回的 *Result 必非 nil
//   - err != nil 时，*Result 可能为 nil，也可能携带拒绝信息（如 FallbackClose）
type Limiter interface {
	// Allow 检查是否允许单个请求通过
	// 如果被限流，返回的 Result.Allowed 为 false
	Allow(ctx context.Context, key Key) (*Result, error)

	// AllowN 检查是否允许 n 个请求通过
	// 用于批量请求场景
	AllowN(ctx context.Context, key Key, n int) (*Result, error)

	// Close 关闭限流器，释放资源
	// 设计决策: 保留 ctx 参数（D-02），当前未使用但预留用于未来超时控制。
	Close(ctx context.Context) error
}

// =============================================================================
// 可选扩展接口（通过类型断言使用）
// =============================================================================

// Querier 配额查询接口
//
// 实现此接口的限流器支持查询当前配额状态（不消耗配额）。
// 使用方式：
//
//	if q, ok := limiter.(xlimit.Querier); ok {
//	    info, err := q.Query(ctx, key)
//	}
type Querier interface {
	// Query 查询当前配额状态（不消耗配额）
	Query(ctx context.Context, key Key) (*QuotaInfo, error)
}

// Resetter 配额重置接口
//
// 实现此接口的限流器支持手动重置配额。
// 使用方式：
//
//	if r, ok := limiter.(xlimit.Resetter); ok {
//	    err := r.Reset(ctx, key)
//	}
type Resetter interface {
	// Reset 重置指定键的限流计数
	Reset(ctx context.Context, key Key) error
}

// =============================================================================
// 策略接口
// =============================================================================

// RuleProvider 规则提供器接口
//
// 用于根据 Key 查找匹配的限流规则。
// 参考 xbreaker 的 TripPolicy 设计模式。
type RuleProvider interface {
	// FindRule 根据 Key 查找适用的规则
	// 如果找到匹配规则返回 (rule, true)，否则返回 (Rule{}, false)
	FindRule(key Key) (Rule, bool)
}

// =============================================================================
// 数据结构
// =============================================================================

// QuotaInfo 配额信息
type QuotaInfo struct {
	// Limit 配额上限
	Limit int
	// Remaining 剩余配额
	Remaining int
	// ResetAt 配额重置时间
	ResetAt time.Time
	// Rule 匹配的规则名称
	Rule string
	// Key 渲染后的限流键
	Key string
}

// =============================================================================
// 工厂函数
// =============================================================================

// New 创建分布式限流器
//
// 使用 Redis 作为后端存储，支持多 Pod 共享配额。
// 如果配置了 Fallback，会自动包装为降级限流器。
func New(rdb redis.UniversalClient, opts ...Option) (Limiter, error) {
	if rdb == nil {
		return nil, ErrNilClient
	}

	cfg := defaultOptions()
	for _, opt := range opts {
		opt(cfg)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// 初始化指标收集器
	if cfg.config.EnableMetrics && cfg.meterProvider != nil {
		metrics, err := NewMetrics(cfg.meterProvider)
		if err != nil {
			return nil, err
		}
		cfg.metrics = metrics
	}

	matcher := newRuleMatcher(cfg.config.Rules)
	backend := newRedisBackend(rdb)
	distributed := newLimiterCore(backend, matcher, cfg)

	if cfg.config.Fallback != "" {
		// 设计决策: FallbackLocal + 默认 PodCount=1 + 无 PodCountProvider 时输出启动告警。
		// 多 Pod 部署下每个 Pod 按完整配额执行本地限流，总放行量可达 N 倍。
		// 不设为硬错误是因为单 Pod 场景（开发/测试/小型服务）默认值合理。
		warnDefaultPodCount(cfg)
		localBackend := newLocalBackend(cfg.config.EffectivePodCount(), cfg.podCountProvider)
		local := newLimiterCore(localBackend, matcher, cfg)
		return newFallbackLimiter(distributed, local, cfg), nil
	}

	return distributed, nil
}

// NewLocal 创建本地限流器
//
// 使用内存作为后端存储，不依赖 Redis。
// 适用于单 Pod 场景或作为降级方案。
// 会根据 PodCount 自动调整本地配额 = 总配额 / PodCount。
func NewLocal(opts ...Option) (Limiter, error) {
	cfg := defaultOptions()
	for _, opt := range opts {
		opt(cfg)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// 初始化指标收集器
	if cfg.config.EnableMetrics && cfg.meterProvider != nil {
		metrics, err := NewMetrics(cfg.meterProvider)
		if err != nil {
			return nil, err
		}
		cfg.metrics = metrics
	}

	matcher := newRuleMatcher(cfg.config.Rules)
	backend := newLocalBackend(cfg.config.EffectivePodCount(), cfg.podCountProvider)
	return newLimiterCore(backend, matcher, cfg), nil
}

// NewWithFallback 创建带降级的分布式限流器
//
// 当 Redis 不可用时自动降级到本地限流。
// 这是推荐的生产环境使用方式。
//
// 设计决策: 默认降级策略 prepend 到用户选项之前，确保用户通过 opts 传入的
// WithFallback 优先级更高（后执行的 Option 覆盖先执行的）。
func NewWithFallback(rdb redis.UniversalClient, opts ...Option) (Limiter, error) {
	// 将默认降级策略放在前面，用户选项后执行可覆盖
	opts = append([]Option{WithFallback(FallbackLocal)}, opts...)
	return New(rdb, opts...)
}

// warnDefaultPodCount 在 FallbackLocal + 默认 PodCount + 无 PodCountProvider 时输出启动告警。
// 多 Pod 部署下每个 Pod 按完整配额执行本地限流，总放行量可达 N 倍配额。
func warnDefaultPodCount(cfg *options) {
	if cfg.config.Fallback != FallbackLocal {
		return
	}
	if cfg.podCountProvider != nil || cfg.config.LocalPodCount > 1 {
		return
	}
	slog.Warn("xlimit: FallbackLocal with default PodCount=1; " +
		"in multi-pod deployments each pod gets the full quota, " +
		"total throughput may reach N × limit. " +
		"Use WithPodCount or WithPodCountProvider to set the real pod count.")
}
