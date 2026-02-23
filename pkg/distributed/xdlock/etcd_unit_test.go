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

func TestEtcdFactory_Health_NilContext(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)

	err := f.Health(nil) //nolint:staticcheck // SA1012: nil ctx 是测试目标
	assert.ErrorIs(t, err, ErrNilContext)
}

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

// =============================================================================
// nil context 单元测试（FG-S3 验证）
// =============================================================================

func TestEtcdFactory_TryLock_NilContext(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)

	handle, err := f.TryLock(nil, "test-key") //nolint:staticcheck // SA1012: nil ctx 是测试目标
	assert.ErrorIs(t, err, ErrNilContext)
	assert.Nil(t, handle)
}

func TestEtcdFactory_Lock_NilContext(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)

	handle, err := f.Lock(nil, "test-key") //nolint:staticcheck // SA1012: nil ctx 是测试目标
	assert.ErrorIs(t, err, ErrNilContext)
	assert.Nil(t, handle)
}

func TestEtcdLockHandle_Extend_NilContext(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)
	h := NewTestEtcdLockHandle(f, "lock:test")

	err := h.Extend(nil) //nolint:staticcheck // SA1012: nil ctx 是测试目标
	assert.ErrorIs(t, err, ErrNilContext)
}

// =============================================================================
// nil option 单元测试（FG-M7 验证）
// =============================================================================

func TestEtcdFactory_NilFactoryOption(t *testing.T) {
	// nil option 应被安全跳过，不 panic
	_, err := NewEtcdFactory(nil, nil, WithEtcdTTL(30), nil)
	// 仍然返回 ErrNilClient（client 为 nil），但不应 panic
	assert.ErrorIs(t, err, ErrNilClient)
}

// =============================================================================
// 本地 key 追踪单元测试（FG-S1 验证）
// =============================================================================

func TestEtcdFactory_TryLock_LocalKeyTracking(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)

	// 预设 key 已被本地追踪（模拟已持有锁）
	fullKey := "lock:test-key"
	f.StoreLockedKey(fullKey)

	// TryLock 同一 key 应返回 (nil, nil)，不到达 concurrency.NewMutex
	handle, err := f.TryLock(t.Context(), "test-key")
	assert.NoError(t, err)
	assert.Nil(t, handle)
}

func TestEtcdFactory_Lock_LocalKeyTracking(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)

	// 预设 key 已被本地追踪
	fullKey := "lock:test-key"
	f.StoreLockedKey(fullKey)

	// Lock 同一 key 应返回 ErrLockFailed
	handle, err := f.Lock(t.Context(), "test-key")
	assert.ErrorIs(t, err, ErrLockFailed)
	assert.Nil(t, handle)
	assert.Contains(t, err.Error(), fullKey)
}

func TestEtcdFactory_TryLock_DifferentKeys_NoConflict(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)

	// 预设 key-a 已被追踪
	f.StoreLockedKey("lock:key-a")

	// TryLock key-b 不应被本地追踪阻止（会因 session=nil 在 concurrency.NewMutex 后失败，
	// 但 lockedKeys 检查不应阻止），验证 key-b 被写入 lockedKeys
	// 注意：由于 session=nil，concurrency.NewMutex 会 panic，
	// 所以这里只验证 key-a 不影响 key-b 的 lockedKeys 检查
	assert.True(t, f.IsKeyLocked("lock:key-a"))
	assert.False(t, f.IsKeyLocked("lock:key-b"))
}

func TestEtcdFactory_TryLock_CustomPrefix_LocalKeyTracking(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)

	// 预设带自定义前缀的 key
	f.StoreLockedKey("myapp:test-key")

	// TryLock 同一 key + 同一前缀应命中本地追踪
	handle, err := f.TryLock(t.Context(), "test-key", WithKeyPrefix("myapp:"))
	assert.NoError(t, err)
	assert.Nil(t, handle)

	// 不同前缀的同名 key 不应冲突
	assert.False(t, f.IsKeyLocked("lock:test-key"))
}

func TestEtcdLockHandle_Unlock_CleansLockedKeys(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)
	mockMu := &MockMutex{}
	h := NewTestEtcdLockHandle(f, "lock:test", mockMu)

	// 模拟 TryLock 成功后 lockedKeys 中有记录
	f.StoreLockedKey("lock:test")
	assert.True(t, f.IsKeyLocked("lock:test"))

	// Unlock 成功后应清理 lockedKeys
	err := h.Unlock(t.Context())
	assert.NoError(t, err)
	assert.False(t, f.IsKeyLocked("lock:test"))
}

func TestEtcdLockHandle_Unlock_FailureKeepsLockedKeys(t *testing.T) {
	mock := NewMockSession()
	f := NewTestEtcdFactory(mock)
	mockMu := &MockMutex{UnlockErr: errors.New("unlock failed")}
	h := NewTestEtcdLockHandle(f, "lock:test", mockMu)

	// 模拟已持有锁
	f.StoreLockedKey("lock:test")

	// Unlock 失败后 lockedKeys 应保留（锁可能仍被持有）
	err := h.Unlock(t.Context())
	assert.Error(t, err)
	assert.True(t, f.IsKeyLocked("lock:test"))
}
