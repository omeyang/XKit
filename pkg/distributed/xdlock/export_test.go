package xdlock

import "context"

// =============================================================================
// 测试辅助：暴露内部构造器用于单元测试
// =============================================================================

// MockSession 实现 sessionProvider 接口，用于 etcd 工厂单元测试。
type MockSession struct {
	DoneCh   chan struct{} // 控制 Done() 返回的 channel
	CloseErr error         // Close() 返回的错误
	Closed   bool          // 标记是否已调用 Close
}

// Done 返回 session done channel。
func (m *MockSession) Done() <-chan struct{} {
	return m.DoneCh
}

// Close 模拟关闭 session。
func (m *MockSession) Close() error {
	m.Closed = true
	return m.CloseErr
}

// NewMockSession 创建默认的 MockSession（未过期、Close 无错误）。
func NewMockSession() *MockSession {
	return &MockSession{
		DoneCh: make(chan struct{}),
	}
}

// NewExpiredMockSession 创建已过期的 MockSession。
func NewExpiredMockSession() *MockSession {
	ch := make(chan struct{})
	close(ch)
	return &MockSession{
		DoneCh: ch,
	}
}

// MockMutex 实现 mutexUnlocker 接口，用于 etcd handle 单元测试。
type MockMutex struct {
	UnlockErr error // Unlock() 返回的错误
}

// Unlock 模拟解锁。
func (m *MockMutex) Unlock(_ context.Context) error {
	return m.UnlockErr
}

// NewTestEtcdFactory 创建用于测试的 etcdFactory，使用 MockSession 替代真实 Session。
// 注意：此工厂的 session 字段为 nil，Session() 方法返回 nil，
// 仅用于测试 checkSession/Close/Health 等不依赖真实 Session 的路径。
func NewTestEtcdFactory(mock *MockSession) *etcdFactory {
	return &etcdFactory{
		sp: mock,
	}
}

// StoreLockedKey 在工厂的 lockedKeys 中写入 key（用于测试本地追踪机制）。
func (f *etcdFactory) StoreLockedKey(key string) {
	f.lockedKeys.Store(key, struct{}{})
}

// DeleteLockedKey 从工厂的 lockedKeys 中删除 key（用于测试）。
func (f *etcdFactory) DeleteLockedKey(key string) {
	f.lockedKeys.Delete(key)
}

// IsKeyLocked 检查 key 是否在工厂的 lockedKeys 中（用于测试）。
func (f *etcdFactory) IsKeyLocked(key string) bool {
	_, ok := f.lockedKeys.Load(key)
	return ok
}

// NewTestEtcdLockHandle 创建用于测试的 etcdLockHandle。
// factory 参数用于 Extend 中的 checkSession 调用。
// mockMu 为 nil 时创建一个默认的 MockMutex（Unlock 成功）。
func NewTestEtcdLockHandle(factory *etcdFactory, key string, mockMu ...mutexUnlocker) *etcdLockHandle {
	var mu mutexUnlocker
	if len(mockMu) > 0 && mockMu[0] != nil {
		mu = mockMu[0]
	} else {
		mu = &MockMutex{}
	}
	return &etcdLockHandle{
		factory: factory,
		mu:      mu,
		key:     key,
	}
}

// SetUnlocked 设置 handle 的 unlocked 标记（用于测试）。
func (h *etcdLockHandle) SetUnlocked(v bool) {
	h.unlocked.Store(v)
}
