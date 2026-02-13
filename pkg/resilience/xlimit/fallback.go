package xlimit

import (
	"context"
	"errors"
	"log/slog"
)

// fallbackLimiter 带降级能力的限流器
// 当分布式限流器（Redis）不可用时，自动降级到备选策略
type fallbackLimiter struct {
	distributed    Limiter
	local          Limiter
	strategy       FallbackStrategy
	opts           *options
	customFallback FallbackFunc
}

// newFallbackLimiter 创建带降级的限流器
func newFallbackLimiter(distributed, local Limiter, opts *options) *fallbackLimiter {
	return &fallbackLimiter{
		distributed:    distributed,
		local:          local,
		strategy:       opts.config.Fallback,
		opts:           opts,
		customFallback: opts.customFallback,
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
	if !IsRedisError(err) {
		return nil, err
	}

	// 记录降级日志和指标
	f.logFallback(ctx, err)
	if f.opts.metrics != nil {
		// 设计决策: 使用 classifyError 将错误归类为低基数标签，
		// 避免 err.Error() 原始字符串导致指标基数膨胀。
		f.opts.metrics.RecordFallback(ctx, f.strategy, classifyError(err))
	}

	// 触发降级回调
	if f.opts.onFallback != nil {
		f.opts.onFallback(key, f.strategy, err)
	}

	// 优先使用自定义降级函数
	if f.customFallback != nil {
		return f.customFallback(ctx, key, n, err)
	}

	// 执行默认降级策略
	return f.fallback(ctx, key, n)
}

// logFallback 记录降级日志
func (f *fallbackLimiter) logFallback(ctx context.Context, err error) {
	if f.opts.logger != nil {
		f.opts.logger.Warn(ctx, "rate limiter falling back due to Redis error",
			slog.String("strategy", string(f.strategy)),
			slog.String("error", err.Error()),
		)
	}
}

// fallback 执行降级策略
func (f *fallbackLimiter) fallback(ctx context.Context, key Key, n int) (*Result, error) {
	switch f.strategy {
	case FallbackLocal:
		return f.local.AllowN(ctx, key, n)

	case FallbackOpen:
		return &Result{
			Allowed: true,
			Rule:    "fallback-open",
		}, nil

	case FallbackClose:
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
	var errs []error

	// 使用类型断言检查 distributed 是否实现 Resetter
	if r, ok := f.distributed.(Resetter); ok {
		if err := r.Reset(ctx, key); err != nil && !IsRedisError(err) {
			errs = append(errs, err)
		}
	}

	// 使用类型断言检查 local 是否实现 Resetter
	if r, ok := f.local.(Resetter); ok {
		if err := r.Reset(ctx, key); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
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

	return errors.Join(errs...)
}

// Query 查询当前配额状态（不消耗配额）
// 优先从分布式限流器查询，失败时降级到本地
func (f *fallbackLimiter) Query(ctx context.Context, key Key) (*QuotaInfo, error) {
	// 使用类型断言检查 distributed 是否实现 Querier
	if q, ok := f.distributed.(Querier); ok {
		info, err := q.Query(ctx, key)
		if err == nil {
			return info, nil
		}

		// 如果是 Redis 错误，尝试从本地查询
		if IsRedisError(err) {
			if localQ, localOK := f.local.(Querier); localOK {
				return localQ.Query(ctx, key)
			}
		}

		return nil, err
	}

	// distributed 不支持 Query，尝试从 local 查询
	if q, ok := f.local.(Querier); ok {
		return q.Query(ctx, key)
	}

	return nil, ErrQueryNotSupported
}

// 确保 fallbackLimiter 实现了可选接口
var (
	_ Limiter  = (*fallbackLimiter)(nil)
	_ Querier  = (*fallbackLimiter)(nil)
	_ Resetter = (*fallbackLimiter)(nil)
)
