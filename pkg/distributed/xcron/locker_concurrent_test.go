package xcron

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"
)

// ============================================================================
// 并发场景测试：验证 LockHandle 模式修复了 Identity 复用问题
// ============================================================================

// TestRedisLocker_ConcurrentAcquire 测试同一进程内多个 goroutine 并发获取同一个锁
// 这是原问题的核心场景：确保只有一个 goroutine 能获取到锁
func TestRedisLocker_ConcurrentAcquire(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	// 使用同一个 locker（同一个 identity）
	locker, err := NewRedisLocker(client, WithRedisIdentity("same-instance"))
	require.NoError(t, err)
	ctx := context.Background()

	const numGoroutines = 10
	var acquired atomic.Int32
	var wg sync.WaitGroup
	var startBarrier sync.WaitGroup
	var acquiredHandle LockHandle
	var handleMu sync.Mutex

	wg.Add(numGoroutines)
	startBarrier.Add(1)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			// 等待所有 goroutine 准备就绪后同时开始
			startBarrier.Wait()

			handle, err := locker.TryLock(ctx, "shared-key", 30*time.Second)
			if err != nil {
				return
			}
			if handle != nil {
				acquired.Add(1)
				// 保存 handle 以便测试结束后释放
				handleMu.Lock()
				if acquiredHandle == nil {
					acquiredHandle = handle
				}
				handleMu.Unlock()
				// 注意：不在并发窗口内释放锁，确保测试准确性
			}
		}()
	}

	// 同时启动所有 goroutine
	startBarrier.Done()
	wg.Wait()

	// 由于所有 goroutine 几乎同时尝试获取锁，只有一个应该成功
	assert.Equal(t, int32(1), acquired.Load(),
		"Only one goroutine should acquire the lock at a time")

	// 测试结束后释放锁
	if acquiredHandle != nil {
		_ = acquiredHandle.Unlock(ctx) //nolint:errcheck // 测试代码忽略错误
	}
}

// TestRedisLocker_UnlockDoesNotAffectOther 验证一个 handle 的 Unlock 不会影响其他 handle
// 这是修复的关键验证：每个 handle 有独立的 token
func TestRedisLocker_UnlockDoesNotAffectOther(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	locker, err := NewRedisLocker(client, WithRedisIdentity("test-instance"))
	require.NoError(t, err)
	ctx := context.Background()

	// Goroutine 1 获取锁
	handle1, err := locker.TryLock(ctx, "job-1", 30*time.Second)
	require.NoError(t, err)
	require.NotNil(t, handle1, "First acquire should succeed")

	// 模拟锁过期
	mr.FastForward(35 * time.Second)

	// Goroutine 2 获取锁（锁已过期，应该成功）
	handle2, err := locker.TryLock(ctx, "job-1", 30*time.Second)
	require.NoError(t, err)
	require.NotNil(t, handle2, "Second acquire after expiry should succeed")

	// Goroutine 1 尝试 Unlock（使用旧的 handle）
	// 这不应该影响 Goroutine 2 持有的锁
	err = handle1.Unlock(ctx)
	assert.ErrorIs(t, err, ErrLockNotHeld,
		"Unlocking expired/released lock should return ErrLockNotHeld")

	// Goroutine 2 的锁应该仍然有效
	// 验证方法：尝试再次获取应该失败
	handle3, err := locker.TryLock(ctx, "job-1", 30*time.Second)
	require.NoError(t, err)
	assert.Nil(t, handle3, "Lock should still be held by handle2")

	// Goroutine 2 正常释放锁
	err = handle2.Unlock(ctx)
	require.NoError(t, err, "handle2 should be able to unlock normally")

	// 现在应该可以获取锁了
	handle4, err := locker.TryLock(ctx, "job-1", 30*time.Second)
	require.NoError(t, err)
	assert.NotNil(t, handle4, "Lock should be available after handle2 unlocked")
}

// TestRedisLocker_TokenUniqueness 验证每次 TryLock 生成唯一 token
func TestRedisLocker_TokenUniqueness(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	locker, err := NewRedisLocker(client, WithRedisIdentity("test-instance"))
	require.NoError(t, err)
	ctx := context.Background()

	const numAcquires = 100
	tokens := make(map[string]bool)

	for i := 0; i < numAcquires; i++ {
		handle, err := locker.TryLock(ctx, "key", 100*time.Millisecond)
		require.NoError(t, err)
		require.NotNil(t, handle)

		// 获取 token（通过类型断言获取内部 token）
		if rh, ok := handle.(*redisLockHandle); ok {
			if tokens[rh.token] {
				t.Fatalf("Duplicate token found: %s", rh.token)
			}
			tokens[rh.token] = true
		}

		_ = handle.Unlock(ctx) //nolint:errcheck // 测试代码忽略错误
	}

	assert.Equal(t, numAcquires, len(tokens),
		"All tokens should be unique")
}

// TestRedisLocker_RenewOnlyAffectsOwnHandle 验证 Renew 只影响自己的 handle
func TestRedisLocker_RenewOnlyAffectsOwnHandle(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	locker, err := NewRedisLocker(client, WithRedisIdentity("test-instance"))
	require.NoError(t, err)
	ctx := context.Background()

	// 获取锁
	handle1, err := locker.TryLock(ctx, "job-1", 100*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, handle1)

	// 模拟锁过期
	mr.FastForward(200 * time.Millisecond)

	// 另一个 goroutine 获取锁
	handle2, err := locker.TryLock(ctx, "job-1", 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, handle2, "Should acquire after expiry")

	// 旧 handle 尝试续期应该失败
	err = handle1.Renew(ctx, 5*time.Second)
	assert.ErrorIs(t, err, ErrLockNotHeld,
		"Renewing with old handle should fail")

	// 新 handle 续期应该成功
	err = handle2.Renew(ctx, 5*time.Second)
	require.NoError(t, err, "Renewing with current handle should succeed")
}

// TestMockLocker_ConcurrentAcquire 测试 mockLocker 的并发行为
func TestMockLocker_ConcurrentAcquire(t *testing.T) {
	locker := newMockLocker()
	ctx := context.Background()

	const numGoroutines = 20
	var acquired atomic.Int32
	var wg sync.WaitGroup
	var startBarrier sync.WaitGroup
	var acquiredHandle LockHandle
	var handleMu sync.Mutex

	wg.Add(numGoroutines)
	startBarrier.Add(1)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			// 等待所有 goroutine 准备就绪后同时开始
			startBarrier.Wait()

			handle, err := locker.TryLock(ctx, "shared-key", 30*time.Second)
			if err != nil {
				return
			}
			if handle != nil {
				acquired.Add(1)
				// 保存 handle 以便测试结束后释放
				handleMu.Lock()
				if acquiredHandle == nil {
					acquiredHandle = handle
				}
				handleMu.Unlock()
				// 注意：不在并发窗口内释放锁，确保测试准确性
			}
		}()
	}

	// 同时启动所有 goroutine
	startBarrier.Done()
	wg.Wait()

	// mockLocker 也应该保证互斥
	assert.Equal(t, int32(1), acquired.Load(),
		"MockLocker should also ensure mutual exclusion")

	// 测试结束后释放锁
	if acquiredHandle != nil {
		_ = acquiredHandle.Unlock(ctx) //nolint:errcheck // 测试代码忽略错误
	}
}

// TestK8sLocker_TokenUniqueness 验证 K8sLocker 每次 TryLock 生成唯一 token
func TestK8sLocker_TokenUniqueness(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	locker, err := NewK8sLocker(K8sLockerOptions{
		Client:    fakeClient,
		Namespace: "test-ns",
		Identity:  "test-pod",
	})
	require.NoError(t, err)

	ctx := context.Background()
	tokens := make(map[string]bool)

	const numAcquires = 10
	for i := 0; i < numAcquires; i++ {
		handle, err := locker.TryLock(ctx, "key", 100*time.Millisecond)
		require.NoError(t, err)
		require.NotNil(t, handle)

		// 获取 token
		if kh, ok := handle.(*k8sLockHandle); ok {
			if tokens[kh.token] {
				t.Fatalf("Duplicate token found: %s", kh.token)
			}
			tokens[kh.token] = true
		}

		_ = handle.Unlock(ctx) //nolint:errcheck // 测试代码忽略错误
	}

	assert.Equal(t, numAcquires, len(tokens),
		"All K8s lock tokens should be unique")
}

// TestNoopLocker_AlwaysSucceeds 验证 NoopLocker 总是成功（用于单实例场景）
func TestNoopLocker_AlwaysSucceeds(t *testing.T) {
	locker := NoopLocker()
	ctx := context.Background()

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	var handles []LockHandle
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			handle, err := locker.TryLock(ctx, "key", time.Second)
			require.NoError(t, err)
			require.NotNil(t, handle)

			mu.Lock()
			handles = append(handles, handle)
			mu.Unlock()
		}()
	}

	wg.Wait()

	// NoopLocker 应该让所有 goroutine 都成功
	assert.Equal(t, numGoroutines, len(handles),
		"NoopLocker should allow all goroutines to 'acquire' the lock")
}
