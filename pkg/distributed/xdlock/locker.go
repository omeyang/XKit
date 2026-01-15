package xdlock

import "context"

// Locker 定义分布式锁的统一接口。
//
// 不同后端的实现差异：
//   - etcd: 使用 Session 自动续期，Extend() 返回 ErrExtendNotSupported
//   - Redis: 需要手动调用 Extend() 续期
type Locker interface {
	// Lock 阻塞式获取锁。
	// 会一直等待直到获取到锁或 context 取消/超时。
	//
	// 错误：
	//   - context.Canceled: context 被取消
	//   - context.DeadlineExceeded: context 超时
	//   - ErrSessionExpired: etcd session 过期
	Lock(ctx context.Context) error

	// TryLock 非阻塞式获取锁。
	// 如果锁被占用，立即返回 ErrLockHeld。
	//
	// 错误：
	//   - ErrLockHeld: 锁被其他持有者占用
	//   - ErrSessionExpired: etcd session 过期
	TryLock(ctx context.Context) error

	// Unlock 释放锁。
	// 只有锁的持有者才能释放锁。
	//
	// 错误：
	//   - ErrLockExpired: 锁已过期或被其他持有者抢走
	//   - ErrNotLocked: 锁未被持有
	Unlock(ctx context.Context) error

	// Extend 续期锁。
	// 延长锁的有效期，防止长任务执行过程中锁过期。
	//
	// 注意：etcd 使用 Session 自动续期，调用此方法返回 ErrExtendNotSupported。
	//
	// 错误：
	//   - ErrExtendNotSupported: 后端不支持手动续期（etcd）
	//   - ErrExtendFailed: 续期失败
	//   - ErrNotLocked: 锁未被持有
	Extend(ctx context.Context) error
}

// Factory 定义锁工厂接口。
// 工厂管理底层连接，并创建锁实例。
type Factory interface {
	// NewMutex 创建指定 key 的分布式锁实例。
	// 每次调用都会创建新的锁实例，即使 key 相同。
	//
	// key: 锁的唯一标识，建议使用业务语义明确的名称
	// opts: 锁实例的配置选项
	NewMutex(key string, opts ...MutexOption) Locker

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

// EtcdLocker 定义 etcd 锁实例接口。
// 扩展 Locker 接口，提供 etcd 特定功能。
type EtcdLocker interface {
	Locker

	// Mutex 返回底层 concurrency.Mutex。
	// 用于需要直接访问 etcd Mutex 的高级场景。
	Mutex() Mutex

	// Key 返回锁在 etcd 中的完整 key。
	Key() string
}

// RedisLocker 定义 Redis 锁实例接口。
// 扩展 Locker 接口，提供 Redis (redsync) 特定功能。
type RedisLocker interface {
	Locker

	// RedisMutex 返回底层 redsync.Mutex。
	// 用于需要直接访问 redsync Mutex 的高级场景。
	RedisMutex() RedisMutex

	// Value 返回锁的唯一值。
	// 用于调试和日志记录。
	Value() string

	// Until 返回锁的过期时间。
	Until() int64
}
