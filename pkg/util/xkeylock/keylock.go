package xkeylock

import (
	"context"
	"io"
)

// Handle 表示一次成功的锁获取。
// Unlock 是幂等的：第一次调用释放锁并返回 nil，后续调用返回 [ErrLockNotHeld]。
type Handle interface {
	// Unlock 释放锁。
	// 幂等：第一次调用返回 nil，后续调用返回 [ErrLockNotHeld]。
	Unlock() error

	// Key 返回锁的 key。
	// 即使在 Unlock 之后调用，Key 仍返回原始 key 值。
	Key() string
}

// Locker 提供基于 key 的进程内互斥锁。
// 所有方法都是并发安全的。
type Locker interface {
	io.Closer

	// Acquire 阻塞式获取锁。
	// 支持 ctx 超时/取消，ctx 取消时返回 [context.Canceled] 或 [context.DeadlineExceeded]。
	// Locker 已关闭时返回 [ErrClosed]。key 不得为空字符串，否则返回 [ErrInvalidKey]。
	// ctx 不得为 nil，否则返回 [ErrNilContext]。
	//
	// 当 Acquire 处于阻塞等待时，若 Close 与 ctx 取消同时发生，
	// 返回 [ErrClosed] 或 ctx.Err() 均有可能（Go select 语义）。
	// 调用方应同时处理这两类错误。
	//
	// 设计决策: 锁是非可重入的（non-reentrant），与 sync.Mutex 一致。
	// 不提供运行时死锁检测（开销不可接受），由调用方负责避免同一 goroutine
	// 对同一 key 重复 Acquire。建议始终使用带 deadline 的 context 以防止
	// 因编程错误导致的永久阻塞。
	Acquire(ctx context.Context, key string) (Handle, error)

	// TryAcquire 非阻塞获取锁。
	// 锁被占用时返回 (nil, [ErrLockOccupied])。
	// Locker 已关闭时返回 (nil, [ErrClosed])。
	// key 不得为空字符串，否则返回 (nil, [ErrInvalidKey])。
	TryAcquire(key string) (Handle, error)

	// Len 返回当前活跃的 key 数量（单次原子读取，瞬时快照）。
	// 比 Keys() 更高效，适用于监控和指标采集。
	// 并发场景下 Len() 与 len(Keys()) 可能不一致，属正常行为。
	// Close 后仍可安全调用，返回值随已持有 Handle 的释放逐渐归零。
	Len() int

	// Keys 返回当前活跃条目的 key 列表（包含持有者和等待者），仅用于调试。
	// 返回值是快照，不保证跨分片原子性。
	// 监控/指标采集场景推荐使用 Len（单次原子读取，无锁开销）。
	// Close 后仍可安全调用，返回值随已持有 Handle 的释放逐渐归零。
	Keys() []string
}

// New 创建一个新的 Locker 实例。
// 配置无效时返回错误（如分片数不是 2 的幂）。
func New(opts ...Option) (Locker, error) {
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	if err := o.validate(); err != nil {
		return nil, err
	}
	return newKeyLockImpl(o), nil
}
