package xcache

import "errors"

// =============================================================================
// 通用错误
// =============================================================================

var (
	// ErrNilClient 表示传入的客户端为 nil。
	ErrNilClient = errors.New("xcache: nil client")

	// ErrClosed 表示缓存已关闭。
	ErrClosed = errors.New("xcache: cache closed")

	// ErrKeyNotFound 表示 key 不存在。
	ErrKeyNotFound = errors.New("xcache: key not found")

	// ErrFieldNotFound 表示 Hash field 不存在。
	ErrFieldNotFound = errors.New("xcache: field not found")
)

// =============================================================================
// Redis 相关错误
// =============================================================================

var (
	// ErrLockFailed 表示获取分布式锁失败。
	ErrLockFailed = errors.New("xcache: failed to acquire lock")

	// ErrLockExpired 表示分布式锁已过期或被其他持有者抢走。
	ErrLockExpired = errors.New("xcache: lock expired or stolen")

	// ErrInvalidLockTTL 表示锁的 TTL 无效。
	ErrInvalidLockTTL = errors.New("xcache: lock TTL must be positive")
)

// =============================================================================
// Memory 相关错误
// =============================================================================

var (
	// ErrMemoryFull 表示内存缓存已满，无法写入。
	ErrMemoryFull = errors.New("xcache: memory cache full")

	// ErrInvalidCost 表示 cost 参数非法。
	ErrInvalidCost = errors.New("xcache: invalid cost")

	// ErrMetricsDisabled 表示未启用缓存统计信息。
	ErrMetricsDisabled = errors.New("xcache: metrics disabled")
)

// =============================================================================
// Loader 相关错误
// =============================================================================

var (
	// ErrNilLoader 表示 loader 函数为 nil。
	ErrNilLoader = errors.New("xcache: nil loader function")

	// ErrInvalidConfig 表示配置参数无效。
	// 这是一个配置错误，应该在开发阶段修复，不应被静默忽略。
	// 当分布式锁返回此类错误时，会直接传递给调用方而非降级处理。
	ErrInvalidConfig = errors.New("xcache: invalid configuration")
)
