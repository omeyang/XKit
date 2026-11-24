package xsemaphore

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

		// goroutine 1: 触发清理
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem.cleanupAllExpired()
		}()

		// goroutine 2: 获取新许可
		wg.Add(1)
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
		for range goroutines {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := range iterations {
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
