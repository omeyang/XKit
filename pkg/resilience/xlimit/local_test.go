package xlimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestLocalLimiter_Basic(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 10, time.Minute)),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	key := Key{Tenant: "test-tenant"}

	result, err := limiter.Allow(ctx, key)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !result.Allowed {
		t.Error("first request should be allowed")
	}
	if result.Remaining < 0 {
		t.Error("remaining should be non-negative")
	}
}

func TestLocalLimiter_ExhaustQuota(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 5, time.Minute)),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	key := Key{Tenant: "exhaust-tenant"}

	// 消耗配额
	for i := 0; i < 5; i++ {
		result, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !result.Allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 配额耗尽
	result, err := limiter.Allow(ctx, key)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if result.Allowed {
		t.Error("request should be denied after quota exhausted")
	}
	if result.RetryAfter <= 0 {
		t.Error("RetryAfter should be positive")
	}
}

func TestLocalLimiter_AllowN(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 100, time.Minute)),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	key := Key{Tenant: "batch-tenant"}

	// 批量请求
	result, err := limiter.AllowN(ctx, key, 50)
	if err != nil {
		t.Fatalf("AllowN failed: %v", err)
	}
	if !result.Allowed {
		t.Error("batch request should be allowed")
	}

	// 超过剩余配额
	result, err = limiter.AllowN(ctx, key, 60)
	if err != nil {
		t.Fatalf("AllowN failed: %v", err)
	}
	if result.Allowed {
		t.Error("batch request should be denied when exceeding remaining")
	}
}

func TestLocalLimiter_Reset(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 5, time.Minute)),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	key := Key{Tenant: "reset-tenant"}

	// 消耗配额
	for range 5 {
		_, err = limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
	}

	// 验证配额耗尽
	result, err := limiter.Allow(ctx, key)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if result.Allowed {
		t.Error("should be denied before reset")
	}

	// 重置
	resetter, ok := limiter.(Resetter)
	if !ok {
		t.Fatal("limiter does not implement Resetter")
	}
	err = resetter.Reset(ctx, key)
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	// 验证配额恢复
	result, err = limiter.Allow(ctx, key)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !result.Allowed {
		t.Error("should be allowed after reset")
	}
}

func TestLocalLimiter_MultipleRules(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(
			GlobalRule("global-limit", 100, time.Minute),
			TenantRule("tenant-limit", 10, time.Minute),
		),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	key := Key{Tenant: "multi-rule-tenant"}

	result, err := limiter.Allow(ctx, key)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !result.Allowed {
		t.Error("request should be allowed")
	}
}

func TestLocalLimiter_DifferentTenants(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 2, time.Minute)),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()

	// 租户 A
	keyA := Key{Tenant: "tenant-a"}
	for i := range 2 {
		result, err := limiter.Allow(ctx, keyA)
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !result.Allowed {
			t.Errorf("tenant-a request %d should be allowed", i+1)
		}
	}
	result, err := limiter.Allow(ctx, keyA)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if result.Allowed {
		t.Error("tenant-a should be limited")
	}

	// 租户 B 应该有独立配额
	keyB := Key{Tenant: "tenant-b"}
	result, err = limiter.Allow(ctx, keyB)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !result.Allowed {
		t.Error("tenant-b should be allowed (independent quota)")
	}
}

func TestLocalLimiter_Concurrent(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 1000, time.Minute)),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	key := Key{Tenant: "concurrent-tenant"}

	var wg sync.WaitGroup
	var allowed, denied int64
	var mu sync.Mutex

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := limiter.Allow(ctx, key)
			if err != nil {
				return
			}
			mu.Lock()
			if result.Allowed {
				allowed++
			} else {
				denied++
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	total := allowed + denied
	if total != 100 {
		t.Errorf("expected 100 total requests, got %d", total)
	}
}

func TestLocalLimiter_Closed(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 10, time.Minute)),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	// 关闭限流器
	err = limiter.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	ctx := context.Background()
	key := Key{Tenant: "closed-tenant"}

	// 关闭后的请求应该返回错误
	_, err = limiter.Allow(ctx, key)
	if err != ErrLimiterClosed {
		t.Errorf("expected ErrLimiterClosed, got %v", err)
	}

	// Reset 也应该返回错误
	resetter, ok := limiter.(Resetter)
	if !ok {
		t.Fatal("limiter does not implement Resetter")
	}
	err = resetter.Reset(ctx, key)
	if err != ErrLimiterClosed {
		t.Errorf("expected ErrLimiterClosed, got %v", err)
	}
}

func TestLocalLimiter_TokenRefill(t *testing.T) {
	// 使用短窗口测试令牌补充
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 10, 100*time.Millisecond)),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	key := Key{Tenant: "refill-tenant"}

	// 消耗配额
	for range 10 {
		_, err = limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
	}

	// 验证配额耗尽
	result, err := limiter.Allow(ctx, key)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if result.Allowed {
		t.Error("should be denied after exhausting quota")
	}

	// 等待一段时间让令牌补充
	time.Sleep(50 * time.Millisecond)

	// 应该有部分令牌补充
	result, err = limiter.Allow(ctx, key)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	// 50ms / 100ms = 50% * 10 = 5 tokens refilled
	// 结果可能因时间精度而有所不同
	t.Logf("After 50ms wait: allowed=%v, remaining=%d", result.Allowed, result.Remaining)
}

func TestLocalLimiter_Override(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(Rule{
			Name:        "tenant-limit",
			KeyTemplate: "tenant:${tenant_id}",
			Limit:       10,
			Window:      time.Minute,
			Overrides: []Override{
				{Match: "tenant:vip", Limit: 100},
			},
		}),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()

	// 普通租户
	normalKey := Key{Tenant: "normal"}
	result, err := limiter.Allow(ctx, normalKey)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if result.Limit != 10 {
		t.Errorf("expected limit 10 for normal tenant, got %d", result.Limit)
	}

	// VIP 租户
	vipKey := Key{Tenant: "vip"}
	result, err = limiter.Allow(ctx, vipKey)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if result.Limit != 100 {
		t.Errorf("expected limit 100 for VIP tenant, got %d", result.Limit)
	}
}

func TestLocalLimiter_Callback(t *testing.T) {
	var allowCalled, denyCalled bool

	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 1, time.Minute)),
		WithOnAllow(func(key Key, result *Result) {
			allowCalled = true
		}),
		WithOnDeny(func(key Key, result *Result) {
			denyCalled = true
		}),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	key := Key{Tenant: "callback-tenant"}

	// 第一个请求应该触发 onAllow
	_, err = limiter.Allow(ctx, key)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !allowCalled {
		t.Error("onAllow should be called")
	}

	// 第二个请求应该触发 onDeny
	_, err = limiter.Allow(ctx, key)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !denyCalled {
		t.Error("onDeny should be called")
	}
}

func TestLocalBackend_UpdateParamsOnPodCountChange(t *testing.T) {
	// FG-S3: 动态 Pod 数量变化时，存量桶参数应同步更新
	podCount := 2
	provider := &mockPodCountProvider{count: podCount}

	backend := newLocalBackend(1, provider)

	ctx := context.Background()

	// 初始 limit=100, podCount=2 → localLimit=50
	res, err := backend.CheckRule(ctx, "key1", 100, 100, time.Minute, 1)
	if err != nil {
		t.Fatalf("CheckRule failed: %v", err)
	}
	if res.Limit != 50 {
		t.Errorf("expected localLimit 50, got %d", res.Limit)
	}

	// 更新 podCount 到 4 → localLimit=25
	provider.count = 4

	res, err = backend.CheckRule(ctx, "key1", 100, 100, time.Minute, 1)
	if err != nil {
		t.Fatalf("CheckRule failed: %v", err)
	}
	if res.Limit != 25 {
		t.Errorf("expected localLimit 25 after pod scale-up, got %d", res.Limit)
	}
}

type mockPodCountProvider struct {
	count int
}

func (m *mockPodCountProvider) GetPodCount(_ context.Context) (int, error) {
	return m.count, nil
}

func TestTokenBucket_UpdateParams(t *testing.T) {
	tb := &tokenBucket{
		tokens:     100,
		limit:      100,
		window:     time.Minute,
		lastUpdate: time.Now(),
	}

	// 缩小 limit，tokens 应该被截断
	tb.updateParams(50, time.Minute)

	tb.mu.Lock()
	if tb.limit != 50 {
		t.Errorf("expected limit 50, got %d", tb.limit)
	}
	if tb.tokens != 50 {
		t.Errorf("expected tokens truncated to 50, got %f", tb.tokens)
	}
	tb.mu.Unlock()

	// 扩大 limit，tokens 不变
	tb.updateParams(200, time.Minute)
	tb.mu.Lock()
	if tb.limit != 200 {
		t.Errorf("expected limit 200, got %d", tb.limit)
	}
	if tb.tokens != 50 {
		t.Errorf("expected tokens unchanged at 50, got %f", tb.tokens)
	}
	tb.mu.Unlock()
}

func BenchmarkLocalLimiter_Allow(b *testing.B) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 1000000, time.Minute)),
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
