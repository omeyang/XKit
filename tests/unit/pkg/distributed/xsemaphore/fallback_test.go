package xsemaphore

import (
	"context"
	"errors"
	"sync/atomic"
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

		var fallbackCalled atomic.Bool
		sem, err := New(client,
			WithFallback(FallbackLocal),
			WithPodCount(2),
			WithOnFallback(func(resource string, strategy FallbackStrategy, err error) {
				fallbackCalled.Store(true)
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
		assert.True(t, fallbackCalled.Load())

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

		// Redis 不可用时 Health 应返回错误（降级状态仍报告 Redis 不健康）
		err = sem.Health(context.Background())
		assert.Error(t, err, "Health should report error when Redis is unavailable")
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

func TestFallbackSemaphore_LocalCapacityFull(t *testing.T) {
	t.Run("TryAcquire returns nil when local capacity full", func(t *testing.T) {
		mr, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackLocal),
			WithPodCount(2),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		// 关闭 Redis 触发降级
		mr.Close()

		ctx := context.Background()
		// capacity=2, podCount=2 → localCapacity = max(1, 2/2) = 1
		permit, err := sem.TryAcquire(ctx, "capacity-full-test",
			WithCapacity(2),
		)
		require.NoError(t, err)
		require.NotNil(t, permit, "first acquire should succeed on local fallback")

		// 本地容量已满，再次 TryAcquire 应返回 (nil, nil)
		permit2, err := sem.TryAcquire(ctx, "capacity-full-test",
			WithCapacity(2),
		)
		assert.NoError(t, err)
		assert.Nil(t, permit2, "TryAcquire should return nil when local capacity full")

		releasePermit(t, ctx, permit)
	})

	t.Run("Acquire returns ErrAcquireFailed when local capacity full", func(t *testing.T) {
		mr, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackLocal),
			WithPodCount(2),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		// 关闭 Redis 触发降级
		mr.Close()

		ctx := context.Background()
		// capacity=2, podCount=2 → localCapacity = max(1, 2/2) = 1
		permit, err := sem.TryAcquire(ctx, "capacity-full-acquire-test",
			WithCapacity(2),
		)
		require.NoError(t, err)
		require.NotNil(t, permit, "first acquire should succeed on local fallback")

		// 本地容量已满，Acquire 应在重试耗尽后返回 ErrAcquireFailed
		_, err = sem.Acquire(ctx, "capacity-full-acquire-test",
			WithCapacity(2),
			WithMaxRetries(2),
			WithRetryDelay(10*time.Millisecond),
		)
		assert.ErrorIs(t, err, ErrAcquireFailed,
			"Acquire should return ErrAcquireFailed when local capacity full")

		releasePermit(t, ctx, permit)
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

	t.Run("FallbackOpen returns interface-nil when ID generation fails", func(t *testing.T) {
		// 回归测试：doFallback 在 FallbackOpen 分支若直接 `return newNoopPermit(...)`，
		// newNoopPermit 返回 (*noopPermit)(nil) 时会被装箱为非 nil 的 Permit 接口
		// （typed-nil through interface 陷阱），调用方 `if permit != nil { permit.Release() }`
		// 清理模式会 nil 解引用 panic。修复后必须显式返回 nil 接口。
		//
		// 测试通过 .(*fallbackSemaphore) 类型断言直接调用未导出的 doFallback，
		// 绕开 redisSemaphore.doAcquire 的前置 ID 生成（line redis.go:346）——
		// 否则相同的失败 IDGenerator 会让分布式路径先于 fallback 返回
		// ErrIDGenerationFailed（非 IsRedisError），导致 doFallback 永远不被执行。
		_, client := setupRedis(t)

		sem, err := New(client,
			WithFallback(FallbackOpen),
			WithPodCount(2),
			WithIDGenerator(func(_ context.Context) (string, error) {
				return "", errors.New("simulated ID generation failure")
			}),
		)
		require.NoError(t, err)
		defer closeSemaphore(t, sem)

		fs, ok := sem.(*fallbackSemaphore)
		require.True(t, ok, "expected *fallbackSemaphore when WithFallback is set")

		ctx := context.Background()
		permit, err := fs.doFallback(ctx, "id-gen-fail-test", []AcquireOption{WithCapacity(10)}, false)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrIDGenerationFailed)

		// 关键断言：用 Go 原生 == nil 检查接口值，**不要用 assert.Nil**——
		// testify 的 Nil 会经反射拆开接口，对 typed-nil 也返回 true，测不出此 bug。
		if permit != nil {
			t.Fatalf("expected interface-nil Permit, got typed-nil (interface holds %T)", permit)
		}
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
