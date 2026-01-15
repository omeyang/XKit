package xdlock

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/go-redsync/redsync/v4"
	rsredis "github.com/go-redsync/redsync/v4/redis"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Redis 工厂实现
// =============================================================================

// redisFactory 实现 RedisFactory 接口。
type redisFactory struct {
	clients []redis.UniversalClient
	rs      *redsync.Redsync
	pools   []rsredis.Pool
	closed  atomic.Bool
	mu      sync.RWMutex
}

// NewRedisFactory 创建 Redis 锁工厂。
// 单节点为标准 Redis 锁；多节点使用 Redlock 算法（需过半成功）。
func NewRedisFactory(clients ...redis.UniversalClient) (RedisFactory, error) {
	if len(clients) == 0 {
		return nil, ErrNilClient
	}

	for i, client := range clients {
		if client == nil {
			return nil, errors.Join(ErrNilClient, errors.New("client at index "+strconv.Itoa(i)+" is nil"))
		}
	}

	// 创建 redsync Pool 列表
	pools := make([]rsredis.Pool, len(clients))
	for i, client := range clients {
		pools[i] = goredis.NewPool(client)
	}

	// 创建 Redsync 实例
	rs := redsync.New(pools...)

	return &redisFactory{
		clients: clients,
		rs:      rs,
		pools:   pools,
	}, nil
}

// NewMutex 创建指定 key 的分布式锁实例。
func (f *redisFactory) NewMutex(key string, opts ...MutexOption) Locker {
	options := defaultMutexOptions()
	for _, opt := range opts {
		opt(options)
	}

	fullKey := options.KeyPrefix + key

	// 构建 redsync 选项
	rsOpts := make([]redsync.Option, 0, 10)
	rsOpts = append(rsOpts, redsync.WithExpiry(options.Expiry))
	rsOpts = append(rsOpts, redsync.WithTries(options.Tries))
	rsOpts = append(rsOpts, redsync.WithRetryDelay(options.RetryDelay))

	if options.RetryDelayFunc != nil {
		rsOpts = append(rsOpts, redsync.WithRetryDelayFunc(
			redsync.DelayFunc(options.RetryDelayFunc),
		))
	}
	rsOpts = append(rsOpts, redsync.WithDriftFactor(options.DriftFactor))
	rsOpts = append(rsOpts, redsync.WithTimeoutFactor(options.TimeoutFactor))
	if options.GenValueFunc != nil {
		rsOpts = append(rsOpts, redsync.WithGenValueFunc(options.GenValueFunc))
	}
	rsOpts = append(rsOpts, redsync.WithFailFast(options.FailFast))
	rsOpts = append(rsOpts, redsync.WithShufflePools(options.ShufflePools))
	if options.SetNXOnExtend {
		rsOpts = append(rsOpts, redsync.WithSetNXOnExtend())
	}

	mutex := f.rs.NewMutex(fullKey, rsOpts...)

	return &redisLocker{
		factory: f,
		mutex:   mutex,
		key:     fullKey,
	}
}

// Close 关闭工厂。
// 注意：此方法不会关闭传入的 Redis 客户端，客户端的生命周期由调用者管理。
func (f *redisFactory) Close() error {
	if f.closed.Swap(true) {
		return nil // 已关闭
	}
	// redsync 没有需要关闭的资源
	// Redis 客户端由调用者管理
	return nil
}

// Health 健康检查。
// 对所有 Redis 节点执行 PING 命令。
func (f *redisFactory) Health(ctx context.Context) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.closed.Load() {
		return ErrFactoryClosed
	}

	// 检查所有节点
	for _, client := range f.clients {
		if err := client.Ping(ctx).Err(); err != nil {
			return err
		}
	}

	return nil
}

// Redsync 返回底层 redsync.Redsync 实例。
func (f *redisFactory) Redsync() Redsync {
	return f.rs
}

// =============================================================================
// Redis 锁实现
// =============================================================================

// redisLocker 实现 RedisLocker 接口。
type redisLocker struct {
	factory *redisFactory
	mutex   *redsync.Mutex
	key     string
	locked  atomic.Bool
}

// Lock 阻塞式获取锁。
// 会根据配置的重试次数和延迟进行重试。
func (l *redisLocker) Lock(ctx context.Context) error {
	if l.factory.closed.Load() {
		return ErrFactoryClosed
	}

	if err := l.mutex.LockContext(ctx); err != nil {
		return wrapRedisError(err)
	}

	l.locked.Store(true)
	return nil
}

// TryLock 非阻塞式获取锁。
// 只尝试一次，失败立即返回。
func (l *redisLocker) TryLock(ctx context.Context) error {
	if l.factory.closed.Load() {
		return ErrFactoryClosed
	}

	if err := l.mutex.TryLockContext(ctx); err != nil {
		return wrapRedisError(err)
	}

	l.locked.Store(true)
	return nil
}

// Unlock 释放锁。
func (l *redisLocker) Unlock(ctx context.Context) error {
	if l.factory.closed.Load() {
		l.locked.Store(false)
		return ErrFactoryClosed
	}

	if !l.locked.Load() {
		return ErrNotLocked
	}

	ok, err := l.mutex.UnlockContext(ctx)
	if err != nil {
		// 解锁失败可能是因为锁已过期，更新状态
		wrappedErr := wrapRedisError(err)
		if errors.Is(wrappedErr, ErrLockExpired) || errors.Is(wrappedErr, ErrLockFailed) {
			l.locked.Store(false)
		}
		return wrappedErr
	}
	if !ok {
		// 锁已过期，更新状态
		l.locked.Store(false)
		return ErrLockExpired
	}

	l.locked.Store(false)
	return nil
}

// Extend 续期锁。
// 延长锁的有效期，适用于长时间运行的任务。
func (l *redisLocker) Extend(ctx context.Context) error {
	if l.factory.closed.Load() {
		return ErrFactoryClosed
	}

	if !l.locked.Load() {
		return ErrNotLocked
	}

	ok, err := l.mutex.ExtendContext(ctx)
	if err != nil {
		wrappedErr := wrapRedisError(err)
		// 续期失败可能是因为锁已过期
		if errors.Is(wrappedErr, ErrLockExpired) || errors.Is(wrappedErr, ErrExtendFailed) {
			l.locked.Store(false)
		}
		return wrappedErr
	}
	if !ok {
		// 续期失败，锁可能已过期
		l.locked.Store(false)
		return ErrExtendFailed
	}

	return nil
}

// RedisMutex 返回底层 redsync.Mutex。
func (l *redisLocker) RedisMutex() RedisMutex {
	return l.mutex
}

// Value 返回锁的唯一值。
func (l *redisLocker) Value() string {
	return l.mutex.Value()
}

// Until 返回锁的过期时间（Unix 时间戳，毫秒）。
func (l *redisLocker) Until() int64 {
	return l.mutex.Until().UnixMilli()
}

// =============================================================================
// 错误转换
// =============================================================================

// wrapRedisError 将 redsync 错误转换为 xdlock 错误。
func wrapRedisError(err error) error {
	if err == nil {
		return nil
	}

	// redsync 错误
	if errors.Is(err, redsync.ErrFailed) {
		return ErrLockFailed
	}
	if errors.Is(err, redsync.ErrExtendFailed) {
		return ErrExtendFailed
	}
	if errors.Is(err, redsync.ErrLockAlreadyExpired) {
		return ErrLockExpired
	}

	// ErrTaken 是一个结构体类型，需要使用 errors.As 检查
	var errTaken *redsync.ErrTaken
	if errors.As(err, &errTaken) {
		return ErrLockHeld
	}

	// context 错误保持原样
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	return err
}
