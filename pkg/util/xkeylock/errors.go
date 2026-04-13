package xkeylock

import "errors"

var (
	// ErrNilContext 表示传入的 context 为 nil。
	// Acquire 在 ctx 为 nil 时返回此错误。
	ErrNilContext = errors.New("xkeylock: nil context")

	// ErrLockNotHeld 表示锁已被释放。
	// Unlock 第二次及后续调用时返回此错误。
	ErrLockNotHeld = errors.New("xkeylock: lock not held")

	// ErrLockOccupied 表示锁已被其他持有者占用。
	// TryAcquire 在锁不可用时返回此错误。
	ErrLockOccupied = errors.New("xkeylock: lock occupied")

	// ErrClosed 表示 Locker 已关闭。
	// Close 后调用 Acquire/TryAcquire 返回此错误。
	ErrClosed = errors.New("xkeylock: closed")

	// ErrMaxKeysExceeded 表示已达到最大 key 数量限制。
	ErrMaxKeysExceeded = errors.New("xkeylock: max keys exceeded")

	// ErrInvalidShardCount 表示分片数量无效（必须为 2 的正整数幂）。
	ErrInvalidShardCount = errors.New("xkeylock: invalid shard count")

	// ErrInvalidKey 表示 key 无效（不得为空字符串）。
	ErrInvalidKey = errors.New("xkeylock: invalid key")
)
