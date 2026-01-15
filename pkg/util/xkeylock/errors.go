package xkeylock

import "errors"

var (
	// ErrLockNotHeld 表示锁已被释放。
	// Unlock 第二次及后续调用时返回此错误。
	ErrLockNotHeld = errors.New("xkeylock: lock not held")

	// ErrClosed 表示 KeyLock 已关闭。
	// Close 后调用 Acquire/TryAcquire 返回此错误。
	ErrClosed = errors.New("xkeylock: closed")

	// ErrMaxKeysExceeded 表示已达到最大 key 数量限制。
	ErrMaxKeysExceeded = errors.New("xkeylock: max keys exceeded")
)
