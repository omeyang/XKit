package xlimit

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// BenchmarkLocal_Allow_Single 测试本地限流器单线程性能
func BenchmarkLocal_Allow_Single(b *testing.B) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant", 1000000, time.Second)),
	)
	if err != nil {
		b.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	key := Key{Tenant: "benchmark-tenant"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = limiter.Allow(ctx, key)
	}
}

// BenchmarkLocal_Allow_Parallel 测试本地限流器并发性能
func BenchmarkLocal_Allow_Parallel(b *testing.B) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant", 10000000, time.Second)),
	)
	if err != nil {
		b.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	key := Key{Tenant: "benchmark-tenant"}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = limiter.Allow(ctx, key)
		}
	})
}

// BenchmarkDistributed_Allow_Single 测试分布式限流器单线程性能
func BenchmarkDistributed_Allow_Single(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	limiter, err := New(rdb,
		WithRules(TenantRule("tenant", 1000000, time.Second)),
	)
	if err != nil {
		b.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	key := Key{Tenant: "benchmark-tenant"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = limiter.Allow(ctx, key)
	}
}

// BenchmarkDistributed_Allow_Parallel 测试分布式限流器并发性能
func BenchmarkDistributed_Allow_Parallel(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	limiter, err := New(rdb,
		WithRules(TenantRule("tenant", 10000000, time.Second)),
	)
	if err != nil {
		b.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	key := Key{Tenant: "benchmark-tenant"}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = limiter.Allow(ctx, key)
		}
	})
}

// BenchmarkKey_RenderTemplate 测试键渲染性能
func BenchmarkKey_RenderTemplate(b *testing.B) {
	key := Key{
		Tenant: "abc123",
		Caller: "order-service",
		Method: "POST",
		Path:   "/v1/orders",
	}
	template := "tenant:${tenant_id}:caller:${caller_id}:api:${method}:${path}"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = key.Render(template)
	}
}

// BenchmarkMultipleRulesCheck 测试多规则场景性能
func BenchmarkMultipleRulesCheck(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	limiter, err := New(rdb,
		WithRules(
			GlobalRule("global", 1000000, time.Second),
			TenantRule("tenant", 100000, time.Second),
			TenantAPIRule("tenant-api", 10000, time.Second),
			CallerRule("caller", 50000, time.Second),
		),
	)
	if err != nil {
		b.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	key := Key{
		Tenant: "benchmark-tenant",
		Caller: "benchmark-caller",
		Method: "POST",
		Path:   "/v1/orders",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = limiter.Allow(ctx, key)
	}
}

// BenchmarkWithOverridesConfig 测试带覆盖配置的性能
func BenchmarkWithOverridesConfig(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	limiter, err := New(rdb,
		WithRules(
			NewRuleBuilder("tenant").
				KeyTemplate("tenant:${tenant_id}").
				Limit(1000).
				Window(time.Second).
				AddOverride("tenant:vip-*", 5000).
				AddOverride("tenant:premium-*", 3000).
				AddOverride("tenant:basic-*", 500).
				Build(),
		),
	)
	if err != nil {
		b.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	keys := []Key{
		{Tenant: "vip-corp"},
		{Tenant: "premium-user"},
		{Tenant: "basic-user"},
		{Tenant: "normal-user"},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; b.Loop(); i++ {
		key := keys[i%len(keys)]
		_, _ = limiter.Allow(ctx, key)
	}
}

// BenchmarkMiddleware_HTTP 测试 HTTP 中间件性能
func BenchmarkMiddleware_HTTP(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	limiter, err := New(rdb,
		WithRules(TenantAPIRule("tenant-api", 1000000, time.Second)),
	)
	if err != nil {
		b.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	key := Key{
		Tenant: "benchmark-tenant",
		Method: "POST",
		Path:   "/v1/orders",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = limiter.Allow(ctx, key)
	}
}

// BenchmarkConcurrentMultiTenant 测试多租户并发性能
func BenchmarkConcurrentMultiTenant(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	limiter, err := New(rdb,
		WithRules(TenantRule("tenant", 1000000, time.Second)),
	)
	if err != nil {
		b.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()

	// 生成多个租户
	tenants := make([]Key, 100)
	for i := range tenants {
		tenants[i] = Key{Tenant: fmt.Sprintf("tenant-%d", i)}
	}

	b.ReportAllocs()
	b.ResetTimer()

	var idx atomic.Uint64

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := idx.Add(1) % uint64(len(tenants))
			_, _ = limiter.Allow(ctx, tenants[i])
		}
	})
}

// BenchmarkFallback_Allow 测试降级限流器性能
func BenchmarkFallback_Allow(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	limiter, err := New(rdb,
		WithRules(TenantRule("tenant", 1000000, time.Second)),
		WithFallback(FallbackLocal),
		WithPodCount(10),
	)
	if err != nil {
		b.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	key := Key{Tenant: "benchmark-tenant"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = limiter.Allow(ctx, key)
	}
}

// BenchmarkRuleMatcherOps 测试规则匹配器性能
func BenchmarkRuleMatcherOps(b *testing.B) {
	rules := []Rule{
		GlobalRule("global", 10000, time.Minute),
		TenantRule("tenant", 1000, time.Minute),
		TenantAPIRule("tenant-api", 100, time.Second),
		CallerRule("caller", 500, time.Minute),
	}

	matcher := newRuleMatcher(rules)
	key := Key{
		Tenant: "tenant123",
		Caller: "order-service",
		Method: "POST",
		Path:   "/v1/orders",
	}

	b.Run("FindRule", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_, _ = matcher.FindRule(key)
		}
	})

	b.Run("getEffectiveLimit", func(b *testing.B) {
		rule, _ := matcher.FindRule(key)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_, _ = matcher.getEffectiveLimit(rule, key)
		}
	})

	b.Run("getAllRules", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_ = matcher.getAllRules()
		}
	})

	b.Run("hasRule", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_ = matcher.hasRule("tenant")
		}
	})
}

// BenchmarkAllowN_Batch 测试批量请求性能
func BenchmarkAllowN_Batch(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	limiter, err := New(rdb,
		WithRules(TenantRule("tenant", 10000000, time.Second)),
	)
	if err != nil {
		b.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	key := Key{Tenant: "benchmark-tenant"}

	for _, n := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_, _ = limiter.AllowN(ctx, key, n)
			}
		})
	}
}
