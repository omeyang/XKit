package xcron

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// RedisLocker Options Tests
// ============================================================================

func TestNewRedisLocker(t *testing.T) {
	// 使用 miniredis 测试选项功能
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	t.Run("default options", func(t *testing.T) {
		locker := NewRedisLocker(client)

		require.NotNil(t, locker)
		assert.Equal(t, "xcron:lock:", locker.prefix)
		assert.NotEmpty(t, locker.identity) // 默认 hostname:pid
	})

	t.Run("with custom prefix", func(t *testing.T) {
		locker := NewRedisLocker(client, WithRedisKeyPrefix("myapp:lock:"))

		assert.Equal(t, "myapp:lock:", locker.prefix)
	})

	t.Run("with custom identity", func(t *testing.T) {
		locker := NewRedisLocker(client, WithRedisIdentity("custom-identity"))

		assert.Equal(t, "custom-identity", locker.identity)
	})

	t.Run("with multiple options", func(t *testing.T) {
		locker := NewRedisLocker(client,
			WithRedisKeyPrefix("test:"),
			WithRedisIdentity("test-id"),
		)

		assert.Equal(t, "test:", locker.prefix)
		assert.Equal(t, "test-id", locker.identity)
	})

	t.Run("nil client panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewRedisLocker(nil)
		})
	})
}

func TestRedisLocker_Identity(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	locker := NewRedisLocker(client, WithRedisIdentity("my-identity"))

	assert.Equal(t, "my-identity", locker.Identity())
}

func TestRedisLocker_Client(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	locker := NewRedisLocker(client)
	assert.Equal(t, client, locker.Client())
}

// TestDefaultIdentity 已在 locker_test.go 中定义

// ============================================================================
// RedisLocker Option Functions
// ============================================================================

func TestWithRedisKeyPrefix(t *testing.T) {
	t.Run("sets prefix", func(t *testing.T) {
		locker := &RedisLocker{}
		opt := WithRedisKeyPrefix("custom:")

		opt(locker)

		assert.Equal(t, "custom:", locker.prefix)
	})

	t.Run("empty prefix", func(t *testing.T) {
		locker := &RedisLocker{}
		opt := WithRedisKeyPrefix("")

		opt(locker)

		assert.Empty(t, locker.prefix)
	})
}

func TestWithRedisIdentity(t *testing.T) {
	t.Run("sets identity", func(t *testing.T) {
		locker := &RedisLocker{}
		opt := WithRedisIdentity("my-pod-123")

		opt(locker)

		assert.Equal(t, "my-pod-123", locker.identity)
	})

	t.Run("empty identity", func(t *testing.T) {
		locker := &RedisLocker{}
		opt := WithRedisIdentity("")

		opt(locker)

		assert.Empty(t, locker.identity)
	})
}

// ============================================================================
// RedisLocker Core Methods Tests (with miniredis)
// ============================================================================

// setupRedisLocker 创建测试用的 RedisLocker（使用 miniredis）
func setupRedisLocker(t *testing.T) (*RedisLocker, *miniredis.Miniredis) {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	locker := NewRedisLocker(client, WithRedisIdentity("test-instance"))

	t.Cleanup(func() {
		client.Close()
		mr.Close()
	})

	return locker, mr
}

func TestRedisLocker_TryLock(t *testing.T) {
	t.Run("acquire lock successfully", func(t *testing.T) {
		locker, _ := setupRedisLocker(t)
		ctx := context.Background()

		handle, err := locker.TryLock(ctx, "job-1", 30*time.Second)

		require.NoError(t, err)
		assert.NotNil(t, handle)
	})

	t.Run("fail to acquire already held lock", func(t *testing.T) {
		locker, _ := setupRedisLocker(t)
		ctx := context.Background()

		// 第一次获取成功
		handle1, err := locker.TryLock(ctx, "job-1", 30*time.Second)
		require.NoError(t, err)
		assert.NotNil(t, handle1)

		// 第二次获取失败（锁已被持有）
		handle2, err := locker.TryLock(ctx, "job-1", 30*time.Second)
		require.NoError(t, err)
		assert.Nil(t, handle2)
	})

	t.Run("acquire different keys independently", func(t *testing.T) {
		locker, _ := setupRedisLocker(t)
		ctx := context.Background()

		handle1, err := locker.TryLock(ctx, "job-1", 30*time.Second)
		require.NoError(t, err)
		assert.NotNil(t, handle1)

		handle2, err := locker.TryLock(ctx, "job-2", 30*time.Second)
		require.NoError(t, err)
		assert.NotNil(t, handle2)
	})

	t.Run("lock expires after TTL", func(t *testing.T) {
		locker, mr := setupRedisLocker(t)
		ctx := context.Background()

		handle1, err := locker.TryLock(ctx, "job-1", 100*time.Millisecond)
		require.NoError(t, err)
		assert.NotNil(t, handle1)

		// 快进时间让锁过期
		mr.FastForward(200 * time.Millisecond)

		// 锁已过期，可以重新获取
		handle2, err := locker.TryLock(ctx, "job-1", 30*time.Second)
		require.NoError(t, err)
		assert.NotNil(t, handle2)
	})
}

func TestRedisLockHandle_Unlock(t *testing.T) {
	t.Run("unlock successfully", func(t *testing.T) {
		locker, _ := setupRedisLocker(t)
		ctx := context.Background()

		// 先获取锁
		handle, err := locker.TryLock(ctx, "job-1", 30*time.Second)
		require.NoError(t, err)
		require.NotNil(t, handle)

		// 释放锁
		err = handle.Unlock(ctx)
		require.NoError(t, err)

		// 可以重新获取
		handle2, err := locker.TryLock(ctx, "job-1", 30*time.Second)
		require.NoError(t, err)
		assert.NotNil(t, handle2)
	})

	t.Run("double unlock returns error", func(t *testing.T) {
		locker, _ := setupRedisLocker(t)
		ctx := context.Background()

		// 获取锁
		handle, err := locker.TryLock(ctx, "job-1", 30*time.Second)
		require.NoError(t, err)
		require.NotNil(t, handle)

		// 第一次释放成功
		err = handle.Unlock(ctx)
		require.NoError(t, err)

		// 第二次释放失败
		err = handle.Unlock(ctx)
		assert.ErrorIs(t, err, ErrLockNotHeld)
	})

	t.Run("different handles are independent", func(t *testing.T) {
		locker1, mr := setupRedisLocker(t)

		// 创建另一个实例的 locker（不同 identity）
		client := redis.NewClient(&redis.Options{
			Addr: mr.Addr(),
		})
		defer client.Close()
		locker2 := NewRedisLocker(client, WithRedisIdentity("another-instance"))

		ctx := context.Background()

		// locker1 获取锁
		handle1, err := locker1.TryLock(ctx, "job-1", 30*time.Second)
		require.NoError(t, err)
		require.NotNil(t, handle1)

		// locker2 无法获取锁
		handle2, err := locker2.TryLock(ctx, "job-1", 30*time.Second)
		require.NoError(t, err)
		assert.Nil(t, handle2)

		// locker1 仍然持有锁（无法重新获取）
		handle3, err := locker1.TryLock(ctx, "job-1", 30*time.Second)
		require.NoError(t, err)
		assert.Nil(t, handle3)
	})
}

func TestRedisLockHandle_Renew(t *testing.T) {
	t.Run("renew successfully", func(t *testing.T) {
		locker, mr := setupRedisLocker(t)
		ctx := context.Background()

		// 获取锁
		handle, err := locker.TryLock(ctx, "job-1", 100*time.Millisecond)
		require.NoError(t, err)
		require.NotNil(t, handle)

		// 续期
		err = handle.Renew(ctx, 5*time.Second)
		require.NoError(t, err)

		// 快进时间（超过原 TTL 但不超过续期后的 TTL）
		mr.FastForward(200 * time.Millisecond)

		// 锁应该仍然被持有（续期生效）
		handle2, err := locker.TryLock(ctx, "job-1", 30*time.Second)
		require.NoError(t, err)
		assert.Nil(t, handle2)
	})

	t.Run("renew after unlock returns error", func(t *testing.T) {
		locker, _ := setupRedisLocker(t)
		ctx := context.Background()

		// 获取锁
		handle, err := locker.TryLock(ctx, "job-1", 30*time.Second)
		require.NoError(t, err)
		require.NotNil(t, handle)

		// 释放锁
		err = handle.Unlock(ctx)
		require.NoError(t, err)

		// 尝试续期已释放的锁
		err = handle.Renew(ctx, 60*time.Second)
		assert.ErrorIs(t, err, ErrLockNotHeld)
	})

	t.Run("renew after expiry returns error", func(t *testing.T) {
		locker, mr := setupRedisLocker(t)
		ctx := context.Background()

		// 获取锁
		handle, err := locker.TryLock(ctx, "job-1", 100*time.Millisecond)
		require.NoError(t, err)
		require.NotNil(t, handle)

		// 快进时间让锁过期
		mr.FastForward(200 * time.Millisecond)

		// 尝试续期已过期的锁
		err = handle.Renew(ctx, 60*time.Second)
		assert.ErrorIs(t, err, ErrLockNotHeld)
	})
}

func TestRedisLocker_WithPrefix(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	ctx := context.Background()

	// 创建两个不同前缀的 locker
	locker1 := NewRedisLocker(client,
		WithRedisKeyPrefix("app1:lock:"),
		WithRedisIdentity("instance-1"))
	locker2 := NewRedisLocker(client,
		WithRedisKeyPrefix("app2:lock:"),
		WithRedisIdentity("instance-2"))

	// 两个 locker 可以独立获取同名锁（因为前缀不同）
	handle1, err := locker1.TryLock(ctx, "job-1", 30*time.Second)
	require.NoError(t, err)
	assert.NotNil(t, handle1)

	handle2, err := locker2.TryLock(ctx, "job-1", 30*time.Second)
	require.NoError(t, err)
	assert.NotNil(t, handle2)
}
