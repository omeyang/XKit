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
	for i := 0; i < 5; i++ {
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
	for i := 0; i < 2; i++ {
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
		for i := 0; i < 3; i++ {
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

	for i := 0; i < goroutines; i++ {
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

func TestLocalPermit_Extend_AfterRelease(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "extend-released-test", WithCapacity(10))
	require.NoError(t, err)
	require.NotNil(t, permit)

	releasePermit(t, ctx, permit)

	err = permit.Extend(ctx)
	assert.ErrorIs(t, err, ErrPermitNotHeld)
}

func TestLocalPermit_Extend_AfterExpiry(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "extend-expired-test",
		WithCapacity(10),
		WithTTL(50*time.Millisecond),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	// 等待过期
	time.Sleep(60 * time.Millisecond)

	// 续期过期许可应返回错误
	err = permit.Extend(ctx)
	assert.ErrorIs(t, err, ErrPermitNotHeld)
}

func TestLocalPermit_AutoExtend(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "auto-extend-test",
		WithCapacity(10),
		WithTTL(200*time.Millisecond),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	originalExpiry := permit.ExpiresAt()

	// 启动自动续租
	stop := permit.StartAutoExtend(50 * time.Millisecond)

	// 等待几次续租
	time.Sleep(150 * time.Millisecond)

	// 过期时间应该已更新
	assert.True(t, permit.ExpiresAt().After(originalExpiry))

	// 停止自动续租
	stop()

	releasePermit(t, ctx, permit)
}

func TestLocalPermit_TenantID(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "tenant-id-test",
		WithCapacity(10),
		WithTenantID("test-tenant"),
		WithTenantQuota(5),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	assert.Equal(t, "test-tenant", permit.TenantID())

	releasePermit(t, ctx, permit)
}

// =============================================================================
// Context 错误检查测试
// =============================================================================

func TestLocalSemaphore_TryAcquire_ContextError(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)

	t.Run("canceled context returns error", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel() // 立即取消

		permit, err := sem.TryAcquire(cancelCtx, "ctx-cancel-test",
			WithCapacity(10),
		)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, permit)
	})

	t.Run("deadline exceeded context returns error", func(t *testing.T) {
		deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		permit, err := sem.TryAcquire(deadlineCtx, "ctx-deadline-test",
			WithCapacity(10),
		)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Nil(t, permit)
	})
}

// =============================================================================
// 内存回收测试
// =============================================================================

func TestLocalSemaphore_MemoryCleanup(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)
	ctx := context.Background()

	t.Run("empty tenant map is cleaned up on release", func(t *testing.T) {
		// 获取带租户配额的许可
		permit, err := sem.TryAcquire(ctx, "cleanup-tenant-test",
			WithCapacity(10),
			WithTenantID("temp-tenant"),
			WithTenantQuota(5),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)

		// 释放后，空的租户 map 应该被清理
		err = permit.Release(ctx)
		assert.NoError(t, err)

		// 验证资源许可集合中不再有该租户
		rp := sem.getResourcePermits("cleanup-tenant-test")
		rp.mu.RLock()
		_, exists := rp.tenants["temp-tenant"]
		rp.mu.RUnlock()
		assert.False(t, exists, "empty tenant map should be cleaned up")
	})

	t.Run("empty resource permits retained after cleanup to avoid orphan bucket race", func(t *testing.T) {
		// 获取短 TTL 许可
		permit, err := sem.TryAcquire(ctx, "cleanup-resource-test",
			WithCapacity(1),
			WithTTL(10*time.Millisecond),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)

		// 等待过期
		time.Sleep(20 * time.Millisecond)

		// 手动触发清理
		sem.cleanupAllExpired()

		// 空的资源节点被保留（避免孤儿 bucket 竞态）
		val, exists := sem.permits.Load("cleanup-resource-test")
		assert.True(t, exists, "empty resource permits should be retained to avoid orphan bucket race")

		// 但内部 map 应该为空
		if exists {
			rp := val.(*resourcePermits) //nolint:errcheck // type is controlled by our code
			rp.mu.RLock()
			assert.Empty(t, rp.global, "global permits should be empty after cleanup")
			rp.mu.RUnlock()
		}
	})
}

// =============================================================================
// 租户配额语义一致性测试
// =============================================================================

func TestLocalSemaphore_TenantQuotaSemantics(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)
	ctx := context.Background()

	t.Run("tenant without quota does not create tenant entry", func(t *testing.T) {
		// 有 tenantID 但没有 tenantQuota
		permit, err := sem.TryAcquire(ctx, "no-quota-test",
			WithCapacity(10),
			WithTenantID("tenant-no-quota"),
			// 不设置 WithTenantQuota
		)
		require.NoError(t, err)
		require.NotNil(t, permit)

		// 验证没有创建租户条目
		rp := sem.getResourcePermits("no-quota-test")
		rp.mu.RLock()
		_, exists := rp.tenants["tenant-no-quota"]
		rp.mu.RUnlock()
		assert.False(t, exists, "tenant entry should not be created without quota")

		// 释放也不应该报错
		err = permit.Release(ctx)
		assert.NoError(t, err)
	})

	t.Run("tenant with quota creates and cleans up entry", func(t *testing.T) {
		// 有 tenantID 和 tenantQuota
		permit, err := sem.TryAcquire(ctx, "with-quota-test",
			WithCapacity(10),
			WithTenantID("tenant-with-quota"),
			WithTenantQuota(5),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)

		// 验证创建了租户条目
		rp := sem.getResourcePermits("with-quota-test")
		rp.mu.RLock()
		_, exists := rp.tenants["tenant-with-quota"]
		rp.mu.RUnlock()
		assert.True(t, exists, "tenant entry should be created with quota")

		// 释放后应该清理
		err = permit.Release(ctx)
		assert.NoError(t, err)

		rp.mu.RLock()
		_, exists = rp.tenants["tenant-with-quota"]
		rp.mu.RUnlock()
		assert.False(t, exists, "tenant entry should be cleaned up after release")
	})
}

// =============================================================================
// 并发竞态测试（P0 修复验证）
// =============================================================================

func TestLocalSemaphore_ConcurrentCleanupAndAcquire(t *testing.T) {
	// 测试场景：后台清理线程和新请求并发执行
	// 验证 CompareAndDelete 能正确处理竞态条件
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)
	ctx := context.Background()

	t.Run("cleanup does not remove bucket with new permits", func(t *testing.T) {
		// 获取短 TTL 许可
		permit1, err := sem.TryAcquire(ctx, "race-test",
			WithCapacity(10),
			WithTTL(10*time.Millisecond),
		)
		require.NoError(t, err)
		require.NotNil(t, permit1)

		// 等待许可过期
		time.Sleep(20 * time.Millisecond)

		// 并发执行：清理 + 获取新许可
		var wg sync.WaitGroup
		var acquiredPermit Permit
		var acquireErr error

		wg.Add(2)

		// goroutine 1: 触发清理
		go func() {
			defer wg.Done()
			sem.cleanupAllExpired()
		}()

		// goroutine 2: 获取新许可
		go func() {
			defer wg.Done()
			acquiredPermit, acquireErr = sem.TryAcquire(ctx, "race-test",
				WithCapacity(10),
				WithTTL(time.Minute),
			)
		}()

		wg.Wait()

		// 验证获取成功
		require.NoError(t, acquireErr)
		require.NotNil(t, acquiredPermit, "should be able to acquire permit after concurrent cleanup")

		// 验证许可仍然有效（释放不报错）
		err = acquiredPermit.Release(ctx)
		assert.NoError(t, err)
	})

	t.Run("high concurrency stress test", func(t *testing.T) {
		goroutines := 50
		iterations := 100
		var wg sync.WaitGroup

		// 并发执行多个清理和获取操作
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					// 交替执行清理和获取
					if j%2 == 0 {
						sem.cleanupAllExpired()
					} else {
						permit, acqErr := sem.TryAcquire(ctx, "stress-test",
							WithCapacity(100),
							WithTTL(50*time.Millisecond),
						)
						if acqErr == nil && permit != nil {
							time.Sleep(time.Millisecond)
							_ = permit.Release(ctx) //nolint:errcheck // best effort release in stress test
						}
					}
				}
			}()
		}

		wg.Wait()
		// 测试通过 = 没有 panic 或死锁
	})
}

// =============================================================================
// Query/Acquire 容量一致性测试（P2 修复验证）
// =============================================================================

func TestLocalSemaphore_QueryAcquireCapacityConsistency(t *testing.T) {
	// 测试场景：当 capacity < podCount 时，Query 和 Acquire 应该使用相同的本地容量
	ctx := context.Background()

	tests := []struct {
		name              string
		capacity          int
		tenantQuota       int
		podCount          int
		expectedLocal     int
		expectedTenantLoc int
	}{
		{
			name:              "capacity equals podCount",
			capacity:          4,
			tenantQuota:       4,
			podCount:          4,
			expectedLocal:     1, // max(1, 4/4) = 1
			expectedTenantLoc: 1,
		},
		{
			name:              "capacity less than podCount",
			capacity:          2,
			tenantQuota:       2,
			podCount:          4,
			expectedLocal:     1, // max(1, 2/4) = max(1, 0) = 1
			expectedTenantLoc: 1,
		},
		{
			name:              "capacity greater than podCount",
			capacity:          10,
			tenantQuota:       10, // 与 capacity 相同，避免租户配额限制
			podCount:          4,
			expectedLocal:     2, // max(1, 10/4) = max(1, 2) = 2
			expectedTenantLoc: 2, // max(1, 10/4) = max(1, 2) = 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultOptions()
			opts.podCount = tt.podCount
			sem := newLocalSemaphore(opts)
			defer closeSemaphore(t, sem)

			// 查询容量
			info, err := sem.Query(ctx, "consistency-test",
				QueryWithCapacity(tt.capacity),
				QueryWithTenantQuota(tt.tenantQuota),
				QueryWithTenantID("test-tenant"),
			)
			require.NoError(t, err)

			// 验证 Query 返回的容量
			assert.Equal(t, tt.expectedLocal, info.GlobalCapacity,
				"Query GlobalCapacity should match expected local capacity")
			assert.Equal(t, tt.expectedTenantLoc, info.TenantQuota,
				"Query TenantQuota should match expected local tenant quota")

			// 验证可以获取到与 Query 报告一致数量的许可
			permits := make([]Permit, 0, tt.expectedLocal)
			for i := 0; i < tt.expectedLocal; i++ {
				permit, acqErr := sem.TryAcquire(ctx, "consistency-test",
					WithCapacity(tt.capacity),
					WithTenantQuota(tt.tenantQuota),
					WithTenantID("test-tenant"),
				)
				require.NoError(t, acqErr)
				require.NotNil(t, permit, "should be able to acquire permit %d", i+1)
				permits = append(permits, permit)
			}

			// 验证容量确实被填满
			extraPermit, acqErr := sem.TryAcquire(ctx, "consistency-test",
				WithCapacity(tt.capacity),
			)
			assert.NoError(t, acqErr)
			assert.Nil(t, extraPermit, "should not be able to acquire more than local capacity")

			// 清理
			for _, p := range permits {
				releasePermit(t, ctx, p)
			}
		})
	}
}
