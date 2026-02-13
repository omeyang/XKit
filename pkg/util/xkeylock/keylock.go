package xkeylock

import "context"

// Handle 表示一次成功的锁获取。
// Unlock 是幂等的：第一次调用释放锁并返回 nil，后续调用返回 [ErrLockNotHeld]。
type Handle interface {
	// Unlock 释放锁。
	// 幂等：第一次调用返回 nil，后续调用返回 [ErrLockNotHeld]。
	Unlock() error

	// Key 返回锁的 key。
	Key() string
}

// KeyLock 提供基于 key 的进程内互斥锁。
// 所有方法都是并发安全的。
type KeyLock interface {
	// Acquire 阻塞式获取锁。
	// 支持 ctx 超时/取消，ctx 取消时返回 [context.Canceled] 或 [context.DeadlineExceeded]。
	// KeyLock 已关闭时返回 [ErrClosed]。
	// ctx 不得为 nil，否则 panic（与标准库 http.NewRequestWithContext 等一致）。
	//
	// 注意：锁是非可重入的（non-reentrant）。同一 goroutine 对同一 key 重复调用
	// Acquire 会死锁。如需可重入语义，请在调用方自行维护持有状态。
	Acquire(ctx context.Context, key string) (Handle, error)

	// TryAcquire 非阻塞获取锁。
	// handle=nil && err=nil 表示锁被占用（与 xdlock.TryLock 语义一致）。
	// KeyLock 已关闭时返回 (nil, [ErrClosed])。
	TryAcquire(key string) (Handle, error)

	// Len 返回当前活跃的 key 数量。
	// 比 Keys() 更高效，适用于监控和指标采集。
	Len() int

	// Keys 返回当前活跃条目的 key 列表（包含持有者和等待者），仅用于调试。
	// 返回值是快照，不保证跨分片原子性。
	// 监控/指标采集场景推荐使用 Len（单次原子读取，无锁开销）。
	Keys() []string

	// Close 关闭 KeyLock。
	// 后续 Acquire/TryAcquire 返回 [ErrClosed]。
	// 已持有的锁不受影响，仍可正常 Unlock。
	// 会唤醒所有已在等待的 Acquire，使其返回 [ErrClosed]。
	// 重复调用返回 [ErrClosed]。
	Close() error
}

// New 创建一个新的 KeyLock 实例。
// 配置无效时返回错误（如分片数不是 2 的幂）。
func New(opts ...Option) (KeyLock, error) {
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	if err := o.validate(); err != nil {
		return nil, err
	}
	return newKeyLockImpl(o), nil
}
