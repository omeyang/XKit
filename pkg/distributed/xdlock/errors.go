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
	//
	// 设计决策: 保持导出，因 xcron 等下游包在 mock 测试中依赖此错误。
	// 在正常使用中，TryLock 检测到此错误后返回 (nil, nil) 表示锁被占用，
	// 因此业务代码通常不会直接看到此错误，但可用于构建 mock/测试。
	ErrLockHeld = errors.New("xdlock: lock is held by another owner")

	// ErrLockFailed 获取锁失败。
	// 重试耗尽或其他获取锁失败的情况返回此错误。
	ErrLockFailed = errors.New("xdlock: failed to acquire lock")

	// ErrExtendFailed 续期失败。
	// Redis 锁续期失败时返回此错误。
	ErrExtendFailed = errors.New("xdlock: failed to extend lock")

	// ErrNilClient 客户端为空。
	// 传入 nil 客户端时返回此错误。
	ErrNilClient = errors.New("xdlock: client is nil")

	// ErrNilContext 上下文为空。
	// 传入 nil context 时返回此错误。
	ErrNilContext = errors.New("xdlock: context must not be nil")

	// ErrSessionExpired etcd Session 已过期。
	// Session 失效时返回此错误，需要重新创建 Factory。
	ErrSessionExpired = errors.New("xdlock: session expired")

	// ErrFactoryClosed 工厂已关闭。
	// 在已关闭的工厂上创建锁时返回此错误。
	ErrFactoryClosed = errors.New("xdlock: factory is closed")

	// ErrNotLocked 锁未被持有。
	// 尝试 Unlock 或 Extend 未持有的锁时返回此错误。
	ErrNotLocked = errors.New("xdlock: not locked")

	// ErrEmptyKey 锁 key 为空。
	// key 为空字符串或仅含空白时返回此错误。
	ErrEmptyKey = errors.New("xdlock: key must not be empty")

	// ErrKeyTooLong 锁 key 超过长度限制。
	// key 长度不能超过 maxKeyLength（512 字节）。
	ErrKeyTooLong = errors.New("xdlock: key exceeds maximum length of 512 bytes")

	// ErrNilConfig 配置为空。
	// 传入 nil 配置时返回此错误。
	ErrNilConfig = errors.New("xdlock: config is nil")

	// ErrNoEndpoints 未配置 endpoints。
	// etcd 配置中未提供 endpoints 时返回此错误。
	ErrNoEndpoints = errors.New("xdlock: no endpoints configured")
)

// 内部错误（不导出）。
//
// 设计决策: errLockExpired 仅在 wrapRedisError 中作为中间转换态使用，
// Unlock/Extend 会将其统一转为 ErrNotLocked 返回给调用方。
// 用户永远不会直接收到此错误，因此不导出，避免误导 API 消费者。
var errLockExpired = errors.New("xdlock: lock expired or stolen")
