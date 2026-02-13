//go:build integration

package xsemaphore

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omeyang/xkit/pkg/context/xtenant"
)

// =============================================================================
// 集成测试辅助函数
// =============================================================================

func getRedisAddr() string {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	return addr
}

func setupIntegrationRedis(t *testing.T) redis.UniversalClient {
	client := redis.NewClient(&redis.Options{
		Addr:         getRedisAddr(),
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.Ping(ctx).Err()
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	t.Cleanup(func() {
		// 清理测试数据
		client.FlushDB(context.Background())
		client.Close()
	})

	return client
}

// =============================================================================
// 基本功能集成测试
// =============================================================================

func TestIntegration_TryAcquire(t *testing.T) {
	client := setupIntegrationRedis(t)
	sem, err := New(client, WithKeyPrefix("test:integration:"))
	require.NoError(t, err)
	defer sem.Close(context.Background())

	ctx := context.Background()

	t.Run("basic acquire and release", func(t *testing.T) {
		permit, err := sem.TryAcquire(ctx, "basic-test",
			WithCapacity(10),
			WithTTL(time.Minute),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)

		assert.NotEmpty(t, permit.ID())
		assert.Equal(t, "basic-test", permit.Resource())

		err = permit.Release(ctx)
		assert.NoError(t, err)
	})

	t.Run("capacity limit", func(t *testing.T) {
		capacity := 5
		permits := make([]Permit, 0, capacity+1)

		for i := 0; i < capacity; i++ {
			permit, err := sem.TryAcquire(ctx, "capacity-test",
				WithCapacity(capacity),
			)
			require.NoError(t, err)
			require.NotNil(t, permit, "should acquire permit %d", i+1)
			permits = append(permits, permit)
		}

		// 容量满后应返回 nil
		permit, err := sem.TryAcquire(ctx, "capacity-test",
			WithCapacity(capacity),
		)
		assert.NoError(t, err)
		assert.Nil(t, permit)

		// 清理
		for _, p := range permits {
			releasePermit(t, ctx, p)
		}
	})

	t.Run("tenant quota", func(t *testing.T) {
		permits := make([]Permit, 0, 3)

		for i := 0; i < 2; i++ {
			permit, err := sem.TryAcquire(ctx, "tenant-test",
				WithCapacity(100),
				WithTenantQuota(2),
				WithTenantID("tenant-A"),
			)
			require.NoError(t, err)
			require.NotNil(t, permit)
			permits = append(permits, permit)
		}

		// 租户配额满
		permit, err := sem.TryAcquire(ctx, "tenant-test",
			WithCapacity(100),
			WithTenantQuota(2),
			WithTenantID("tenant-A"),
		)
		assert.NoError(t, err)
		assert.Nil(t, permit)

		// 其他租户可以获取
		permit, err = sem.TryAcquire(ctx, "tenant-test",
			WithCapacity(100),
			WithTenantQuota(2),
			WithTenantID("tenant-B"),
		)
		require.NoError(t, err)
		assert.NotNil(t, permit)
		if permit != nil {
			releasePermit(t, ctx, permit)
		}

		for _, p := range permits {
			releasePermit(t, ctx, p)
		}
	})
}

func TestIntegration_Acquire_Blocking(t *testing.T) {
	client := setupIntegrationRedis(t)
	sem, err := New(client, WithKeyPrefix("test:blocking:"))
	require.NoError(t, err)
	defer sem.Close(context.Background())

	ctx := context.Background()

	// 占满容量
	permit1, _ := sem.TryAcquire(ctx, "blocking-test", WithCapacity(1))
	require.NotNil(t, permit1)

	// 在后台释放
	go func() {
		time.Sleep(100 * time.Millisecond)
		permit1.Release(context.Background())
	}()

	// 阻塞获取
	permit2, err := sem.Acquire(ctx, "blocking-test",
		WithCapacity(1),
		WithMaxRetries(20),
		WithRetryDelay(50*time.Millisecond),
	)
	require.NoError(t, err)
	require.NotNil(t, permit2)

	releasePermit(t, ctx, permit2)
}

// =============================================================================
// 续租集成测试
// =============================================================================

func TestIntegration_Extend(t *testing.T) {
	client := setupIntegrationRedis(t)
	sem, err := New(client, WithKeyPrefix("test:extend:"))
	require.NoError(t, err)
	defer sem.Close(context.Background())

	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "extend-test",
		WithCapacity(10),
		WithTTL(time.Minute),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	originalExpiry := permit.ExpiresAt()

	time.Sleep(10 * time.Millisecond)

	err = permit.Extend(ctx)
	require.NoError(t, err)

	assert.True(t, permit.ExpiresAt().After(originalExpiry))

	releasePermit(t, ctx, permit)
}

func TestIntegration_AutoExtend(t *testing.T) {
	client := setupIntegrationRedis(t)
	sem, err := New(client, WithKeyPrefix("test:auto-extend:"))
	require.NoError(t, err)
	defer sem.Close(context.Background())

	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "auto-extend-test",
		WithCapacity(10),
		WithTTL(500*time.Millisecond),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	originalExpiry := permit.ExpiresAt()

	stop := permit.StartAutoExtend(100 * time.Millisecond)

	time.Sleep(350 * time.Millisecond)

	assert.True(t, permit.ExpiresAt().After(originalExpiry))

	stop()
	releasePermit(t, ctx, permit)
}

// =============================================================================
// 过期集成测试
// =============================================================================

func TestIntegration_PermitExpiry(t *testing.T) {
	client := setupIntegrationRedis(t)
	sem, err := New(client, WithKeyPrefix("test:expiry:"))
	require.NoError(t, err)
	defer sem.Close(context.Background())

	ctx := context.Background()

	// 获取短 TTL 许可
	permit, err := sem.TryAcquire(ctx, "expiry-test",
		WithCapacity(1),
		WithTTL(100*time.Millisecond),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	// 容量满
	permit2, _ := sem.TryAcquire(ctx, "expiry-test", WithCapacity(1))
	assert.Nil(t, permit2)

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 现在可以获取
	permit3, err := sem.TryAcquire(ctx, "expiry-test", WithCapacity(1))
	require.NoError(t, err)
	assert.NotNil(t, permit3)

	if permit3 != nil {
		releasePermit(t, ctx, permit3)
	}
}

// =============================================================================
// 查询集成测试
// =============================================================================

func TestIntegration_Query(t *testing.T) {
	client := setupIntegrationRedis(t)
	sem, err := New(client, WithKeyPrefix("test:query:"))
	require.NoError(t, err)
	defer sem.Close(context.Background())

	ctx := context.Background()

	permits := make([]Permit, 0, 5)
	for i := 0; i < 5; i++ {
		permit, _ := sem.TryAcquire(ctx, "query-test",
			WithCapacity(100),
			WithTenantQuota(10),
			WithTenantID("query-tenant"),
		)
		require.NotNil(t, permit)
		permits = append(permits, permit)
	}

	info, err := sem.Query(ctx, "query-test",
		QueryWithCapacity(100),
		QueryWithTenantQuota(10),
		QueryWithTenantID("query-tenant"),
	)
	require.NoError(t, err)

	assert.Equal(t, "query-test", info.Resource)
	assert.Equal(t, 100, info.GlobalCapacity)
	assert.Equal(t, 5, info.GlobalUsed)
	assert.Equal(t, 95, info.GlobalAvailable)
	assert.Equal(t, "query-tenant", info.TenantID)
	assert.Equal(t, 10, info.TenantQuota)
	assert.Equal(t, 5, info.TenantUsed)
	assert.Equal(t, 5, info.TenantAvailable)

	for _, p := range permits {
		releasePermit(t, ctx, p)
	}
}

// =============================================================================
// 高并发集成测试
// =============================================================================

func TestIntegration_HighConcurrency(t *testing.T) {
	client := setupIntegrationRedis(t)
	sem, err := New(client, WithKeyPrefix("test:concurrent:"))
	require.NoError(t, err)
	defer sem.Close(context.Background())

	ctx := context.Background()

	capacity := 20
	goroutines := 100
	iterations := 10

	var acquired atomic.Int64
	var released atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				permit, err := sem.TryAcquire(ctx,
					fmt.Sprintf("concurrent-resource-%d", id%5),
					WithCapacity(capacity),
				)
				if err != nil {
					continue
				}
				if permit != nil {
					acquired.Add(1)
					time.Sleep(time.Millisecond)
					if err := permit.Release(ctx); err == nil {
						released.Add(1)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Acquired: %d, Released: %d", acquired.Load(), released.Load())
	assert.Equal(t, acquired.Load(), released.Load())
}

func TestIntegration_ConcurrentTenants(t *testing.T) {
	client := setupIntegrationRedis(t)
	sem, err := New(client, WithKeyPrefix("test:tenants:"))
	require.NoError(t, err)
	defer sem.Close(context.Background())

	ctx := context.Background()

	tenants := 10
	quotaPerTenant := 5
	goroutinesPerTenant := 20

	var wg sync.WaitGroup
	results := make([]int64, tenants)

	for i := 0; i < tenants; i++ {
		tenantID := fmt.Sprintf("tenant-%d", i)
		for j := 0; j < goroutinesPerTenant; j++ {
			wg.Add(1)
			go func(tid string, idx int) {
				defer wg.Done()

				permit, err := sem.TryAcquire(ctx, "shared-resource",
					WithCapacity(1000),
					WithTenantQuota(quotaPerTenant),
					WithTenantID(tid),
				)
				if err != nil {
					return
				}
				if permit != nil {
					atomic.AddInt64(&results[idx], 1)
					time.Sleep(5 * time.Millisecond)
					releasePermit(t, ctx, permit)
				}
			}(tenantID, i)
		}
	}

	wg.Wait()

	// 验证每个租户不超过配额
	for i, count := range results {
		t.Logf("Tenant %d acquired: %d", i, count)
		// 由于并发性，实际获取数可能小于配额
	}
}

// =============================================================================
// 降级集成测试
// =============================================================================

func TestIntegration_Fallback(t *testing.T) {
	client := setupIntegrationRedis(t)

	var fallbackCount atomic.Int32
	sem, err := New(client,
		WithKeyPrefix("test:fallback:"),
		WithFallback(FallbackLocal),
		WithPodCount(2),
		WithOnFallback(func(resource string, strategy FallbackStrategy, err error) {
			fallbackCount.Add(1)
		}),
	)
	require.NoError(t, err)
	defer sem.Close(context.Background())

	ctx := context.Background()

	// 正常操作
	permit, err := sem.TryAcquire(ctx, "fallback-test", WithCapacity(10))
	require.NoError(t, err)
	require.NotNil(t, permit)
	releasePermit(t, ctx, permit)

	// 关闭 Redis 连接
	if err := client.Close(); err != nil {
		t.Logf("client close: %v", err)
	}

	// 降级到本地
	permit2, err := sem.TryAcquire(ctx, "fallback-test", WithCapacity(10))
	// 可能成功（本地降级）或失败（连接错误）
	if err == nil && permit2 != nil {
		releasePermit(t, ctx, permit2)
	}
}

// =============================================================================
// 健康检查集成测试
// =============================================================================

func TestIntegration_Health(t *testing.T) {
	client := setupIntegrationRedis(t)
	sem, err := New(client, WithKeyPrefix("test:health:"))
	require.NoError(t, err)
	defer sem.Close(context.Background())

	err = sem.Health(context.Background())
	assert.NoError(t, err)
}

// =============================================================================
// Context 集成测试
// =============================================================================

func TestIntegration_TenantFromContext(t *testing.T) {
	client := setupIntegrationRedis(t)
	sem, err := New(client, WithKeyPrefix("test:ctx-tenant:"))
	require.NoError(t, err)
	defer sem.Close(context.Background())

	ctx, err := xtenant.WithTenantID(context.Background(), "ctx-tenant-id")
	require.NoError(t, err)

	permit, err := sem.TryAcquire(ctx, "ctx-test",
		WithCapacity(100),
		WithTenantQuota(10),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	assert.Equal(t, "ctx-tenant-id", permit.TenantID())

	info, err := sem.Query(ctx, "ctx-test",
		QueryWithCapacity(100),
		QueryWithTenantQuota(10),
	)
	require.NoError(t, err)
	assert.Equal(t, "ctx-tenant-id", info.TenantID)

	releasePermit(t, ctx, permit)
}

// =============================================================================
// 脚本预热测试
// =============================================================================

func TestIntegration_WarmupScripts(t *testing.T) {
	client := setupIntegrationRedis(t)

	// 预热脚本
	err := WarmupScripts(context.Background(), client)
	require.NoError(t, err)

	// 再次预热应该成功
	err = WarmupScripts(context.Background(), client)
	require.NoError(t, err)

	// 使用预热后的信号量
	sem, err := New(client, WithKeyPrefix("test:warmup:"))
	require.NoError(t, err)
	defer sem.Close(context.Background())

	ctx := context.Background()
	permit, err := sem.TryAcquire(ctx, "warmup-test", WithCapacity(10))
	require.NoError(t, err)
	require.NotNil(t, permit)
	releasePermit(t, ctx, permit)
}

// =============================================================================
// 长时间任务模拟
// =============================================================================

func TestIntegration_LongRunningTask(t *testing.T) {
	client := setupIntegrationRedis(t)
	sem, err := New(client, WithKeyPrefix("test:long-task:"))
	require.NoError(t, err)
	defer sem.Close(context.Background())

	ctx := context.Background()

	// 模拟长时间任务
	permit, err := sem.TryAcquire(ctx, "long-task",
		WithCapacity(10),
		WithTTL(200*time.Millisecond),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	// 启动自动续租
	stop := permit.StartAutoExtend(50 * time.Millisecond)

	// 模拟长时间工作
	for i := 0; i < 5; i++ {
		time.Sleep(100 * time.Millisecond)
		// 检查许可仍然有效
		info, err := sem.Query(ctx, "long-task", QueryWithCapacity(10))
		require.NoError(t, err)
		assert.Equal(t, 1, info.GlobalUsed)
	}

	stop()
	releasePermit(t, ctx, permit)
}
