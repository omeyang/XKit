package xdlock

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// =============================================================================
// checkSession 单元测试
// =============================================================================

func TestEtcdFactory_checkSession_FactoryClosed(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)
	f.closed.Store(true)

	err := f.checkSession()
	assert.ErrorIs(t, err, ErrFactoryClosed)
}

func TestEtcdFactory_checkSession_SessionExpired(t *testing.T) {
	mock := NewExpiredMockSession()
	f := NewTestEtcdFactory(mock)

	err := f.checkSession()
	assert.ErrorIs(t, err, ErrSessionExpired)
}

func TestEtcdFactory_checkSession_OK(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)

	err := f.checkSession()
	assert.NoError(t, err)
}

// =============================================================================
// Close 单元测试
// =============================================================================

func TestEtcdFactory_Close_Success(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)

	err := f.Close(t.Context())
	assert.NoError(t, err)
	assert.True(t, mock.Closed)
	assert.True(t, f.closed.Load())
}

func TestEtcdFactory_Close_Idempotent(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)

	// 第一次关闭
	err := f.Close(t.Context())
	assert.NoError(t, err)
	assert.True(t, mock.Closed)

	// 第二次关闭，不应再调用 session.Close()
	mock.Closed = false // 重置标记
	err = f.Close(t.Context())
	assert.NoError(t, err)
	assert.False(t, mock.Closed) // 第二次不应调用
}

func TestEtcdFactory_Close_WithError(t *testing.T) {
	mock := NewMockSession()
	mock.CloseErr = errors.New("close failed")
	f := NewTestEtcdFactory(mock)

	err := f.Close(t.Context())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "close failed")
}

// =============================================================================
// Health 单元测试（无 client 的路径）
// =============================================================================

func TestEtcdFactory_Health_FactoryClosed(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)
	f.closed.Store(true)

	err := f.Health(t.Context())
	assert.ErrorIs(t, err, ErrFactoryClosed)
}

func TestEtcdFactory_Health_SessionExpired(t *testing.T) {
	mock := NewExpiredMockSession()
	f := NewTestEtcdFactory(mock)

	err := f.Health(t.Context())
	assert.ErrorIs(t, err, ErrSessionExpired)
}

// =============================================================================
// Session 单元测试
// =============================================================================

func TestEtcdFactory_Session_ReturnsNilForTestFactory(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)

	// 测试工厂的 session 为 nil
	assert.Nil(t, f.Session())
}

// =============================================================================
// TryLock/Lock 前置检查单元测试
// =============================================================================

func TestEtcdFactory_TryLock_FactoryClosed(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)
	f.closed.Store(true)

	handle, err := f.TryLock(t.Context(), "test-key")
	assert.ErrorIs(t, err, ErrFactoryClosed)
	assert.Nil(t, handle)
}

func TestEtcdFactory_TryLock_SessionExpired(t *testing.T) {
	mock := NewExpiredMockSession()
	f := NewTestEtcdFactory(mock)

	handle, err := f.TryLock(t.Context(), "test-key")
	assert.ErrorIs(t, err, ErrSessionExpired)
	assert.Nil(t, handle)
}

func TestEtcdFactory_TryLock_EmptyKey(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)

	handle, err := f.TryLock(t.Context(), "")
	assert.ErrorIs(t, err, ErrEmptyKey)
	assert.Nil(t, handle)
}

func TestEtcdFactory_TryLock_KeyTooLong(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)

	longKey := make([]byte, maxKeyLength+1)
	for i := range longKey {
		longKey[i] = 'x'
	}

	handle, err := f.TryLock(t.Context(), string(longKey))
	assert.ErrorIs(t, err, ErrKeyTooLong)
	assert.Nil(t, handle)
}

func TestEtcdFactory_Lock_FactoryClosed(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)
	f.closed.Store(true)

	handle, err := f.Lock(t.Context(), "test-key")
	assert.ErrorIs(t, err, ErrFactoryClosed)
	assert.Nil(t, handle)
}

func TestEtcdFactory_Lock_SessionExpired(t *testing.T) {
	mock := NewExpiredMockSession()
	f := NewTestEtcdFactory(mock)

	handle, err := f.Lock(t.Context(), "test-key")
	assert.ErrorIs(t, err, ErrSessionExpired)
	assert.Nil(t, handle)
}

func TestEtcdFactory_Lock_EmptyKey(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)

	handle, err := f.Lock(t.Context(), "  ")
	assert.ErrorIs(t, err, ErrEmptyKey)
	assert.Nil(t, handle)
}

// =============================================================================
// etcdLockHandle.Extend 单元测试
// =============================================================================

func TestEtcdLockHandle_Extend_OK(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)
	h := NewTestEtcdLockHandle(f, "lock:test")

	err := h.Extend(t.Context())
	assert.NoError(t, err)
}

func TestEtcdLockHandle_Extend_Unlocked(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)
	h := NewTestEtcdLockHandle(f, "lock:test")
	h.SetUnlocked(true)

	err := h.Extend(t.Context())
	assert.ErrorIs(t, err, ErrNotLocked)
}

func TestEtcdLockHandle_Extend_SessionExpired(t *testing.T) {
	mock := NewExpiredMockSession()
	f := NewTestEtcdFactory(mock)
	h := NewTestEtcdLockHandle(f, "lock:test")

	err := h.Extend(t.Context())
	assert.ErrorIs(t, err, ErrSessionExpired)
}

func TestEtcdLockHandle_Extend_FactoryClosed(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)
	f.closed.Store(true)
	h := NewTestEtcdLockHandle(f, "lock:test")

	err := h.Extend(t.Context())
	assert.ErrorIs(t, err, ErrFactoryClosed)
}

// =============================================================================
// etcdLockHandle.Key 单元测试
// =============================================================================

func TestEtcdLockHandle_Key(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"simple", "lock:test"},
		{"with prefix", "myapp:lock:resource"},
		{"unicode", "锁:测试"},
	}

	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewTestEtcdLockHandle(f, tt.key)
			assert.Equal(t, tt.key, h.Key())
		})
	}
}

// =============================================================================
// etcdLockHandle.Unlock 单元测试
// =============================================================================

func TestEtcdLockHandle_Unlock_Success(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)
	mockMu := &MockMutex{}
	h := NewTestEtcdLockHandle(f, "lock:test", mockMu)

	err := h.Unlock(t.Context())
	assert.NoError(t, err)
	// unlocked 应在成功后被标记为 true
	assert.True(t, h.unlocked.Load())
}

func TestEtcdLockHandle_Unlock_Error(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)
	mockMu := &MockMutex{UnlockErr: errors.New("unlock failed")}
	h := NewTestEtcdLockHandle(f, "lock:test", mockMu)

	err := h.Unlock(t.Context())
	assert.Error(t, err)
	// unlocked 不应被标记（解锁失败）
	assert.False(t, h.unlocked.Load())
}

func TestEtcdLockHandle_Unlock_CanceledContext(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)
	mockMu := &MockMutex{}
	h := NewTestEtcdLockHandle(f, "lock:test", mockMu)

	// 使用已取消的 ctx，验证清理上下文机制
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := h.Unlock(ctx)
	assert.NoError(t, err)
	assert.True(t, h.unlocked.Load())
}

func TestEtcdLockHandle_Unlock_SessionExpiredError(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)
	// 模拟 Session 过期导致的 Unlock 错误
	mockMu := &MockMutex{UnlockErr: concurrency.ErrSessionExpired}
	h := NewTestEtcdLockHandle(f, "lock:test", mockMu)

	err := h.Unlock(t.Context())
	assert.ErrorIs(t, err, ErrSessionExpired)
	assert.False(t, h.unlocked.Load())
}

func TestEtcdLockHandle_Unlock_NilContext(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)
	mockMu := &MockMutex{}
	h := NewTestEtcdLockHandle(f, "lock:test", mockMu)

	err := h.Unlock(nil) //nolint:staticcheck // SA1012: nil ctx 是测试目标
	assert.ErrorIs(t, err, ErrNilContext)
	assert.False(t, h.unlocked.Load())
}

func TestEtcdLockHandle_Unlock_AlreadyUnlocked(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)
	mockMu := &MockMutex{}
	h := NewTestEtcdLockHandle(f, "lock:test", mockMu)
	h.SetUnlocked(true)

	err := h.Unlock(t.Context())
	assert.ErrorIs(t, err, ErrNotLocked)
}

// =============================================================================
// NewEtcdFactory 单元测试（补充构造器覆盖）
// =============================================================================

func TestNewEtcdFactory_NilClient_Internal(t *testing.T) {
	factory, err := NewEtcdFactory(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilClient)
	assert.Nil(t, factory)
}
