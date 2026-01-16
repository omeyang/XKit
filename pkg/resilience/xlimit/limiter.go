package xlimit

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// Limiter 限流器接口
type Limiter interface {
	// Allow 检查是否允许单个请求通过
	// 如果被限流，返回的 Result.Allowed 为 false
	Allow(ctx context.Context, key Key) (*Result, error)

	// AllowN 检查是否允许 n 个请求通过
	// 用于批量请求场景
	AllowN(ctx context.Context, key Key, n int) (*Result, error)

	// Reset 重置指定键的限流计数
	// 用于手动清除配额
	Reset(ctx context.Context, key Key) error

	// Close 关闭限流器，释放资源
	Close() error
}

// New 创建分布式限流器
// 使用 Redis 作为后端存储，支持多 Pod 共享配额
func New(rdb redis.UniversalClient, opts ...Option) (Limiter, error) {
	cfg := defaultOptions()
	for _, opt := range opts {
		opt(cfg)
	}

	// 验证配置
	if err := cfg.config.Validate(); err != nil {
		return nil, err
	}

	// 创建规则匹配器
	matcher := NewRuleMatcher(cfg.config.Rules)

	// 创建分布式限流器
	distributed := newDistributedLimiter(rdb, matcher, cfg)

	// 如果配置了降级策略，包装降级逻辑
	if cfg.config.Fallback != "" {
		local := newLocalLimiter(matcher, cfg)
		return newFallbackLimiter(distributed, local, cfg.config.Fallback), nil
	}

	return distributed, nil
}

// NewLocal 创建本地限流器
// 使用内存作为后端存储，不依赖 Redis
// 适用于单 Pod 场景或作为降级方案
func NewLocal(opts ...Option) (Limiter, error) {
	cfg := defaultOptions()
	for _, opt := range opts {
		opt(cfg)
	}

	// 验证配置
	if err := cfg.config.Validate(); err != nil {
		return nil, err
	}

	// 创建规则匹配器
	matcher := NewRuleMatcher(cfg.config.Rules)

	return newLocalLimiter(matcher, cfg), nil
}
