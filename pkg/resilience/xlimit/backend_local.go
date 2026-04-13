package xlimit

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xlog"
)

// maxBuckets 桶数量上限（安全阀）
// 防止长时间降级 + 高基数 key 场景下内存无界增长。
// 超过上限后新 key 被拒绝（fail-close），已有 key 正常限流。
const maxBuckets = 1 << 16 // 65536

// localBackend 本地令牌桶后端
// 使用内存存储，适用于单 Pod 场景或作为分布式限流的降级方案
//
// 设计决策: buckets 使用 sync.Map 存储，无自动过期清理，但有 maxBuckets 安全阀。
// 理由：(1) 本地后端主要用于降级场景，非常驻存储；
// (2) 高基数场景应使用分布式后端（Redis 自带 TTL）；
// (3) 可通过 Reset 方法手动清理特定键；
// (4) bucketCount 超过 maxBuckets 时拒绝创建新桶（fail-close），防止 OOM。
// 如需定期清理，由调用方通过重建 limiter 实例实现。
type localBackend struct {
	buckets          sync.Map // map[string]*tokenBucket
	bucketCount      atomic.Int64
	podCount         int
	podCountProvider PodCountProvider
	logger           xlog.Logger // 可选，nil 时使用 slog 降级
}

// newLocalBackend 创建本地后端
func newLocalBackend(podCount int, podCountProvider PodCountProvider, logger xlog.Logger) *localBackend {
	return &localBackend{
		podCount:         podCount,
		podCountProvider: podCountProvider,
		logger:           logger,
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
	if bucket == nil {
		// 桶数量超过 maxBuckets 安全阀，拒绝新 key（fail-close）
		return CheckResult{
			Allowed:    false,
			Limit:      localLimit,
			Remaining:  0,
			ResetAt:    time.Now().Add(window),
			RetryAfter: window,
		}, nil
	}
	// take 内部会先按旧参数补令牌到 now，再切换到新参数，避免参数变更
	// retroactive 应用于过去区间导致的过放/欠放（FG-M2 fix）。
	allowed, remaining, retryAfter := bucket.takeWithParams(localLimit, localBurst, window, n)

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
	if _, loaded := b.buckets.LoadAndDelete(key); loaded {
		b.bucketCount.Add(-1)
	}
	return nil
}

// Query 查询当前配额状态（不消耗配额）
//
// 设计决策: 已有桶时按经过时间补充令牌后返回（只读，不修改桶状态），
// 与 take() 的补令牌逻辑一致；无桶时返回 localBurst（桶容量）作为剩余配额，
// 与新创建桶的初始令牌数（burst）语义对齐。
func (b *localBackend) Query(ctx context.Context, key string, limit, burst int, window time.Duration) (
	effectiveLimit, remaining int, resetAt time.Time, err error) {
	// 与 CheckRule 的 ctx 契约一致（FG-M4 fix）
	if cerr := ctx.Err(); cerr != nil {
		return 0, 0, time.Time{}, cerr
	}
	// 获取当前 Pod 数量
	podCount := b.getPodCount(ctx)
	localLimit := max(limit/podCount, 1)
	localBurst := max(burst/podCount, 1)

	resetAt = time.Now().Add(window)
	remaining = localBurst // 无桶时默认为桶容量，与新建桶初始 tokens=burst 一致

	if val, ok := b.buckets.Load(key); ok {
		if bucket, ok := val.(*tokenBucket); ok {
			remaining = bucket.currentTokens(localLimit, localBurst, window)
		}
	}

	return localLimit, remaining, resetAt, nil
}

// Close 关闭后端
func (b *localBackend) Close(_ context.Context) error {
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
//
// 设计决策: 复用已有桶时由 takeWithParams 在桶锁内刷新参数，
// 确保动态 Pod 数量变化后存量桶的补令牌速率和容量按旧参数补齐到 now 再切换新参数
// （FG-M2 fix，避免 retroactive 应用）。
// 当桶数量超过 maxBuckets 时返回 nil，由调用方执行 fail-close。
//
// maxBuckets 使用 CAS 预留名额保证并发下限制严格生效（FG-M3 fix）：
// 先 CAS 自增 bucketCount，若超限则回退并返回 nil；LoadOrStore 命中已有 key 时释放预留。
func (b *localBackend) getOrCreateBucket(key string, limit, burst int, window time.Duration) *tokenBucket {
	if val, ok := b.buckets.Load(key); ok {
		if bucket, ok := val.(*tokenBucket); ok {
			return bucket
		}
	}

	// CAS 预留桶名额
	for {
		cur := b.bucketCount.Load()
		if cur >= maxBuckets {
			return nil
		}
		if b.bucketCount.CompareAndSwap(cur, cur+1) {
			break
		}
	}

	bucket := &tokenBucket{
		tokens:     float64(burst),
		limit:      limit,
		burst:      burst,
		window:     window,
		lastUpdate: time.Now(),
	}

	actual, loaded := b.buckets.LoadOrStore(key, bucket)
	if loaded {
		// 已有他人创建，释放预留名额
		b.bucketCount.Add(-1)
	}
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
	limit      int // 补令牌速率基数
	burst      int // 桶容量（突发上限）
	window     time.Duration
	lastUpdate time.Time
}

// takeWithParams 在桶锁内：先按旧参数补令牌到 now，再切换新参数（截断到新 burst），
// 最后尝试消耗 n 个令牌。确保参数变更（Pod 数/规则覆盖）不会 retroactive 地
// 用新速率重新计算过去时段（FG-M2 fix）。
func (tb *tokenBucket) takeWithParams(limit, burst int, window time.Duration, n int) (
	allowed bool, remaining int, retryAfter time.Duration) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastUpdate)
	if elapsed < 0 {
		// 时钟回拨防御：不补令牌，仅推进 lastUpdate
		elapsed = 0
	}
	// 先按旧参数补令牌
	if tb.window > 0 {
		oldRate := float64(tb.limit) / tb.window.Seconds()
		tb.tokens += oldRate * elapsed.Seconds()
	}
	if tb.tokens > float64(tb.burst) {
		tb.tokens = float64(tb.burst)
	}
	tb.lastUpdate = now

	// 切换到新参数并按新 burst 截断
	tb.limit = limit
	tb.burst = burst
	tb.window = window
	if tb.tokens > float64(burst) {
		tb.tokens = float64(burst)
	}

	if float64(n) <= tb.tokens {
		tb.tokens -= float64(n)
		return true, int(tb.tokens), 0
	}

	// 令牌不足
	if window <= 0 || limit <= 0 {
		return false, 0, 0
	}
	rate := float64(limit) / window.Seconds()
	deficit := float64(n) - tb.tokens
	waitTime := time.Duration(deficit / rate * float64(time.Second))
	return false, 0, waitTime
}

// currentTokens 返回当前令牌数（只读查询，不修改桶状态）
// 按经过时间补充令牌后返回，与 take() 的补令牌逻辑一致。
func (tb *tokenBucket) currentTokens(limit, burst int, window time.Duration) int {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	elapsed := time.Since(tb.lastUpdate)
	if elapsed < 0 {
		elapsed = 0
	}
	if window <= 0 {
		return int(tb.tokens)
	}
	rate := float64(limit) / window.Seconds()
	tokens := tb.tokens + rate*elapsed.Seconds()

	if tokens > float64(burst) {
		tokens = float64(burst)
	}
	return int(tokens)
}

// 确保 localBackend 实现了 Backend 接口
var _ Backend = (*localBackend)(nil)
