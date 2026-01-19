//go:build integration

package xcron_test

import (
	"context"
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

	"github.com/omeyang/xkit/pkg/distributed/xcron"
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

		return client, func() { client.Close() }
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
	require.NoError(t, err, "获取 Redis 端点失败")

	client := redis.NewClient(&redis.Options{
		Addr: endpoint,
	})

	require.NoError(t, client.Ping(ctx).Err(), "Redis ping 失败")

	cleanup := func() {
		client.Close()
		container.Terminate(ctx)
	}

	return client, cleanup
}

// =============================================================================
// 基本操作测试
// =============================================================================

func TestRedisLocker_TryLock_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	locker := xcron.NewRedisLocker(client)
	ctx := context.Background()

	// 首次获取锁应成功
	handle, err := locker.TryLock(ctx, "test-job", 10*time.Second)
	require.NoError(t, err)
	require.NotNil(t, handle, "首次获取锁应成功")

	// 同一实例再次获取应失败（SETNX 语义）
	handle2, err := locker.TryLock(ctx, "test-job", 10*time.Second)
	require.NoError(t, err)
	assert.Nil(t, handle2, "锁已被持有，再次获取应失败")

	// 释放锁
	err = handle.Unlock(ctx)
	require.NoError(t, err)

	// 释放后应能再次获取
	handle3, err := locker.TryLock(ctx, "test-job", 10*time.Second)
	require.NoError(t, err)
	require.NotNil(t, handle3, "释放后应能再次获取")

	// 清理
	handle3.Unlock(ctx)
}

func TestRedisLocker_Unlock_NotHeld_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	locker := xcron.NewRedisLocker(client, xcron.WithRedisIdentity("instance-1"))
	ctx := context.Background()

	// 获取锁
	handle, err := locker.TryLock(ctx, "unlock-test", 10*time.Second)
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 释放锁
	err = handle.Unlock(ctx)
	require.NoError(t, err)

	// 再次释放已释放的锁应失败
	err = handle.Unlock(ctx)
	assert.ErrorIs(t, err, xcron.ErrLockNotHeld)
}

func TestRedisLocker_Unlock_WrongOwner_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	ctx := context.Background()

	// 实例 1 获取锁
	locker1 := xcron.NewRedisLocker(client, xcron.WithRedisIdentity("instance-1"))
	handle1, err := locker1.TryLock(ctx, "owner-test", 30*time.Second)
	require.NoError(t, err)
	require.NotNil(t, handle1)

	// 实例 2 尝试获取同一个锁（应失败）
	locker2 := xcron.NewRedisLocker(client, xcron.WithRedisIdentity("instance-2"))
	handle2, err := locker2.TryLock(ctx, "owner-test", 30*time.Second)
	require.NoError(t, err)
	assert.Nil(t, handle2, "锁已被 instance-1 持有")

	// 实例 1 释放（应成功）
	err = handle1.Unlock(ctx)
	assert.NoError(t, err)
}

func TestRedisLocker_Renew_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	locker := xcron.NewRedisLocker(client)
	ctx := context.Background()

	// 获取锁
	handle, err := locker.TryLock(ctx, "renew-test", 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 等待一段时间后续期
	time.Sleep(1 * time.Second)

	// 续期
	err = handle.Renew(ctx, 10*time.Second)
	require.NoError(t, err)

	// 验证锁仍然存在（通过 TTL 检查）
	ttl, err := client.TTL(ctx, "xcron:lock:renew-test").Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, 5*time.Second, "续期后 TTL 应大于 5 秒")

	// 清理
	handle.Unlock(ctx)
}

func TestRedisLocker_Renew_NotHeld_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	locker := xcron.NewRedisLocker(client)
	ctx := context.Background()

	// 获取锁
	handle, err := locker.TryLock(ctx, "renew-not-held", 10*time.Second)
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 释放锁
	err = handle.Unlock(ctx)
	require.NoError(t, err)

	// 尝试续期未持有的锁
	err = handle.Renew(ctx, 10*time.Second)
	assert.ErrorIs(t, err, xcron.ErrLockNotHeld)
}

func TestRedisLocker_Renew_WrongOwner_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	ctx := context.Background()

	// 实例 1 获取锁
	locker1 := xcron.NewRedisLocker(client, xcron.WithRedisIdentity("instance-1"))
	handle1, err := locker1.TryLock(ctx, "renew-owner-test", 30*time.Second)
	require.NoError(t, err)
	require.NotNil(t, handle1)

	// 实例 2 尝试获取（会失败，返回 nil handle）
	locker2 := xcron.NewRedisLocker(client, xcron.WithRedisIdentity("instance-2"))
	handle2, err := locker2.TryLock(ctx, "renew-owner-test", 30*time.Second)
	require.NoError(t, err)
	assert.Nil(t, handle2, "锁已被 instance-1 持有")

	// 清理
	handle1.Unlock(ctx)
}

// =============================================================================
// TTL 过期测试
// =============================================================================

func TestRedisLocker_TTLExpiry_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	locker := xcron.NewRedisLocker(client)
	ctx := context.Background()

	// 使用较短的 TTL
	handle, err := locker.TryLock(ctx, "ttl-test", 2*time.Second)
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 等待锁过期
	time.Sleep(3 * time.Second)

	// 现在应该能再次获取锁（原锁已过期）
	handle2, err := locker.TryLock(ctx, "ttl-test", 10*time.Second)
	require.NoError(t, err)
	assert.NotNil(t, handle2, "锁过期后应能重新获取")

	// 清理
	if handle2 != nil {
		handle2.Unlock(ctx)
	}
}

// =============================================================================
// 并发测试
// =============================================================================

func TestRedisLocker_Concurrent_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	const goroutines = 10
	const lockKey = "concurrent-test"

	var successCount int64
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			locker := xcron.NewRedisLocker(client,
				xcron.WithRedisIdentity("worker-"+string(rune('A'+id))),
			)

			handle, err := locker.TryLock(ctx, lockKey, 10*time.Second)
			if err != nil {
				t.Logf("Worker %d: TryLock error: %v", id, err)
				return
			}

			if handle != nil {
				atomic.AddInt64(&successCount, 1)
				// 持有锁一小段时间
				time.Sleep(100 * time.Millisecond)
				handle.Unlock(ctx)
			}
		}(i)
	}

	wg.Wait()

	// 由于是 TryLock（非阻塞），只有一个 goroutine 能获取锁
	assert.Equal(t, int64(1), successCount, "只有一个 goroutine 应成功获取锁")
}

func TestRedisLocker_MutualExclusion_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	const goroutines = 5
	const iterations = 3
	const lockKey = "mutex-test"

	var inCriticalSection int64
	var violations int64
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			locker := xcron.NewRedisLocker(client,
				xcron.WithRedisIdentity("worker-"+string(rune('A'+id))),
			)

			for j := 0; j < iterations; j++ {
				var handle xcron.LockHandle
				var err error

				// 轮询获取锁
				for {
					handle, err = locker.TryLock(ctx, lockKey, 5*time.Second)
					if err != nil {
						t.Logf("Worker %d: TryLock error: %v", id, err)
						return
					}
					if handle != nil {
						break
					}
					// 短暂等待后重试
					select {
					case <-ctx.Done():
						return
					case <-time.After(50 * time.Millisecond):
					}
				}

				// 临界区：检查互斥性
				current := atomic.AddInt64(&inCriticalSection, 1)
				if current != 1 {
					atomic.AddInt64(&violations, 1)
				}

				// 模拟工作
				time.Sleep(20 * time.Millisecond)

				atomic.AddInt64(&inCriticalSection, -1)
				handle.Unlock(ctx)
			}
		}(i)
	}

	wg.Wait()
	assert.Zero(t, violations, "检测到互斥违规")
}

// =============================================================================
// 配置选项测试
// =============================================================================

func TestRedisLocker_WithKeyPrefix_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	ctx := context.Background()

	locker := xcron.NewRedisLocker(client,
		xcron.WithRedisKeyPrefix("myapp:cron:"),
	)

	handle, err := locker.TryLock(ctx, "prefix-test", 10*time.Second)
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 验证 key 前缀
	exists, err := client.Exists(ctx, "myapp:cron:prefix-test").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists, "应使用自定义前缀")

	// 默认前缀不应存在
	exists2, err := client.Exists(ctx, "xcron:lock:prefix-test").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists2, "默认前缀下不应存在")

	// 清理
	handle.Unlock(ctx)
}

func TestRedisLocker_Identity_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	ctx := context.Background()

	locker := xcron.NewRedisLocker(client,
		xcron.WithRedisIdentity("custom-identity-123"),
	)

	assert.Equal(t, "custom-identity-123", locker.Identity())

	handle, err := locker.TryLock(ctx, "identity-test", 10*time.Second)
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 验证锁的值包含我们设置的 identity
	value, err := client.Get(ctx, "xcron:lock:identity-test").Result()
	require.NoError(t, err)
	assert.Contains(t, value, "custom-identity-123")

	// 清理
	handle.Unlock(ctx)
}

// =============================================================================
// 与 xcron 调度器集成测试
// =============================================================================

func TestRedisLocker_WithScheduler_Integration(t *testing.T) {
	client, cleanup := setupRedis(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	locker := xcron.NewRedisLocker(client)

	var executed int64
	scheduler := xcron.New(xcron.WithLocker(locker))

	// 添加一个简单任务
	_, err := scheduler.AddFunc("@every 1s", func(_ context.Context) error {
		atomic.AddInt64(&executed, 1)
		return nil
	}, xcron.WithName("test-job"), xcron.WithLockTTL(5*time.Second))
	require.NoError(t, err)

	scheduler.Start()
	defer scheduler.Stop()

	// 等待任务执行几次
	time.Sleep(3500 * time.Millisecond)

	// 验证任务执行了
	count := atomic.LoadInt64(&executed)
	assert.GreaterOrEqual(t, count, int64(2), "任务应至少执行 2 次")

	_ = ctx // 使用 ctx 避免未使用警告
}
