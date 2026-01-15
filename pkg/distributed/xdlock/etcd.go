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

// NewMutex 创建指定 key 的分布式锁实例。
func (f *etcdFactory) NewMutex(key string, opts ...MutexOption) Locker {
	options := defaultMutexOptions()
	for _, opt := range opts {
		opt(options)
	}

	fullKey := options.KeyPrefix + key
	mutex := concurrency.NewMutex(f.session, fullKey)

	return &etcdLocker{
		factory: f,
		mutex:   mutex,
		key:     fullKey,
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
// etcd 锁实现
// =============================================================================

// etcdLocker 实现 EtcdLocker 接口。
type etcdLocker struct {
	factory *etcdFactory
	mutex   *concurrency.Mutex
	key     string
	locked  atomic.Bool
}

// Lock 阻塞式获取锁。
func (l *etcdLocker) Lock(ctx context.Context) error {
	if err := l.checkSession(); err != nil {
		return err
	}

	if err := l.mutex.Lock(ctx); err != nil {
		return wrapEtcdError(err)
	}

	l.locked.Store(true)
	return nil
}

// TryLock 非阻塞式获取锁。
func (l *etcdLocker) TryLock(ctx context.Context) error {
	if err := l.checkSession(); err != nil {
		return err
	}

	if err := l.mutex.TryLock(ctx); err != nil {
		return wrapEtcdError(err)
	}

	l.locked.Store(true)
	return nil
}

// Unlock 释放锁。
func (l *etcdLocker) Unlock(ctx context.Context) error {
	// 先检查 session 是否有效
	if err := l.checkSession(); err != nil {
		// session 已过期，锁已自动释放
		l.locked.Store(false)
		return err
	}

	if !l.locked.Load() {
		return ErrNotLocked
	}

	if err := l.mutex.Unlock(ctx); err != nil {
		// 解锁失败可能是因为锁已过期，更新状态
		wrappedErr := wrapEtcdError(err)
		if errors.Is(wrappedErr, ErrSessionExpired) || errors.Is(wrappedErr, ErrNotLocked) {
			l.locked.Store(false)
		}
		return wrappedErr
	}

	l.locked.Store(false)
	return nil
}

// Extend etcd 使用 Session 自动续期，不支持手动续期。
func (l *etcdLocker) Extend(_ context.Context) error {
	return ErrExtendNotSupported
}

// Mutex 返回底层 concurrency.Mutex。
func (l *etcdLocker) Mutex() Mutex {
	return l.mutex
}

// Key 返回锁的完整 key。
func (l *etcdLocker) Key() string {
	return l.key
}

// checkSession 检查 Session 是否有效。
func (l *etcdLocker) checkSession() error {
	if l.factory.closed.Load() {
		return ErrFactoryClosed
	}
	select {
	case <-l.factory.session.Done():
		return ErrSessionExpired
	default:
		return nil
	}
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
