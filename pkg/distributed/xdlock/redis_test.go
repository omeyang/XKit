//go:build integration

package xdlock_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/omeyang/xkit/pkg/distributed/xdlock"
)

// setupRedis 启动 Redis 容器并返回客户端。
func setupRedis(t *testing.T) (redis.UniversalClient, func()) {
	t.Helper()

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "redis:7-alpine",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start redis container")

	endpoint, err := container.Endpoint(ctx, "")
	require.NoError(t, err, "failed to get redis endpoint")

	client := redis.NewClient(&redis.Options{
		Addr: endpoint,
	})

	// 验证连接
	require.NoError(t, client.Ping(ctx).Err(), "failed to ping redis")

	cleanup := func() {
		client.Close()
		container.Terminate(ctx)
	}

	return client, cleanup
}

// =============================================================================
// 工厂测试
// =============================================================================

func TestNewRedisFactory_Success(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	assert.NotNil(t, factory.Redsync())
}

func TestRedisFactory_Health(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = factory.Health(ctx)
	assert.NoError(t, err)
}

func TestRedisFactory_HealthAfterClose(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)

	factory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = factory.Health(ctx)
	assert.ErrorIs(t, err, xdlock.ErrFactoryClosed)
}

func TestRedisFactory_CloseIdempotent(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)

	// 多次关闭不应报错
	assert.NoError(t, factory.Close())
	assert.NoError(t, factory.Close())
}

// =============================================================================
// 锁基本操作测试
// =============================================================================

func TestRedisLocker_LockUnlock(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	locker := factory.NewMutex("test-lock")

	// 获取锁
	err = locker.Lock(ctx)
	require.NoError(t, err)

	// 释放锁
	err = locker.Unlock(ctx)
	assert.NoError(t, err)
}

func TestRedisLocker_TryLock(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	locker := factory.NewMutex("test-trylock", xdlock.WithTries(1))

	// TryLock 成功
	err = locker.TryLock(ctx)
	require.NoError(t, err)

	// 释放锁
	err = locker.Unlock(ctx)
	assert.NoError(t, err)
}

func TestRedisLocker_TryLockFailed(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 第一个 locker 获取锁
	locker1 := factory.NewMutex("test-trylock-fail", xdlock.WithTries(1))
	err = locker1.Lock(ctx)
	require.NoError(t, err)
	defer locker1.Unlock(ctx)

	// 第二个 locker TryLock 应该失败
	locker2 := factory.NewMutex("test-trylock-fail", xdlock.WithTries(1))
	err = locker2.TryLock(ctx)
	assert.ErrorIs(t, err, xdlock.ErrLockFailed)
}

func TestRedisLocker_Extend(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	locker := factory.NewMutex("test-extend", xdlock.WithExpiry(5*time.Second))
	err = locker.Lock(ctx)
	require.NoError(t, err)
	defer locker.Unlock(ctx)

	// 等待一段时间后续期
	time.Sleep(1 * time.Second)

	// Redis 支持续期
	err = locker.Extend(ctx)
	assert.NoError(t, err)
}

func TestRedisLocker_ExtendNotLocked(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	locker := factory.NewMutex("test-extend-not-locked")

	// 未获取锁时调用 Extend
	err = locker.Extend(ctx)
	assert.ErrorIs(t, err, xdlock.ErrNotLocked)
}

func TestRedisLocker_WithKeyPrefix(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	locker := factory.NewMutex("mykey", xdlock.WithKeyPrefix("myapp:"))

	// 正常获取和释放锁
	err = locker.Lock(ctx)
	require.NoError(t, err)
	defer locker.Unlock(ctx)

	// 验证锁值存在
	redisLocker, ok := locker.(xdlock.RedisLocker)
	require.True(t, ok)
	assert.NotEmpty(t, redisLocker.Value())
}

func TestRedisLocker_ContextCanceled(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	// 第一个 locker 持有锁
	ctx1, cancel1 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel1()
	locker1 := factory.NewMutex("test-ctx-cancel")
	err = locker1.Lock(ctx1)
	require.NoError(t, err)
	defer locker1.Unlock(ctx1)

	// 第二个 locker 尝试获取锁，但 context 会被取消
	ctx2, cancel2 := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel2()
	locker2 := factory.NewMutex("test-ctx-cancel",
		xdlock.WithTries(100),
		xdlock.WithRetryDelay(100*time.Millisecond),
	)
	err = locker2.Lock(ctx2)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
}

// =============================================================================
// 并发测试
// =============================================================================

func TestRedisLocker_Concurrent(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	const goroutines = 10
	var counter int64
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			locker := factory.NewMutex("concurrent-lock",
				xdlock.WithTries(50),
				xdlock.WithRetryDelay(100*time.Millisecond),
			)
			if err := locker.Lock(ctx); err != nil {
				t.Logf("Lock failed: %v", err)
				return
			}
			defer locker.Unlock(ctx)

			// 临界区：递增计数器
			atomic.AddInt64(&counter, 1)
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(goroutines), counter)
}

func TestRedisLocker_MutualExclusion(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	const goroutines = 5
	const iterations = 10
	var counter int64
	var violations int64
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				locker := factory.NewMutex("mutual-exclusion",
					xdlock.WithTries(50),
					xdlock.WithRetryDelay(100*time.Millisecond),
				)
				if err := locker.Lock(ctx); err != nil {
					t.Logf("Lock failed: %v", err)
					continue
				}

				// 检查互斥性
				current := atomic.AddInt64(&counter, 1)
				if current != 1 {
					atomic.AddInt64(&violations, 1)
				}

				// 模拟工作
				time.Sleep(10 * time.Millisecond)

				atomic.AddInt64(&counter, -1)
				locker.Unlock(ctx)
			}
		}()
	}

	wg.Wait()
	assert.Zero(t, violations, "mutex violation detected")
}

// =============================================================================
// 选项测试
// =============================================================================

func TestRedisLocker_WithExpiry(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 使用较短的过期时间
	locker := factory.NewMutex("test-expiry", xdlock.WithExpiry(1*time.Second))

	err = locker.Lock(ctx)
	require.NoError(t, err)

	// 验证锁存在
	redisLocker := locker.(xdlock.RedisLocker)
	assert.NotZero(t, redisLocker.Until())

	locker.Unlock(ctx)
}

func TestRedisLocker_WithRetryDelayFunc(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 自定义重试延迟函数（指数退避）
	locker := factory.NewMutex("test-retry-func",
		xdlock.WithRetryDelayFunc(func(tries int) time.Duration {
			return time.Duration(tries) * 50 * time.Millisecond
		}),
	)

	err = locker.Lock(ctx)
	require.NoError(t, err)
	defer locker.Unlock(ctx)
}

// =============================================================================
// 接口实现验证
// =============================================================================

func TestRedisLocker_ImplementsRedisLocker(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer factory.Close()

	locker := factory.NewMutex("test")
	redisLocker, ok := locker.(xdlock.RedisLocker)
	assert.True(t, ok)
	assert.NotNil(t, redisLocker.RedisMutex())
}
