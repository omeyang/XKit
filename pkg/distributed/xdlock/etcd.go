package xdlock

import (
	"context"
	"errors"
	"fmt"
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
		return nil, fmt.Errorf("xdlock: create etcd session: %w", err)
	}

	return &etcdFactory{
		client:  client,
		session: session,
		options: options,
	}, nil
}

// TryLock 非阻塞式获取锁，返回 LockHandle。
//
// 设计决策: etcd 后端仅使用 KeyPrefix 选项，Redis 专用选项（Expiry、Tries 等）
// 被忽略，因为 etcd 的锁生命周期由 Session TTL 控制。
func (f *etcdFactory) TryLock(ctx context.Context, key string, opts ...MutexOption) (LockHandle, error) {
	if err := f.checkSession(); err != nil {
		return nil, err
	}
	if err := validateKey(key); err != nil {
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
		key:     fullKey,
	}, nil
}

// Lock 阻塞式获取锁，返回 LockHandle。
//
// 设计决策: etcd 后端仅使用 KeyPrefix 选项，Redis 专用选项被忽略。
func (f *etcdFactory) Lock(ctx context.Context, key string, opts ...MutexOption) (LockHandle, error) {
	if err := f.checkSession(); err != nil {
		return nil, err
	}
	if err := validateKey(key); err != nil {
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
		key:     fullKey,
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
	if f.closed.Load() {
		return ErrFactoryClosed
	}

	// 检查 Session 是否已过期
	select {
	case <-f.session.Done():
		return ErrSessionExpired
	default:
	}

	// 使用 Status API 验证连接，不依赖特定 key 的 RBAC 权限
	for _, ep := range f.client.Endpoints() {
		if _, err := f.client.Status(ctx, ep); err != nil {
			return err
		}
	}
	return nil
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
//
// 设计决策: 允许在 factory 关闭后尝试解锁。factory.Close() 会关闭 Session
// 并撤销 Lease，锁已自动释放。即使解锁失败（Session 已关闭），也不会造成锁悬挂。
func (h *etcdLockHandle) Unlock(ctx context.Context) error {
	if err := h.mutex.Unlock(ctx); err != nil {
		return wrapEtcdError(err)
	}
	return nil
}

// Extend 检查 Session 状态。
//
// etcd 使用 Session 自动续期（keep-alive），无需手动续期。
// 此方法仅验证 Session 是否仍然有效，若已过期返回 ErrSessionExpired。
//
// 与 Redis 后端不同，etcd 的续期完全自动，调用此方法不会延长锁的有效期，
// 而是检查 Session 是否健康。推荐在长时间运行的任务中定期调用以检测 Session 状态。
func (h *etcdLockHandle) Extend(_ context.Context) error {
	// 检查 Session 是否仍然有效
	if err := h.factory.checkSession(); err != nil {
		return err
	}
	return nil
}

// Key 返回锁的 key。
func (h *etcdLockHandle) Key() string {
	return h.key
}

// =============================================================================
// 错误转换
// =============================================================================

// wrapEtcdError 将 etcd 错误转换为 xdlock 错误，保留原始错误链。
func wrapEtcdError(err error) error {
	if err == nil {
		return nil
	}

	// context 错误保持原样
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	// etcd concurrency 包的错误，使用双 %w 保留原始错误链
	if errors.Is(err, concurrency.ErrLocked) {
		return fmt.Errorf("%w: %w", ErrLockHeld, err)
	}
	if errors.Is(err, concurrency.ErrSessionExpired) {
		return fmt.Errorf("%w: %w", ErrSessionExpired, err)
	}
	if errors.Is(err, concurrency.ErrLockReleased) {
		return fmt.Errorf("%w: %w", ErrNotLocked, err)
	}

	return err
}
