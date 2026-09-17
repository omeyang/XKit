package xcache

import "errors"

// =============================================================================
// 通用错误
// =============================================================================

var (
	// ErrNilClient 表示传入的客户端为 nil。
	ErrNilClient = errors.New("xcache: nil client")

	// ErrNilContext 表示传入的 context 为 nil。
	// go-redis 内部会直接使用 ctx 而不做 nil 检查，传入 nil 会导致 panic。
	// 所有接受 context.Context 的公开入口方法（Load, LoadHash, Lock）
	// 均在入口处进行 fail-fast 检查。
	ErrNilContext = errors.New("xcache: nil context")

	// ErrClosed 表示缓存实例已关闭。
	// 在调用 Close() 后继续使用缓存实例会返回此错误。
	ErrClosed = errors.New("xcache: closed")
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
	// ErrMetricsDisabled 表示未启用缓存统计信息。
	ErrMetricsDisabled = errors.New("xcache: metrics disabled")
)

// =============================================================================
// Loader 相关错误
// =============================================================================

var (
	// ErrEmptyKey 表示传入的 key 为空字符串。
	// 空字符串 key 在 Redis 中合法但几乎总是使用错误，应在入口处 fail-fast。
	ErrEmptyKey = errors.New("xcache: empty key")

	// ErrNilLoader 表示 loader 函数为 nil。
	ErrNilLoader = errors.New("xcache: nil loader function")

	// ErrInvalidConfig 表示配置参数无效。
	// 这是一个配置错误，应该在开发阶段修复，不应被静默忽略。
	// 当分布式锁返回此类错误时，会直接传递给调用方而非降级处理。
	ErrInvalidConfig = errors.New("xcache: invalid configuration")

	// errUnexpectedResultType 表示 singleflight 返回了非预期的结果类型。
	// 此错误正常不可达，仅在内部类型系统被破坏时触发，因此不导出。
	errUnexpectedResultType = errors.New("xcache: unexpected result type from singleflight")

	// ErrLoadPanic 表示 loadFn（用户提供的回源函数）发生了 panic。
	// 设计决策: 在 singleflight DoChan 模式下，loadFn 的 panic 会被
	// singleflight 捕获后在新 goroutine 中 re-panic，导致进程级崩溃。
	// xcache 通过 recover 将 panic 转为此错误，保护进程不被用户代码拖垮。
	ErrLoadPanic = errors.New("xcache: load function panicked")
)
