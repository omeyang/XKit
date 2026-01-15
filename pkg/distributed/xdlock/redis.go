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

// TryLock 非阻塞式获取锁，返回 LockHandle。
func (f *redisFactory) TryLock(ctx context.Context, key string, opts ...MutexOption) (LockHandle, error) {
	if f.closed.Load() {
		return nil, ErrFactoryClosed
	}

	mutex, fullKey := f.createMutex(key, opts...)

	if err := mutex.TryLockContext(ctx); err != nil {
		err = wrapRedisError(err)
		if errors.Is(err, ErrLockHeld) {
			return nil, nil // 锁被占用，返回 (nil, nil)
		}
		return nil, err
	}

	return &redisLockHandle{
		factory: f,
		mutex:   mutex,
		key:     fullKey,
	}, nil
}

// Lock 阻塞式获取锁，返回 LockHandle。
func (f *redisFactory) Lock(ctx context.Context, key string, opts ...MutexOption) (LockHandle, error) {
	if f.closed.Load() {
		return nil, ErrFactoryClosed
	}

	mutex, fullKey := f.createMutex(key, opts...)

	if err := mutex.LockContext(ctx); err != nil {
		// redsync 不会传递 context 错误，需要单独检查
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, wrapRedisError(err)
	}

	return &redisLockHandle{
		factory: f,
		mutex:   mutex,
		key:     fullKey,
	}, nil
}

// createMutex 创建 redsync.Mutex（内部方法）。
// 返回 mutex 和完整的 key（包含前缀）。
func (f *redisFactory) createMutex(key string, opts ...MutexOption) (*redsync.Mutex, string) {
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

	return f.rs.NewMutex(fullKey, rsOpts...), fullKey
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
// Redis LockHandle 实现
// =============================================================================

// redisLockHandle 实现 LockHandle 接口。
// 每次成功获取锁时创建，封装了唯一的锁标识。
type redisLockHandle struct {
	factory *redisFactory
	mutex   *redsync.Mutex
	key     string
}

// Unlock 释放锁。
func (h *redisLockHandle) Unlock(ctx context.Context) error {
	if h.factory.closed.Load() {
		return ErrFactoryClosed
	}

	ok, err := h.mutex.UnlockContext(ctx)
	if err != nil {
		wrappedErr := wrapRedisError(err)
		// 锁过期也视为"未持有锁"
		if errors.Is(wrappedErr, ErrLockExpired) {
			return ErrLockNotHeld
		}
		return wrappedErr
	}
	if !ok {
		return ErrLockNotHeld
	}
	return nil
}

// Extend 续期锁。
func (h *redisLockHandle) Extend(ctx context.Context) error {
	if h.factory.closed.Load() {
		return ErrFactoryClosed
	}

	ok, err := h.mutex.ExtendContext(ctx)
	if err != nil {
		wrappedErr := wrapRedisError(err)
		// 续期失败视为"未持有锁"
		if errors.Is(wrappedErr, ErrExtendFailed) || errors.Is(wrappedErr, ErrLockExpired) {
			return ErrLockNotHeld
		}
		return wrappedErr
	}
	if !ok {
		return ErrLockNotHeld
	}
	return nil
}

// Key 返回锁的 key。
func (h *redisLockHandle) Key() string {
	return h.key
}

// =============================================================================
// 错误转换
// =============================================================================

// wrapRedisError 将 redsync 错误转换为 xdlock 错误。
func wrapRedisError(err error) error {
	if err == nil {
		return nil
	}

	// context 错误优先保持原样（用于取消和超时场景）
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	// ErrTaken 是一个结构体类型，需要使用 errors.As 检查
	var errTaken *redsync.ErrTaken
	if errors.As(err, &errTaken) {
		return ErrLockHeld
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

	return err
}
