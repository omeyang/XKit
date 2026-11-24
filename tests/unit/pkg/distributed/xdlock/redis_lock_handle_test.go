package xdlock_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omeyang/xkit/pkg/distributed/xdlock"
)

// =============================================================================
// Redis Lock context 传播测试
// =============================================================================

func TestRedisFactory_Lock_ContextPropagation_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	// 第一个 handle 持有锁
	ctx1 := context.Background()
	handle1, err := factory.TryLock(ctx1, "test-lock-ctx")
	require.NoError(t, err)
	require.NotNil(t, handle1)
	defer func() { _ = handle1.Unlock(ctx1) }()

	// Lock 使用已取消的 context 应传播 context 错误
	ctx2, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel2()
	handle2, err := factory.Lock(ctx2, "test-lock-ctx",
		xdlock.WithTries(100),
		xdlock.WithRetryDelay(50*time.Millisecond),
	)
	assert.Error(t, err)
	assert.Nil(t, handle2)
}

// =============================================================================
// Key 验证测试
// =============================================================================

func TestRedisFactory_TryLock_EmptyKey_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	tests := []struct {
		name string
		key  string
	}{
		{"empty", ""},
		{"space", " "},
		{"tabs", "\t\t"},
		{"whitespace", "  \t\n  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handle, err := factory.TryLock(ctx, tt.key)
			assert.ErrorIs(t, err, xdlock.ErrEmptyKey)
			assert.Nil(t, handle)
		})
	}
}

func TestRedisFactory_Lock_EmptyKey_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	handle, err := factory.Lock(ctx, "", xdlock.WithTries(1))
	assert.ErrorIs(t, err, xdlock.ErrEmptyKey)
	assert.Nil(t, handle)
}

// =============================================================================
// Redis Lock 重试耗尽测试（覆盖 Lock 非 context 错误路径）
// =============================================================================

func TestRedisFactory_Lock_RetriesExhausted_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	// 第一个 handle 持有锁
	handle1, err := factory.TryLock(ctx, "test-retries-exhausted")
	require.NoError(t, err)
	require.NotNil(t, handle1)
	defer func() { _ = handle1.Unlock(ctx) }()

	// 第二个 Lock 使用 Tries=1 使其立即失败（不是 context 取消）
	handle2, err := factory.Lock(ctx, "test-retries-exhausted", xdlock.WithTries(1))
	assert.Error(t, err)
	assert.Nil(t, handle2)
	// 应该是锁获取失败，不是 context 错误
	assert.False(t, errors.Is(err, context.Canceled))
	assert.False(t, errors.Is(err, context.DeadlineExceeded))
}

// =============================================================================
// Redis Unlock 错误路径测试
// =============================================================================

func TestRedisLockHandle_Unlock_ServerError_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	handle, err := factory.TryLock(ctx, "test-unlock-server-error", xdlock.WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 关闭 miniredis 模拟 Redis 不可用
	mr.Close()

	// Unlock 应返回非 ErrLockExpired 的错误（覆盖 wrappedErr 返回路径）
	err = handle.Unlock(ctx)
	assert.Error(t, err)
}

// =============================================================================
// Redis Extend 错误路径测试
// =============================================================================

func TestRedisLockHandle_Extend_ServerError_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	handle, err := factory.TryLock(ctx, "test-extend-server-error", xdlock.WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 关闭 miniredis 模拟 Redis 不可用
	mr.Close()

	// Extend 应返回错误（覆盖非 ErrLockExpired 的错误路径）
	err = handle.Extend(ctx)
	assert.Error(t, err)
}

// =============================================================================
// Redis Extend ErrExtendFailed 保留测试
// =============================================================================

func TestRedisLockHandle_Unlock_StolenLock_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	// handle1 获取锁
	handle1, err := factory.TryLock(ctx, "test-stolen", xdlock.WithExpiry(100*time.Millisecond))
	require.NoError(t, err)
	require.NotNil(t, handle1)

	// 让锁过期
	mr.FastForward(200 * time.Millisecond)

	// handle2 获取同一个锁（新的 value）
	handle2, err := factory.TryLock(ctx, "test-stolen", xdlock.WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle2)
	defer func() { _ = handle2.Unlock(ctx) }()

	// handle1 尝试 Unlock — 锁已被 handle2 持有，值不匹配
	// redsync 返回 ErrTaken → handle 层统一转为 ErrNotLocked（所有权已丢失）
	err = handle1.Unlock(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, xdlock.ErrNotLocked)
}

func TestRedisLockHandle_Extend_StolenLock_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	// handle1 获取锁
	handle1, err := factory.TryLock(ctx, "test-extend-stolen", xdlock.WithExpiry(100*time.Millisecond))
	require.NoError(t, err)
	require.NotNil(t, handle1)

	// 让锁过期
	mr.FastForward(200 * time.Millisecond)

	// handle2 获取同一个锁
	handle2, err := factory.TryLock(ctx, "test-extend-stolen", xdlock.WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle2)
	defer func() { _ = handle2.Unlock(ctx) }()

	// handle1 尝试 Extend — 锁已被 handle2 持有
	// redsync 返回 ErrTaken → handle 层统一转为 ErrNotLocked（所有权已丢失）
	err = handle1.Extend(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, xdlock.ErrNotLocked)
}

func TestRedisLockHandle_Extend_PreservesErrExtendFailed_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	handle, err := factory.TryLock(ctx, "test-extend-failed", xdlock.WithExpiry(100*time.Millisecond))
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 让锁过期
	mr.FastForward(200 * time.Millisecond)

	// Extend 过期锁应返回 ErrNotLocked（ErrLockExpired 转换）
	// 或 ErrExtendFailed（如果 redsync 返回 extend failed）
	err = handle.Extend(ctx)
	assert.Error(t, err)
	// 不应将 ErrExtendFailed 转换为 ErrNotLocked（FG-M1 修复验证）
	if errors.Is(err, xdlock.ErrExtendFailed) {
		assert.False(t, errors.Is(err, xdlock.ErrNotLocked),
			"ErrExtendFailed should not be converted to ErrNotLocked")
	}
}

// =============================================================================
// Key 长度限制测试 (FG-M9)
// =============================================================================

func TestRedisFactory_TryLock_KeyTooLong_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	tests := []struct {
		name    string
		keyLen  int
		wantErr error
	}{
		{"at limit (512)", 512, nil},
		{"over limit (513)", 513, xdlock.ErrKeyTooLong},
		{"way over limit (1024)", 1024, xdlock.ErrKeyTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := strings.Repeat("x", tt.keyLen)
			handle, err := factory.TryLock(ctx, key)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, handle)
			} else {
				require.NoError(t, err)
				require.NotNil(t, handle)
				_ = handle.Unlock(ctx)
			}
		})
	}
}

func TestRedisFactory_Lock_KeyTooLong_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	key := strings.Repeat("x", 513)
	handle, err := factory.Lock(ctx, key, xdlock.WithTries(1))
	assert.ErrorIs(t, err, xdlock.ErrKeyTooLong)
	assert.Nil(t, handle)
}

// =============================================================================
// Unlock 清理上下文测试 (FG-S1)
// =============================================================================

func TestRedisLockHandle_Unlock_CanceledContext_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	// 使用正常 ctx 获取锁
	ctx := context.Background()
	handle, err := factory.TryLock(ctx, "test-cleanup-unlock", xdlock.WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 使用已取消的 ctx 调用 Unlock — 应该仍然成功（使用清理上下文）
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	err = handle.Unlock(canceledCtx)
	assert.NoError(t, err)
}

func TestRedisLockHandle_Unlock_NilContext_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()
	handle, err := factory.TryLock(ctx, "test-nil-ctx-unlock", xdlock.WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle)
	defer func() { _ = handle.Unlock(ctx) }()

	err = handle.Unlock(nil) //nolint:staticcheck // SA1012: nil ctx 是测试目标
	assert.ErrorIs(t, err, xdlock.ErrNilContext)
}

// =============================================================================
// Redis nil context 测试（FG-S3 验证）
// =============================================================================

func TestRedisFactory_TryLock_NilContext_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	handle, err := factory.TryLock(nil, "test-nil-ctx") //nolint:staticcheck // SA1012: nil ctx 是测试目标
	assert.ErrorIs(t, err, xdlock.ErrNilContext)
	assert.Nil(t, handle)
}

func TestRedisFactory_Lock_NilContext_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	handle, err := factory.Lock(nil, "test-nil-ctx", xdlock.WithTries(1)) //nolint:staticcheck // SA1012: nil ctx 是测试目标
	assert.ErrorIs(t, err, xdlock.ErrNilContext)
	assert.Nil(t, handle)
}

func TestRedisLockHandle_Extend_NilContext_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()
	handle, err := factory.TryLock(ctx, "test-nil-ctx-extend", xdlock.WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle)
	defer func() { _ = handle.Unlock(ctx) }()

	err = handle.Extend(nil) //nolint:staticcheck // SA1012: nil ctx 是测试目标
	assert.ErrorIs(t, err, xdlock.ErrNilContext)
}

func TestRedisFactory_Health_NilContext_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	err = factory.Health(nil) //nolint:staticcheck // SA1012: nil ctx 是测试目标
	assert.ErrorIs(t, err, xdlock.ErrNilContext)
}

// =============================================================================
// Redis unlocked 标记测试（FG-M8 验证）
// =============================================================================

// TestRedisLockHandle_Unlock_StolenLock_SetsUnlocked 验证 FG-M9 修复：
// 当锁被其他持有者抢走后，Unlock 返回 ErrNotLocked 且设置 unlocked 标记，
// 后续 Extend 也立即返回 ErrNotLocked，不再发送无意义的 Redis 请求。
func TestRedisLockHandle_Unlock_StolenLock_SetsUnlocked_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	// handle1 获取锁
	handle1, err := factory.TryLock(ctx, "test-stolen-unlocked", xdlock.WithExpiry(100*time.Millisecond))
	require.NoError(t, err)
	require.NotNil(t, handle1)

	// 让锁过期
	mr.FastForward(200 * time.Millisecond)

	// handle2 获取同一个锁（新的 value）
	handle2, err := factory.TryLock(ctx, "test-stolen-unlocked", xdlock.WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle2)
	defer func() { _ = handle2.Unlock(ctx) }()

	// handle1 尝试 Unlock — 锁已被 handle2 持有
	err = handle1.Unlock(ctx)
	assert.ErrorIs(t, err, xdlock.ErrNotLocked)

	// FG-M9 修复验证：后续 Extend 应立即返回 ErrNotLocked（通过 unlocked 标记快速路径）
	err = handle1.Extend(ctx)
	assert.ErrorIs(t, err, xdlock.ErrNotLocked)
}

func TestRedisLockHandle_DoubleUnlock_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()
	handle, err := factory.TryLock(ctx, "test-double-unlock", xdlock.WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 第一次 Unlock 成功
	err = handle.Unlock(ctx)
	assert.NoError(t, err)

	// 第二次 Unlock 应返回 ErrNotLocked（unlocked 标记阻止重复操作）
	err = handle.Unlock(ctx)
	assert.ErrorIs(t, err, xdlock.ErrNotLocked)
}

func TestRedisLockHandle_ExtendAfterUnlock_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()
	handle, err := factory.TryLock(ctx, "test-extend-after-unlock", xdlock.WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle)

	// Unlock 成功
	err = handle.Unlock(ctx)
	assert.NoError(t, err)

	// Extend 已解锁的 handle 应返回 ErrNotLocked
	err = handle.Extend(ctx)
	assert.ErrorIs(t, err, xdlock.ErrNotLocked)
}

// =============================================================================
// Redis nil option 测试（FG-M7 验证）
// =============================================================================

func TestRedisFactory_TryLock_NilOption_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	// nil option 应被安全跳过，不 panic
	handle, err := factory.TryLock(ctx, "test-nil-opt", nil, xdlock.WithExpiry(5*time.Second), nil)
	require.NoError(t, err)
	require.NotNil(t, handle)
	_ = handle.Unlock(ctx)
}

func TestRedisLockHandle_Unlock_TimedOutContext_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	// 使用正常 ctx 获取锁
	ctx := context.Background()
	handle, err := factory.TryLock(ctx, "test-timeout-unlock", xdlock.WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 使用已超时的 ctx 调用 Unlock
	timedOutCtx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond) // 确保超时

	err = handle.Unlock(timedOutCtx)
	assert.NoError(t, err)
}

func TestRedisLockHandle_Extend_StolenLock_SetsUnlocked_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	handle1, err := factory.TryLock(ctx, "test-extend-stolen-unlocked", xdlock.WithExpiry(100*time.Millisecond))
	require.NoError(t, err)
	require.NotNil(t, handle1)

	mr.FastForward(200 * time.Millisecond)

	handle2, err := factory.TryLock(ctx, "test-extend-stolen-unlocked", xdlock.WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle2)
	defer func() { _ = handle2.Unlock(ctx) }()

	err = handle1.Extend(ctx)
	assert.ErrorIs(t, err, xdlock.ErrNotLocked)

	err = handle1.Extend(ctx)
	assert.ErrorIs(t, err, xdlock.ErrNotLocked)
}
