package xlimit

import (
	"context"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"
)

// redisBackend 基于 Redis 的分布式限流后端
type redisBackend struct {
	limiter *redis_rate.Limiter
	rdb     redis.UniversalClient
}

// newRedisBackend 创建 Redis 后端
func newRedisBackend(rdb redis.UniversalClient) *redisBackend {
	return &redisBackend{
		limiter: redis_rate.NewLimiter(rdb),
		rdb:     rdb,
	}
}

// Type 返回后端类型
func (b *redisBackend) Type() string {
	return "distributed"
}

// CheckRule 检查单个规则是否允许请求通过
func (b *redisBackend) CheckRule(ctx context.Context, key string, limit, burst int, window time.Duration, n int) (CheckResult, error) {
	rateLimit := redis_rate.Limit{
		Rate:   limit,
		Burst:  burst,
		Period: window,
	}

	res, err := b.limiter.AllowN(ctx, key, rateLimit, n)
	if err != nil {
		return CheckResult{}, err
	}

	return CheckResult{
		Allowed:    res.Allowed > 0,
		Limit:      limit,
		Remaining:  res.Remaining,
		ResetAt:    time.Now().Add(res.ResetAfter),
		RetryAfter: res.RetryAfter,
	}, nil
}

// Reset 重置指定键的限流计数
func (b *redisBackend) Reset(ctx context.Context, key string) error {
	return b.limiter.Reset(ctx, key)
}

// Query 查询当前配额状态（不消耗配额）
func (b *redisBackend) Query(ctx context.Context, key string, limit, burst int, window time.Duration) (
	remaining int, resetAt time.Time, err error) {

	rateLimit := redis_rate.Limit{
		Rate:   limit,
		Burst:  burst,
		Period: window,
	}

	// 使用 AllowN(0) 来查询当前状态而不消耗配额
	res, err := b.limiter.AllowN(ctx, key, rateLimit, 0)
	if err != nil {
		return 0, time.Time{}, err
	}

	return res.Remaining, time.Now().Add(res.ResetAfter), nil
}

// Close 关闭后端
func (b *redisBackend) Close() error {
	return nil
}

// 确保 redisBackend 实现了 Backend 接口
var _ Backend = (*redisBackend)(nil)
