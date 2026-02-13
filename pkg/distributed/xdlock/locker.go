package xdlock

import "context"

// =============================================================================
// LockHandle - 推荐的锁操作接口
// =============================================================================

// LockHandle 表示一次成功的锁获取。
//
// 每次 TryLock/Lock 成功都会返回一个新的 handle，内部封装了唯一标识。
// 通过 handle 进行 Unlock 和 Extend 操作，确保不同获取之间不会互相干扰。
//
// # 设计目的
//
// Handle 模式解决了传统 Locker 接口的几个问题：
//   - 避免同一进程内多个 goroutine 使用同一 Locker 实例导致的状态混乱
//   - 每次获取锁时生成唯一标识，只有持有该标识的 handle 才能操作锁
//   - 更清晰的所有权语义：持有 handle 即持有锁
//
// # 使用模式
//
//	handle, err := factory.TryLock(ctx, "my-resource", xdlock.WithExpiry(5*time.Minute))
//	if err != nil {
//	    return err // 锁服务异常
//	}
//	if handle == nil {
//	    return nil // 被其他实例持有，跳过执行
//	}
//	defer handle.Unlock(ctx)
//
//	// 执行任务...
type LockHandle interface {
	// Unlock 释放锁。
	//
	// 只释放本次获取的锁，不会影响其他 goroutine 或实例持有的锁。
	// 返回 [ErrNotLocked] 表示锁已过期或被其他获取覆盖。
	Unlock(ctx context.Context) error

	// Extend 续期锁。
	//
	// 延长锁的 TTL，用于长时间运行的任务。
	// 续期时间使用创建锁时配置的 Expiry。
	//
	// 注意：etcd 使用 Session 自动续期，调用此方法仅检查 Session 健康状态，
	// 若 Session 有效返回 nil，若已过期返回 [ErrSessionExpired]。
	//
	// 返回 [ErrNotLocked] 表示锁已过期或被其他获取覆盖（所有权已丢失）。
	// 返回 [ErrExtendFailed] 表示续期操作失败（锁可能仍在，可重试）。
	Extend(ctx context.Context) error

	// Key 返回锁的 key。
	//
	// 用于日志记录等场景。
	Key() string
}

// Factory 定义锁工厂接口。
// 工厂管理底层连接，并提供锁操作。
type Factory interface {
	// TryLock 非阻塞式获取锁。
	//
	// 每次调用生成唯一标识，确保不同获取之间不会互相干扰。
	// 成功时返回 LockHandle，锁被占用时返回 (nil, nil)。
	//
	// 参数：
	//   - ctx: 上下文，用于超时控制
	//   - key: 锁标识，建议使用业务语义明确的名称
	//   - opts: 锁配置选项（如 WithExpiry 设置 TTL）
	//
	// 返回：
	//   - handle: 成功时返回 LockHandle，未获取到返回 nil
	//   - err: 锁服务异常（如 Redis 不可用）
	//
	// 注意：handle=nil 且 err=nil 表示锁被其他实例持有，这是正常情况。
	TryLock(ctx context.Context, key string, opts ...MutexOption) (LockHandle, error)

	// Lock 阻塞式获取锁。
	//
	// 会根据配置的重试策略进行重试，直到获取到锁或 context 取消/超时。
	// 成功时返回 LockHandle。
	//
	// 参数：
	//   - ctx: 上下文，用于超时控制
	//   - key: 锁标识
	//   - opts: 锁配置选项
	//
	// 错误：
	//   - context.Canceled: context 被取消
	//   - context.DeadlineExceeded: context 超时
	//   - ErrLockFailed: 重试耗尽仍未获取到锁
	Lock(ctx context.Context, key string, opts ...MutexOption) (LockHandle, error)

	// Close 关闭工厂，释放底层资源。
	// 关闭后不应再创建新的锁实例。
	Close() error

	// Health 健康检查。
	// 检查底层连接是否正常。
	Health(ctx context.Context) error
}

// EtcdFactory 定义 etcd 锁工厂接口。
// 扩展 Factory 接口，提供 etcd 特定功能。
type EtcdFactory interface {
	Factory

	// Session 返回底层 concurrency.Session。
	// 用于需要直接访问 etcd Session 的高级场景。
	Session() Session
}

// RedisFactory 定义 Redis 锁工厂接口。
// 扩展 Factory 接口，提供 Redis (redsync) 特定功能。
type RedisFactory interface {
	Factory

	// Redsync 返回底层 redsync.Redsync 实例。
	// 用于需要直接访问 redsync 的高级场景。
	Redsync() Redsync
}
