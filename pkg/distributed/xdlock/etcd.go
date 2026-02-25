package xdlock

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// 确保 etcdLockHandle 实现 LockHandle 接口。
var _ LockHandle = (*etcdLockHandle)(nil)

// =============================================================================
// etcd 工厂实现
// =============================================================================

// sessionProvider 定义 etcd factory 内部使用的 Session 操作。
// 生产环境使用 *concurrency.Session，内部测试使用 mock 实现。
type sessionProvider interface {
	Done() <-chan struct{}
	Close() error
}

// mutexUnlocker 定义 etcd handle 内部使用的 Mutex 解锁操作。
// 生产环境使用 *concurrency.Mutex，内部测试使用 mock 实现。
type mutexUnlocker interface {
	Unlock(ctx context.Context) error
}

// etcdFactory 实现 EtcdFactory 接口。
type etcdFactory struct {
	client  *clientv3.Client
	session *concurrency.Session
	sp      sessionProvider // checkSession/Close 使用，通常等于 session
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
		if opt != nil {
			opt(options)
		}
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
		sp:      session,
		options: options,
	}, nil
}

// TryLock 非阻塞式获取锁，返回 LockHandle。
//
// 设计决策: etcd 后端仅使用 KeyPrefix 选项，Redis 专用选项（Expiry、Tries 等）
// 被忽略，因为 etcd 的锁生命周期由 Session TTL 控制。
func (f *etcdFactory) TryLock(ctx context.Context, key string, opts ...MutexOption) (LockHandle, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if err := f.checkSession(); err != nil {
		return nil, err
	}
	if err := validateKey(key); err != nil {
		return nil, err
	}

	options := defaultMutexOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(options)
		}
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
		mu:      mutex,
		key:     fullKey,
	}, nil
}

// Lock 阻塞式获取锁，返回 LockHandle。
//
// 设计决策: etcd 后端仅使用 KeyPrefix 选项，Redis 专用选项被忽略。
func (f *etcdFactory) Lock(ctx context.Context, key string, opts ...MutexOption) (LockHandle, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if err := f.checkSession(); err != nil {
		return nil, err
	}
	if err := validateKey(key); err != nil {
		return nil, err
	}

	options := defaultMutexOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(options)
		}
	}

	fullKey := options.KeyPrefix + key
	mutex := concurrency.NewMutex(f.session, fullKey)

	if err := mutex.Lock(ctx); err != nil {
		return nil, wrapEtcdError(err)
	}

	return &etcdLockHandle{
		factory: f,
		mu:      mutex,
		key:     fullKey,
	}, nil
}

// checkSession 检查 Session 是否有效（内部方法）。
func (f *etcdFactory) checkSession() error {
	if f.closed.Load() {
		return ErrFactoryClosed
	}
	select {
	case <-f.sp.Done():
		return ErrSessionExpired
	default:
		return nil
	}
}

// Close 关闭工厂，释放 Session。
func (f *etcdFactory) Close(_ context.Context) error {
	if f.closed.Swap(true) {
		return nil // 已关闭
	}
	return f.sp.Close()
}

// Health 健康检查。
// 检查 Session 是否仍然有效。
func (f *etcdFactory) Health(ctx context.Context) error {
	if f.closed.Load() {
		return ErrFactoryClosed
	}

	// 检查 Session 是否已过期
	select {
	case <-f.sp.Done():
		return ErrSessionExpired
	default:
	}

	// 使用 Status API 验证连接，不依赖特定 key 的 RBAC 权限
	for _, ep := range f.client.Endpoints() {
		if _, err := f.client.Status(ctx, ep); err != nil {
			return fmt.Errorf("xdlock: health check endpoint %s: %w", ep, err)
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
	factory  *etcdFactory
	mu       mutexUnlocker // Unlock 使用，通常为 *concurrency.Mutex
	key      string
	unlocked atomic.Bool // 标记锁是否已被显式释放
}

// Unlock 释放锁。
//
// 设计决策: 允许在 factory 关闭后尝试解锁。factory.Close() 会关闭 Session
// 并撤销 Lease，锁已自动释放。即使解锁失败（Session 已关闭），也不会造成锁悬挂。
//
// 设计决策: 当调用方 ctx 已取消/超时时，使用独立清理上下文确保解锁尽力完成，
// 避免锁残留到 Lease TTL 到期（默认 60s）。
func (h *etcdLockHandle) Unlock(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}

	// 设计决策: 已解锁的 handle 直接返回 ErrNotLocked，避免向 etcd 发送无效请求。
	// 与 Extend 的 unlocked 检查保持对称。
	if h.unlocked.Load() {
		return ErrNotLocked
	}

	// 当业务 ctx 已取消/超时时，使用独立清理上下文确保解锁能完成
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), unlockTimeout)
		defer cancel()
	}

	if err := h.mu.Unlock(ctx); err != nil {
		return wrapEtcdError(err)
	}
	// 设计决策: unlocked 标记放在成功解锁之后，避免 Unlock 失败时 Extend 误判为
	// "锁已释放"。网络抖动时 Unlock 可能失败但锁仍由 Session KeepAlive 维持，
	// 此时 Extend 应继续报告锁状态正常，而非错误返回 ErrNotLocked。
	h.unlocked.Store(true)
	return nil
}

// Extend 检查 Session 健康状态和本地解锁标记。
//
// etcd 使用 Session 自动续期（keep-alive），无需手动续期。
// 此方法验证 Session 是否仍然有效且锁未被显式释放，
// 若 Session 已过期返回 [ErrSessionExpired]，若锁已释放返回 [ErrNotLocked]。
//
// 设计决策: 此方法不执行 etcd 远程所有权校验（revision/txn compare），
// 仅检查本地 unlocked 标记和 Session.Done() channel。在 Session 存活期间，
// Lease 保护 key 不被回收，因此本地检查已足够。外部直接删除 etcd key 属于
// 越过锁 API 的越权操作，不在检测范围内。
//
// 与 Redis 后端不同，etcd 的续期完全自动，调用此方法不会延长锁的有效期，
// 而是检查锁状态是否健康。推荐在长时间运行的任务中定期调用以检测锁状态。
func (h *etcdLockHandle) Extend(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}
	// 检查锁是否已被显式释放
	if h.unlocked.Load() {
		return ErrNotLocked
	}
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
