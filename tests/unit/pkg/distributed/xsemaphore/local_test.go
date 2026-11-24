package xsemaphore

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omeyang/xkit/pkg/context/xtenant"
)

// =============================================================================
// 本地信号量完整测试
// =============================================================================

func TestLocalSemaphore_TryAcquire(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)
	ctx := context.Background()

	t.Run("basic acquire and release", func(t *testing.T) {
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
	})

	t.Run("closed semaphore", func(t *testing.T) {
		closedSem := newLocalSemaphore(defaultOptions())
		closeSemaphore(t, closedSem)

		_, err := closedSem.TryAcquire(ctx, "test", WithCapacity(10))
		assert.ErrorIs(t, err, ErrSemaphoreClosed)
	})

	t.Run("tenant from context", func(t *testing.T) {
		tenantCtx, err := xtenant.WithTenantID(ctx, "ctx-tenant")
		require.NoError(t, err)
		permit, err := sem.TryAcquire(tenantCtx, "tenant-ctx-test",
			WithCapacity(10),
			WithTenantQuota(5),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)
		assert.Equal(t, "ctx-tenant", permit.TenantID())
		releasePermit(t, tenantCtx, permit)
	})

	t.Run("explicit tenant overrides context", func(t *testing.T) {
		tenantCtx, err := xtenant.WithTenantID(ctx, "ctx-tenant")
		require.NoError(t, err)
		permit, err := sem.TryAcquire(tenantCtx, "tenant-override-test",
			WithCapacity(10),
			WithTenantID("explicit-tenant"),
			WithTenantQuota(5),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)
		assert.Equal(t, "explicit-tenant", permit.TenantID())
		releasePermit(t, tenantCtx, permit)
	})
}

func TestLocalSemaphore_Acquire(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)
	ctx := context.Background()

	t.Run("successful blocking acquire", func(t *testing.T) {
		permit, err := sem.Acquire(ctx, "blocking-test",
			WithCapacity(10),
			WithMaxRetries(3),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)
		releasePermit(t, ctx, permit)
	})

	t.Run("closed semaphore", func(t *testing.T) {
		closedSem := newLocalSemaphore(defaultOptions())
		closeSemaphore(t, closedSem)

		_, err := closedSem.Acquire(ctx, "test", WithCapacity(10))
		assert.ErrorIs(t, err, ErrSemaphoreClosed)
	})

	t.Run("context canceled", func(t *testing.T) {
		// 先占满容量
		permit1, err := sem.TryAcquire(ctx, "cancel-test", WithCapacity(1))
		require.NoError(t, err)
		require.NotNil(t, permit1)
		defer releasePermit(t, ctx, permit1)

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel() // 立即取消

		_, err = sem.Acquire(cancelCtx, "cancel-test",
			WithCapacity(1),
			WithMaxRetries(100),
		)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("context deadline exceeded", func(t *testing.T) {
		permit1, err := sem.TryAcquire(ctx, "deadline-test", WithCapacity(1))
		require.NoError(t, err)
		require.NotNil(t, permit1)
		defer releasePermit(t, ctx, permit1)

		timeoutCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()

		_, err = sem.Acquire(timeoutCtx, "deadline-test",
			WithCapacity(1),
			WithMaxRetries(100),
			WithRetryDelay(20*time.Millisecond),
		)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("retries exhausted", func(t *testing.T) {
		permit1, err := sem.TryAcquire(ctx, "retry-test", WithCapacity(1))
		require.NoError(t, err)
		require.NotNil(t, permit1)
		defer releasePermit(t, ctx, permit1)

		_, err = sem.Acquire(ctx, "retry-test",
			WithCapacity(1),
			WithMaxRetries(2),
			WithRetryDelay(10*time.Millisecond),
		)
		assert.ErrorIs(t, err, ErrAcquireFailed)
	})
}

func TestLocalSemaphore_CapacityLimit(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 2 // 本地容量 = 全局 / 2
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)
	ctx := context.Background()

	// 全局容量 10，本地容量 5
	permits := make([]Permit, 0, 6)
	for i := range 5 {
		permit, err := sem.TryAcquire(ctx, "capacity-test",
			WithCapacity(10),
		)
		require.NoError(t, err)
		require.NotNil(t, permit, "should acquire permit %d", i+1)
		permits = append(permits, permit)
	}

	// 第6个应该失败
	permit, err := sem.TryAcquire(ctx, "capacity-test", WithCapacity(10))
	assert.NoError(t, err)
	assert.Nil(t, permit, "should return nil when local capacity is full")

	// 释放一个后可以再获取
	releasePermit(t, ctx, permits[0])
	permit, err = sem.TryAcquire(ctx, "capacity-test", WithCapacity(10))
	require.NoError(t, err)
	assert.NotNil(t, permit)
	if permit != nil {
		releasePermit(t, ctx, permit)
	}

	for _, p := range permits[1:] {
		releasePermit(t, ctx, p)
	}
}

func TestLocalSemaphore_TenantQuota(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)
	ctx := context.Background()

	// 租户配额为 2
	permits := make([]Permit, 0, 3)
	for range 2 {
		permit, err := sem.TryAcquire(ctx, "quota-test",
			WithCapacity(100),
			WithTenantQuota(2),
			WithTenantID("tenant-A"),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)
		permits = append(permits, permit)
	}

	// 租户 A 第3个应该失败
	permit, err := sem.TryAcquire(ctx, "quota-test",
		WithCapacity(100),
		WithTenantQuota(2),
		WithTenantID("tenant-A"),
	)
	assert.NoError(t, err)
	assert.Nil(t, permit)

	// 租户 B 可以获取
	permit, err = sem.TryAcquire(ctx, "quota-test",
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
}

func TestLocalSemaphore_Query(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)
	ctx := context.Background()

	t.Run("basic query", func(t *testing.T) {
		permits := make([]Permit, 0, 3)
		for range 3 {
			permit, acqErr := sem.TryAcquire(ctx, "query-resource",
				WithCapacity(10),
				WithTenantQuota(5),
				WithTenantID("tenant-Q"),
			)
			require.NoError(t, acqErr)
			require.NotNil(t, permit)
			permits = append(permits, permit)
		}

		info, err := sem.Query(ctx, "query-resource",
			QueryWithCapacity(10),
			QueryWithTenantQuota(5),
			QueryWithTenantID("tenant-Q"),
		)
		require.NoError(t, err)

		assert.Equal(t, "query-resource", info.Resource)
		assert.Equal(t, 10, info.GlobalCapacity)
		assert.Equal(t, 3, info.GlobalUsed)
		assert.Equal(t, 7, info.GlobalAvailable)
		assert.Equal(t, "tenant-Q", info.TenantID)
		assert.Equal(t, 5, info.TenantQuota)
		assert.Equal(t, 3, info.TenantUsed)
		assert.Equal(t, 2, info.TenantAvailable)

		for _, p := range permits {
			releasePermit(t, ctx, p)
		}
	})

	t.Run("closed semaphore", func(t *testing.T) {
		closedSem := newLocalSemaphore(defaultOptions())
		closeSemaphore(t, closedSem)

		_, err := closedSem.Query(ctx, "test", QueryWithCapacity(10))
		assert.ErrorIs(t, err, ErrSemaphoreClosed)
	})

	t.Run("tenant from context", func(t *testing.T) {
		tenantCtx, err := xtenant.WithTenantID(ctx, "ctx-tenant-query")
		require.NoError(t, err)
		info, err := sem.Query(tenantCtx, "ctx-query-test",
			QueryWithCapacity(10),
			QueryWithTenantQuota(5),
		)
		require.NoError(t, err)
		assert.Equal(t, "ctx-tenant-query", info.TenantID)
	})
}

func TestLocalSemaphore_Health(t *testing.T) {
	opts := defaultOptions()
	sem := newLocalSemaphore(opts)
	ctx := context.Background()

	err := sem.Health(ctx)
	assert.NoError(t, err)

	closeSemaphore(t, sem)

	err = sem.Health(ctx)
	assert.ErrorIs(t, err, ErrSemaphoreClosed)
}

func TestLocalSemaphore_Close(t *testing.T) {
	sem := newLocalSemaphore(defaultOptions())

	// 首次关闭成功
	err := sem.Close(context.Background())
	assert.NoError(t, err)

	// 重复关闭静默返回
	err = sem.Close(context.Background())
	assert.NoError(t, err)
}

func TestLocalSemaphore_BackgroundCleanup(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	ctx := context.Background()

	// 获取短 TTL 许可
	permit, err := sem.TryAcquire(ctx, "cleanup-test",
		WithCapacity(1),
		WithTTL(50*time.Millisecond),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	// 容量已满
	permit2, _ := sem.TryAcquire(ctx, "cleanup-test", WithCapacity(1))
	assert.Nil(t, permit2)

	// 等待过期并手动触发清理
	time.Sleep(60 * time.Millisecond)
	sem.cleanupAllExpired()

	// 现在应该可以获取
	permit3, err := sem.TryAcquire(ctx, "cleanup-test", WithCapacity(1))
	require.NoError(t, err)
	assert.NotNil(t, permit3)

	if permit3 != nil {
		releasePermit(t, ctx, permit3)
	}
	closeSemaphore(t, sem)
}

func TestLocalSemaphore_Concurrent(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)
	ctx := context.Background()

	capacity := 10
	goroutines := 100
	var acquired atomic.Int32
	var wg sync.WaitGroup

	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			permit, err := sem.TryAcquire(ctx, "concurrent-local",
				WithCapacity(capacity),
			)
			if err != nil {
				return
			}
			if permit != nil {
				acquired.Add(1)
				time.Sleep(5 * time.Millisecond)
				releasePermit(t, ctx, permit)
			}
		}()
	}

	wg.Wait()
	t.Logf("Acquired %d local permits concurrently", acquired.Load())
}

// =============================================================================
// 本地许可测试
// =============================================================================

func TestLocalPermit_Release(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "release-test", WithCapacity(10))
	require.NoError(t, err)
	require.NotNil(t, permit)

	// 首次释放成功
	err = permit.Release(ctx)
	assert.NoError(t, err)

	// 重复释放静默返回
	err = permit.Release(ctx)
	assert.NoError(t, err)
}

func TestLocalPermit_Extend(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)
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
