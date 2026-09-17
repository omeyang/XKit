package xdbg

// Leveler 日志级别控制器接口。
// 此接口与 xlog.Leveler 兼容。
type Leveler interface {
	// Level 返回当前日志级别。
	Level() string

	// SetLevel 设置日志级别。
	// 支持的级别: trace, debug, info, warn, error
	SetLevel(level string) error
}

// BreakerInfo 熔断器信息。
type BreakerInfo struct {
	// Name 熔断器名称。
	Name string

	// State 熔断器状态（Closed, Open, HalfOpen）。
	State string

	// Requests 总请求数。
	Requests uint64

	// TotalSuccesses 成功数。
	TotalSuccesses uint64

	// TotalFailures 失败数。
	TotalFailures uint64

	// ConsecutiveSuccesses 连续成功数。
	ConsecutiveSuccesses uint64

	// ConsecutiveFailures 连续失败数。
	ConsecutiveFailures uint64
}

// BreakerRegistry 熔断器注册表接口。
// 此接口与 xbreaker 兼容。
type BreakerRegistry interface {
	// List 返回所有熔断器名称。
	List() []string

	// Get 获取熔断器信息。
	Get(name string) (*BreakerInfo, bool)

	// Reset 重置熔断器状态。
	Reset(name string) error
}

// LimiterInfo 限流器信息。
type LimiterInfo struct {
	// Name 限流器名称。
	Name string

	// Type 限流器类型。
	Type string

	// Limit 限流配额。
	Limit int64

	// Remaining 剩余配额。
	Remaining int64

	// Reset 重置时间（Unix 时间戳）。
	Reset int64
}

// LimiterRegistry 限流器注册表接口。
// 此接口与 xlimit 兼容。
type LimiterRegistry interface {
	// List 返回所有限流器名称。
	List() []string

	// Get 获取限流器信息。
	Get(name string) (*LimiterInfo, bool)
}

// CacheStats 缓存统计信息。
type CacheStats struct {
	// Name 缓存名称。
	Name string

	// Type 缓存类型。
	Type string

	// Hits 命中次数。
	Hits uint64

	// Misses 未命中次数。
	Misses uint64

	// Size 当前大小。
	Size int64

	// MaxSize 最大大小。
	MaxSize int64
}

// CacheRegistry 缓存注册表接口。
// 此接口与 xcache 兼容。
type CacheRegistry interface {
	// List 返回所有缓存名称。
	List() []string

	// Get 获取缓存统计。
	Get(name string) (*CacheStats, bool)
}

// ConfigProvider 配置提供者接口。
// 此接口与 xconf 兼容。
//
// 安全警告: Dump 返回的配置会通过 config 命令输出。
// 实现方有责任在 Dump 中对敏感字段（密码、Token、DSN 等）进行脱敏处理，
// 框架层不会自动过滤。如果配置中包含敏感信息，建议：
//   - 在 Dump 实现中过滤或掩码敏感字段
//   - 或使用 WithCommandWhitelist 禁用 config 命令
type ConfigProvider interface {
	// Dump 导出当前配置。
	// 实现方应确保返回值不包含敏感信息（密码、密钥等）。
	Dump() map[string]any
}
