package xsemaphore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
