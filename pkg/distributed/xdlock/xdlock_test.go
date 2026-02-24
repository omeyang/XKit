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
// 错误定义测试
// =============================================================================

func TestErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"ErrLockHeld", xdlock.ErrLockHeld, "xdlock: lock is held by another owner"},
		{"ErrLockFailed", xdlock.ErrLockFailed, "xdlock: failed to acquire lock"},
		{"ErrLockExpired", xdlock.ErrLockExpired, "xdlock: lock expired or stolen"},
		{"ErrExtendFailed", xdlock.ErrExtendFailed, "xdlock: failed to extend lock"},
		{"ErrNilClient", xdlock.ErrNilClient, "xdlock: client is nil"},
		{"ErrSessionExpired", xdlock.ErrSessionExpired, "xdlock: session expired"},
		{"ErrFactoryClosed", xdlock.ErrFactoryClosed, "xdlock: factory is closed"},
		{"ErrNotLocked", xdlock.ErrNotLocked, "xdlock: not locked"},
		{"ErrEmptyKey", xdlock.ErrEmptyKey, "xdlock: key must not be empty"},
		{"ErrKeyTooLong", xdlock.ErrKeyTooLong, "xdlock: key exceeds maximum length of 512 bytes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Error())
		})
	}
}

// =============================================================================
// 工厂测试（无需容器）
// =============================================================================

func TestNewEtcdFactory_NilClient(t *testing.T) {
	_, err := xdlock.NewEtcdFactory(nil)
	assert.ErrorIs(t, err, xdlock.ErrNilClient)
}

// =============================================================================
// 选项测试
// =============================================================================

func TestWithKeyPrefix(t *testing.T) {
	// 选项函数可以正常创建（不会 panic）
	opt := xdlock.WithKeyPrefix("myapp:")
	assert.NotNil(t, opt)
}

func TestWithEtcdTTL(t *testing.T) {
	opt := xdlock.WithEtcdTTL(30)
	assert.NotNil(t, opt)
}

func TestWithExpiry(t *testing.T) {
	opt := xdlock.WithExpiry(10)
	assert.NotNil(t, opt)
}

func TestWithTries(t *testing.T) {
	opt := xdlock.WithTries(5)
	assert.NotNil(t, opt)
}

func TestWithRetryDelay(t *testing.T) {
	opt := xdlock.WithRetryDelay(100)
	assert.NotNil(t, opt)
}

func TestWithDriftFactor(t *testing.T) {
	opt := xdlock.WithDriftFactor(0.02)
	assert.NotNil(t, opt)
}

func TestWithTimeoutFactor(t *testing.T) {
	opt := xdlock.WithTimeoutFactor(0.1)
	assert.NotNil(t, opt)
}

func TestWithFailFast(t *testing.T) {
	opt := xdlock.WithFailFast(true)
	assert.NotNil(t, opt)
}

func TestWithShufflePools(t *testing.T) {
	opt := xdlock.WithShufflePools(true)
	assert.NotNil(t, opt)
}

func TestWithSetNXOnExtend(t *testing.T) {
	opt := xdlock.WithSetNXOnExtend(true)
	assert.NotNil(t, opt)
}

// =============================================================================
// 接口编译检查
// =============================================================================

// 确保接口定义正确（编译时检查）
var (
	_ xdlock.LockHandle   = (*mockLockHandle)(nil)
	_ xdlock.Factory      = (*mockFactory)(nil)
	_ xdlock.EtcdFactory  = (*mockEtcdFactory)(nil)
	_ xdlock.RedisFactory = (*mockRedisFactory)(nil)
)

// mockLockHandle 用于编译时接口检查。
type mockLockHandle struct{}

func (m *mockLockHandle) Unlock(_ context.Context) error { return nil }
func (m *mockLockHandle) Extend(_ context.Context) error { return nil }
func (m *mockLockHandle) Key() string                    { return "" }

// mockFactory 用于编译时接口检查。
type mockFactory struct{}

func (m *mockFactory) TryLock(_ context.Context, _ string, _ ...xdlock.MutexOption) (xdlock.LockHandle, error) {
	return nil, nil
}
func (m *mockFactory) Lock(_ context.Context, _ string, _ ...xdlock.MutexOption) (xdlock.LockHandle, error) {
	return nil, nil
}
func (m *mockFactory) Close() error                   { return nil }
func (m *mockFactory) Health(_ context.Context) error { return nil }

// mockEtcdFactory 用于编译时接口检查。
type mockEtcdFactory struct{ mockFactory }

func (m *mockEtcdFactory) Session() xdlock.Session { return nil }

// mockRedisFactory 用于编译时接口检查。
type mockRedisFactory struct{ mockFactory }

func (m *mockRedisFactory) Redsync() xdlock.Redsync { return nil }

// =============================================================================
// Redis 工厂测试（使用 miniredis，无需容器）
// =============================================================================

func TestNewRedisFactory_NilClient(t *testing.T) {
	_, err := xdlock.NewRedisFactory()
	assert.ErrorIs(t, err, xdlock.ErrNilClient)
}

func TestNewRedisFactory_NilClientInList(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	_, err := xdlock.NewRedisFactory(client, nil)
	assert.ErrorIs(t, err, xdlock.ErrNilClient)
}

func TestNewRedisFactory_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	assert.NotNil(t, factory.Redsync())
}

func TestRedisFactory_Health_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = factory.Health(ctx)
	assert.NoError(t, err)
}

// =============================================================================
// Redis 选项测试
// =============================================================================

func TestWithRetryDelayFunc(t *testing.T) {
	opt := xdlock.WithRetryDelayFunc(func(tries int) time.Duration {
		return time.Duration(tries) * 100 * time.Millisecond
	})
	assert.NotNil(t, opt)
}

func TestWithGenValueFunc(t *testing.T) {
	opt := xdlock.WithGenValueFunc(func() (string, error) {
		return "custom-value", nil
	})
	assert.NotNil(t, opt)
}

// =============================================================================
// Redis 错误处理测试
// =============================================================================

func TestRedisFactory_HealthAfterClose_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)

	// 关闭工厂
	err = factory.Close()
	require.NoError(t, err)

	ctx := context.Background()

	// 关闭后健康检查
	err = factory.Health(ctx)
	assert.ErrorIs(t, err, xdlock.ErrFactoryClosed)
}

func TestRedisFactory_CloseIdempotent_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)

	// 多次关闭不应报错
	assert.NoError(t, factory.Close())
	assert.NoError(t, factory.Close())
	assert.NoError(t, factory.Close())
}

// =============================================================================
// etcd 选项测试（补充）
// =============================================================================

func TestWithEtcdContext(t *testing.T) {
	ctx := context.Background()
	opt := xdlock.WithEtcdContext(ctx)
	assert.NotNil(t, opt)
}

// =============================================================================
// 多客户端工厂测试（Redlock 场景）
// =============================================================================

func TestNewRedisFactory_MultipleClients_WithMiniredis(t *testing.T) {
	// 创建多个 miniredis 实例模拟 Redlock
	mr1 := miniredis.RunT(t)
	mr2 := miniredis.RunT(t)
	mr3 := miniredis.RunT(t)

	client1 := redis.NewClient(&redis.Options{Addr: mr1.Addr()})
	client2 := redis.NewClient(&redis.Options{Addr: mr2.Addr()})
	client3 := redis.NewClient(&redis.Options{Addr: mr3.Addr()})
	defer func() { _ = client1.Close() }()
	defer func() { _ = client2.Close() }()
	defer func() { _ = client3.Close() }()

	factory, err := xdlock.NewRedisFactory(client1, client2, client3)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 健康检查应检查所有节点
	err = factory.Health(ctx)
	assert.NoError(t, err)

	// 使用新 API 测试锁操作
	handle, err := factory.TryLock(ctx, "test-redlock")
	require.NoError(t, err)
	require.NotNil(t, handle)
	defer func() { _ = handle.Unlock(ctx) }()
}

// =============================================================================
// 健康检查失败测试
// =============================================================================

func TestRedisFactory_HealthFailed_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 正常情况下健康检查成功
	err = factory.Health(ctx)
	assert.NoError(t, err)

	// 关闭 miniredis 模拟连接失败
	mr.Close()

	// 健康检查应该失败
	err = factory.Health(ctx)
	assert.Error(t, err)
}

// =============================================================================
// Context 取消测试
// =============================================================================

func TestRedisFactory_LockContextCanceled_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

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

	// 第二个 Lock 尝试获取锁，context 立即取消
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2() // 立即取消

	handle2, err := factory.Lock(ctx2, "test-ctx-cancel", xdlock.WithTries(1))
	assert.True(t, errors.Is(err, context.Canceled) || errors.Is(err, xdlock.ErrLockHeld))
	assert.Nil(t, handle2)
}

// =============================================================================
// Redsync 接口测试
// =============================================================================

func TestRedisFactory_Redsync_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	redsync := factory.Redsync()
	assert.NotNil(t, redsync)
}

// =============================================================================
// Handle-based API 测试（推荐新 API）
// =============================================================================

func TestRedisFactory_TryLock_Success_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// TryLock 应该成功并返回 handle
	handle, err := factory.TryLock(ctx, "test-handle-trylock")
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 验证 Key
	assert.Contains(t, handle.Key(), "test-handle-trylock")

	// Unlock 应该成功
	err = handle.Unlock(ctx)
	assert.NoError(t, err)
}

func TestRedisFactory_TryLock_LockHeld_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 第一个 TryLock 成功
	handle1, err := factory.TryLock(ctx, "test-handle-held")
	require.NoError(t, err)
	require.NotNil(t, handle1)
	defer func() { _ = handle1.Unlock(ctx) }()

	// 第二个 TryLock 应该返回 (nil, nil) 表示锁被占用
	handle2, err := factory.TryLock(ctx, "test-handle-held")
	assert.NoError(t, err)
	assert.Nil(t, handle2)
}

func TestRedisFactory_Lock_Success_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Lock 应该成功并返回 handle
	handle, err := factory.Lock(ctx, "test-handle-lock", xdlock.WithTries(1))
	require.NoError(t, err)
	require.NotNil(t, handle)

	// Unlock 应该成功
	err = handle.Unlock(ctx)
	assert.NoError(t, err)
}

func TestRedisLockHandle_Extend_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	handle, err := factory.TryLock(ctx, "test-handle-extend", xdlock.WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle)
	defer func() { _ = handle.Unlock(ctx) }()

	// Extend 应该成功
	err = handle.Extend(ctx)
	assert.NoError(t, err)
}

func TestRedisLockHandle_UnlockNotHeld_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	handle, err := factory.TryLock(ctx, "test-handle-unlock-expired", xdlock.WithExpiry(100*time.Millisecond))
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 让锁过期
	mr.FastForward(200 * time.Millisecond)

	// Unlock 应该返回 ErrNotLocked
	err = handle.Unlock(ctx)
	assert.ErrorIs(t, err, xdlock.ErrNotLocked)
}

func TestRedisFactory_TryLockAfterClose_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)

	// 关闭工厂
	err = factory.Close()
	require.NoError(t, err)

	ctx := context.Background()

	// 关闭后 TryLock 应该返回 ErrFactoryClosed
	handle, err := factory.TryLock(ctx, "test-after-close")
	assert.ErrorIs(t, err, xdlock.ErrFactoryClosed)
	assert.Nil(t, handle)
}

func TestRedisFactory_LockAfterClose_Handle_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)

	// 关闭工厂
	err = factory.Close()
	require.NoError(t, err)

	ctx := context.Background()

	// 关闭后 Lock 应该返回 ErrFactoryClosed
	handle, err := factory.Lock(ctx, "test-after-close", xdlock.WithTries(1))
	assert.ErrorIs(t, err, xdlock.ErrFactoryClosed)
	assert.Nil(t, handle)
}

// =============================================================================
// Redis Extend 覆盖测试
// =============================================================================

func TestRedisLockHandle_ExtendAfterExpiry_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx := context.Background()

	handle, err := factory.TryLock(ctx, "test-extend-expired", xdlock.WithExpiry(100*time.Millisecond))
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 让锁过期
	mr.FastForward(200 * time.Millisecond)

	// Extend 过期的锁应返回错误（具体错误取决于 redsync 行为）
	err = handle.Extend(ctx)
	assert.Error(t, err)
}

func TestRedisLockHandle_ExtendAfterClose_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)

	ctx := context.Background()

	handle, err := factory.TryLock(ctx, "test-extend-closed", xdlock.WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 关闭工厂后 Extend 仍可操作（Redis 连接由调用者管理）
	_ = factory.Close()

	err = handle.Extend(ctx)
	assert.NoError(t, err)

	// 清理
	_ = handle.Unlock(ctx)
}

func TestRedisLockHandle_UnlockAfterClose_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)

	ctx := context.Background()

	handle, err := factory.TryLock(ctx, "test-unlock-closed", xdlock.WithExpiry(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, handle)

	// 关闭工厂后 Unlock 仍可操作，避免锁悬挂
	_ = factory.Close()

	err = handle.Unlock(ctx)
	assert.NoError(t, err)
}

// =============================================================================
// Redis createMutex 选项覆盖测试
// =============================================================================

func TestRedisFactory_CreateMutexWithAllOptions_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx := context.Background()

	// 使用所有可选选项来覆盖 createMutex 分支
	handle, err := factory.TryLock(ctx, "test-all-opts",
		xdlock.WithKeyPrefix("prefix:"),
		xdlock.WithExpiry(5*time.Second),
		xdlock.WithTries(1),
		xdlock.WithRetryDelay(100*time.Millisecond),
		xdlock.WithRetryDelayFunc(func(tries int) time.Duration {
			return time.Duration(tries) * 50 * time.Millisecond
		}),
		xdlock.WithDriftFactor(0.02),
		xdlock.WithTimeoutFactor(0.1),
		xdlock.WithGenValueFunc(func() (string, error) {
			return "custom-value", nil
		}),
		xdlock.WithFailFast(true),
		xdlock.WithShufflePools(true),
		xdlock.WithSetNXOnExtend(true),
	)
	require.NoError(t, err)
	require.NotNil(t, handle)
	defer func() { _ = handle.Unlock(ctx) }()

	// 验证前缀
	assert.Contains(t, handle.Key(), "prefix:")
	assert.Contains(t, handle.Key(), "test-all-opts")
}

// =============================================================================
// Redis Lock context 传播测试
// =============================================================================

func TestRedisFactory_Lock_ContextPropagation_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

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
	defer func() { _ = factory.Close() }()

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
	defer func() { _ = factory.Close() }()

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
	defer func() { _ = factory.Close() }()

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
	defer func() { _ = factory.Close() }()

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
	defer func() { _ = factory.Close() }()

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
	defer func() { _ = factory.Close() }()

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
	defer func() { _ = factory.Close() }()

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
	defer func() { _ = factory.Close() }()

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
	defer func() { _ = factory.Close() }()

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
	defer func() { _ = factory.Close() }()

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
	defer func() { _ = factory.Close() }()

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

func TestRedisLockHandle_Unlock_TimedOutContext_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

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
