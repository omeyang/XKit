package xsemaphore

import "time"

// =============================================================================
// 默认配置常量
// =============================================================================

const (
	// DefaultKeyPrefix 默认 Redis 键前缀
	DefaultKeyPrefix = "xsemaphore:"

	// DefaultCapacity 默认全局容量
	DefaultCapacity = 1

	// DefaultTTL 默认许可过期时间
	DefaultTTL = 5 * time.Minute

	// DefaultMaxRetries Acquire 默认最大尝试次数（首次尝试 + 重试）
	DefaultMaxRetries = 10

	// DefaultRetryDelay Acquire 默认重试间隔
	DefaultRetryDelay = 100 * time.Millisecond

	// DefaultPodCount 默认 Pod 数量
	DefaultPodCount = 1
)

// =============================================================================
// 内部常量
// =============================================================================

const (
	// keyTTLMargin Redis 键过期时间余量
	// 键的过期时间 = 许可过期时间 + 此余量，确保许可过期后键不会立即删除
	keyTTLMargin = 60 * time.Second

	// autoExtendTimeout 自动续租的超时时间
	autoExtendTimeout = 10 * time.Second

	// localCleanupInterval 本地信号量后台清理间隔
	localCleanupInterval = 30 * time.Second

	// fallbackCallbackMinInterval onFallback 回调的最小触发间隔
	// 在 Redis 故障风暴期间，限制回调频率，避免下游雪崩
	fallbackCallbackMinInterval = 10 * time.Second
)

// =============================================================================
// 仪表化版本（Metrics + Trace 共享）
// =============================================================================

const (
	// instrumentationVersion 仪表化版本号
	instrumentationVersion = "1.0.0"
)

// =============================================================================
// 信号量类型标识（用于指标）
// =============================================================================

const (
	// SemaphoreTypeDistributed 分布式信号量
	SemaphoreTypeDistributed = "distributed"

	// SemaphoreTypeLocal 本地信号量
	SemaphoreTypeLocal = "local"
)
