package xsemaphore

import (
	"context"
	"testing"
	"time"

	"github.com/omeyang/xkit/internal/rediscompat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Pipeline 兼容模式测试
// =============================================================================

func setupCompatSemaphore(t *testing.T) (Semaphore, context.Context) {
	t.Helper()
	sem, _ := setupSemaphore(t, WithScriptMode(rediscompat.ScriptModeCompat))
	return sem, context.Background()
}

func TestCompat_TryAcquire(t *testing.T) {
	t.Run("acquire single permit", func(t *testing.T) {
		sem, ctx := setupCompatSemaphore(t)

		p, err := sem.TryAcquire(ctx, "res1", WithCapacity(5), WithTTL(time.Minute))
		require.NoError(t, err)
		require.NotNil(t, p)
		assert.NotEmpty(t, p.ID())
		assert.Equal(t, "res1", p.Resource())
		releasePermit(t, ctx, p)
	})

	t.Run("capacity full returns nil permit", func(t *testing.T) {
		sem, ctx := setupCompatSemaphore(t)

		p1, err := sem.TryAcquire(ctx, "res2", WithCapacity(1), WithTTL(time.Minute))
		require.NoError(t, err)
		require.NotNil(t, p1)

		p2, err := sem.TryAcquire(ctx, "res2", WithCapacity(1), WithTTL(time.Minute))
		require.NoError(t, err)
		assert.Nil(t, p2, "should not acquire when capacity is full")

		releasePermit(t, ctx, p1)
	})

	t.Run("multiple permits up to capacity", func(t *testing.T) {
		sem, ctx := setupCompatSemaphore(t)

		permits := make([]Permit, 3)
		for i := range permits {
			p, err := sem.TryAcquire(ctx, "res3", WithCapacity(3), WithTTL(time.Minute))
			require.NoError(t, err)
			require.NotNil(t, p, "should acquire permit %d", i)
			permits[i] = p
		}

		// 4th should fail
		p4, err := sem.TryAcquire(ctx, "res3", WithCapacity(3), WithTTL(time.Minute))
		require.NoError(t, err)
		assert.Nil(t, p4)

		for _, p := range permits {
			releasePermit(t, ctx, p)
		}
	})
}

func TestCompat_TryAcquire_WithTenant(t *testing.T) {
	t.Run("tenant quota exceeded", func(t *testing.T) {
		sem, ctx := setupCompatSemaphore(t)

		p1, err := sem.TryAcquire(ctx, "res-t1",
			WithCapacity(10), WithTTL(time.Minute),
			WithTenantID("t1"), WithTenantQuota(1),
		)
		require.NoError(t, err)
		require.NotNil(t, p1)

		// Same tenant, quota=1, should fail
		p2, err := sem.TryAcquire(ctx, "res-t1",
			WithCapacity(10), WithTTL(time.Minute),
			WithTenantID("t1"), WithTenantQuota(1),
		)
		require.NoError(t, err)
		assert.Nil(t, p2, "tenant quota exceeded")

		releasePermit(t, ctx, p1)
	})

	t.Run("different tenants share global capacity", func(t *testing.T) {
		sem, ctx := setupCompatSemaphore(t)

		p1, err := sem.TryAcquire(ctx, "res-t2",
			WithCapacity(2), WithTTL(time.Minute),
			WithTenantID("t1"), WithTenantQuota(2),
		)
		require.NoError(t, err)
		require.NotNil(t, p1)

		p2, err := sem.TryAcquire(ctx, "res-t2",
			WithCapacity(2), WithTTL(time.Minute),
			WithTenantID("t2"), WithTenantQuota(2),
		)
		require.NoError(t, err)
		require.NotNil(t, p2)

		// Global capacity 2, both taken
		p3, err := sem.TryAcquire(ctx, "res-t2",
			WithCapacity(2), WithTTL(time.Minute),
			WithTenantID("t3"), WithTenantQuota(2),
		)
		require.NoError(t, err)
		assert.Nil(t, p3, "global capacity full")

		releasePermit(t, ctx, p1)
		releasePermit(t, ctx, p2)
	})
}

func TestCompat_Release(t *testing.T) {
	t.Run("release successfully", func(t *testing.T) {
		sem, ctx := setupCompatSemaphore(t)

		p, err := sem.TryAcquire(ctx, "res-rel", WithCapacity(1), WithTTL(time.Minute))
		require.NoError(t, err)
		require.NotNil(t, p)

		err = p.Release(ctx)
		require.NoError(t, err)
	})

	t.Run("release frees capacity", func(t *testing.T) {
		sem, ctx := setupCompatSemaphore(t)

		p1, err := sem.TryAcquire(ctx, "res-rel2", WithCapacity(1), WithTTL(time.Minute))
		require.NoError(t, err)
		require.NotNil(t, p1)

		err = p1.Release(ctx)
		require.NoError(t, err)

		// Should be able to acquire again
		p2, err := sem.TryAcquire(ctx, "res-rel2", WithCapacity(1), WithTTL(time.Minute))
		require.NoError(t, err)
		assert.NotNil(t, p2)
		releasePermit(t, ctx, p2)
	})

	t.Run("release with tenant", func(t *testing.T) {
		sem, ctx := setupCompatSemaphore(t)

		p, err := sem.TryAcquire(ctx, "res-rel3",
			WithCapacity(5), WithTTL(time.Minute),
			WithTenantID("t1"), WithTenantQuota(1),
		)
		require.NoError(t, err)
		require.NotNil(t, p)

		err = p.Release(ctx)
		require.NoError(t, err)

		// Should be able to acquire again for same tenant
		p2, err := sem.TryAcquire(ctx, "res-rel3",
			WithCapacity(5), WithTTL(time.Minute),
			WithTenantID("t1"), WithTenantQuota(1),
		)
		require.NoError(t, err)
		assert.NotNil(t, p2)
		releasePermit(t, ctx, p2)
	})
}

func TestCompat_Extend(t *testing.T) {
	t.Run("extend successfully", func(t *testing.T) {
		sem, ctx := setupCompatSemaphore(t)

		p, err := sem.TryAcquire(ctx, "res-ext", WithCapacity(1), WithTTL(time.Minute))
		require.NoError(t, err)
		require.NotNil(t, p)

		oldExpiry := p.ExpiresAt()
		// Short pause to ensure new expiry is later
		time.Sleep(5 * time.Millisecond)
		err = p.Extend(ctx)
		require.NoError(t, err)
		assert.True(t, p.ExpiresAt().After(oldExpiry), "expiry should be extended")

		releasePermit(t, ctx, p)
	})

	t.Run("extend after release returns error", func(t *testing.T) {
		sem, ctx := setupCompatSemaphore(t)

		p, err := sem.TryAcquire(ctx, "res-ext2", WithCapacity(1), WithTTL(time.Minute))
		require.NoError(t, err)
		require.NotNil(t, p)

		err = p.Release(ctx)
		require.NoError(t, err)

		err = p.Extend(ctx)
		assert.ErrorIs(t, err, ErrPermitNotHeld)
	})
}

func TestCompat_Expire(t *testing.T) {
	t.Run("expired permits are cleaned on next acquire", func(t *testing.T) {
		sem, _ := setupCompatSemaphore(t)
		ctx := context.Background()

		// Acquire with very short TTL
		p1, err := sem.TryAcquire(ctx, "res-exp", WithCapacity(1), WithTTL(50*time.Millisecond))
		require.NoError(t, err)
		require.NotNil(t, p1)

		// Wait for real time to pass (compat mode uses time.Now() for cleanup)
		time.Sleep(100 * time.Millisecond)

		// Should be able to acquire again (expired permit cleaned during acquire)
		p2, err := sem.TryAcquire(ctx, "res-exp", WithCapacity(1), WithTTL(time.Minute))
		require.NoError(t, err)
		assert.NotNil(t, p2, "expired permit should be cleaned")
		releasePermit(t, ctx, p2)
	})
}

func TestCompat_Query(t *testing.T) {
	t.Run("query empty resource", func(t *testing.T) {
		sem, ctx := setupCompatSemaphore(t)

		info, err := sem.Query(ctx, "res-q1", QueryWithCapacity(10))
		require.NoError(t, err)
		assert.Equal(t, 0, info.GlobalUsed)
		assert.Equal(t, 10, info.GlobalAvailable)
	})

	t.Run("query with permits", func(t *testing.T) {
		sem, ctx := setupCompatSemaphore(t)

		p1, err := sem.TryAcquire(ctx, "res-q2", WithCapacity(5), WithTTL(time.Minute))
		require.NoError(t, err)
		require.NotNil(t, p1)

		p2, err := sem.TryAcquire(ctx, "res-q2", WithCapacity(5), WithTTL(time.Minute))
		require.NoError(t, err)
		require.NotNil(t, p2)

		info, err := sem.Query(ctx, "res-q2", QueryWithCapacity(5))
		require.NoError(t, err)
		assert.Equal(t, 2, info.GlobalUsed)
		assert.Equal(t, 3, info.GlobalAvailable)

		releasePermit(t, ctx, p1)
		releasePermit(t, ctx, p2)
	})

	t.Run("query with tenant", func(t *testing.T) {
		sem, ctx := setupCompatSemaphore(t)

		p, err := sem.TryAcquire(ctx, "res-q3",
			WithCapacity(10), WithTTL(time.Minute),
			WithTenantID("t1"), WithTenantQuota(5),
		)
		require.NoError(t, err)
		require.NotNil(t, p)

		info, err := sem.Query(ctx, "res-q3",
			QueryWithCapacity(10), QueryWithTenantID("t1"), QueryWithTenantQuota(5),
		)
		require.NoError(t, err)
		assert.Equal(t, 1, info.GlobalUsed)
		assert.Equal(t, 1, info.TenantUsed)
		assert.Equal(t, 4, info.TenantAvailable)

		releasePermit(t, ctx, p)
	})
}

func TestCompat_WithScriptMode(t *testing.T) {
	t.Run("explicit compat mode skips detection", func(t *testing.T) {
		_, client := setupRedis(t)
		sem, err := New(client, WithScriptMode(rediscompat.ScriptModeCompat))
		require.NoError(t, err)
		closeSemaphore(t, sem)
	})

	t.Run("explicit lua mode skips detection", func(t *testing.T) {
		_, client := setupRedis(t)
		sem, err := New(client, WithScriptMode(rediscompat.ScriptModeLua))
		require.NoError(t, err)
		closeSemaphore(t, sem)
	})

	t.Run("auto mode detects lua with miniredis", func(t *testing.T) {
		_, client := setupRedis(t)
		sem, err := New(client, WithScriptMode(rediscompat.ScriptModeAuto))
		require.NoError(t, err)
		closeSemaphore(t, sem)
	})

	t.Run("invalid script mode returns error", func(t *testing.T) {
		_, client := setupRedis(t)
		_, err := New(client, WithScriptMode(rediscompat.ScriptMode(99)))
		assert.ErrorIs(t, err, ErrInvalidScriptMode)
	})
}
