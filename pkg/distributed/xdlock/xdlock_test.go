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
	_ xdlock.LockHandle  = (*mockLockHandle)(nil)
	_ xdlock.Factory     = (*mockFactory)(nil)
	_ xdlock.EtcdFactory = (*mockEtcdFactory)(nil)
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

	// Unlock 应该返回 ErrLockNotHeld
	err = handle.Unlock(ctx)
	assert.ErrorIs(t, err, xdlock.ErrLockNotHeld)
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
