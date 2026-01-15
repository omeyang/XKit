//go:build integration

package xlimit

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// uniquePrefix 生成唯一测试前缀，避免测试间的状态干扰
func uniquePrefix(base string) string {
	return fmt.Sprintf("%s:%d:%d:", base, time.Now().UnixNano(), rand.Int63())
}

// =============================================================================
// 测试环境设置
// =============================================================================

func setupRedis(t *testing.T) (*redis.Client, func()) {
	t.Helper()

	if addr := os.Getenv("XKIT_REDIS_ADDR"); addr != "" {
		client := redis.NewClient(&redis.Options{Addr: addr})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Ping(ctx).Err(); err != nil {
			t.Skipf("无法连接到 Redis %s: %v", addr, err)
		}
		return client, func() { client.Close() }
	}

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "redis:7.2-alpine",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForListeningPort("6379/tcp"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("redis container not available: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("redis host failed: %v", err)
	}
	port, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("redis port failed: %v", err)
	}

	addr := fmt.Sprintf("%s:%s", host, port.Port())
	client := redis.NewClient(&redis.Options{Addr: addr})
	return client, func() {
		client.Close()
		_ = container.Terminate(ctx)
	}
}

// =============================================================================
// 基础限流功能测试
// =============================================================================

func TestDistributed_BasicRateLimiting_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("单租户基本限流", func(t *testing.T) {
		limiter, err := New(client,
			WithRules(TenantRule("tenant", 5, time.Minute)),
			WithKeyPrefix(uniquePrefix("test:basic:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		key := Key{Tenant: "test-tenant"}

		// 前 5 个请求应该通过
		for i := 0; i < 5; i++ {
			result, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "请求 %d 应该被允许", i+1)
			assert.Equal(t, 5, result.Limit)
			assert.Equal(t, 5-i-1, result.Remaining)
		}

		// 第 6 个请求应该被拒绝
		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.False(t, result.Allowed, "第 6 个请求应该被拒绝")
		assert.Equal(t, "tenant", result.Rule)
		assert.Greater(t, result.RetryAfter, time.Duration(0))
	})

	t.Run("AllowN 批量请求", func(t *testing.T) {
		limiter, err := New(client,
			WithRules(TenantRule("tenant", 10, time.Minute)),
			WithKeyPrefix(uniquePrefix("test:allowN:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		key := Key{Tenant: "batch-tenant"}

		// 一次请求 5 个配额
		result, err := limiter.AllowN(ctx, key, 5)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
		assert.Equal(t, 5, result.Remaining)

		// 再请求 5 个配额
		result, err = limiter.AllowN(ctx, key, 5)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
		assert.Equal(t, 0, result.Remaining)

		// 再请求 1 个应该被拒绝
		result, err = limiter.AllowN(ctx, key, 1)
		require.NoError(t, err)
		assert.False(t, result.Allowed)
	})

	t.Run("Reset 重置配额", func(t *testing.T) {
		limiter, err := New(client,
			WithRules(TenantRule("tenant", 2, time.Minute)),
			WithKeyPrefix(uniquePrefix("test:reset:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		key := Key{Tenant: "reset-tenant"}

		// 耗尽配额
		for i := 0; i < 2; i++ {
			_, _ = limiter.Allow(ctx, key)
		}

		// 验证被限流
		result, _ := limiter.Allow(ctx, key)
		assert.False(t, result.Allowed)

		// 重置配额（使用 Resetter 接口）
		resetter, ok := limiter.(Resetter)
		require.True(t, ok, "limiter should implement Resetter interface")
		err = resetter.Reset(ctx, key)
		require.NoError(t, err)

		// 等待 Redis 删除操作完成
		time.Sleep(50 * time.Millisecond)

		// 应该可以再次请求
		result, err = limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	})
}

// =============================================================================
// 多规则测试
// =============================================================================

func TestDistributed_MultipleRules_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("分层限流 - 全局/租户/API", func(t *testing.T) {
		limiter, err := New(client,
			WithRules(
				GlobalRule("global", 100, time.Minute),
				TenantRule("tenant", 50, time.Minute),
				TenantAPIRule("tenant-api", 10, time.Second),
			),
			WithKeyPrefix(uniquePrefix("test:multilayer:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		key := Key{
			Tenant: "test-tenant",
			Method: "POST",
			Path:   "/v1/orders",
		}

		// 前 10 个请求应该通过（受 tenant-api 规则限制）
		for i := 0; i < 10; i++ {
			result, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "请求 %d 应该被允许", i+1)
		}

		// 第 11 个请求应该被 tenant-api 规则拒绝
		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.False(t, result.Allowed)
		assert.Equal(t, "tenant-api", result.Rule)
	})

	t.Run("不同 API 独立计数", func(t *testing.T) {
		limiter, err := New(client,
			WithRules(TenantAPIRule("api", 3, time.Minute)),
			WithKeyPrefix(uniquePrefix("test:apiindep:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		// API 1
		key1 := Key{Tenant: "tenant1", Method: "POST", Path: "/v1/orders"}
		// API 2
		key2 := Key{Tenant: "tenant1", Method: "GET", Path: "/v1/products"}

		// API 1 的请求
		for i := 0; i < 3; i++ {
			result, err := limiter.Allow(ctx, key1)
			require.NoError(t, err)
			assert.True(t, result.Allowed)
		}

		// API 2 仍然可以请求
		result, err := limiter.Allow(ctx, key2)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "不同 API 应该独立计数")

		// API 1 被限流
		result, err = limiter.Allow(ctx, key1)
		require.NoError(t, err)
		assert.False(t, result.Allowed)
	})

	t.Run("调用方限流", func(t *testing.T) {
		limiter, err := New(client,
			WithRules(CallerRule("caller", 5, time.Minute)),
			WithKeyPrefix(uniquePrefix("test:caller:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		key := Key{Tenant: "tenant", Caller: "order-service"}

		for i := 0; i < 5; i++ {
			result, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, result.Allowed)
		}

		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.False(t, result.Allowed)
		assert.Equal(t, "caller", result.Rule)
	})
}

// =============================================================================
// Override 测试
// =============================================================================

func TestDistributed_Overrides_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("VIP 租户配额覆盖", func(t *testing.T) {
		limiter, err := New(client,
			WithRules(
				NewRuleBuilder("tenant").
					KeyTemplate("tenant:${tenant_id}").
					Limit(5).
					Window(time.Minute).
					AddOverride("tenant:vip-*", 20). // VIP 租户有更高配额
					Build(),
			),
			WithKeyPrefix(uniquePrefix("test:override:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		// 普通租户
		normalKey := Key{Tenant: "normal-corp"}
		for i := 0; i < 5; i++ {
			result, err := limiter.Allow(ctx, normalKey)
			require.NoError(t, err)
			assert.True(t, result.Allowed)
		}
		result, _ := limiter.Allow(ctx, normalKey)
		assert.False(t, result.Allowed, "普通租户应该被限流在 5 次")

		// VIP 租户
		vipKey := Key{Tenant: "vip-enterprise"}
		for i := 0; i < 15; i++ {
			result, err := limiter.Allow(ctx, vipKey)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "VIP 租户第 %d 次请求应该被允许", i+1)
		}
	})

	t.Run("多级覆盖优先级", func(t *testing.T) {
		limiter, err := New(client,
			WithRules(
				NewRuleBuilder("tenant").
					KeyTemplate("tenant:${tenant_id}").
					Limit(10).
					Window(time.Minute).
					AddOverride("tenant:premium-*", 50).
					AddOverride("tenant:basic-*", 5).
					Build(),
			),
			WithKeyPrefix(uniquePrefix("test:multioverride:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		// Premium 租户应该有 50 的配额
		premiumKey := Key{Tenant: "premium-gold"}
		result, err := limiter.Allow(ctx, premiumKey)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
		assert.Equal(t, 50, result.Limit)

		// Basic 租户应该有 5 的配额
		basicKey := Key{Tenant: "basic-free"}
		result, err = limiter.Allow(ctx, basicKey)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
		assert.Equal(t, 5, result.Limit)
	})
}

// =============================================================================
// 并发测试
// =============================================================================

func TestDistributed_Concurrent_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("并发请求准确计数", func(t *testing.T) {
		limiter, err := New(client,
			WithRules(TenantRule("tenant", 100, time.Minute)),
			WithKeyPrefix(uniquePrefix("test:concurrent:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		key := Key{Tenant: "concurrent-tenant"}

		const goroutines = 20
		const requestsPerGoroutine = 5
		totalRequests := goroutines * requestsPerGoroutine

		var allowedCount atomic.Int64
		var wg sync.WaitGroup

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < requestsPerGoroutine; j++ {
					result, err := limiter.Allow(ctx, key)
					if err == nil && result.Allowed {
						allowedCount.Add(1)
					}
				}
			}()
		}

		wg.Wait()

		// 所有请求都应该被允许（总共 100 次，我们只发了 100 次）
		assert.Equal(t, int64(totalRequests), allowedCount.Load(), "所有并发请求都应该成功")
	})

	t.Run("并发多租户隔离", func(t *testing.T) {
		limiter, err := New(client,
			WithRules(TenantRule("tenant", 10, time.Minute)),
			WithKeyPrefix(uniquePrefix("test:multitenant:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		const tenants = 5
		const requestsPerTenant = 15 // 超过限制

		results := make([]atomic.Int64, tenants)
		var wg sync.WaitGroup

		for i := 0; i < tenants; i++ {
			wg.Add(1)
			go func(tenantID int) {
				defer wg.Done()
				key := Key{Tenant: fmt.Sprintf("tenant-%d", tenantID)}
				for j := 0; j < requestsPerTenant; j++ {
					result, err := limiter.Allow(ctx, key)
					if err == nil && result.Allowed {
						results[tenantID].Add(1)
					}
				}
			}(i)
		}

		wg.Wait()

		// 每个租户应该只有 10 次成功
		for i := 0; i < tenants; i++ {
			assert.Equal(t, int64(10), results[i].Load(),
				"租户 %d 应该只允许 10 次请求", i)
		}
	})
}

// =============================================================================
// 降级策略测试
// =============================================================================

func TestDistributed_Fallback_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("FallbackLocal 策略", func(t *testing.T) {
		limiter, err := New(client,
			WithRules(TenantRule("tenant", 100, time.Minute)),
			WithFallback(FallbackLocal),
			WithPodCount(10),
			WithKeyPrefix(uniquePrefix("test:fallback:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		key := Key{Tenant: "fallback-tenant"}

		// 正常请求应该通过
		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	})

	t.Run("不同 Pod 配额分配", func(t *testing.T) {
		// 模拟 5 个 Pod，每个 Pod 分配 1/5 的配额
		limiter, err := New(client,
			WithRules(TenantRule("tenant", 100, time.Minute)),
			WithFallback(FallbackLocal),
			WithPodCount(5),
			WithKeyPrefix(uniquePrefix("test:podcount:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		// 获取内部状态验证配置
		// 这里我们只验证限流器正常工作
		key := Key{Tenant: "pod-tenant"}
		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	})
}

// =============================================================================
// 回调函数测试
// =============================================================================

func TestDistributed_Callbacks_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("OnAllow 和 OnDeny 回调", func(t *testing.T) {
		var allowCalls, denyCalls atomic.Int64

		limiter, err := New(client,
			WithRules(TenantRule("tenant", 2, time.Minute)),
			WithOnAllow(func(key Key, result *Result) {
				allowCalls.Add(1)
			}),
			WithOnDeny(func(key Key, result *Result) {
				denyCalls.Add(1)
			}),
			WithKeyPrefix(uniquePrefix("test:callbacks:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		key := Key{Tenant: "callback-tenant"}

		// 2 次允许
		for i := 0; i < 2; i++ {
			_, _ = limiter.Allow(ctx, key)
		}

		// 3 次拒绝
		for i := 0; i < 3; i++ {
			_, _ = limiter.Allow(ctx, key)
		}

		assert.Equal(t, int64(2), allowCalls.Load())
		assert.Equal(t, int64(3), denyCalls.Load())
	})
}

// =============================================================================
// Headers 测试
// =============================================================================

func TestDistributed_Headers_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	ctx := context.Background()

	limiter, err := New(client,
		WithRules(TenantRule("tenant", 10, time.Minute)),
		WithKeyPrefix(uniquePrefix("test:headers:")),
	)
	require.NoError(t, err)
	defer limiter.Close()

	key := Key{Tenant: "header-tenant"}

	result, err := limiter.Allow(ctx, key)
	require.NoError(t, err)

	headers := result.Headers()

	assert.Contains(t, headers, "X-RateLimit-Limit")
	assert.Contains(t, headers, "X-RateLimit-Remaining")
	assert.Contains(t, headers, "X-RateLimit-Reset")

	assert.Equal(t, "10", headers["X-RateLimit-Limit"])
	assert.Equal(t, "9", headers["X-RateLimit-Remaining"])
}

// =============================================================================
// 窗口滑动测试
// =============================================================================

func TestDistributed_SlidingWindow_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("短窗口配额恢复", func(t *testing.T) {
		limiter, err := New(client,
			WithRules(TenantRule("tenant", 3, 2*time.Second)),
			WithKeyPrefix(uniquePrefix("test:sliding:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		key := Key{Tenant: "sliding-tenant"}

		// 耗尽配额
		for i := 0; i < 3; i++ {
			result, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, result.Allowed)
		}

		// 验证被限流
		result, _ := limiter.Allow(ctx, key)
		assert.False(t, result.Allowed)

		// 等待窗口滑动（部分恢复）
		time.Sleep(2500 * time.Millisecond)

		// 应该可以再次请求
		result, err = limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "窗口滑动后应该恢复配额")
	})
}

// =============================================================================
// Burst 测试
// =============================================================================

func TestDistributed_Burst_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Burst 配置", func(t *testing.T) {
		limiter, err := New(client,
			WithRules(
				NewRuleBuilder("tenant").
					KeyTemplate("tenant:${tenant_id}").
					Limit(10).
					Window(time.Minute).
					Burst(15). // 允许突发到 15
					Build(),
			),
			WithKeyPrefix(uniquePrefix("test:burst:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		key := Key{Tenant: "burst-tenant"}

		// 由于 GCRA 算法的工作方式，burst 影响的是令牌桶的容量
		// 这里我们主要验证配置正确应用
		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
		assert.Equal(t, 10, result.Limit) // Limit 仍然是 10
	})
}

// =============================================================================
// 错误处理测试
// =============================================================================

func TestDistributed_ErrorHandling_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Context 取消", func(t *testing.T) {
		limiter, err := New(client,
			WithRules(TenantRule("tenant", 100, time.Minute)),
			WithKeyPrefix(uniquePrefix("test:cancel:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel() // 立即取消

		key := Key{Tenant: "cancel-tenant"}
		_, err = limiter.Allow(cancelCtx, key)
		assert.Error(t, err)
	})

	t.Run("Context 超时", func(t *testing.T) {
		limiter, err := New(client,
			WithRules(TenantRule("tenant", 100, time.Minute)),
			WithKeyPrefix(uniquePrefix("test:timeout:")),
		)
		require.NoError(t, err)
		defer limiter.Close()

		// 极短的超时
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Nanosecond)
		defer cancel()
		time.Sleep(time.Millisecond) // 确保超时

		key := Key{Tenant: "timeout-tenant"}
		_, err = limiter.Allow(timeoutCtx, key)
		assert.Error(t, err)
	})
}

// =============================================================================
// 性能基准测试
// =============================================================================

func BenchmarkDistributed_Integration(b *testing.B) {
	addr := os.Getenv("XKIT_REDIS_ADDR")
	if addr == "" {
		b.Skip("XKIT_REDIS_ADDR not set")
	}

	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		b.Skipf("无法连接到 Redis: %v", err)
	}

	limiter, err := New(client,
		WithRules(TenantRule("tenant", 1000000, time.Second)),
		WithKeyPrefix(uniquePrefix("bench:")),
	)
	if err != nil {
		b.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	key := Key{Tenant: "benchmark-tenant"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = limiter.Allow(ctx, key)
	}
}

func BenchmarkDistributed_Parallel_Integration(b *testing.B) {
	addr := os.Getenv("XKIT_REDIS_ADDR")
	if addr == "" {
		b.Skip("XKIT_REDIS_ADDR not set")
	}

	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		b.Skipf("无法连接到 Redis: %v", err)
	}

	limiter, err := New(client,
		WithRules(TenantRule("tenant", 10000000, time.Second)),
		WithKeyPrefix(uniquePrefix("benchparallel:")),
	)
	if err != nil {
		b.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	key := Key{Tenant: "benchmark-parallel-tenant"}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = limiter.Allow(ctx, key)
		}
	})
}
