package xsemaphore

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// 测试辅助函数
// =============================================================================

func setupRedis(t *testing.T) (*miniredis.Miniredis, redis.UniversalClient) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Logf("redis client close returned error: %v", err)
		}
	})

	return mr, client
}

func setupSemaphore(t *testing.T, opts ...Option) (Semaphore, *miniredis.Miniredis) {
	t.Helper()
	mr, client := setupRedis(t)
	sem, err := New(client, opts...)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := sem.Close(context.Background()); err != nil {
			t.Logf("semaphore close returned error: %v", err)
		}
	})
	return sem, mr
}

// releasePermit 测试辅助函数：释放许可并处理错误
func releasePermit(t *testing.T, ctx context.Context, p Permit) {
	t.Helper()
	if p != nil {
		if err := p.Release(ctx); err != nil {
			t.Logf("permit release returned error (may be expected): %v", err)
		}
	}
}

// closeSemaphore 测试辅助函数：关闭信号量并处理错误
func closeSemaphore(t *testing.T, sem Semaphore) {
	t.Helper()
	if err := sem.Close(context.Background()); err != nil {
		t.Errorf("semaphore close failed: %v", err)
	}
}

// =============================================================================
// 基本功能测试
// =============================================================================

func TestNew(t *testing.T) {
	t.Run("nil client returns error", func(t *testing.T) {
		_, err := New(nil)
		assert.ErrorIs(t, err, ErrNilClient)
	})

	t.Run("valid client succeeds", func(t *testing.T) {
		_, client := setupRedis(t)
		sem, err := New(client)
		require.NoError(t, err)
		assert.NotNil(t, sem)
		closeSemaphore(t, sem)
	})

	t.Run("with options", func(t *testing.T) {
		// 使用 setupSemaphore 并传递 opts 参数（覆盖 unparam lint）
		sem, _ := setupSemaphore(t, WithKeyPrefix("test:"), WithPodCount(3))
		assert.NotNil(t, sem)
	})
}

func TestTryAcquire_Basic(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "test-resource",
		WithCapacity(10),
		WithTTL(time.Minute),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	assert.NotEmpty(t, permit.ID())
	assert.Equal(t, "test-resource", permit.Resource())
	assert.False(t, permit.ExpiresAt().IsZero())

	err = permit.Release(ctx)
	assert.NoError(t, err)
}

func TestTryAcquire_CapacityLimit(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	capacity := 3
	permits := make([]Permit, 0, capacity+1)

	// 获取到容量上限
	for i := range capacity {
		permit, err := sem.TryAcquire(ctx, "limited",
			WithCapacity(capacity),
		)
		require.NoError(t, err)
		require.NotNil(t, permit, "should acquire permit %d", i+1)
		permits = append(permits, permit)
	}

	// 再次获取应返回 nil
	permit, err := sem.TryAcquire(ctx, "limited",
		WithCapacity(capacity),
	)
	assert.NoError(t, err)
	assert.Nil(t, permit, "should return nil when capacity is full")

	// 释放后可以再次获取
	err = permits[0].Release(ctx)
	require.NoError(t, err)

	permit, err = sem.TryAcquire(ctx, "limited",
		WithCapacity(capacity),
	)
	require.NoError(t, err)
	assert.NotNil(t, permit, "should acquire after release")

	// 清理
	releasePermit(t, ctx, permit)
	for _, p := range permits[1:] {
		releasePermit(t, ctx, p)
	}
}

func TestTryAcquire_TenantQuota(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	// 租户 A 获取 2 个许可（配额为 2）
	permits := make([]Permit, 0, 3)
	for range 2 {
		permit, err := sem.TryAcquire(ctx, "shared",
			WithCapacity(100),
			WithTenantQuota(2),
			WithTenantID("tenant-A"),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)
		permits = append(permits, permit)
	}

	// 租户 A 第 3 个应该被拒绝
	permit, err := sem.TryAcquire(ctx, "shared",
		WithCapacity(100),
		WithTenantQuota(2),
		WithTenantID("tenant-A"),
	)
	assert.NoError(t, err)
	assert.Nil(t, permit, "tenant A should be at quota")

	// 租户 B 仍然可以获取
	permit, err = sem.TryAcquire(ctx, "shared",
		WithCapacity(100),
		WithTenantQuota(2),
		WithTenantID("tenant-B"),
	)
	require.NoError(t, err)
	assert.NotNil(t, permit, "tenant B should be able to acquire")
	releasePermit(t, ctx, permit)

	// 清理
	for _, p := range permits {
		releasePermit(t, ctx, p)
	}
}

// =============================================================================
// Acquire 测试
// =============================================================================

func TestAcquire_Success(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.Acquire(ctx, "blocking",
		WithCapacity(10),
		WithMaxRetries(3),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	releasePermit(t, ctx, permit)
}

func TestAcquire_Timeout(t *testing.T) {
	sem, _ := setupSemaphore(t)

	// 先占满容量
	ctx := context.Background()
	permit1, err := sem.TryAcquire(ctx, "full",
		WithCapacity(1),
	)
	require.NoError(t, err)
	require.NotNil(t, permit1)

	// 带超时的 Acquire
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err = sem.Acquire(timeoutCtx, "full",
		WithCapacity(1),
		WithMaxRetries(100),
		WithRetryDelay(50*time.Millisecond),
	)
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	releasePermit(t, context.Background(), permit1)
}

// =============================================================================
// Release 测试
// =============================================================================

func TestRelease_Basic(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "release-test",
		WithCapacity(10),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	// 首次释放成功
	err = permit.Release(ctx)
	assert.NoError(t, err)

	// 重复释放静默返回
	err = permit.Release(ctx)
	assert.NoError(t, err)
}

func TestRelease_AfterExpiry(t *testing.T) {
	sem, mr := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "expiry-test",
		WithCapacity(10),
		WithTTL(100*time.Millisecond),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	// 快进时间使许可过期
	mr.FastForward(200 * time.Millisecond)

	// 释放过期许可
	err = permit.Release(ctx)
	// 第一次释放会标记 released，不会返回错误
	assert.NoError(t, err)
}

// =============================================================================
// Extend 测试
// =============================================================================

func TestExtend_Basic(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "extend-test",
		WithCapacity(10),
		WithTTL(time.Minute),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	originalExpiry := permit.ExpiresAt()

	// 续期
	time.Sleep(10 * time.Millisecond)
	err = permit.Extend(ctx)
	require.NoError(t, err)

	// 新过期时间应该更晚
	assert.True(t, permit.ExpiresAt().After(originalExpiry))

	releasePermit(t, ctx, permit)
}

func TestExtend_AfterRelease(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "extend-after-release",
		WithCapacity(10),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	releasePermit(t, ctx, permit)

	// 释放后续期应返回错误
	err = permit.Extend(ctx)
	assert.ErrorIs(t, err, ErrPermitNotHeld)
}

// =============================================================================
// Query 测试
// =============================================================================

func TestQuery_Basic(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	// 先获取一些许可
	permits := make([]Permit, 0, 3)
	for range 3 {
		permit, err := sem.TryAcquire(ctx, "query-test",
			WithCapacity(10),
			WithTenantQuota(5),
			WithTenantID("tenant-X"),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)
		permits = append(permits, permit)
	}

	// 查询状态
	info, err := sem.Query(ctx, "query-test",
		QueryWithCapacity(10),
		QueryWithTenantQuota(5),
		QueryWithTenantID("tenant-X"),
	)
	require.NoError(t, err)

	assert.Equal(t, "query-test", info.Resource)
	assert.Equal(t, 10, info.GlobalCapacity)
	assert.Equal(t, 3, info.GlobalUsed)
	assert.Equal(t, 7, info.GlobalAvailable)
	assert.Equal(t, "tenant-X", info.TenantID)
	assert.Equal(t, 5, info.TenantQuota)
	assert.Equal(t, 3, info.TenantUsed)
	assert.Equal(t, 2, info.TenantAvailable)

	// 清理
	for _, p := range permits {
		releasePermit(t, ctx, p)
	}
}

// =============================================================================
// 并发测试
// =============================================================================

func TestConcurrentAcquire(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	capacity := 10
	goroutines := 50
	var concurrent atomic.Int32
	var peak atomic.Int32
	var wg sync.WaitGroup

	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			permit, err := sem.TryAcquire(ctx, "concurrent",
				WithCapacity(capacity),
			)
			if err != nil {
				return
			}
			if permit != nil {
				cur := concurrent.Add(1)
				// 记录峰值并发数
				for {
					old := peak.Load()
					if cur <= old || peak.CompareAndSwap(old, cur) {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
				concurrent.Add(-1)
				_ = permit.Release(ctx) //nolint:errcheck // concurrent release may return error
			}
		}()
	}

	wg.Wait()

	// 峰值并发持有数不应超过容量
	t.Logf("Peak concurrent permits held: %d", peak.Load())
	assert.LessOrEqual(t, int(peak.Load()), capacity,
		"peak concurrent permits should not exceed capacity")
}

func TestConcurrentRelease(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "concurrent-release",
		WithCapacity(10),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	// 并发释放同一个许可
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = permit.Release(ctx) //nolint:errcheck // concurrent release may return error
		}()
	}

	wg.Wait()
	// 验证没有 panic
}
