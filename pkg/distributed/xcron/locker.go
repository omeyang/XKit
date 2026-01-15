package xcron

import (
	"context"
	"errors"
	"time"
)

// LockHandle 表示一次成功的锁获取。
//
// 每次 TryLock 成功都会返回一个新的 handle，内部封装了唯一 token。
// 通过 handle 进行 Unlock 和 Renew 操作，确保不同获取之间不会互相干扰。
//
// # 设计目的
//
// 解决同一进程内多个 goroutine 使用相同 identity 导致的锁误释放问题。
// 每次获取锁时生成唯一 token，只有持有该 token 的 handle 才能操作锁。
//
// # 使用模式
//
//	handle, err := locker.TryLock(ctx, "my-job", 5*time.Minute)
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
	// 返回 [ErrLockNotHeld] 表示锁已过期或被其他获取覆盖。
	Unlock(ctx context.Context) error

	// Renew 续期锁。
	//
	// 延长锁的 TTL，用于长时间运行的任务。
	// 只能续期本次获取的锁。
	//
	// 返回 [ErrLockNotHeld] 表示锁已过期或被其他获取覆盖。
	Renew(ctx context.Context, ttl time.Duration) error

	// Key 返回锁的 key。
	//
	// 用于日志记录等场景。
	Key() string
}

// Locker 分布式锁接口。
//
// 用于多副本场景，确保同一时刻只有一个实例执行任务。
// 典型实现包括：
//   - [NoopLocker]: 无锁实现，用于单副本场景
//   - [RedisLocker]: 基于 Redis 的分布式锁
//   - [K8sLocker]: 基于 K8S Lease 的分布式锁
//
// # 实现要求
//
//   - TryLock 必须是非阻塞的
//   - 每次 TryLock 成功必须返回独立的 LockHandle
//   - 锁必须有 TTL，防止死锁
//   - 实现必须是并发安全的
//
// # 使用模式
//
//	handle, err := locker.TryLock(ctx, "my-job", 5*time.Minute)
//	if err != nil {
//	    // 锁服务异常
//	    return err
//	}
//	if handle == nil {
//	    // 被其他实例持有，跳过执行
//	    return nil
//	}
//	defer handle.Unlock(ctx)
//
//	// 执行任务...
type Locker interface {
	// TryLock 尝试获取锁（非阻塞）。
	//
	// 每次调用生成唯一 token，确保不同获取之间不会互相干扰。
	//
	// 参数：
	//   - ctx: 上下文，用于超时控制
	//   - key: 锁标识，通常是任务名
	//   - ttl: 锁自动过期时间，防止死锁
	//
	// 返回：
	//   - handle: 成功时返回 LockHandle，未获取到返回 nil
	//   - err: 锁服务异常（如 Redis 不可用）
	//
	// 注意：handle=nil 且 err=nil 表示锁被其他实例持有，这是正常情况。
	TryLock(ctx context.Context, key string, ttl time.Duration) (handle LockHandle, err error)
}

// ErrLockNotHeld 表示尝试操作未持有的锁。
// 当 Unlock 或 Renew 时锁已过期或被其他实例持有时返回此错误。
var ErrLockNotHeld = errors.New("xcron: lock not held by this instance")

// ErrLockAcquireFailed 表示获取锁失败（服务异常）。
// 与 TryLock 返回 acquired=false 不同，此错误表示锁服务本身出现问题。
var ErrLockAcquireFailed = errors.New("xcron: failed to acquire lock")
