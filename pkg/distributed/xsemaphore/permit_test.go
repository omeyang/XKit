package xsemaphore

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Permit 基础测试
// =============================================================================

func TestPermitBase_ExpiresAt(t *testing.T) {
	t.Run("normal expiry", func(t *testing.T) {
		var base permitBase
		expiry := time.Now().Add(time.Minute)
		initPermitBase(&base, "id", "resource", "tenant", expiry, time.Minute, true, nil)

		assert.Equal(t, expiry, base.ExpiresAt())
	})

	t.Run("nil pointer returns zero time", func(t *testing.T) {
		var base permitBase
		// 不初始化 expiresAt
		assert.True(t, base.ExpiresAt().IsZero())
	})
}

func TestPermitBase_SetExpiresAt(t *testing.T) {
	var base permitBase
	expiry := time.Now().Add(time.Minute)
	initPermitBase(&base, "id", "resource", "tenant", expiry, time.Minute, true, nil)

	newExpiry := time.Now().Add(2 * time.Minute)
	base.setExpiresAt(newExpiry)

	assert.Equal(t, newExpiry, base.ExpiresAt())
}

func TestPermitBase_Released(t *testing.T) {
	var base permitBase
	initPermitBase(&base, "id", "resource", "tenant", time.Now(), time.Minute, true, nil)

	assert.False(t, base.isReleased())

	wasReleased := base.markReleased()
	assert.False(t, wasReleased) // 之前未释放

	assert.True(t, base.isReleased())

	wasReleased = base.markReleased()
	assert.True(t, wasReleased) // 现在已经是释放状态
}

func TestPermitBase_Metadata(t *testing.T) {
	t.Run("nil metadata", func(t *testing.T) {
		var base permitBase
		initPermitBase(&base, "id", "resource", "tenant", time.Now(), time.Minute, true, nil)

		assert.Nil(t, base.Metadata())
	})

	t.Run("empty metadata", func(t *testing.T) {
		var base permitBase
		initPermitBase(&base, "id", "resource", "tenant", time.Now(), time.Minute, true, map[string]string{})

		assert.Nil(t, base.Metadata())
	})

	t.Run("with metadata", func(t *testing.T) {
		var base permitBase
		meta := map[string]string{"key1": "value1", "key2": "value2"}
		initPermitBase(&base, "id", "resource", "tenant", time.Now(), time.Minute, true, meta)

		result := base.Metadata()
		assert.Equal(t, meta, result)

		// 验证返回的是副本
		result["key3"] = "value3"
		assert.NotContains(t, base.Metadata(), "key3")
	})

	t.Run("original not modified", func(t *testing.T) {
		var base permitBase
		meta := map[string]string{"key1": "value1"}
		initPermitBase(&base, "id", "resource", "tenant", time.Now(), time.Minute, true, meta)

		// 修改原始 map
		meta["key2"] = "value2"

		// base 中的 metadata 不应该受影响
		result := base.Metadata()
		assert.NotContains(t, result, "key2")
	})
}

// =============================================================================
// Redis Permit 自动续租测试
// =============================================================================

func TestRedisPermit_AutoExtend(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "auto-extend-test",
		WithCapacity(10),
		WithTTL(500*time.Millisecond),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	originalExpiry := permit.ExpiresAt()

	// 启动自动续租
	stop := permit.StartAutoExtend(100 * time.Millisecond)

	// 等待几次续租
	time.Sleep(350 * time.Millisecond)

	// 过期时间应该已更新
	assert.True(t, permit.ExpiresAt().After(originalExpiry))

	// 停止自动续租
	stop()

	releasePermit(t, ctx, permit)
}

func TestRedisPermit_AutoExtend_StopBeforeRelease(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "auto-stop-test",
		WithCapacity(10),
		WithTTL(time.Minute),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	stop := permit.StartAutoExtend(50 * time.Millisecond)

	// 多次停止应该安全
	stop()
	stop()

	releasePermit(t, ctx, permit)
}

func TestRedisPermit_AutoExtend_DoubleStart(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "double-start-test",
		WithCapacity(10),
		WithTTL(time.Minute),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	// 启动两次
	stop1 := permit.StartAutoExtend(50 * time.Millisecond)
	stop2 := permit.StartAutoExtend(50 * time.Millisecond)

	time.Sleep(100 * time.Millisecond)

	// 停止两次都应该安全
	stop1()
	stop2()

	releasePermit(t, ctx, permit)
}

func TestRedisPermit_AutoExtend_StopsOnRelease(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "stop-on-release-test",
		WithCapacity(10),
		WithTTL(time.Minute),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	stop := permit.StartAutoExtend(50 * time.Millisecond)
	_ = stop // 不调用 stop，让 Release 停止自动续租

	// Release 会停止自动续租
	releasePermit(t, ctx, permit)

	// 等待确保没有 panic
	time.Sleep(100 * time.Millisecond)
}

func TestRedisPermit_AutoExtend_Concurrent(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "concurrent-extend-test",
		WithCapacity(10),
		WithTTL(time.Minute),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	var wg sync.WaitGroup
	stops := make([]func(), 10)

	// 并发启动和停止
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			stops[idx] = permit.StartAutoExtend(50 * time.Millisecond)
		}(i)
	}

	wg.Wait()

	for _, stop := range stops {
		if stop != nil {
			stop()
		}
	}

	releasePermit(t, ctx, permit)
}

// =============================================================================
// Permit 属性测试
// =============================================================================

func TestPermit_Properties(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "prop-test",
		WithCapacity(10),
		WithTenantID("test-tenant"),
		WithTenantQuota(5),
		WithTTL(time.Minute),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	t.Run("ID", func(t *testing.T) {
		assert.NotEmpty(t, permit.ID())
	})

	t.Run("Resource", func(t *testing.T) {
		assert.Equal(t, "prop-test", permit.Resource())
	})

	t.Run("TenantID", func(t *testing.T) {
		assert.Equal(t, "test-tenant", permit.TenantID())
	})

	t.Run("ExpiresAt", func(t *testing.T) {
		assert.False(t, permit.ExpiresAt().IsZero())
		assert.True(t, permit.ExpiresAt().After(time.Now()))
	})

	releasePermit(t, ctx, permit)
}

// =============================================================================
// Permit 扩展测试
// =============================================================================

func TestPermit_Extend_UpdatesExpiry(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "extend-expiry-test",
		WithCapacity(10),
		WithTTL(time.Minute),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	original := permit.ExpiresAt()

	time.Sleep(10 * time.Millisecond)

	err = permit.Extend(ctx)
	require.NoError(t, err)

	// 新过期时间应该更晚
	assert.True(t, permit.ExpiresAt().After(original))

	releasePermit(t, ctx, permit)
}

func TestPermit_Extend_MultipleExtensions(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "multi-extend-test",
		WithCapacity(10),
		WithTTL(time.Minute),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	for i := 0; i < 5; i++ {
		time.Sleep(5 * time.Millisecond)
		err = permit.Extend(ctx)
		require.NoError(t, err)
	}

	releasePermit(t, ctx, permit)
}

// =============================================================================
// Permit 释放测试
// =============================================================================

func TestPermit_Release_Idempotent(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "idempotent-release-test",
		WithCapacity(10),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	// 释放多次应该安全
	for i := 0; i < 5; i++ {
		err = permit.Release(ctx)
		assert.NoError(t, err)
	}
}

func TestPermit_Release_ConcurrentSafe(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "concurrent-release-test",
		WithCapacity(10),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			releasePermit(t, ctx, permit)
		}()
	}

	wg.Wait()
}
