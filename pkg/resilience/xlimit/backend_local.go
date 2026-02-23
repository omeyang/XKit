package xlimit

import (
	"context"
	"sync"
	"time"
)

// localBackend 本地令牌桶后端
// 使用内存存储，适用于单 Pod 场景或作为分布式限流的降级方案
//
// 设计决策: buckets 使用 sync.Map 存储，当前无自动过期清理。
// 理由：(1) 本地后端主要用于降级场景，非常驻存储；
// (2) 高基数场景应使用分布式后端（Redis 自带 TTL）；
// (3) 可通过 Reset 方法手动清理特定键。
// 如需定期清理，由调用方通过重建 limiter 实例实现。
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
//
// 设计决策: burst 用作令牌桶容量（突发上限），limit/window 用作补令牌速率，
// 与 Redis 后端（redis_rate.Limit{Rate: limit, Burst: burst}）语义对齐。
// 降级场景下本地后端可正确处理突发流量。
func (b *localBackend) CheckRule(ctx context.Context, key string, limit, burst int, window time.Duration, n int) (CheckResult, error) {
	if err := ctx.Err(); err != nil {
		return CheckResult{}, err
	}

	// 获取当前 Pod 数量（支持动态获取）
	podCount := b.getPodCount(ctx)

	// 本地限流时，按 Pod 数量分摊配额
	localLimit := max(limit/podCount, 1)
	localBurst := max(burst/podCount, 1)

	bucket := b.getOrCreateBucket(key, localLimit, localBurst, window)
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
	effectiveLimit, remaining int, resetAt time.Time, err error) {
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

	return localLimit, remaining, resetAt, nil
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
// 设计决策: 复用已有桶时刷新参数，确保动态 Pod 数量变化后
// 存量桶的补令牌速率和容量与新计算的 localLimit/localBurst 一致。
func (b *localBackend) getOrCreateBucket(key string, limit, burst int, window time.Duration) *tokenBucket {
	if val, ok := b.buckets.Load(key); ok {
		if bucket, ok := val.(*tokenBucket); ok {
			bucket.updateParams(limit, burst, window)
			return bucket
		}
	}

	bucket := &tokenBucket{
		tokens:     float64(burst),
		limit:      limit,
		burst:      burst,
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
// limit 控制补令牌速率（limit/window），burst 控制桶容量（突发上限）
type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	limit      int           // 补令牌速率基数
	burst      int           // 桶容量（突发上限）
	window     time.Duration
	lastUpdate time.Time
}

// updateParams 原子刷新桶参数，使动态 Pod 数量变化对存量桶生效。
// 若 burst 缩小导致当前 tokens 超过新容量，截断到新容量。
func (tb *tokenBucket) updateParams(limit, burst int, window time.Duration) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.limit = limit
	tb.burst = burst
	tb.window = window

	if tb.tokens > float64(burst) {
		tb.tokens = float64(burst)
	}
}

// take 尝试从令牌桶获取 n 个令牌
func (tb *tokenBucket) take(n int) (allowed bool, remaining int, retryAfter time.Duration) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastUpdate)
	tb.lastUpdate = now

	// 计算新增的令牌数（速率 = limit / window）
	rate := float64(tb.limit) / tb.window.Seconds()
	tb.tokens += rate * elapsed.Seconds()

	// 令牌数不超过桶容量（burst）
	if tb.tokens > float64(tb.burst) {
		tb.tokens = float64(tb.burst)
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
