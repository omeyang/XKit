package xdlock_test

import (
	"context"
	"errors"
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
		{"ErrExtendNotSupported", xdlock.ErrExtendNotSupported, "xdlock: extend not supported by backend"},
		{"ErrNilClient", xdlock.ErrNilClient, "xdlock: client is nil"},
		{"ErrSessionExpired", xdlock.ErrSessionExpired, "xdlock: session expired"},
		{"ErrFactoryClosed", xdlock.ErrFactoryClosed, "xdlock: factory is closed"},
		{"ErrNotLocked", xdlock.ErrNotLocked, "xdlock: not locked"},
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
	_ xdlock.Locker      = (*mockLocker)(nil)
	_ xdlock.Factory     = (*mockFactory)(nil)
	_ xdlock.EtcdFactory = (*mockEtcdFactory)(nil)
)

// mockLocker 用于编译时接口检查。
type mockLocker struct{}

func (m *mockLocker) Lock(_ context.Context) error    { return nil }
func (m *mockLocker) TryLock(_ context.Context) error { return nil }
func (m *mockLocker) Unlock(_ context.Context) error  { return nil }
func (m *mockLocker) Extend(_ context.Context) error  { return nil }

// mockFactory 用于编译时接口检查。
type mockFactory struct{}

func (m *mockFactory) NewMutex(_ string, _ ...xdlock.MutexOption) xdlock.Locker { return nil }
func (m *mockFactory) Close() error                                             { return nil }
func (m *mockFactory) Health(_ context.Context) error                           { return nil }

// mockEtcdFactory 用于编译时接口检查。
type mockEtcdFactory struct {
	mockFactory
}

func (m *mockEtcdFactory) Session() xdlock.Session { return nil }

// mockRedisFactory 用于编译时接口检查。
type mockRedisFactory struct {
	mockFactory
}

func (m *mockRedisFactory) Redsync() xdlock.Redsync { return nil }

// mockRedisLocker 用于编译时接口检查。
type mockRedisLocker struct {
	mockLocker
}

func (m *mockRedisLocker) RedisMutex() xdlock.RedisMutex { return nil }
func (m *mockRedisLocker) Value() string                 { return "" }
func (m *mockRedisLocker) Until() int64                  { return 0 }

// 确保 Redis 接口定义正确（编译时检查）
var (
	_ xdlock.RedisFactory = (*mockRedisFactory)(nil)
	_ xdlock.RedisLocker  = (*mockRedisLocker)(nil)
)

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

func TestRedisLocker_LockUnlock_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	locker := factory.NewMutex("test-lock", xdlock.WithTries(1))

	// 获取锁
	err = locker.Lock(ctx)
	require.NoError(t, err)

	// 释放锁
	err = locker.Unlock(ctx)
	assert.NoError(t, err)
}

func TestRedisLocker_NotLockedErrors_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Run("Unlock without lock", func(t *testing.T) {
		locker := factory.NewMutex("test-unlock-not-locked")
		err := locker.Unlock(ctx)
		assert.ErrorIs(t, err, xdlock.ErrNotLocked)
	})

	t.Run("Extend without lock", func(t *testing.T) {
		locker := factory.NewMutex("test-extend-not-locked")
		err := locker.Extend(ctx)
		assert.ErrorIs(t, err, xdlock.ErrNotLocked)
	})
}

func TestRedisLocker_Value_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	locker := factory.NewMutex("test-value", xdlock.WithTries(1))

	err = locker.Lock(ctx)
	require.NoError(t, err)
	defer func() { _ = locker.Unlock(ctx) }()

	redisLocker, ok := locker.(xdlock.RedisLocker)
	require.True(t, ok)
	assert.NotEmpty(t, redisLocker.Value())
	assert.NotZero(t, redisLocker.Until())
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

func TestRedisFactory_LockAfterClose_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)

	locker := factory.NewMutex("test-lock-after-close")

	// 关闭工厂
	err = factory.Close()
	require.NoError(t, err)

	ctx := context.Background()

	// 关闭后尝试获取锁
	err = locker.Lock(ctx)
	assert.ErrorIs(t, err, xdlock.ErrFactoryClosed)

	// 关闭后尝试 TryLock
	err = locker.TryLock(ctx)
	assert.ErrorIs(t, err, xdlock.ErrFactoryClosed)
}

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

func TestRedisLocker_TryLockFailed_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 第一个 locker 获取锁
	locker1 := factory.NewMutex("test-trylock-fail", xdlock.WithTries(1))
	err = locker1.Lock(ctx)
	require.NoError(t, err)
	defer func() { _ = locker1.Unlock(ctx) }()

	// 第二个 locker TryLock 应该失败（锁被其他持有者占用）
	locker2 := factory.NewMutex("test-trylock-fail", xdlock.WithTries(1))
	err = locker2.TryLock(ctx)
	assert.ErrorIs(t, err, xdlock.ErrLockHeld)
}

func TestRedisLocker_LockFailed_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 第一个 locker 获取锁
	locker1 := factory.NewMutex("test-lock-fail", xdlock.WithTries(1))
	err = locker1.Lock(ctx)
	require.NoError(t, err)
	defer func() { _ = locker1.Unlock(ctx) }()

	// 第二个 locker Lock 应该失败（锁被其他持有者占用，tries=1 不重试）
	locker2 := factory.NewMutex("test-lock-fail", xdlock.WithTries(1))
	err = locker2.Lock(ctx)
	assert.ErrorIs(t, err, xdlock.ErrLockHeld)
}

func TestRedisLocker_Extend_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	locker := factory.NewMutex("test-extend", xdlock.WithExpiry(5*time.Second))
	err = locker.Lock(ctx)
	require.NoError(t, err)
	defer func() { _ = locker.Unlock(ctx) }()

	// 续期应该成功
	err = locker.Extend(ctx)
	assert.NoError(t, err)
}

func TestRedisLocker_ExtendAfterClose_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	locker := factory.NewMutex("test-extend-after-close", xdlock.WithExpiry(30*time.Second))
	err = locker.Lock(ctx)
	require.NoError(t, err)

	// 关闭工厂
	_ = factory.Close()

	// 关闭后续期应该返回 ErrFactoryClosed
	err = locker.Extend(ctx)
	assert.ErrorIs(t, err, xdlock.ErrFactoryClosed)
}

func TestRedisLocker_UnlockAfterClose_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	locker := factory.NewMutex("test-unlock-after-close", xdlock.WithExpiry(30*time.Second))
	err = locker.Lock(ctx)
	require.NoError(t, err)

	// 关闭工厂
	_ = factory.Close()

	// 关闭后释放锁应该返回 ErrFactoryClosed
	err = locker.Unlock(ctx)
	assert.ErrorIs(t, err, xdlock.ErrFactoryClosed)
}

// =============================================================================
// Redis 选项应用测试
// =============================================================================

func TestRedisLocker_AllOptions_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 使用所有选项创建锁
	locker := factory.NewMutex("test-all-options",
		xdlock.WithKeyPrefix("myapp:"),
		xdlock.WithExpiry(10*time.Second),
		xdlock.WithTries(3),
		xdlock.WithRetryDelay(100*time.Millisecond),
		xdlock.WithRetryDelayFunc(func(tries int) time.Duration {
			return time.Duration(tries) * 50 * time.Millisecond
		}),
		xdlock.WithDriftFactor(0.01),
		xdlock.WithTimeoutFactor(0.05),
		xdlock.WithGenValueFunc(func() (string, error) {
			return "custom-value-12345", nil
		}),
		xdlock.WithFailFast(true),
		xdlock.WithShufflePools(false),
		xdlock.WithSetNXOnExtend(true),
	)

	err = locker.Lock(ctx)
	require.NoError(t, err)
	defer func() { _ = locker.Unlock(ctx) }()

	redisLocker, ok := locker.(xdlock.RedisLocker)
	require.True(t, ok)
	assert.Equal(t, "custom-value-12345", redisLocker.Value())
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
	defer client1.Close()
	defer client2.Close()
	defer client3.Close()

	factory, err := xdlock.NewRedisFactory(client1, client2, client3)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 健康检查应检查所有节点
	err = factory.Health(ctx)
	assert.NoError(t, err)

	// 锁操作应该成功
	locker := factory.NewMutex("test-redlock")
	err = locker.Lock(ctx)
	require.NoError(t, err)
	defer func() { _ = locker.Unlock(ctx) }()
}

// =============================================================================
// 锁状态边界测试
// =============================================================================

func TestRedisLocker_UnlockNotLocked_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 创建锁但不获取
	locker := factory.NewMutex("test-unlock-not-locked")

	// Unlock 应该返回 ErrNotLocked
	err = locker.Unlock(ctx)
	assert.ErrorIs(t, err, xdlock.ErrNotLocked)
}

func TestRedisLocker_ExtendNotLocked_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 创建锁但不获取
	locker := factory.NewMutex("test-extend-not-locked")

	// Extend 应该返回 ErrNotLocked
	err = locker.Extend(ctx)
	assert.ErrorIs(t, err, xdlock.ErrNotLocked)
}

func TestRedisLocker_UnlockExpired_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 使用短过期时间获取锁
	locker := factory.NewMutex("test-unlock-expired", xdlock.WithExpiry(100*time.Millisecond))
	err = locker.Lock(ctx)
	require.NoError(t, err)

	// 让锁过期
	mr.FastForward(200 * time.Millisecond)

	// Unlock 应该返回 ErrLockExpired
	err = locker.Unlock(ctx)
	assert.ErrorIs(t, err, xdlock.ErrLockExpired)
}

func TestRedisLocker_ExtendExpired_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 使用短过期时间获取锁
	locker := factory.NewMutex("test-extend-expired", xdlock.WithExpiry(100*time.Millisecond))
	err = locker.Lock(ctx)
	require.NoError(t, err)

	// 让锁过期
	mr.FastForward(200 * time.Millisecond)

	// Extend 应该返回错误（续期失败）
	err = locker.Extend(ctx)
	assert.Error(t, err)
}

func TestRedisLocker_LockAfterClose_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)

	locker := factory.NewMutex("test-lock-after-close")

	// 关闭工厂
	err = factory.Close()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Lock 应该返回 ErrFactoryClosed
	err = locker.Lock(ctx)
	assert.ErrorIs(t, err, xdlock.ErrFactoryClosed)
}

func TestRedisLocker_TryLockAfterClose_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)

	locker := factory.NewMutex("test-trylock-after-close")

	// 关闭工厂
	err = factory.Close()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// TryLock 应该返回 ErrFactoryClosed
	err = locker.TryLock(ctx)
	assert.ErrorIs(t, err, xdlock.ErrFactoryClosed)
}

func TestRedisLocker_DoubleUnlock_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	locker := factory.NewMutex("test-double-unlock")
	err = locker.Lock(ctx)
	require.NoError(t, err)

	// 第一次 Unlock 应该成功
	err = locker.Unlock(ctx)
	assert.NoError(t, err)

	// 第二次 Unlock 应该返回 ErrNotLocked
	err = locker.Unlock(ctx)
	assert.ErrorIs(t, err, xdlock.ErrNotLocked)
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

func TestRedisLocker_LockContextCanceled_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	// 第一个 locker 持有锁
	ctx1, cancel1 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel1()
	locker1 := factory.NewMutex("test-ctx-cancel", xdlock.WithTries(1))
	err = locker1.Lock(ctx1)
	require.NoError(t, err)
	defer func() { _ = locker1.Unlock(ctx1) }()

	// 第二个 locker 尝试获取锁，context 立即取消
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2() // 立即取消

	locker2 := factory.NewMutex("test-ctx-cancel", xdlock.WithTries(1))
	err = locker2.Lock(ctx2)
	assert.True(t, errors.Is(err, context.Canceled) || errors.Is(err, xdlock.ErrLockHeld))
}


// =============================================================================
// Value 和 Until 测试
// =============================================================================

func TestRedisLocker_ValueAndUntil_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	factory, err := xdlock.NewRedisFactory(client)
	require.NoError(t, err)
	defer func() { _ = factory.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	locker := factory.NewMutex("test-value-until", xdlock.WithExpiry(10*time.Second))
	err = locker.Lock(ctx)
	require.NoError(t, err)
	defer func() { _ = locker.Unlock(ctx) }()

	redisLocker, ok := locker.(xdlock.RedisLocker)
	require.True(t, ok)

	// Value 应该非空
	value := redisLocker.Value()
	assert.NotEmpty(t, value)

	// Until 应该在未来
	until := redisLocker.Until()
	assert.Greater(t, until, time.Now().UnixMilli())

	// RedisMutex 应该非空
	assert.NotNil(t, redisLocker.RedisMutex())
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
