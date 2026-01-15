package xlimit

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"
)

// limiterCore 限流器核心实现
// 使用组合模式，将公共的限流流程与具体的后端实现分离
// 职责:
//   - 遍历规则并调用后端检查
//   - 处理可观测性（span、metrics）
//   - 调用回调函数
type limiterCore struct {
	backend Backend
	matcher *ruleMatcher
	opts    *options
	closed  atomic.Bool
}

// newLimiterCore 创建限流器核心
func newLimiterCore(backend Backend, matcher *ruleMatcher, opts *options) *limiterCore {
	return &limiterCore{
		backend: backend,
		matcher: matcher,
		opts:    opts,
	}
}

// Allow 检查是否允许单个请求通过
func (c *limiterCore) Allow(ctx context.Context, key Key) (*Result, error) {
	return c.AllowN(ctx, key, 1)
}

// AllowN 检查是否允许 n 个请求通过
// 这是核心限流逻辑，统一处理:
//   - 关闭状态检查
//   - 可观测性（span 创建、指标记录）
//   - 规则遍历
//   - 回调调用
func (c *limiterCore) AllowN(ctx context.Context, key Key, n int) (*Result, error) {
	if c.closed.Load() {
		return nil, ErrLimiterClosed
	}

	start := time.Now()
	limiterType := c.backend.Type()

	// 启动追踪 span
	ctx, span := xmetrics.Start(ctx, c.opts.observer, xmetrics.SpanOptions{
		Component: "xlimit",
		Operation: "allow",
		Kind:      xmetrics.KindInternal,
		Attrs: []xmetrics.Attr{
			xmetrics.String("limiter.type", limiterType),
			xmetrics.Int("request.count", n),
		},
	})

	var lastResult *Result
	var err error

	defer func() {
		result := xmetrics.Result{Err: err}
		if lastResult != nil {
			result.Attrs = []xmetrics.Attr{
				xmetrics.Bool("allowed", lastResult.Allowed),
				xmetrics.String("rule", lastResult.Rule),
				xmetrics.Int("remaining", lastResult.Remaining),
			}
			// 记录指标
			c.opts.metrics.RecordAllow(ctx, limiterType, lastResult.Rule, lastResult.Allowed, time.Since(start))
		}
		span.End(result)
	}()

	// 遍历所有规则
	for _, ruleName := range c.matcher.getAllRules() {
		rule, found := c.matcher.findRule(key, ruleName)
		if !found {
			continue
		}

		lastResult, err = c.checkRule(ctx, key, rule, n)
		if err != nil {
			return nil, err
		}

		if !lastResult.Allowed {
			c.callOnDeny(ctx, key, lastResult)
			return lastResult, nil
		}
	}

	if lastResult == nil {
		lastResult = AllowedResult(0, 0)
	}

	c.callOnAllow(ctx, key, lastResult)
	return lastResult, nil
}

// checkRule 检查单个规则
func (c *limiterCore) checkRule(ctx context.Context, key Key, rule Rule, n int) (*Result, error) {
	limit, window := c.matcher.getEffectiveLimit(rule, key)
	burst := c.matcher.getEffectiveBurst(rule, key)
	renderedKey := c.matcher.renderKey(rule, key, c.opts.config.KeyPrefix)

	res, err := c.backend.CheckRule(ctx, renderedKey, limit, burst, window, n)
	if err != nil {
		return nil, err
	}

	return &Result{
		Allowed:    res.Allowed,
		Limit:      res.Limit, // 使用后端返回的实际 limit（本地后端可能会调整）
		Remaining:  res.Remaining,
		ResetAt:    res.ResetAt,
		RetryAfter: res.RetryAfter,
		Rule:       rule.Name,
		Key:        key.Render(rule.KeyTemplate),
	}, nil
}

// Reset 重置指定键的限流计数
func (c *limiterCore) Reset(ctx context.Context, key Key) error {
	if c.closed.Load() {
		return ErrLimiterClosed
	}

	for _, ruleName := range c.matcher.getAllRules() {
		rule, found := c.matcher.findRule(key, ruleName)
		if !found {
			continue
		}

		renderedKey := c.matcher.renderKey(rule, key, c.opts.config.KeyPrefix)
		if err := c.backend.Reset(ctx, renderedKey); err != nil {
			return err
		}
	}

	return nil
}

// Query 查询当前配额状态（不消耗配额）
func (c *limiterCore) Query(ctx context.Context, key Key) (*QuotaInfo, error) {
	if c.closed.Load() {
		return nil, ErrLimiterClosed
	}

	for _, ruleName := range c.matcher.getAllRules() {
		rule, found := c.matcher.findRule(key, ruleName)
		if !found {
			continue
		}

		limit, window := c.matcher.getEffectiveLimit(rule, key)
		burst := c.matcher.getEffectiveBurst(rule, key)
		renderedKey := c.matcher.renderKey(rule, key, c.opts.config.KeyPrefix)

		remaining, resetAt, err := c.backend.Query(ctx, renderedKey, limit, burst, window)
		if err != nil {
			return nil, err
		}

		return &QuotaInfo{
			Limit:     limit,
			Remaining: remaining,
			ResetAt:   resetAt,
			Rule:      rule.Name,
			Key:       key.Render(rule.KeyTemplate),
		}, nil
	}

	return nil, ErrConfigNotFound
}

// Close 关闭限流器
func (c *limiterCore) Close() error {
	c.closed.Store(true)
	return c.backend.Close()
}

// callOnAllow 调用允许回调并记录日志
func (c *limiterCore) callOnAllow(ctx context.Context, key Key, result *Result) {
	if c.opts.onAllow != nil {
		c.opts.onAllow(key, result)
	}

	if c.opts.logger != nil {
		c.opts.logger.Debug(ctx, "rate limit allowed",
			slog.String("limiter_type", c.backend.Type()),
			slog.String("rule", result.Rule),
			slog.String("key", result.Key),
			slog.Int("remaining", result.Remaining),
		)
	}
}

// callOnDeny 调用拒绝回调并记录日志
func (c *limiterCore) callOnDeny(ctx context.Context, key Key, result *Result) {
	if c.opts.onDeny != nil {
		c.opts.onDeny(key, result)
	}

	if c.opts.logger != nil {
		c.opts.logger.Warn(ctx, "rate limit exceeded",
			slog.String("limiter_type", c.backend.Type()),
			slog.String("rule", result.Rule),
			slog.String("key", result.Key),
			slog.Int("limit", result.Limit),
			slog.Duration("retry_after", result.RetryAfter),
		)
	}
}

// 确保 limiterCore 实现了必要接口
var (
	_ Limiter  = (*limiterCore)(nil)
	_ Querier  = (*limiterCore)(nil)
	_ Resetter = (*limiterCore)(nil)
)
