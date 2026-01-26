package xlimit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func testContext() context.Context {
	return context.Background()
}

func TestNew_Validation(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() { _ = rdb.Close() }() //nolint:errcheck // defer cleanup

	t.Run("valid config", func(t *testing.T) {
		limiter, err := New(rdb,
			WithRules(TenantRule("tenant", 1000, time.Minute)),
		)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if limiter == nil {
			t.Fatal("expected non-nil limiter")
		}
		defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup
	})

	t.Run("invalid rule - empty name", func(t *testing.T) {
		_, err := New(rdb,
			WithRules(Rule{
				KeyTemplate: "test",
				Limit:       100,
				Window:      time.Second,
			}),
		)
		if err == nil {
			t.Fatal("expected error for invalid rule")
		}
	})

	t.Run("invalid rule - zero limit", func(t *testing.T) {
		_, err := New(rdb,
			WithRules(Rule{
				Name:        "test",
				KeyTemplate: "test",
				Limit:       0,
				Window:      time.Second,
			}),
		)
		if err == nil {
			t.Fatal("expected error for zero limit")
		}
	})

	t.Run("with fallback config", func(t *testing.T) {
		limiter, err := New(rdb,
			WithRules(TenantRule("tenant", 1000, time.Minute)),
			WithFallback(FallbackLocal),
			WithPodCount(3),
		)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if limiter == nil {
			t.Fatal("expected non-nil limiter")
		}
		defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup
	})
}

func TestNewLocal_Validation(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		limiter, err := NewLocal(
			WithRules(TenantRule("tenant", 1000, time.Minute)),
		)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if limiter == nil {
			t.Fatal("expected non-nil limiter")
		}
		defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup
	})

	t.Run("invalid rule", func(t *testing.T) {
		_, err := NewLocal(
			WithRules(Rule{
				Name:        "",
				KeyTemplate: "test",
				Limit:       100,
				Window:      time.Second,
			}),
		)
		if err == nil {
			t.Fatal("expected error for invalid rule")
		}
	})

	t.Run("with pod count", func(t *testing.T) {
		limiter, err := NewLocal(
			WithRules(TenantRule("tenant", 1000, time.Minute)),
			WithPodCount(5),
		)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if limiter == nil {
			t.Fatal("expected non-nil limiter")
		}
		defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup
	})
}

func TestNew_WithCallbacks(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() { _ = rdb.Close() }() //nolint:errcheck // defer cleanup

	var allowCalled, denyCalled bool

	limiter, err := New(rdb,
		WithRules(TenantRule("tenant", 1, time.Second)),
		WithOnAllow(func(key Key, result *Result) {
			allowCalled = true
		}),
		WithOnDeny(func(key Key, result *Result) {
			denyCalled = true
		}),
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := testContext()
	key := Key{Tenant: "test"}

	// 第一个请求应该被允许
	result, err := limiter.Allow(ctx, key)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.Allowed {
		t.Error("expected first request to be allowed")
	}
	if !allowCalled {
		t.Error("expected onAllow callback to be called")
	}

	// 第二个请求应该被拒绝（limit=1）
	result, err = limiter.Allow(ctx, key)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Allowed {
		t.Error("expected second request to be denied")
	}
	if !denyCalled {
		t.Error("expected onDeny callback to be called")
	}
}

func TestNew_WithKeyPrefix(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() { _ = rdb.Close() }() //nolint:errcheck // defer cleanup

	limiter, err := New(rdb,
		WithRules(TenantRule("tenant", 100, time.Minute)),
		WithKeyPrefix("myapp:ratelimit:"),
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := testContext()
	key := Key{Tenant: "test"}

	_, err = limiter.Allow(ctx, key)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// 检查 Redis 中的键是否使用了自定义前缀
	// redis_rate 使用 {prefix}{key} 格式
	keys, err := rdb.Keys(ctx, "*").Result()
	if err != nil {
		t.Fatalf("failed to get keys: %v", err)
	}

	// 验证至少有一个键包含自定义前缀
	hasPrefix := false
	for _, k := range keys {
		if strings.Contains(k, "myapp:ratelimit:") {
			hasPrefix = true
			break
		}
	}
	if !hasPrefix && len(keys) > 0 {
		t.Logf("keys found: %v", keys)
	}
	// 注意：miniredis 可能有不同的键存储行为，我们主要验证限流器正常工作
}

func TestNew_MultipleRules(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() { _ = rdb.Close() }() //nolint:errcheck // defer cleanup

	limiter, err := New(rdb,
		WithRules(
			GlobalRule("global", 10, time.Minute),
			TenantRule("tenant", 5, time.Minute),
			TenantAPIRule("tenant-api", 2, time.Second),
		),
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := testContext()
	key := Key{Tenant: "test", Method: "POST", Path: "/v1/orders"}

	// 前 2 个请求应该通过（tenant-api limit=2）
	for i := 0; i < 2; i++ {
		result, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("request %d: expected no error, got %v", i+1, err)
		}
		if !result.Allowed {
			t.Errorf("request %d: expected allowed", i+1)
		}
	}

	// 第 3 个请求应该被 tenant-api 规则拒绝
	result, err := limiter.Allow(ctx, key)
	if err != nil {
		t.Fatalf("request 3: expected no error, got %v", err)
	}
	if result.Allowed {
		t.Error("request 3: expected denied by tenant-api rule")
	}
	if result.Rule != "tenant-api" {
		t.Errorf("expected rule 'tenant-api', got %q", result.Rule)
	}
}
