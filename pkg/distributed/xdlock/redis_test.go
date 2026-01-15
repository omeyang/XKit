//go:build integration

package xdlock_test

import (
	"context"
	"errors"
	"os"
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

// setupRedis 启动 Redis 容器或连接到已有 Redis。
// 如果设置了 XKIT_REDIS_ADDR 环境变量，直接使用外部 Redis。
func setupRedis(t *testing.T) (redis.UniversalClient, func()) {
	t.Helper()

	// 优先使用环境变量指定的 Redis
	if addr := os.Getenv("XKIT_REDIS_ADDR"); addr != "" {
		client := redis.NewClient(&redis.Options{
			Addr: addr,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Ping(ctx).Err(); err != nil {
			t.Skipf("无法连接到 Redis %s: %v", addr, err)
		}

		return client, func() { _ = client.Close() }
	}

	// 使用 testcontainers 启动 Redis
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
	if err != nil {
		t.Skipf("无法启动 Redis 容器: %v", err)
	}

	endpoint, err := container.Endpoint(ctx, "")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("获取 Redis 端点失败: %v", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr: endpoint,
	})

	// 验证连接
	if err := client.Ping(ctx).Err(); err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("无法连接到 Redis: %v", err)
	}

	cleanup := func() {
		_ = client.Close()
		_ = container.Terminate(ctx)
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
	defer func() { _ = factory.Close() }()

	assert.NotNil(t, factory.Redsync())
}

func TestRedisFactory_Health(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

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

	_ = factory.Close()

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
// 锁基本操作测试（使用 Handle API）
// =============================================================================

func TestRedisFactory_LockUnlock(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 获取锁
	handle, err := factory.Lock(ctx, "test-lock", xdlock.WithTries(1))
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 释放锁
	err = handle.Unlock(ctx)
	assert.NoError(t, err)
}

func TestRedisFactory_TryLock_Success(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// TryLock 成功
	handle, err := factory.TryLock(ctx, "test-trylock")
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 验证 Key
	assert.Contains(t, handle.Key(), "test-trylock")

	// 释放锁
	err = handle.Unlock(ctx)
	assert.NoError(t, err)
}

func TestRedisFactory_TryLock_LockHeld(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 第一个 TryLock 成功
	handle1, err := factory.TryLock(ctx, "test-trylock-fail")
	require.NoError(t, err)
	require.NotNil(t, handle1)
	defer func() { _ = handle1.Unlock(ctx) }()

	// 第二个 TryLock 应该返回 (nil, nil) 表示锁被占用
	handle2, err := factory.TryLock(ctx, "test-trylock-fail")
	assert.NoError(t, err)
	assert.Nil(t, handle2)
}

func TestRedisFactory_Lock_LockHeld(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 第一个 Lock 成功
	handle1, err := factory.Lock(ctx, "test-lock-fail", xdlock.WithTries(1))
	require.NoError(t, err)
	require.NotNil(t, handle1)
	defer func() { _ = handle1.Unlock(ctx) }()

	// 第二个 Lock 应该失败（锁被其他持有者占用，tries=1 不重试）
	handle2, err := factory.Lock(ctx, "test-lock-fail", xdlock.WithTries(1))
	assert.Error(t, err)
	assert.Nil(t, handle2)
}

func TestRedisLockHandle_Extend(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	handle, err := factory.TryLock(ctx, "test-extend", xdlock.WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle)
	defer func() { _ = handle.Unlock(ctx) }()

	// 等待一段时间后续期
	time.Sleep(1 * time.Second)

	// Redis 支持续期
	err = handle.Extend(ctx)
	assert.NoError(t, err)
}

func TestRedisFactory_WithKeyPrefix(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	handle, err := factory.TryLock(ctx, "mykey", xdlock.WithKeyPrefix("myapp:"))
	require.NoError(t, err)
	require.NotNil(t, handle)
	defer func() { _ = handle.Unlock(ctx) }()

	// 验证 Key 包含前缀
	assert.Contains(t, handle.Key(), "myapp:")
	assert.Contains(t, handle.Key(), "mykey")
}

func TestRedisFactory_Lock_ContextCanceled(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	// 第一个 handle 持有锁
	ctx1, cancel1 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel1()
	handle1, err := factory.TryLock(ctx1, "test-ctx-cancel")
	require.NoError(t, err)
	require.NotNil(t, handle1)
	defer func() { _ = handle1.Unlock(ctx1) }()

	// 第二个 Lock 尝试获取锁，但 context 会被取消
	ctx2, cancel2 := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel2()
	handle2, err := factory.Lock(ctx2, "test-ctx-cancel",
		xdlock.WithTries(100),
		xdlock.WithRetryDelay(100*time.Millisecond),
	)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
	assert.Nil(t, handle2)
}

// =============================================================================
// 并发测试
// =============================================================================

func TestRedisFactory_Lock_Concurrent(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	const goroutines = 10
	var counter int64
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			handle, err := factory.Lock(ctx, "concurrent-lock",
				xdlock.WithTries(50),
				xdlock.WithRetryDelay(100*time.Millisecond),
			)
			if err != nil {
				t.Logf("Lock failed: %v", err)
				return
			}
			defer func() { _ = handle.Unlock(ctx) }()

			// 临界区：递增计数器
			atomic.AddInt64(&counter, 1)
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(goroutines), counter)
}

func TestRedisFactory_Lock_MutualExclusion(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

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
				handle, err := factory.Lock(ctx, "mutual-exclusion",
					xdlock.WithTries(50),
					xdlock.WithRetryDelay(100*time.Millisecond),
				)
				if err != nil {
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
				_ = handle.Unlock(ctx)
			}
		}()
	}

	wg.Wait()
	assert.Zero(t, violations, "mutex violation detected")
}

// =============================================================================
// 选项测试
// =============================================================================

func TestRedisFactory_Lock_WithExpiry(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 使用较短的过期时间
	handle, err := factory.TryLock(ctx, "test-expiry", xdlock.WithExpiry(1*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle)
	defer func() { _ = handle.Unlock(ctx) }()

	// 验证 Key
	assert.Contains(t, handle.Key(), "test-expiry")
}

func TestRedisFactory_Lock_WithRetryDelayFunc(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 自定义重试延迟函数（指数退避）
	handle, err := factory.Lock(ctx, "test-retry-func",
		xdlock.WithTries(1),
		xdlock.WithRetryDelayFunc(func(tries int) time.Duration {
			return time.Duration(tries) * 50 * time.Millisecond
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, handle)
	defer func() { _ = handle.Unlock(ctx) }()
}

// =============================================================================
// Redsync 接口测试
// =============================================================================

func TestRedisFactory_Redsync(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	redsync := factory.Redsync()
	assert.NotNil(t, redsync)
}
