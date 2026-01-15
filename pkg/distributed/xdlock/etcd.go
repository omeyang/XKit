package xdlock

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// =============================================================================
// etcd 工厂实现
// =============================================================================

// etcdFactory 实现 EtcdFactory 接口。
type etcdFactory struct {
	client  *clientv3.Client
	session *concurrency.Session
	options *etcdFactoryOptions
	closed  atomic.Bool
	mu      sync.RWMutex
}

// NewEtcdFactory 创建 etcd 锁工厂。
// client 必须是已初始化的 etcd 客户端。
func NewEtcdFactory(client *clientv3.Client, opts ...EtcdFactoryOption) (EtcdFactory, error) {
	if client == nil {
		return nil, ErrNilClient
	}

	options := defaultEtcdFactoryOptions()
	for _, opt := range opts {
		opt(options)
	}

	// 创建 Session
	session, err := concurrency.NewSession(
		client,
		concurrency.WithTTL(options.TTL),
		concurrency.WithContext(options.Context),
	)
	if err != nil {
		return nil, err
	}

	return &etcdFactory{
		client:  client,
		session: session,
		options: options,
	}, nil
}

// TryLock 非阻塞式获取锁，返回 LockHandle。
func (f *etcdFactory) TryLock(ctx context.Context, key string, opts ...MutexOption) (LockHandle, error) {
	if err := f.checkSession(); err != nil {
		return nil, err
	}

	options := defaultMutexOptions()
	for _, opt := range opts {
		opt(options)
	}

	fullKey := options.KeyPrefix + key
	mutex := concurrency.NewMutex(f.session, fullKey)

	if err := mutex.TryLock(ctx); err != nil {
		err = wrapEtcdError(err)
		if errors.Is(err, ErrLockHeld) {
			return nil, nil // 锁被占用，返回 (nil, nil)
		}
		return nil, err
	}

	return &etcdLockHandle{
		factory: f,
		mutex:   mutex,
		key:     key,
	}, nil
}

// Lock 阻塞式获取锁，返回 LockHandle。
func (f *etcdFactory) Lock(ctx context.Context, key string, opts ...MutexOption) (LockHandle, error) {
	if err := f.checkSession(); err != nil {
		return nil, err
	}

	options := defaultMutexOptions()
	for _, opt := range opts {
		opt(options)
	}

	fullKey := options.KeyPrefix + key
	mutex := concurrency.NewMutex(f.session, fullKey)

	if err := mutex.Lock(ctx); err != nil {
		return nil, wrapEtcdError(err)
	}

	return &etcdLockHandle{
		factory: f,
		mutex:   mutex,
		key:     key,
	}, nil
}

// checkSession 检查 Session 是否有效（内部方法）。
func (f *etcdFactory) checkSession() error {
	if f.closed.Load() {
		return ErrFactoryClosed
	}
	select {
	case <-f.session.Done():
		return ErrSessionExpired
	default:
		return nil
	}
}

// Close 关闭工厂，释放 Session。
func (f *etcdFactory) Close() error {
	if f.closed.Swap(true) {
		return nil // 已关闭
	}
	return f.session.Close()
}

// Health 健康检查。
// 检查 Session 是否仍然有效。
func (f *etcdFactory) Health(ctx context.Context) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.closed.Load() {
		return ErrFactoryClosed
	}

	// 检查 Session 是否已过期
	select {
	case <-f.session.Done():
		return ErrSessionExpired
	default:
	}

	// 尝试执行一个简单的 Get 操作验证连接
	_, err := f.client.Get(ctx, "health-check-key", clientv3.WithLimit(1))
	return err
}

// Session 返回底层 concurrency.Session。
func (f *etcdFactory) Session() Session {
	return f.session
}

// =============================================================================
// etcd LockHandle 实现
// =============================================================================

// etcdLockHandle 实现 LockHandle 接口。
// 每次成功获取锁时创建，封装了唯一的锁标识。
type etcdLockHandle struct {
	factory *etcdFactory
	mutex   *concurrency.Mutex
	key     string
}

// Unlock 释放锁。
func (h *etcdLockHandle) Unlock(ctx context.Context) error {
	if err := h.factory.checkSession(); err != nil {
		return err
	}

	if err := h.mutex.Unlock(ctx); err != nil {
		return wrapEtcdError(err)
	}
	return nil
}

// Extend etcd 使用 Session 自动续期，此方法返回 nil。
// etcd 的 Session 会自动保持心跳，无需手动续期。
func (h *etcdLockHandle) Extend(_ context.Context) error {
	// etcd 使用 Session 自动续期，无需手动操作
	return nil
}

// Key 返回锁的 key。
func (h *etcdLockHandle) Key() string {
	return h.key
}

// =============================================================================
// 错误转换
// =============================================================================

// wrapEtcdError 将 etcd 错误转换为 xdlock 错误。
func wrapEtcdError(err error) error {
	if err == nil {
		return nil
	}

	// etcd concurrency 包的错误
	if errors.Is(err, concurrency.ErrLocked) {
		return ErrLockHeld
	}
	if errors.Is(err, concurrency.ErrSessionExpired) {
		return ErrSessionExpired
	}
	if errors.Is(err, concurrency.ErrLockReleased) {
		return ErrNotLocked
	}

	// context 错误保持原样
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	return err
}
