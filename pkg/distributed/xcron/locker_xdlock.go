package xcron

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/omeyang/xkit/pkg/distributed/xdlock"
)

// XdlockAdapter 将 xdlock.Factory 适配为 xcron.Locker 接口。
//
// 使用此适配器可以复用 xdlock 包提供的分布式锁实现（etcd、Redis Redlock）。
// xdlock 提供了更丰富的配置选项和更健壮的实现。
//
// # 使用场景
//
//   - 需要使用 etcd 分布式锁（xdlock 的 etcd 实现有自动续期能力）
//   - 需要使用 Redis Redlock 算法（多节点高可用）
//   - 已有 xdlock.Factory 实例，希望复用
//
// # etcd vs Redis
//
//   - etcd: 使用 Session 自动续期，适合长时间任务，无需手动续期
//   - Redis: 需要手动调用 Renew 续期，通过 xcron 的续期机制自动处理
//
// # 示例
//
// etcd:
//
//	client, _ := clientv3.New(clientv3.Config{Endpoints: []string{"localhost:2379"}})
//	factory, _ := xdlock.NewEtcdFactory(client, xdlock.WithEtcdTTL(30))
//	adapter := xcron.NewXdlockAdapter(factory)
//	scheduler := xcron.New(xcron.WithLocker(adapter))
//
// Redis:
//
//	pool := goredis.NewPool(redisClient)
//	factory := xdlock.NewRedisFactory(pool)
//	adapter := xcron.NewXdlockAdapter(factory)
//	scheduler := xcron.New(xcron.WithLocker(adapter))
type XdlockAdapter struct {
	factory   xdlock.Factory
	keyPrefix string
}

// XdlockAdapterOption 配置选项
type XdlockAdapterOption func(*XdlockAdapter)

// WithXdlockKeyPrefix 设置锁 key 的前缀。
// 默认值："xcron:"。
func WithXdlockKeyPrefix(prefix string) XdlockAdapterOption {
	return func(a *XdlockAdapter) {
		a.keyPrefix = prefix
	}
}

// ErrNilFactory 表示 xdlock.Factory 为 nil。
var ErrNilFactory = errors.New("xcron: xdlock factory cannot be nil")

// NewXdlockAdapter 创建 xdlock 适配器。
//
// factory 是 xdlock 的工厂实例，可以是：
//   - xdlock.NewEtcdFactory() 创建的 etcd 工厂
//   - xdlock.NewRedisFactory() 创建的 Redis 工厂
//
// 如果 factory 为 nil，返回 [ErrNilFactory]。
// 调用者负责在不需要时关闭 factory。
func NewXdlockAdapter(factory xdlock.Factory, opts ...XdlockAdapterOption) (*XdlockAdapter, error) {
	if factory == nil {
		return nil, ErrNilFactory
	}
	a := &XdlockAdapter{
		factory:   factory,
		keyPrefix: "xcron:",
	}
	for _, opt := range opts {
		opt(a)
	}
	return a, nil
}

// TryLock 尝试获取锁（非阻塞）。
//
// 使用 xdlock.Factory 的新 handle-based API 直接获取锁。
//
// 对于 etcd 后端，ttl 参数会被忽略，因为 etcd 使用 Session TTL。
// 对于 Redis 后端，ttl 将作为锁的过期时间。
func (a *XdlockAdapter) TryLock(ctx context.Context, key string, ttl time.Duration) (LockHandle, error) {
	fullKey := a.keyPrefix + key

	// 配置选项
	mutexOpts := []xdlock.MutexOption{
		xdlock.WithKeyPrefix(""), // 已在 fullKey 中包含前缀
	}
	if ttl > 0 {
		mutexOpts = append(mutexOpts, xdlock.WithExpiry(ttl))
	}

	// 直接使用 Factory 的 TryLock 方法
	handle, err := a.factory.TryLock(ctx, fullKey, mutexOpts...)
	if err != nil {
		return nil, fmt.Errorf("xcron: xdlock try lock failed: %w", err)
	}
	if handle == nil {
		return nil, nil // 锁被占用
	}

	return &xdlockHandle{
		handle: handle,
		key:    key,
	}, nil
}

// Factory 返回底层的 xdlock.Factory。
// 用于需要直接访问工厂的高级场景。
func (a *XdlockAdapter) Factory() xdlock.Factory {
	return a.factory
}

// xdlockHandle 包装 xdlock.LockHandle，实现 xcron.LockHandle 接口
type xdlockHandle struct {
	handle xdlock.LockHandle
	key    string
}

// Unlock 释放锁。
func (h *xdlockHandle) Unlock(ctx context.Context) error {
	err := h.handle.Unlock(ctx)
	if err != nil {
		// 转换 xdlock 错误为 xcron 错误
		if errors.Is(err, xdlock.ErrNotLocked) {
			return ErrLockNotHeld
		}
		return fmt.Errorf("xcron: xdlock unlock failed: %w", err)
	}
	return nil
}

// Renew 续期锁。
//
// 对于 etcd 后端，此操作返回 nil（etcd 使用 Session 自动续期）。
// 对于 Redis 后端，调用 Extend 续期。
//
// 设计决策: ttl 参数被忽略，因为 xdlock.LockHandle.Extend 使用工厂创建时的 TTL 续期，
// 不支持按次指定。若需自定义续期 TTL，请在创建 xdlock.Factory 时配置。
func (h *xdlockHandle) Renew(ctx context.Context, _ time.Duration) error {
	err := h.handle.Extend(ctx)
	if err != nil {
		// 转换 xdlock 错误为 xcron 错误
		if errors.Is(err, xdlock.ErrNotLocked) || errors.Is(err, xdlock.ErrExtendFailed) {
			return ErrLockNotHeld
		}
		return fmt.Errorf("xcron: xdlock renew failed: %w", err)
	}
	return nil
}

// Key 返回锁的 key。
func (h *xdlockHandle) Key() string {
	return h.key
}

// 确保 XdlockAdapter 实现了 Locker 接口
var _ Locker = (*XdlockAdapter)(nil)

// 确保 xdlockHandle 实现了 LockHandle 接口
var _ LockHandle = (*xdlockHandle)(nil)
