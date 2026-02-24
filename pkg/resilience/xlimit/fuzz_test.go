package xlimit

import (
	"context"
	"testing"
	"time"
)

// FuzzRule_Validate 测试规则验证的模糊测试
func FuzzRule_Validate(f *testing.F) {
	// 添加种子数据
	f.Add("rule1", "tenant:${tenant_id}", 100, int64(time.Second), 150)
	f.Add("", "", 0, int64(0), 0)
	f.Add("test", "key", -1, int64(-time.Second), -1)
	f.Add("a", "b", 1, int64(time.Millisecond), 1)
	f.Add("very-long-name-for-a-rule-that-might-cause-issues", "complex:${tenant_id}:${caller_id}:${method}:${path}", 999999, int64(time.Hour), 1000000)

	f.Fuzz(func(t *testing.T, name, keyTemplate string, limit int, window int64, burst int) {
		rule := Rule{
			Name:        name,
			KeyTemplate: keyTemplate,
			Limit:       limit,
			Window:      time.Duration(window),
			Burst:       burst,
		}

		// Validate 不应该 panic
		_ = rule.Validate()

		// IsEnabled 不应该 panic
		_ = rule.IsEnabled()

		// EffectiveBurst 不应该 panic
		_ = rule.EffectiveBurst()
	})
}

// FuzzOverride_Validate 测试覆盖配置验证的模糊测试
func FuzzOverride_Validate(f *testing.F) {
	f.Add("tenant:vip-*", 5000, int64(time.Second), 7500)
	f.Add("", 0, int64(0), 0)
	f.Add("*", -1, int64(-time.Minute), -1)
	f.Add("a:b:c:*", 1, int64(time.Millisecond), 2)

	f.Fuzz(func(t *testing.T, match string, limit int, window int64, burst int) {
		override := Override{
			Match:  match,
			Limit:  limit,
			Window: time.Duration(window),
			Burst:  burst,
		}

		// Validate 不应该 panic
		_ = override.Validate()
	})
}

// FuzzLocalLimiter_Allow 测试本地限流器的模糊测试
func FuzzLocalLimiter_Allow(f *testing.F) {
	f.Add("tenant123", 1, 100, int64(time.Second))
	f.Add("", 1, 1, int64(time.Millisecond))
	f.Add("tenant-with-dashes", 10, 1000, int64(time.Minute))
	f.Add("特殊租户", 5, 50, int64(time.Second))

	f.Fuzz(func(t *testing.T, tenant string, n int, limit int, window int64) {
		// 限制参数范围以避免资源耗尽
		if limit < 1 || limit > 10000 {
			limit = 100
		}
		if window < int64(time.Millisecond) || window > int64(time.Hour) {
			window = int64(time.Second)
		}
		if n < 1 || n > 100 {
			n = 1
		}

		limiter, err := NewLocal(
			WithRules(TenantRule("tenant", limit, time.Duration(window))),
		)
		if err != nil {
			return // 无效配置，跳过
		}
		defer limiter.Close(context.Background())

		ctx := context.Background()
		key := Key{Tenant: tenant}

		// AllowN 不应该 panic
		_, _ = limiter.AllowN(ctx, key, n)
	})
}

// FuzzRuleMatcher 测试规则匹配器的模糊测试
func FuzzRuleMatcher(f *testing.F) {
	f.Add("tenant123", "caller-service", "POST", "/v1/users")
	f.Add("", "", "", "")
	f.Add("vip-corp", "order-service", "GET", "/health")
	f.Add("tenant:special", "caller/slash", "DELETE", "path?query")

	f.Fuzz(func(t *testing.T, tenant, caller, method, path string) {
		rules := []Rule{
			TenantRule("tenant", 1000, time.Minute),
			GlobalRule("global", 10000, time.Minute),
			TenantAPIRule("tenant-api", 100, time.Second),
			CallerRule("caller", 500, time.Minute),
		}

		matcher := newRuleMatcher(rules)
		key := Key{
			Tenant: tenant,
			Caller: caller,
			Method: method,
			Path:   path,
		}

		// 所有操作都不应该 panic
		for _, ruleName := range []string{"tenant", "global", "tenant-api", "caller", "nonexistent"} {
			rule, found := matcher.findRule(ruleName)
			if found {
				_, _ = matcher.getEffectiveLimit(rule, key)
				_ = matcher.getEffectiveBurst(rule, key)
				_ = matcher.renderKey(rule, key, "prefix:")
			}
		}

		_ = matcher.getAllRules()
		_ = matcher.hasRule("tenant")
	})
}

// FuzzRuleBuilder 测试规则构建器的模糊测试
func FuzzRuleBuilder(f *testing.F) {
	f.Add("rule1", "tenant:${tenant_id}", 100, int64(time.Second), 150, "tenant:vip-*", 500)
	f.Add("", "", 0, int64(0), 0, "", 0)
	f.Add("test", "key", 1, int64(time.Millisecond), 2, "*", 1)

	f.Fuzz(func(t *testing.T, name, keyTemplate string, limit int, window int64, burst int, overrideMatch string, overrideLimit int) {
		// 构建器操作不应该 panic
		builder := NewRuleBuilder(name).
			KeyTemplate(keyTemplate).
			Limit(limit).
			Window(time.Duration(window)).
			Burst(burst)

		if overrideMatch != "" && overrideLimit > 0 {
			builder.AddOverride(overrideMatch, overrideLimit)
		}

		_ = builder.Build()
	})
}

// FuzzConfig_Validate 测试配置验证的模糊测试
func FuzzConfig_Validate(f *testing.F) {
	f.Add("ratelimit:", 3, true, true)
	f.Add("", 0, false, false)
	f.Add("custom:prefix:", 10, true, false)

	f.Fuzz(func(t *testing.T, keyPrefix string, podCount int, enableMetrics, enableHeaders bool) {
		config := Config{
			KeyPrefix:     keyPrefix,
			LocalPodCount: podCount,
			EnableMetrics: enableMetrics,
			EnableHeaders: enableHeaders,
			Rules: []Rule{
				TenantRule("tenant", 1000, time.Minute),
			},
		}

		// Validate 不应该 panic
		_ = config.Validate()

		// EffectivePodCount 不应该 panic
		_ = config.EffectivePodCount()

		// Clone 不应该 panic
		_ = config.Clone()
	})
}

// FuzzResult_Headers 测试结果头部的模糊测试
func FuzzResult_Headers(f *testing.F) {
	f.Add(true, 100, 50, int64(time.Second), int64(time.Minute), "rule1", "key1")
	f.Add(false, 0, 0, int64(0), int64(0), "", "")
	f.Add(true, 1000000, 999999, int64(time.Hour), int64(time.Hour), "test", "test")

	f.Fuzz(func(t *testing.T, allowed bool, limit, remaining int, retryAfter, resetAfter int64, rule, key string) {
		result := &Result{
			Allowed:    allowed,
			Limit:      limit,
			Remaining:  remaining,
			RetryAfter: time.Duration(retryAfter),
			ResetAt:    time.Now().Add(time.Duration(resetAfter)),
			Rule:       rule,
			Key:        key,
		}

		// Headers 不应该 panic
		_ = result.Headers()
	})
}
