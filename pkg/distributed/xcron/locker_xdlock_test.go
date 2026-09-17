package xcron

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omeyang/xkit/pkg/distributed/xdlock"
)

// mockXdlockHandle 实现 xdlock.LockHandle 接口用于测试
type mockXdlockHandle struct {
	key          string
	unlockErr    error
	extendErr    error
	unlockCalled bool
	extendCalled bool
}

func (h *mockXdlockHandle) Unlock(_ context.Context) error {
	h.unlockCalled = true
	return h.unlockErr
}

func (h *mockXdlockHandle) Extend(_ context.Context) error {
	h.extendCalled = true
	return h.extendErr
}

func (h *mockXdlockHandle) Key() string {
	return h.key
}

// mockXdlockFactory 实现 xdlock.Factory 接口用于测试
type mockXdlockFactory struct {
	handle        *mockXdlockHandle
	tryLockErr    error
	lockErr       error
	closeCalled   bool
	healthErr     error
	tryLockCalled bool
	lockCalled    bool
}

func (f *mockXdlockFactory) TryLock(_ context.Context, key string, _ ...xdlock.MutexOption) (xdlock.LockHandle, error) {
	f.tryLockCalled = true
	if f.tryLockErr != nil {
		// ErrLockHeld 表示锁被占用，返回 (nil, nil)
		if f.tryLockErr == xdlock.ErrLockHeld {
			return nil, nil
		}
		return nil, f.tryLockErr
	}
	if f.handle == nil {
		f.handle = &mockXdlockHandle{key: key}
	}
	f.handle.key = key
	return f.handle, nil
}

func (f *mockXdlockFactory) Lock(_ context.Context, key string, _ ...xdlock.MutexOption) (xdlock.LockHandle, error) {
	f.lockCalled = true
	if f.lockErr != nil {
		return nil, f.lockErr
	}
	if f.handle == nil {
		f.handle = &mockXdlockHandle{key: key}
	}
	f.handle.key = key
	return f.handle, nil
}

func (f *mockXdlockFactory) Close(_ context.Context) error {
	f.closeCalled = true
	return nil
}

func (f *mockXdlockFactory) Health(_ context.Context) error {
	return f.healthErr
}

func TestNewXdlockAdapter(t *testing.T) {
	factory := &mockXdlockFactory{}
	adapter, err := NewXdlockAdapter(factory)
	require.NoError(t, err)

	require.NotNil(t, adapter)
	assert.Equal(t, "xcron:", adapter.keyPrefix)
	assert.Equal(t, factory, adapter.Factory())
}

func TestNewXdlockAdapter_NilFactory(t *testing.T) {
	adapter, err := NewXdlockAdapter(nil)

	assert.Nil(t, adapter)
	assert.ErrorIs(t, err, ErrNilFactory)
}

func TestNewXdlockAdapter_WithPrefix(t *testing.T) {
	factory := &mockXdlockFactory{}
	adapter, err := NewXdlockAdapter(factory, WithXdlockKeyPrefix("custom:"))
	require.NoError(t, err)

	assert.Equal(t, "custom:", adapter.keyPrefix)
}

func TestXdlockAdapter_TryLock_Success(t *testing.T) {
	handle := &mockXdlockHandle{}
	factory := &mockXdlockFactory{handle: handle}
	adapter, err := NewXdlockAdapter(factory)
	require.NoError(t, err)

	result, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, factory.tryLockCalled)
	assert.Equal(t, "test-job", result.Key())
}

func TestXdlockAdapter_TryLock_LockHeld(t *testing.T) {
	factory := &mockXdlockFactory{tryLockErr: xdlock.ErrLockHeld}
	adapter, err := NewXdlockAdapter(factory)
	require.NoError(t, err)

	result, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)

	require.NoError(t, err)
	assert.Nil(t, result) // 锁被占用返回 nil handle
}

func TestXdlockAdapter_TryLock_Error(t *testing.T) {
	factory := &mockXdlockFactory{tryLockErr: xdlock.ErrSessionExpired}
	adapter, err := NewXdlockAdapter(factory)
	require.NoError(t, err)

	result, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, xdlock.ErrSessionExpired)
}

func TestXdlockHandle_Unlock_Success(t *testing.T) {
	handle := &mockXdlockHandle{}
	factory := &mockXdlockFactory{handle: handle}
	adapter, err := NewXdlockAdapter(factory)
	require.NoError(t, err)

	result, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, result)

	err = result.Unlock(context.Background())
	require.NoError(t, err)
	assert.True(t, handle.unlockCalled)
}

func TestXdlockHandle_Unlock_LockNotHeld(t *testing.T) {
	handle := &mockXdlockHandle{unlockErr: xdlock.ErrNotLocked}
	factory := &mockXdlockFactory{handle: handle}
	adapter, err := NewXdlockAdapter(factory)
	require.NoError(t, err)

	result, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, result)

	err = result.Unlock(context.Background())
	assert.ErrorIs(t, err, ErrLockNotHeld)
}

func TestXdlockHandle_Renew_Success(t *testing.T) {
	handle := &mockXdlockHandle{}
	factory := &mockXdlockFactory{handle: handle}
	adapter, err := NewXdlockAdapter(factory)
	require.NoError(t, err)

	result, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, result)

	err = result.Renew(context.Background(), 5*time.Minute)
	require.NoError(t, err)
	assert.True(t, handle.extendCalled)
}

func TestXdlockHandle_Renew_ExtendFailed(t *testing.T) {
	handle := &mockXdlockHandle{extendErr: xdlock.ErrExtendFailed}
	factory := &mockXdlockFactory{handle: handle}
	adapter, err := NewXdlockAdapter(factory)
	require.NoError(t, err)

	result, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, result)

	err = result.Renew(context.Background(), 5*time.Minute)
	assert.ErrorIs(t, err, ErrLockNotHeld)
}

func TestXdlockHandle_Renew_LockNotHeld(t *testing.T) {
	handle := &mockXdlockHandle{extendErr: xdlock.ErrNotLocked}
	factory := &mockXdlockFactory{handle: handle}
	adapter, err := NewXdlockAdapter(factory)
	require.NoError(t, err)

	result, err := adapter.TryLock(context.Background(), "test-job", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, result)

	err = result.Renew(context.Background(), 5*time.Minute)
	assert.ErrorIs(t, err, ErrLockNotHeld)
}

func TestXdlockAdapter_WithScheduler(t *testing.T) {
	// 测试适配器与调度器的集成
	handle := &mockXdlockHandle{}
	factory := &mockXdlockFactory{handle: handle}
	adapter, err := NewXdlockAdapter(factory)
	require.NoError(t, err)

	scheduler := New(WithLocker(adapter), WithSeconds())
	require.NotNil(t, scheduler)

	executed := make(chan struct{}, 1)
	_, err = scheduler.AddFunc("*/1 * * * * *", func(ctx context.Context) error {
		executed <- struct{}{}
		return nil
	}, WithName("test-job"), WithLockTTL(5*time.Minute))
	require.NoError(t, err)

	scheduler.Start()
	defer scheduler.Stop()

	// 等待任务执行
	select {
	case <-executed:
		assert.True(t, factory.tryLockCalled)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for execution")
	}
}
