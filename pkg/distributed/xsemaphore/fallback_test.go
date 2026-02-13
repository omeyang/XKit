package xsemaphore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// 降级信号量测试
// =============================================================================

func TestFallbackSemaphore_TryAcquire(t *testing.T) {
	t.Run("normal operation uses distributed", func(t *testing.T) {
		mr, client := setupRedis(t)
		_ = mr // keep miniredis running

		sem, err := New(client,
			WithFallback(FallbackLocal),
			WithPodCount(2),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		ctx := context.Background()
		permit, err := sem.TryAcquire(ctx, "test-resource",
			WithCapacity(10),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)

		releasePermit(t, ctx, permit)
	})

	t.Run("fallback local on redis error", func(t *testing.T) {
		mr, client := setupRedis(t)

		var fallbackCalled bool
		sem, err := New(client,
			WithFallback(FallbackLocal),
			WithPodCount(2),
			WithOnFallback(func(resource string, strategy FallbackStrategy, err error) {
				fallbackCalled = true
				assert.Equal(t, "fallback-test", resource)
				assert.Equal(t, FallbackLocal, strategy)
			}),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		// 关闭 Redis 模拟故障
		mr.Close()

		ctx := context.Background()
		permit, err := sem.TryAcquire(ctx, "fallback-test",
			WithCapacity(10),
		)
		require.NoError(t, err)
		require.NotNil(t, permit, "should fallback to local semaphore")
		assert.True(t, fallbackCalled)

		releasePermit(t, ctx, permit)
	})

	t.Run("fallback open on redis error", func(t *testing.T) {
		mr, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackOpen),
			WithPodCount(2),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		mr.Close()

		ctx := context.Background()
		permit, err := sem.TryAcquire(ctx, "open-fallback-test",
			WithCapacity(10),
		)
		require.NoError(t, err)
		require.NotNil(t, permit, "should return open permit")

		// open permit 的特性
		assert.NotEmpty(t, permit.ID())
		assert.Equal(t, "open-fallback-test", permit.Resource())

		releasePermit(t, ctx, permit)
	})

	t.Run("fallback close on redis error", func(t *testing.T) {
		mr, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackClose),
			WithPodCount(2),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		mr.Close()

		ctx := context.Background()
		permit, err := sem.TryAcquire(ctx, "close-fallback-test",
			WithCapacity(10),
		)
		// FallbackClose returns error when Redis is unavailable
		assert.ErrorIs(t, err, ErrRedisUnavailable)
		assert.Nil(t, permit, "should return nil for fallback close")
	})
}

func TestFallbackSemaphore_Acquire(t *testing.T) {
	t.Run("normal operation", func(t *testing.T) {
		mr, client := setupRedis(t)
		_ = mr

		sem, err := New(client,
			WithFallback(FallbackLocal),
			WithPodCount(2),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		ctx := context.Background()
		permit, err := sem.Acquire(ctx, "acquire-test",
			WithCapacity(10),
			WithMaxRetries(3),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)

		releasePermit(t, ctx, permit)
	})

	t.Run("fallback on redis error", func(t *testing.T) {
		mr, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackLocal),
			WithPodCount(2),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		mr.Close()

		ctx := context.Background()
		permit, err := sem.Acquire(ctx, "acquire-fallback-test",
			WithCapacity(10),
			WithMaxRetries(3),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)

		releasePermit(t, ctx, permit)
	})
}

func TestFallbackSemaphore_Query(t *testing.T) {
	t.Run("normal operation", func(t *testing.T) {
		mr, client := setupRedis(t)
		_ = mr

		sem, err := New(client,
			WithFallback(FallbackLocal),
			WithPodCount(2),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		ctx := context.Background()
		info, err := sem.Query(ctx, "query-test",
			QueryWithCapacity(10),
			QueryWithTenantQuota(5),
		)
		require.NoError(t, err)
		require.NotNil(t, info)
	})

	t.Run("fallback on redis error", func(t *testing.T) {
		mr, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackLocal),
			WithPodCount(2),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		mr.Close()

		ctx := context.Background()
		info, err := sem.Query(ctx, "query-fallback-test",
			QueryWithCapacity(10),
		)
		require.NoError(t, err)
		require.NotNil(t, info)
	})
}

func TestFallbackSemaphore_Health(t *testing.T) {
	t.Run("healthy redis", func(t *testing.T) {
		mr, client := setupRedis(t)
		_ = mr

		sem, err := New(client,
			WithFallback(FallbackLocal),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		err = sem.Health(context.Background())
		assert.NoError(t, err)
	})

	t.Run("unhealthy redis returns local health", func(t *testing.T) {
		mr, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackLocal),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		mr.Close()

		// 即使 Redis 不可用，本地信号量仍然健康
		_ = sem.Health(context.Background()) //nolint:errcheck // only verify no panic
		// 可能返回 Redis 错误或成功（取决于实现）
		// 这里我们只验证不 panic
	})

	t.Run("closed semaphore", func(t *testing.T) {
		mr, client := setupRedis(t)
		_ = mr

		sem, err := New(client,
			WithFallback(FallbackLocal),
		)
		require.NoError(t, err)

		closeSemaphore(t, sem)

		err = sem.Health(context.Background())
		assert.ErrorIs(t, err, ErrSemaphoreClosed)
	})
}

func TestFallbackSemaphore_Close(t *testing.T) {
	mr, client := setupRedis(t)
	_ = mr

	sem, err := New(client,
		WithFallback(FallbackLocal),
	)
	require.NoError(t, err)

	err = sem.Close(context.Background())
	assert.NoError(t, err)

	// 重复关闭
	err = sem.Close(context.Background())
	assert.NoError(t, err)
}

// =============================================================================
// Open Permit 测试
// =============================================================================

func TestOpenPermit(t *testing.T) {
	mr, client := setupRedis(t)

	sem, err := New(client,
		WithFallback(FallbackOpen),
	)
	require.NoError(t, err)
	defer closeSemaphore(t, sem)

	mr.Close() // 触发降级

	ctx := context.Background()
	permit, err := sem.TryAcquire(ctx, "open-permit-test",
		WithCapacity(10),
		WithTenantID("tenant-open"),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	// 测试 open permit 的所有方法
	t.Run("ID", func(t *testing.T) {
		assert.NotEmpty(t, permit.ID())
	})

	t.Run("Resource", func(t *testing.T) {
		assert.Equal(t, "open-permit-test", permit.Resource())
	})

	t.Run("TenantID", func(t *testing.T) {
		assert.Equal(t, "tenant-open", permit.TenantID())
	})

	t.Run("ExpiresAt", func(t *testing.T) {
		assert.False(t, permit.ExpiresAt().IsZero())
	})

	t.Run("Release", func(t *testing.T) {
		err := permit.Release(ctx)
		assert.NoError(t, err)
	})

	t.Run("Extend", func(t *testing.T) {
		// 获取新的 permit
		permit2, err := sem.TryAcquire(ctx, "extend-test", WithCapacity(10))
		require.NoError(t, err)
		require.NotNil(t, permit2)

		err = permit2.Extend(ctx)
		assert.NoError(t, err) // open permit 的 extend 总是成功

		releasePermit(t, ctx, permit2)
	})

	t.Run("StartAutoExtend", func(t *testing.T) {
		permit3, err := sem.TryAcquire(ctx, "auto-extend-test", WithCapacity(10))
		require.NoError(t, err)
		require.NotNil(t, permit3)

		stop := permit3.StartAutoExtend(50 * time.Millisecond)
		assert.NotNil(t, stop)

		time.Sleep(100 * time.Millisecond)
		stop()

		releasePermit(t, ctx, permit3)
	})
}

// =============================================================================
// 降级策略覆盖测试
// =============================================================================

func TestFallbackAcquire_AllStrategies(t *testing.T) {
	t.Run("FallbackLocal uses local.Acquire", func(t *testing.T) {
		mr, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackLocal),
			WithPodCount(2),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		mr.Close() // 触发降级

		ctx := context.Background()
		permit, err := sem.Acquire(ctx, "local-acquire-test",
			WithCapacity(10),
			WithMaxRetries(2),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)

		// 本地许可应该有唯一 ID
		assert.NotEmpty(t, permit.ID())
		assert.NotContains(t, permit.ID(), "noop")

		releasePermit(t, ctx, permit)
	})

	t.Run("FallbackOpen returns noop permit for Acquire", func(t *testing.T) {
		mr, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackOpen),
			WithPodCount(2),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		mr.Close() // 触发降级

		ctx := context.Background()
		permit, err := sem.Acquire(ctx, "open-acquire-test",
			WithCapacity(10),
			WithMaxRetries(2),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)

		// noop permit 应该有 "noop-" 前缀
		assert.Contains(t, permit.ID(), "noop-")

		releasePermit(t, ctx, permit)
	})

	t.Run("FallbackClose returns error for Acquire", func(t *testing.T) {
		mr, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackClose),
			WithPodCount(2),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		mr.Close() // 触发降级

		ctx := context.Background()
		permit, err := sem.Acquire(ctx, "close-acquire-test",
			WithCapacity(10),
			WithMaxRetries(2),
		)
		assert.ErrorIs(t, err, ErrRedisUnavailable)
		assert.Nil(t, permit)
	})
}

func TestFallbackQuery_AllStrategies(t *testing.T) {
	t.Run("FallbackLocal queries local", func(t *testing.T) {
		mr, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackLocal),
			WithPodCount(2),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		mr.Close() // 触发降级

		ctx := context.Background()
		info, err := sem.Query(ctx, "query-local-test",
			QueryWithCapacity(10),
			QueryWithTenantQuota(5),
		)
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "query-local-test", info.Resource)
	})

	t.Run("FallbackOpen returns full capacity", func(t *testing.T) {
		mr, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackOpen),
			WithPodCount(2),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		mr.Close() // 触发降级

		ctx := context.Background()
		info, err := sem.Query(ctx, "query-open-test",
			QueryWithCapacity(100),
			QueryWithTenantQuota(10),
		)
		require.NoError(t, err)
		require.NotNil(t, info)

		// FallbackOpen 应该返回全部可用
		assert.Equal(t, "query-open-test", info.Resource)
		assert.Equal(t, 100, info.GlobalCapacity)
		assert.Equal(t, 0, info.GlobalUsed)
		assert.Equal(t, 100, info.GlobalAvailable)
		assert.Equal(t, 10, info.TenantQuota)
		assert.Equal(t, 0, info.TenantUsed)
		assert.Equal(t, 10, info.TenantAvailable)
	})

	t.Run("FallbackClose returns error", func(t *testing.T) {
		mr, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackClose),
			WithPodCount(2),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		mr.Close() // 触发降级

		ctx := context.Background()
		info, err := sem.Query(ctx, "query-close-test",
			QueryWithCapacity(10),
		)
		assert.ErrorIs(t, err, ErrRedisUnavailable)
		assert.Nil(t, info)
	})
}

func TestNoopPermit_UniqueID(t *testing.T) {
	mr, client := setupRedis(t)

	sem, err := New(client,
		WithFallback(FallbackOpen),
	)
	require.NoError(t, err)
	defer closeSemaphore(t, sem)

	mr.Close() // 触发降级

	ctx := context.Background()

	// 获取多个 noop permit，ID 应该都不同
	permit1, err := sem.TryAcquire(ctx, "same-resource", WithCapacity(10))
	require.NoError(t, err)
	permit2, err := sem.TryAcquire(ctx, "same-resource", WithCapacity(10))
	require.NoError(t, err)
	permit3, err := sem.TryAcquire(ctx, "same-resource", WithCapacity(10))
	require.NoError(t, err)

	require.NotNil(t, permit1)
	require.NotNil(t, permit2)
	require.NotNil(t, permit3)

	// 每个 permit 应该有不同的 ID
	assert.NotEqual(t, permit1.ID(), permit2.ID())
	assert.NotEqual(t, permit2.ID(), permit3.ID())
	assert.NotEqual(t, permit1.ID(), permit3.ID())

	// 所有 ID 应该都有 noop- 前缀
	assert.Contains(t, permit1.ID(), "noop-")
	assert.Contains(t, permit2.ID(), "noop-")
	assert.Contains(t, permit3.ID(), "noop-")
}

// =============================================================================
// safeOnFallback panic recovery 测试
// =============================================================================

func TestSafeOnFallback_PanicRecovery(t *testing.T) {
	mr, client := setupRedis(t)

	sem, err := New(client,
		WithFallback(FallbackLocal),
		WithPodCount(2),
		WithOnFallback(func(resource string, strategy FallbackStrategy, err error) {
			panic("intentional panic for testing")
		}),
	)
	require.NoError(t, err)
	defer closeSemaphore(t, sem)

	mr.Close() // 触发降级

	ctx := context.Background()

	// 即使 onFallback 回调 panic，也不应该影响正常操作
	permit, err := sem.TryAcquire(ctx, "panic-test", WithCapacity(10))
	assert.NoError(t, err, "should not fail even if callback panics")
	assert.NotNil(t, permit)

	releasePermit(t, ctx, permit)
}

// =============================================================================
// Permit metadata 测试
// =============================================================================

func TestPermit_Metadata(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	t.Run("with metadata", func(t *testing.T) {
		meta := map[string]string{
			"trace_id":   "abc123",
			"request_id": "req-456",
		}
		permit, err := sem.TryAcquire(ctx, "metadata-test",
			WithCapacity(10),
			WithMetadata(meta),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)

		result := permit.Metadata()
		assert.Equal(t, "abc123", result["trace_id"])
		assert.Equal(t, "req-456", result["request_id"])

		releasePermit(t, ctx, permit)
	})

	t.Run("without metadata", func(t *testing.T) {
		permit, err := sem.TryAcquire(ctx, "no-metadata-test",
			WithCapacity(10),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)

		assert.Nil(t, permit.Metadata())

		releasePermit(t, ctx, permit)
	})

	t.Run("metadata is copied", func(t *testing.T) {
		meta := map[string]string{"key": "value"}
		permit, err := sem.TryAcquire(ctx, "copy-metadata-test",
			WithCapacity(10),
			WithMetadata(meta),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)

		// 修改原始 map
		meta["new_key"] = "new_value"

		// permit 中的 metadata 不应该受影响
		result := permit.Metadata()
		assert.NotContains(t, result, "new_key")

		releasePermit(t, ctx, permit)
	})
}

func TestNoopPermit_Metadata(t *testing.T) {
	mr, client := setupRedis(t)

	sem, err := New(client,
		WithFallback(FallbackOpen),
	)
	require.NoError(t, err)
	defer closeSemaphore(t, sem)

	mr.Close() // 触发降级

	ctx := context.Background()

	t.Run("with metadata", func(t *testing.T) {
		meta := map[string]string{"key": "value"}
		permit, err := sem.TryAcquire(ctx, "noop-metadata-test",
			WithCapacity(10),
			WithMetadata(meta),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)

		result := permit.Metadata()
		assert.Equal(t, "value", result["key"])

		releasePermit(t, ctx, permit)
	})

	t.Run("metadata immutability", func(t *testing.T) {
		meta := map[string]string{"key": "value"}
		permit, err := sem.TryAcquire(ctx, "noop-immutable-test",
			WithCapacity(10),
			WithMetadata(meta),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)

		// 修改返回的 metadata
		result := permit.Metadata()
		result["new_key"] = "new_value"

		// 再次获取应该不包含新 key
		result2 := permit.Metadata()
		assert.NotContains(t, result2, "new_key")

		releasePermit(t, ctx, permit)
	})
}

// =============================================================================
// startAutoExtendLoop interval <= 0 测试
// =============================================================================

func TestStartAutoExtend_InvalidInterval(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "invalid-interval-test",
		WithCapacity(10),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)
	defer releasePermit(t, ctx, permit)

	t.Run("zero interval", func(t *testing.T) {
		stop := permit.StartAutoExtend(0)
		assert.NotNil(t, stop)
		stop() // 应该是空操作，不 panic
	})

	t.Run("negative interval", func(t *testing.T) {
		stop := permit.StartAutoExtend(-time.Second)
		assert.NotNil(t, stop)
		stop() // 应该是空操作，不 panic
	})
}

// =============================================================================
// 延迟初始化测试（P3 修复验证）
// =============================================================================

func TestFallbackSemaphore_LazyLocalInitialization(t *testing.T) {
	t.Run("FallbackOpen does not create localSemaphore until needed", func(t *testing.T) {
		mr, client := setupRedis(t)
		_ = mr // keep running

		sem, err := New(client,
			WithFallback(FallbackOpen),
			WithPodCount(2),
		)
		require.NoError(t, err)

		// 类型断言获取 fallbackSemaphore
		fs, ok := sem.(*fallbackSemaphore)
		require.True(t, ok, "should be fallbackSemaphore")

		// 在没有触发降级之前，local 应该为 nil
		assert.Nil(t, fs.local, "local should be nil before fallback is triggered")

		ctx := context.Background()

		// 正常操作（Redis 可用），不触发降级
		permit, err := sem.TryAcquire(ctx, "normal-test", WithCapacity(10))
		require.NoError(t, err)
		require.NotNil(t, permit)
		releasePermit(t, ctx, permit)

		// local 仍然应该为 nil（FallbackOpen 不需要 local）
		assert.Nil(t, fs.local, "local should still be nil for FallbackOpen")

		closeSemaphore(t, sem)
	})

	t.Run("FallbackClose does not create localSemaphore", func(t *testing.T) {
		mr, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackClose),
			WithPodCount(2),
		)
		require.NoError(t, err)

		fs, ok := sem.(*fallbackSemaphore)
		require.True(t, ok)
		assert.Nil(t, fs.local, "local should be nil for FallbackClose")

		// 触发降级
		mr.Close()

		ctx := context.Background()
		_, err = sem.TryAcquire(ctx, "close-test", WithCapacity(10))
		assert.ErrorIs(t, err, ErrRedisUnavailable)

		// local 仍然应该为 nil
		assert.Nil(t, fs.local, "local should still be nil for FallbackClose")

		closeSemaphore(t, sem)
	})

	t.Run("FallbackLocal creates localSemaphore on first fallback", func(t *testing.T) {
		mr, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackLocal),
			WithPodCount(2),
		)
		require.NoError(t, err)

		fs, ok := sem.(*fallbackSemaphore)
		require.True(t, ok)

		// 初始时 local 应该为 nil
		assert.Nil(t, fs.local, "local should be nil initially")

		ctx := context.Background()

		// 正常操作，不触发降级
		permit, err := sem.TryAcquire(ctx, "normal-test", WithCapacity(10))
		require.NoError(t, err)
		require.NotNil(t, permit)
		releasePermit(t, ctx, permit)

		// local 仍然应该为 nil（还没有降级）
		assert.Nil(t, fs.local, "local should still be nil before fallback")

		// 触发降级
		mr.Close()

		permit2, err := sem.TryAcquire(ctx, "fallback-test", WithCapacity(10))
		require.NoError(t, err)
		require.NotNil(t, permit2)

		// 现在 local 应该被创建了
		assert.NotNil(t, fs.local, "local should be initialized after fallback")

		releasePermit(t, ctx, permit2)
		closeSemaphore(t, sem)
	})

	t.Run("ensureLocalSemaphore is idempotent", func(t *testing.T) {
		mr, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackLocal),
			WithPodCount(2),
		)
		require.NoError(t, err)

		fs, ok := sem.(*fallbackSemaphore)
		require.True(t, ok)

		mr.Close() // 触发降级

		ctx := context.Background()

		// 多次降级应该使用同一个 local 实例
		permit1, err := sem.TryAcquire(ctx, "test1", WithCapacity(10))
		require.NoError(t, err)
		local1 := fs.local

		permit2, err := sem.TryAcquire(ctx, "test2", WithCapacity(10))
		require.NoError(t, err)
		local2 := fs.local

		assert.Same(t, local1, local2, "should reuse the same local instance")

		if permit1 != nil {
			releasePermit(t, ctx, permit1)
		}
		if permit2 != nil {
			releasePermit(t, ctx, permit2)
		}
		closeSemaphore(t, sem)
	})
}

// setupRedis is defined in semaphore_test.go
