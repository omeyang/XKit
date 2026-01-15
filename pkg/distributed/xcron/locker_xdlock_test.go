package xcron

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omeyang/xkit/pkg/distributed/xdlock"
)

// mockXdlockLocker 实现 xdlock.Locker 接口用于测试
type mockXdlockLocker struct {
	locked        bool
	tryLockErr    error
	unlockErr     error
	extendErr     error
	tryLockCalled bool
	unlockCalled  bool
	extendCalled  bool
}

func (m *mockXdlockLocker) Lock(ctx context.Context) error {
	return nil
}

func (m *mockXdlockLocker) TryLock(ctx context.Context) error {
	m.tryLockCalled = true
	if m.tryLockErr != nil {
		return m.tryLockErr
	}
	m.locked = true
	return nil
}

func (m *mockXdlockLocker) Unlock(ctx context.Context) error {
	m.unlockCalled = true
	if m.unlockErr != nil {
		return m.unlockErr
	}
	m.locked = false
	return nil
}

func (m *mockXdlockLocker) Extend(ctx context.Context) error {
	m.extendCalled = true
	return m.extendErr
}

// mockXdlockFactory 实现 xdlock.Factory 接口用于测试
type mockXdlockFactory struct {
	locker    *mockXdlockLocker
	closeCalled bool
	healthErr error
}

func (f *mockXdlockFactory) NewMutex(key string, opts ...xdlock.MutexOption) xdlock.Locker {
	if f.locker == nil {
		f.locker = &mockXdlockLocker{}
	}
	return f.locker
}

func (f *mockXdlockFactory) Close() error {
	f.closeCalled = true
	return nil
}

func (f *mockXdlockFactory) Health(ctx context.Context) error {
	return f.healthErr
}

func TestNewXdlockAdapter(t *testing.T) {
	factory := &mockXdlockFactory{}
	adapter := NewXdlockAdapter(factory)

	require.NotNil(t, adapter)
	assert.Equal(t, "xcron:", adapter.keyPrefix)
	assert.Equal(t, factory, adapter.Factory())
}

func TestNewXdlockAdapter_WithPrefix(t *testing.T) {
	factory := &mockXdlockFactory{}
	adapter := NewXdlockAdapter(factory, WithXdlockKeyPrefix("custom:"))

	assert.Equal(t, "custom:", adapter.keyPrefix)
}

func TestXdlockAdapter_TryLock_Success(t *testing.T) {
	locker := &mockXdlockLocker{}
	factory := &mockXdlockFactory{locker: locker}
	adapter := NewXdlockAdapter(factory)

	handle, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)

	require.NoError(t, err)
	require.NotNil(t, handle)
	assert.True(t, locker.tryLockCalled)
	assert.Equal(t, "test-job", handle.Key())
}

func TestXdlockAdapter_TryLock_LockHeld(t *testing.T) {
	locker := &mockXdlockLocker{tryLockErr: xdlock.ErrLockHeld}
	factory := &mockXdlockFactory{locker: locker}
	adapter := NewXdlockAdapter(factory)

	handle, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)

	require.NoError(t, err)
	assert.Nil(t, handle) // 锁被占用返回 nil handle
}

func TestXdlockAdapter_TryLock_Error(t *testing.T) {
	locker := &mockXdlockLocker{tryLockErr: xdlock.ErrSessionExpired}
	factory := &mockXdlockFactory{locker: locker}
	adapter := NewXdlockAdapter(factory)

	handle, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)

	require.Error(t, err)
	assert.Nil(t, handle)
	assert.ErrorIs(t, err, xdlock.ErrSessionExpired)
}

func TestXdlockHandle_Unlock_Success(t *testing.T) {
	locker := &mockXdlockLocker{}
	factory := &mockXdlockFactory{locker: locker}
	adapter := NewXdlockAdapter(factory)

	handle, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, handle)

	err = handle.Unlock(context.Background())
	require.NoError(t, err)
	assert.True(t, locker.unlockCalled)
}

func TestXdlockHandle_Unlock_LockExpired(t *testing.T) {
	locker := &mockXdlockLocker{unlockErr: xdlock.ErrLockExpired}
	factory := &mockXdlockFactory{locker: locker}
	adapter := NewXdlockAdapter(factory)

	handle, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, handle)

	err = handle.Unlock(context.Background())
	assert.ErrorIs(t, err, ErrLockNotHeld)
}

func TestXdlockHandle_Unlock_NotLocked(t *testing.T) {
	locker := &mockXdlockLocker{unlockErr: xdlock.ErrNotLocked}
	factory := &mockXdlockFactory{locker: locker}
	adapter := NewXdlockAdapter(factory)

	handle, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, handle)

	err = handle.Unlock(context.Background())
	assert.ErrorIs(t, err, ErrLockNotHeld)
}

func TestXdlockHandle_Renew_Success(t *testing.T) {
	locker := &mockXdlockLocker{}
	factory := &mockXdlockFactory{locker: locker}
	adapter := NewXdlockAdapter(factory)

	handle, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, handle)

	err = handle.Renew(context.Background(), 5*time.Minute)
	require.NoError(t, err)
	assert.True(t, locker.extendCalled)
}

func TestXdlockHandle_Renew_ExtendNotSupported(t *testing.T) {
	// etcd 不支持手动续期，应该静默忽略
	locker := &mockXdlockLocker{extendErr: xdlock.ErrExtendNotSupported}
	factory := &mockXdlockFactory{locker: locker}
	adapter := NewXdlockAdapter(factory)

	handle, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, handle)

	err = handle.Renew(context.Background(), 5*time.Minute)
	require.NoError(t, err) // 应该成功，因为 etcd 自动续期
}

func TestXdlockHandle_Renew_ExtendFailed(t *testing.T) {
	locker := &mockXdlockLocker{extendErr: xdlock.ErrExtendFailed}
	factory := &mockXdlockFactory{locker: locker}
	adapter := NewXdlockAdapter(factory)

	handle, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, handle)

	err = handle.Renew(context.Background(), 5*time.Minute)
	assert.ErrorIs(t, err, ErrLockNotHeld)
}

func TestXdlockHandle_Renew_NotLocked(t *testing.T) {
	locker := &mockXdlockLocker{extendErr: xdlock.ErrNotLocked}
	factory := &mockXdlockFactory{locker: locker}
	adapter := NewXdlockAdapter(factory)

	handle, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, handle)

	err = handle.Renew(context.Background(), 5*time.Minute)
	assert.ErrorIs(t, err, ErrLockNotHeld)
}

func TestXdlockAdapter_WithScheduler(t *testing.T) {
	// 测试适配器与调度器的集成
	locker := &mockXdlockLocker{}
	factory := &mockXdlockFactory{locker: locker}
	adapter := NewXdlockAdapter(factory)

	scheduler := New(WithLocker(adapter), WithSeconds())
	require.NotNil(t, scheduler)

	executed := make(chan struct{}, 1)
	_, err := scheduler.AddFunc("*/1 * * * * *", func(ctx context.Context) error {
		executed <- struct{}{}
		return nil
	}, WithName("test-job"), WithLockTTL(5*time.Minute))
	require.NoError(t, err)

	scheduler.Start()
	defer scheduler.Stop()

	// 等待任务执行
	select {
	case <-executed:
		assert.True(t, locker.tryLockCalled)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for execution")
	}
}
