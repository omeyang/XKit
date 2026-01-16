package xlimit

import (
	"context"
	"errors"
)

// fallbackLimiter 带降级能力的限流器
// 当分布式限流器（Redis）不可用时，自动降级到备选策略
type fallbackLimiter struct {
	distributed Limiter
	local       Limiter
	strategy    FallbackStrategy
}

// newFallbackLimiter 创建带降级的限流器
func newFallbackLimiter(distributed, local Limiter, strategy FallbackStrategy) *fallbackLimiter {
	return &fallbackLimiter{
		distributed: distributed,
		local:       local,
		strategy:    strategy,
	}
}

// Allow 检查是否允许单个请求通过
func (f *fallbackLimiter) Allow(ctx context.Context, key Key) (*Result, error) {
	return f.AllowN(ctx, key, 1)
}

// AllowN 检查是否允许 n 个请求通过
func (f *fallbackLimiter) AllowN(ctx context.Context, key Key, n int) (*Result, error) {
	result, err := f.distributed.AllowN(ctx, key, n)
	if err == nil {
		return result, nil
	}

	// 检查是否是 Redis 不可用错误
	if !isRedisError(err) {
		return nil, err
	}

	// 执行降级策略
	return f.fallback(ctx, key, n)
}

// fallback 执行降级策略
func (f *fallbackLimiter) fallback(ctx context.Context, key Key, n int) (*Result, error) {
	switch f.strategy {
	case FallbackLocal:
		// 降级到本地限流
		return f.local.AllowN(ctx, key, n)

	case FallbackOpen:
		// 放行所有请求
		return &Result{
			Allowed: true,
			Rule:    "fallback-open",
		}, nil

	case FallbackClose:
		// 拒绝所有请求
		return &Result{
			Allowed: false,
			Rule:    "fallback-close",
		}, ErrRedisUnavailable

	default:
		// 默认使用本地限流
		return f.local.AllowN(ctx, key, n)
	}
}

// Reset 重置指定键的限流计数
func (f *fallbackLimiter) Reset(ctx context.Context, key Key) error {
	// 同时重置分布式和本地计数
	var errs []error

	if err := f.distributed.Reset(ctx, key); err != nil && !isRedisError(err) {
		errs = append(errs, err)
	}

	if err := f.local.Reset(ctx, key); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Close 关闭限流器
func (f *fallbackLimiter) Close() error {
	var errs []error

	if err := f.distributed.Close(); err != nil {
		errs = append(errs, err)
	}

	if err := f.local.Close(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// isRedisError 检查是否是 Redis 相关错误
func isRedisError(err error) bool {
	if err == nil {
		return false
	}

	// 检查常见的 Redis 错误
	errStr := err.Error()
	redisErrors := []string{
		"connection refused",
		"connection reset",
		"i/o timeout",
		"EOF",
		"no connection",
		"broken pipe",
		"redis:",
	}

	for _, pattern := range redisErrors {
		if contains(errStr, pattern) {
			return true
		}
	}

	return errors.Is(err, ErrRedisUnavailable)
}

// contains 检查字符串是否包含子串（简单实现）
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
