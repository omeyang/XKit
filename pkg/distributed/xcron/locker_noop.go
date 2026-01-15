package xcron

import (
	"context"
	"time"
)

// noopLocker 无锁实现，用于单副本场景。
// 所有锁操作都直接返回成功，不做任何实际锁定。
type noopLocker struct{}

// noopLockHandle 无锁实现的 LockHandle
type noopLockHandle struct {
	key string
}

// NoopLocker 返回无锁实现。
//
// 适用于单副本部署场景（如租户侧服务），所有任务直接执行，
// 无需分布式锁协调。
//
// 用法：
//
//	scheduler := xcron.New(xcron.WithLocker(xcron.NoopLocker()))
//
// 或者不设置 locker，默认就是 NoopLocker：
//
//	scheduler := xcron.New() // 等同于 WithLocker(NoopLocker())
func NoopLocker() Locker {
	return &noopLocker{}
}

// TryLock 总是返回成功的 LockHandle。
func (l *noopLocker) TryLock(_ context.Context, key string, _ time.Duration) (LockHandle, error) {
	return &noopLockHandle{key: key}, nil
}

// Unlock 空操作，总是返回 nil。
func (h *noopLockHandle) Unlock(_ context.Context) error {
	return nil
}

// Renew 空操作，总是返回 nil。
func (h *noopLockHandle) Renew(_ context.Context, _ time.Duration) error {
	return nil
}

// Key 返回锁的 key。
func (h *noopLockHandle) Key() string {
	return h.key
}

// 确保 noopLocker 实现了 Locker 接口
var _ Locker = (*noopLocker)(nil)

// 确保 noopLockHandle 实现了 LockHandle 接口
var _ LockHandle = (*noopLockHandle)(nil)
