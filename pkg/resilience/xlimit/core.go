package xlimit

import (
	"context"
	"fmt"
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
//   - 参数校验（n 必须为正整数）
//   - 关闭状态检查
//   - 可观测性（span 创建、指标记录）
//   - 规则遍历
//   - 回调调用
func (c *limiterCore) AllowN(ctx context.Context, key Key, n int) (*Result, error) {
	if n <= 0 {
		return nil, fmt.Errorf("%w: must be positive, got %d", ErrInvalidN, n)
	}

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
		duration := time.Since(start)
		result := xmetrics.Result{Err: err}
		if lastResult != nil {
			result.Attrs = []xmetrics.Attr{
				xmetrics.Bool("allowed", lastResult.Allowed),
				xmetrics.String("rule", lastResult.Rule),
				xmetrics.Int("remaining", lastResult.Remaining),
			}
			c.opts.metrics.RecordAllow(ctx, limiterType, lastResult.Rule, lastResult.Allowed, duration)
		} else if err != nil {
			// 设计决策: 错误路径也记录检查时延，确保监控面板能完整观测限流器内部错误率。
			// 使用 rule="error" 标签区分正常路径和错误路径。
			c.opts.metrics.RecordAllow(ctx, limiterType, "error", false, duration)
		}
		span.End(result)
	}()

	// 设计决策: 遍历所有规则，跟踪 Remaining 最小的结果（mostRestrictive）返回给调用方。
	// 与 Query 方法返回"最受限规则"的语义保持一致，确保 HTTP 头 X-RateLimit-Remaining
	// 反映真实的最小剩余配额，避免误导客户端。
	lastResult, err = c.evaluateRules(ctx, key, n)
	if err != nil {
		return nil, err
	}
	if lastResult != nil && !lastResult.Allowed {
		c.callOnDeny(ctx, key, lastResult)
		return lastResult, nil
	}

	// 设计决策: 无匹配规则时默认放行（fail-open）。
	// 理由：(1) 动态配置场景中可能临时无规则；(2) 明确拒绝应通过规则配置实现。
	// 如需强制限流，确保至少配置一条启用的规则。
	if lastResult == nil {
		lastResult = AllowedResult(0, 0)
	}

	c.callOnAllow(ctx, key, lastResult)
	return lastResult, nil
}

// evaluateRules 遍历所有规则并返回最受限的允许结果。
// 若某条规则拒绝请求，立即返回该拒绝结果；
// 若所有规则通过，返回 Remaining 最小的结果；
// 若无匹配规则，返回 (nil, nil)。
func (c *limiterCore) evaluateRules(ctx context.Context, key Key, n int) (*Result, error) {
	var mostRestrictive *Result

	for _, ruleName := range c.matcher.getAllRules() {
		rule, found := c.matcher.findRule(ruleName)
		if !found {
			continue
		}

		result, err := c.checkRule(ctx, key, rule, n)
		if err != nil {
			return nil, err
		}

		if !result.Allowed {
			return result, nil
		}

		if mostRestrictive == nil || result.Remaining < mostRestrictive.Remaining {
			mostRestrictive = result
		}
	}

	return mostRestrictive, nil
}

// checkRule 检查单个规则
//
// 设计决策: 在入口处调用一次 key.Render，将结果传递给 getEffectiveLimit、
// getEffectiveBurst 和 renderKey，避免热路径上 3 次重复的模板解析和字符串分配。
func (c *limiterCore) checkRule(ctx context.Context, key Key, rule Rule, n int) (*Result, error) {
	rendered := key.Render(rule.KeyTemplate)
	limit, window := c.matcher.getEffectiveLimit(rule, rendered)
	burst := c.matcher.getEffectiveBurst(rule, rendered)
	fullKey := c.matcher.renderKey(rendered, c.opts.config.KeyPrefix)

	res, err := c.backend.CheckRule(ctx, fullKey, limit, burst, window, n)
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
		Key:        rendered,
	}, nil
}

// Reset 重置指定键的限流计数
func (c *limiterCore) Reset(ctx context.Context, key Key) error {
	if c.closed.Load() {
		return ErrLimiterClosed
	}

	for _, ruleName := range c.matcher.getAllRules() {
		rule, found := c.matcher.findRule(ruleName)
		if !found {
			continue
		}

		rendered := key.Render(rule.KeyTemplate)
		fullKey := c.matcher.renderKey(rendered, c.opts.config.KeyPrefix)
		if err := c.backend.Reset(ctx, fullKey); err != nil {
			return err
		}
	}

	return nil
}

// Query 查询当前配额状态（不消耗配额）
//
// 设计决策: 遍历所有匹配规则，返回剩余配额最少（最受限）的那条规则信息，
// 与 AllowN 的"任一规则拒绝即拒绝"语义保持一致。
func (c *limiterCore) Query(ctx context.Context, key Key) (*QuotaInfo, error) {
	if c.closed.Load() {
		return nil, ErrLimiterClosed
	}

	var mostRestrictive *QuotaInfo

	for _, ruleName := range c.matcher.getAllRules() {
		rule, found := c.matcher.findRule(ruleName)
		if !found {
			continue
		}

		rendered := key.Render(rule.KeyTemplate)
		limit, window := c.matcher.getEffectiveLimit(rule, rendered)
		burst := c.matcher.getEffectiveBurst(rule, rendered)
		fullKey := c.matcher.renderKey(rendered, c.opts.config.KeyPrefix)

		effectiveLimit, remaining, resetAt, err := c.backend.Query(ctx, fullKey, limit, burst, window)
		if err != nil {
			return nil, err
		}

		info := &QuotaInfo{
			Limit:     effectiveLimit,
			Remaining: remaining,
			ResetAt:   resetAt,
			Rule:      rule.Name,
			Key:       rendered,
		}

		if mostRestrictive == nil || remaining < mostRestrictive.Remaining {
			mostRestrictive = info
		}
	}

	if mostRestrictive == nil {
		return nil, ErrNoRuleMatched
	}

	return mostRestrictive, nil
}

// Close 关闭限流器
func (c *limiterCore) Close(ctx context.Context) error {
	c.closed.Store(true)
	return c.backend.Close(ctx)
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
