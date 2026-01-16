package xlimit

import (
	"context"
	"sync"
	"time"
)

// localLimiter 本地限流器，使用内存存储
// 适用于单 Pod 场景或作为分布式限流的降级方案
type localLimiter struct {
	matcher  *RuleMatcher
	opts     *options
	buckets  sync.Map // map[string]*tokenBucket
	podCount int
	closed   bool
	mu       sync.RWMutex
}

// tokenBucket 令牌桶实现
type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	limit      int
	window     time.Duration
	lastUpdate time.Time
}

// newLocalLimiter 创建本地限流器
func newLocalLimiter(matcher *RuleMatcher, opts *options) *localLimiter {
	return &localLimiter{
		matcher:  matcher,
		opts:     opts,
		podCount: opts.config.EffectivePodCount(),
	}
}

// Allow 检查是否允许单个请求通过
func (l *localLimiter) Allow(ctx context.Context, key Key) (*Result, error) {
	return l.AllowN(ctx, key, 1)
}

// AllowN 检查是否允许 n 个请求通过
func (l *localLimiter) AllowN(ctx context.Context, key Key, n int) (*Result, error) {
	l.mu.RLock()
	if l.closed {
		l.mu.RUnlock()
		return nil, ErrLimiterClosed
	}
	l.mu.RUnlock()

	// 遍历所有规则进行检查
	var lastResult *Result
	for _, ruleName := range l.matcher.GetAllRules() {
		rule, found := l.matcher.FindRule(key, ruleName)
		if !found {
			continue
		}

		result, err := l.checkRule(ctx, key, rule, n)
		if err != nil {
			return nil, err
		}

		// 如果被任意规则拒绝，立即返回
		if !result.Allowed {
			l.callOnDeny(key, result)
			return result, nil
		}

		lastResult = result
	}

	// 如果没有任何规则匹配，返回默认允许
	if lastResult == nil {
		lastResult = AllowedResult(0, 0)
	}

	l.callOnAllow(key, lastResult)
	return lastResult, nil
}

// checkRule 检查单个规则
func (l *localLimiter) checkRule(ctx context.Context, key Key, rule Rule, n int) (*Result, error) {
	// 检查 context 是否已取消
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 获取有效限流配置
	limit, window := l.matcher.GetEffectiveLimit(rule, key)

	// 本地限流时，按 Pod 数量分摊配额
	localLimit := limit / l.podCount
	if localLimit < 1 {
		localLimit = 1
	}

	// 获取或创建令牌桶
	bucketKey := l.matcher.RenderKey(rule, key, "")
	bucket := l.getOrCreateBucket(bucketKey, localLimit, window)

	// 尝试获取令牌
	allowed, remaining, retryAfter := bucket.take(n)

	result := &Result{
		Allowed:    allowed,
		Limit:      localLimit,
		Remaining:  remaining,
		ResetAt:    time.Now().Add(window),
		RetryAfter: retryAfter,
		Rule:       rule.Name,
		Key:        key.Render(rule.KeyTemplate),
	}

	return result, nil
}

// getOrCreateBucket 获取或创建令牌桶
func (l *localLimiter) getOrCreateBucket(key string, limit int, window time.Duration) *tokenBucket {
	if val, ok := l.buckets.Load(key); ok {
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

	actual, _ := l.buckets.LoadOrStore(key, bucket)
	if tb, ok := actual.(*tokenBucket); ok {
		return tb
	}
	return bucket
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

// Reset 重置指定键的限流计数
func (l *localLimiter) Reset(ctx context.Context, key Key) error {
	l.mu.RLock()
	if l.closed {
		l.mu.RUnlock()
		return ErrLimiterClosed
	}
	l.mu.RUnlock()

	// 重置所有规则对应的键
	for _, ruleName := range l.matcher.GetAllRules() {
		rule, found := l.matcher.FindRule(key, ruleName)
		if !found {
			continue
		}

		bucketKey := l.matcher.RenderKey(rule, key, "")
		l.buckets.Delete(bucketKey)
	}

	return nil
}

// Close 关闭限流器
func (l *localLimiter) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	return nil
}

// callOnAllow 调用允许回调
func (l *localLimiter) callOnAllow(key Key, result *Result) {
	if l.opts.onAllow != nil {
		l.opts.onAllow(key, result)
	}
}

// callOnDeny 调用拒绝回调
func (l *localLimiter) callOnDeny(key Key, result *Result) {
	if l.opts.onDeny != nil {
		l.opts.onDeny(key, result)
	}
}
