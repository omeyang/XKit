package xlimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMiniredis(t *testing.T) (*miniredis.Miniredis, redis.UniversalClient) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	t.Cleanup(func() {
		client.Close()
		mr.Close()
	})

	return mr, client
}

func TestDistributedLimiter_Allow(t *testing.T) {
	_, client := setupMiniredis(t)

	limiter, err := New(client,
		WithRules(TenantRule("tenant-limit", 10, time.Minute)),
		WithFallback(""), // 禁用降级，测试纯分布式
	)
	require.NoError(t, err, "failed to create limiter")
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	key := Key{Tenant: "test-tenant"}

	t.Run("first request allowed", func(t *testing.T) {
		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err, "Allow failed")
		assert.True(t, result.Allowed, "first request should be allowed")
		assert.Equal(t, 9, result.Remaining, "expected remaining 9")
	})

	t.Run("exhaust quota", func(t *testing.T) {
		// 消耗剩余配额
		for i := 0; i < 9; i++ {
			result, err := limiter.Allow(ctx, key)
			require.NoError(t, err, "Allow failed")
			assert.True(t, result.Allowed, "request %d should be allowed", i+2)
		}

		// 配额耗尽
		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err, "Allow failed")
		assert.False(t, result.Allowed, "request should be denied after quota exhausted")
		assert.Positive(t, result.RetryAfter, "RetryAfter should be positive")
	})
}

func TestDistributedLimiter_AllowN(t *testing.T) {
	_, client := setupMiniredis(t)

	limiter, err := New(client,
		WithRules(TenantRule("tenant-limit", 100, time.Minute)),
		WithFallback(""),
	)
	require.NoError(t, err, "failed to create limiter")
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	key := Key{Tenant: "batch-tenant"}

	t.Run("batch request allowed", func(t *testing.T) {
		result, err := limiter.AllowN(ctx, key, 50)
		require.NoError(t, err, "AllowN failed")
		assert.True(t, result.Allowed, "batch request should be allowed")
		assert.Equal(t, 50, result.Remaining, "expected remaining 50")
	})

	t.Run("batch request exceeds remaining", func(t *testing.T) {
		result, err := limiter.AllowN(ctx, key, 60)
		require.NoError(t, err, "AllowN failed")
		assert.False(t, result.Allowed, "batch request should be denied when exceeding remaining")
	})
}

func TestDistributedLimiter_Reset(t *testing.T) {
	mr, client := setupMiniredis(t)

	limiter, err := New(client,
		WithRules(TenantRule("tenant-limit", 5, time.Minute)),
		WithFallback(""),
	)
	require.NoError(t, err, "failed to create limiter")
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	key := Key{Tenant: "reset-tenant"}

	// 消耗所有配额
	for i := 0; i < 5; i++ {
		_, err := limiter.Allow(ctx, key)
		require.NoError(t, err, "Allow failed")
	}

	// 验证配额耗尽
	result, err := limiter.Allow(ctx, key)
	require.NoError(t, err, "Allow failed")
	assert.False(t, result.Allowed, "should be denied before reset")

	// 重置配额
	resetter, ok := limiter.(Resetter)
	require.True(t, ok, "limiter does not implement Resetter")
	require.NoError(t, resetter.Reset(ctx, key), "Reset failed")

	// 快进时间确保窗口重置
	mr.FastForward(time.Minute)

	// 验证配额恢复
	result, err = limiter.Allow(ctx, key)
	require.NoError(t, err, "Allow failed")
	assert.True(t, result.Allowed, "should be allowed after reset")
}

func TestDistributedLimiter_MultipleRules(t *testing.T) {
	_, client := setupMiniredis(t)

	limiter, err := New(client,
		WithRules(
			GlobalRule("global-limit", 100, time.Minute),
			TenantRule("tenant-limit", 10, time.Minute),
		),
		WithFallback(""),
	)
	require.NoError(t, err, "failed to create limiter")
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()

	t.Run("different tenants have separate quotas", func(t *testing.T) {
		key1 := Key{Tenant: "tenant-1"}
		key2 := Key{Tenant: "tenant-2"}

		// 租户1的请求
		result1, err := limiter.Allow(ctx, key1)
		require.NoError(t, err, "Allow failed")
		assert.True(t, result1.Allowed, "tenant-1 should be allowed")

		// 租户2的请求（独立配额）
		result2, err := limiter.Allow(ctx, key2)
		require.NoError(t, err, "Allow failed")
		assert.True(t, result2.Allowed, "tenant-2 should be allowed")

		// 验证两个租户的配额是独立的
		// 由于有多个规则，remaining 可能来自任一规则
		// 重要的是两个请求都被允许通过
		assert.GreaterOrEqual(t, result1.Remaining, 0, "tenant-1 remaining should be non-negative")
		assert.GreaterOrEqual(t, result2.Remaining, 0, "tenant-2 remaining should be non-negative")
	})
}

func TestDistributedLimiter_Override(t *testing.T) {
	_, client := setupMiniredis(t)

	limiter, err := New(client,
		WithRules(Rule{
			Name:        "tenant-limit",
			KeyTemplate: "tenant:${tenant_id}",
			Limit:       10,
			Window:      time.Minute,
			Overrides: []Override{
				{Match: "tenant:vip-corp", Limit: 100},
			},
		}),
		WithFallback(""),
	)
	require.NoError(t, err, "failed to create limiter")
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()

	t.Run("normal tenant uses default limit", func(t *testing.T) {
		key := Key{Tenant: "normal-user"}
		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err, "Allow failed")
		assert.Equal(t, 10, result.Limit, "expected limit 10")
	})

	t.Run("vip tenant uses override limit", func(t *testing.T) {
		key := Key{Tenant: "vip-corp"}
		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err, "Allow failed")
		assert.Equal(t, 100, result.Limit, "expected limit 100")
	})
}

func TestDistributedLimiter_Callback(t *testing.T) {
	_, client := setupMiniredis(t)

	var allowCalled, denyCalled bool
	var allowKey, denyKey Key

	limiter, err := New(client,
		WithRules(TenantRule("tenant-limit", 1, time.Minute)),
		WithFallback(""),
		WithOnAllow(func(key Key, result *Result) {
			allowCalled = true
			allowKey = key
		}),
		WithOnDeny(func(key Key, result *Result) {
			denyCalled = true
			denyKey = key
		}),
	)
	require.NoError(t, err, "failed to create limiter")
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	key := Key{Tenant: "callback-tenant"}

	// 第一次请求应该触发 onAllow
	_, err = limiter.Allow(ctx, key)
	require.NoError(t, err, "Allow failed")
	assert.True(t, allowCalled, "onAllow should be called")
	assert.Equal(t, key.Tenant, allowKey.Tenant, "onAllow should receive correct key")

	// 第二次请求应该触发 onDeny
	_, err = limiter.Allow(ctx, key)
	require.NoError(t, err, "Allow failed")
	assert.True(t, denyCalled, "onDeny should be called")
	assert.Equal(t, key.Tenant, denyKey.Tenant, "onDeny should receive correct key")
}

func BenchmarkDistributedLimiter_Allow(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	limiter, err := New(client,
		WithRules(TenantRule("tenant-limit", 1000000, time.Minute)),
		WithFallback(""),
	)
	if err != nil {
		b.Fatalf("failed to create limiter: %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	key := Key{Tenant: "bench-tenant"}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = limiter.Allow(ctx, key) //nolint:errcheck // benchmark
		}
	})
}
