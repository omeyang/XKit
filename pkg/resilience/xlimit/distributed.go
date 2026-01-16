package xlimit

import (
	"context"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"
)

// distributedLimiter 基于 Redis 的分布式限流器
type distributedLimiter struct {
	limiter   *redis_rate.Limiter
	rdb       redis.UniversalClient
	matcher   *RuleMatcher
	keyPrefix string
	opts      *options
	closed    bool
}

// newDistributedLimiter 创建分布式限流器
func newDistributedLimiter(rdb redis.UniversalClient, matcher *RuleMatcher, opts *options) *distributedLimiter {
	return &distributedLimiter{
		limiter:   redis_rate.NewLimiter(rdb),
		rdb:       rdb,
		matcher:   matcher,
		keyPrefix: opts.config.KeyPrefix,
		opts:      opts,
	}
}

// Allow 检查是否允许单个请求通过
func (d *distributedLimiter) Allow(ctx context.Context, key Key) (*Result, error) {
	return d.AllowN(ctx, key, 1)
}

// AllowN 检查是否允许 n 个请求通过
func (d *distributedLimiter) AllowN(ctx context.Context, key Key, n int) (*Result, error) {
	if d.closed {
		return nil, ErrLimiterClosed
	}

	// 遍历所有规则进行检查（层级限流）
	// 保存最后一个通过的结果，返回时使用
	var lastResult *Result
	ruleNames := d.matcher.GetAllRules()

	for _, ruleName := range ruleNames {
		rule, found := d.matcher.FindRule(key, ruleName)
		if !found {
			continue
		}

		result, err := d.checkRule(ctx, key, rule, n)
		if err != nil {
			return nil, err
		}

		// 如果被任意规则拒绝，立即返回
		if !result.Allowed {
			d.callOnDeny(key, result)
			return result, nil
		}

		// 保存结果（用于返回）
		lastResult = result
	}

	// 如果没有任何规则匹配，返回默认允许
	if lastResult == nil {
		lastResult = AllowedResult(0, 0)
	}

	d.callOnAllow(key, lastResult)
	return lastResult, nil
}

// checkRule 检查单个规则
func (d *distributedLimiter) checkRule(ctx context.Context, key Key, rule Rule, n int) (*Result, error) {
	// 获取有效限流配置
	limit, window := d.matcher.GetEffectiveLimit(rule, key)
	burst := d.matcher.GetEffectiveBurst(rule, key)

	// 渲染 Redis 键
	redisKey := d.matcher.RenderKey(rule, key, d.keyPrefix)

	// 调用 redis_rate 进行限流检查
	rateLimit := redis_rate.Limit{
		Rate:   limit,
		Burst:  burst,
		Period: window,
	}

	res, err := d.limiter.AllowN(ctx, redisKey, rateLimit, n)
	if err != nil {
		return nil, err
	}

	return d.convertResult(res, rule, key, limit), nil
}

// convertResult 将 redis_rate 结果转换为 xlimit 结果
func (d *distributedLimiter) convertResult(res *redis_rate.Result, rule Rule, key Key, limit int) *Result {
	result := &Result{
		Allowed:    res.Allowed > 0,
		Limit:      limit,
		Remaining:  res.Remaining,
		ResetAt:    time.Now().Add(res.ResetAfter),
		RetryAfter: res.RetryAfter,
		Rule:       rule.Name,
		Key:        key.Render(rule.KeyTemplate),
	}
	return result
}

// Reset 重置指定键的限流计数
func (d *distributedLimiter) Reset(ctx context.Context, key Key) error {
	if d.closed {
		return ErrLimiterClosed
	}

	// 重置所有规则对应的键
	for _, ruleName := range d.matcher.GetAllRules() {
		rule, found := d.matcher.FindRule(key, ruleName)
		if !found {
			continue
		}

		redisKey := d.matcher.RenderKey(rule, key, d.keyPrefix)
		// 使用 redis_rate 的 Reset 方法确保正确重置
		if err := d.limiter.Reset(ctx, redisKey); err != nil {
			return err
		}
	}

	return nil
}

// Close 关闭限流器
func (d *distributedLimiter) Close() error {
	d.closed = true
	return nil
}

// callOnAllow 调用允许回调
func (d *distributedLimiter) callOnAllow(key Key, result *Result) {
	if d.opts.onAllow != nil {
		d.opts.onAllow(key, result)
	}
}

// callOnDeny 调用拒绝回调
func (d *distributedLimiter) callOnDeny(key Key, result *Result) {
	if d.opts.onDeny != nil {
		d.opts.onDeny(key, result)
	}
}
