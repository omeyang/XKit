package xlimit

import (
	"context"
	"sync"
	"time"
)

// localBackend 本地令牌桶后端
// 使用内存存储，适用于单 Pod 场景或作为分布式限流的降级方案
type localBackend struct {
	buckets          sync.Map // map[string]*tokenBucket
	podCount         int
	podCountProvider PodCountProvider
}

// newLocalBackend 创建本地后端
func newLocalBackend(podCount int, podCountProvider PodCountProvider) *localBackend {
	return &localBackend{
		podCount:         podCount,
		podCountProvider: podCountProvider,
	}
}

// Type 返回后端类型
func (b *localBackend) Type() string {
	return "local"
}

// CheckRule 检查单个规则是否允许请求通过
func (b *localBackend) CheckRule(ctx context.Context, key string, limit, _ int, window time.Duration, n int) (CheckResult, error) {
	if err := ctx.Err(); err != nil {
		return CheckResult{}, err
	}

	// 获取当前 Pod 数量（支持动态获取）
	podCount := b.getPodCount(ctx)

	// 本地限流时，按 Pod 数量分摊配额
	localLimit := max(limit/podCount, 1)

	bucket := b.getOrCreateBucket(key, localLimit, window)
	allowed, remaining, retryAfter := bucket.take(n)

	return CheckResult{
		Allowed:    allowed,
		Limit:      localLimit,
		Remaining:  remaining,
		ResetAt:    time.Now().Add(window),
		RetryAfter: retryAfter,
	}, nil
}

// Reset 重置指定键的限流计数
func (b *localBackend) Reset(_ context.Context, key string) error {
	b.buckets.Delete(key)
	return nil
}

// Query 查询当前配额状态（不消耗配额）
func (b *localBackend) Query(ctx context.Context, key string, limit, _ int, window time.Duration) (
	remaining int, resetAt time.Time, err error) {
	// 获取当前 Pod 数量
	podCount := b.getPodCount(ctx)
	localLimit := max(limit/podCount, 1)

	resetAt = time.Now().Add(window)
	remaining = localLimit

	if val, ok := b.buckets.Load(key); ok {
		if bucket, ok := val.(*tokenBucket); ok {
			bucket.mu.Lock()
			remaining = int(bucket.tokens)
			bucket.mu.Unlock()
		}
	}

	return remaining, resetAt, nil
}

// Close 关闭后端
func (b *localBackend) Close() error {
	return nil
}

// getPodCount 获取当前 Pod 数量
func (b *localBackend) getPodCount(ctx context.Context) int {
	if b.podCountProvider != nil {
		if count, err := b.podCountProvider.GetPodCount(ctx); err == nil && count > 0 {
			return count
		}
	}
	if b.podCount > 0 {
		return b.podCount
	}
	return 1
}

// getOrCreateBucket 获取或创建令牌桶
func (b *localBackend) getOrCreateBucket(key string, limit int, window time.Duration) *tokenBucket {
	if val, ok := b.buckets.Load(key); ok {
		if bucket, ok := val.(*tokenBucket); ok {
			return bucket
		}
	}

	bucket := &tokenBucket{
		tokens:     float64(limit),
		limit:      limit,
		window:     window,
		lastUpdate: time.Now(),
	}

	actual, _ := b.buckets.LoadOrStore(key, bucket)
	if tb, ok := actual.(*tokenBucket); ok {
		return tb
	}
	return bucket
}

// tokenBucket 令牌桶实现
type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	limit      int
	window     time.Duration
	lastUpdate time.Time
}

// take 尝试从令牌桶获取 n 个令牌
func (tb *tokenBucket) take(n int) (allowed bool, remaining int, retryAfter time.Duration) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastUpdate)
	tb.lastUpdate = now

	// 计算新增的令牌数
	rate := float64(tb.limit) / tb.window.Seconds()
	tb.tokens += rate * elapsed.Seconds()

	// 令牌数不超过上限
	if tb.tokens > float64(tb.limit) {
		tb.tokens = float64(tb.limit)
	}

	// 检查是否有足够的令牌
	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return true, int(tb.tokens), 0
	}

	// 令牌不足，计算需要等待的时间
	deficit := float64(n) - tb.tokens
	waitTime := time.Duration(deficit / rate * float64(time.Second))

	return false, 0, waitTime
}

// 确保 localBackend 实现了 Backend 接口
var _ Backend = (*localBackend)(nil)
