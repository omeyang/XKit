package xdlock

import "errors"

// 预定义错误。
// 使用 errors.Is 进行错误匹配，例如：
//
//	if errors.Is(err, xdlock.ErrLockHeld) {
//	    // 锁被占用
//	}
var (
	// ErrLockHeld 锁被其他持有者占用。
	// TryLock 失败时返回此错误。
	ErrLockHeld = errors.New("xdlock: lock is held by another owner")

	// ErrLockFailed 获取锁失败。
	// 重试耗尽或其他获取锁失败的情况返回此错误。
	ErrLockFailed = errors.New("xdlock: failed to acquire lock")

	// ErrLockExpired 锁已过期或被其他持有者抢走。
	// 解锁时发现锁已不属于当前持有者返回此错误。
	ErrLockExpired = errors.New("xdlock: lock expired or stolen")

	// ErrExtendFailed 续期失败。
	// Redis 锁续期失败时返回此错误。
	ErrExtendFailed = errors.New("xdlock: failed to extend lock")

	// ErrExtendNotSupported 后端不支持手动续期。
	// etcd 使用 Session 自动续期，调用 Extend() 返回此错误。
	ErrExtendNotSupported = errors.New("xdlock: extend not supported by backend")

	// ErrNilClient 客户端为空。
	// 传入 nil 客户端时返回此错误。
	ErrNilClient = errors.New("xdlock: client is nil")

	// ErrSessionExpired etcd Session 已过期。
	// Session 失效时返回此错误，需要重新创建 Factory。
	ErrSessionExpired = errors.New("xdlock: session expired")

	// ErrFactoryClosed 工厂已关闭。
	// 在已关闭的工厂上创建锁时返回此错误。
	ErrFactoryClosed = errors.New("xdlock: factory is closed")

	// ErrNotLocked 锁未被持有。
	// 尝试 Unlock 或 Extend 未持有的锁时返回此错误。
	ErrNotLocked = errors.New("xdlock: not locked")

	// ErrNilConfig 配置为空。
	// 传入 nil 配置时返回此错误。
	ErrNilConfig = errors.New("xdlock: config is nil")

	// ErrNoEndpoints 未配置 endpoints。
	// etcd 配置中未提供 endpoints 时返回此错误。
	ErrNoEndpoints = errors.New("xdlock: no endpoints configured")
)
