package xdbg

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// 默认配置值。
const (
	// DefaultAutoShutdown 默认自动关闭时间。
	DefaultAutoShutdown = 5 * time.Minute

	// DefaultMaxSessions 默认最大并发会话数。
	DefaultMaxSessions = 1

	// DefaultMaxConcurrentCommands 默认最大并发命令数。
	DefaultMaxConcurrentCommands = 5

	// DefaultCommandTimeout 默认命令执行超时。
	DefaultCommandTimeout = 30 * time.Second

	// DefaultShutdownTimeout 默认优雅关闭超时。
	DefaultShutdownTimeout = 10 * time.Second

	// DefaultSessionReadTimeout 默认会话读超时（防止 DoS）。
	DefaultSessionReadTimeout = 60 * time.Second

	// DefaultSessionWriteTimeout 默认会话写超时（防止客户端不读取数据）。
	DefaultSessionWriteTimeout = 30 * time.Second
)

// Options 服务器配置选项。
type Options struct {
	// SocketPath Unix Socket 路径。
	SocketPath string

	// SocketPerm Unix Socket 文件权限。
	SocketPerm uint32

	// AutoShutdown 自动关闭时间（0 表示不自动关闭）。
	AutoShutdown time.Duration

	// MaxSessions 最大并发会话数。
	MaxSessions int

	// MaxConcurrentCommands 最大并发命令数。
	MaxConcurrentCommands int

	// CommandTimeout 单命令执行超时。
	CommandTimeout time.Duration

	// ShutdownTimeout 优雅关闭超时。
	ShutdownTimeout time.Duration

	// MaxOutputSize 最大输出大小（字节）。
	MaxOutputSize int

	// SessionReadTimeout 会话读超时（防止 DoS 攻击）。
	SessionReadTimeout time.Duration

	// SessionWriteTimeout 会话写超时（防止客户端不读取数据阻塞 goroutine）。
	SessionWriteTimeout time.Duration

	// CommandWhitelist 命令白名单（nil 表示允许所有）。
	CommandWhitelist []string

	// AuditLogger 审计日志记录器。
	AuditLogger AuditLogger

	// AuditSanitizer 审计参数脱敏函数（可选）。
	// 用于在记录审计日志前对敏感参数进行脱敏。
	AuditSanitizer AuditSanitizer

	// BackgroundMode 后台模式（不监听信号，仅通过 Enable/Disable 控制）。
	BackgroundMode bool

	// Leveler 日志级别控制器（用于 setlog 命令）。
	Leveler Leveler

	// BreakerRegistry 熔断器注册表（用于 breaker 命令）。
	BreakerRegistry BreakerRegistry

	// LimiterRegistry 限流器注册表（用于 limit 命令）。
	LimiterRegistry LimiterRegistry

	// CacheRegistry 缓存注册表（用于 cache 命令）。
	CacheRegistry CacheRegistry

	// ConfigProvider 配置提供者（用于 config 命令）。
	ConfigProvider ConfigProvider

	// Transport 自定义传输层（可选）。
	// 用于测试或自定义传输实现。
	Transport Transport
}

// Option 配置选项函数类型。
type Option func(*Options)

// defaultOptions 返回默认配置。
func defaultOptions() *Options {
	return &Options{
		SocketPath:            DefaultSocketPath,
		SocketPerm:            DefaultSocketPerm,
		AutoShutdown:          DefaultAutoShutdown,
		MaxSessions:           DefaultMaxSessions,
		MaxConcurrentCommands: DefaultMaxConcurrentCommands,
		CommandTimeout:        DefaultCommandTimeout,
		ShutdownTimeout:       DefaultShutdownTimeout,
		MaxOutputSize:         DefaultMaxOutputSize,
		SessionReadTimeout:    DefaultSessionReadTimeout,
		SessionWriteTimeout:   DefaultSessionWriteTimeout,
		AuditLogger:           NewDefaultAuditLogger(),
	}
}

// WithSocketPath 设置 Unix Socket 路径。
func WithSocketPath(path string) Option {
	return func(o *Options) {
		o.SocketPath = path
	}
}

// WithSocketPerm 设置 Unix Socket 文件权限。
func WithSocketPerm(perm uint32) Option {
	return func(o *Options) {
		o.SocketPerm = perm
	}
}

// WithAutoShutdown 设置自动关闭时间。
// 设置为 0 表示不自动关闭。
func WithAutoShutdown(d time.Duration) Option {
	return func(o *Options) {
		o.AutoShutdown = d
	}
}

// WithMaxSessions 设置最大并发会话数。
func WithMaxSessions(n int) Option {
	return func(o *Options) {
		o.MaxSessions = n
	}
}

// WithMaxConcurrentCommands 设置最大并发命令数。
func WithMaxConcurrentCommands(n int) Option {
	return func(o *Options) {
		o.MaxConcurrentCommands = n
	}
}

// WithCommandTimeout 设置命令执行超时。
func WithCommandTimeout(d time.Duration) Option {
	return func(o *Options) {
		o.CommandTimeout = d
	}
}

// WithShutdownTimeout 设置优雅关闭超时。
func WithShutdownTimeout(d time.Duration) Option {
	return func(o *Options) {
		o.ShutdownTimeout = d
	}
}

// WithMaxOutputSize 设置最大输出大小。
func WithMaxOutputSize(size int) Option {
	return func(o *Options) {
		o.MaxOutputSize = size
	}
}

// WithSessionReadTimeout 设置会话读超时。
// 防止恶意客户端连接后不发送数据占用会话槽。
func WithSessionReadTimeout(d time.Duration) Option {
	return func(o *Options) {
		o.SessionReadTimeout = d
	}
}

// WithSessionWriteTimeout 设置会话写超时。
// 防止恶意客户端连接后不读取数据阻塞服务端 goroutine。
func WithSessionWriteTimeout(d time.Duration) Option {
	return func(o *Options) {
		o.SessionWriteTimeout = d
	}
}

// WithCommandWhitelist 设置命令白名单。
// 设置为 nil 表示允许所有命令。
func WithCommandWhitelist(whitelist []string) Option {
	return func(o *Options) {
		o.CommandWhitelist = whitelist
	}
}

// WithAuditLogger 设置审计日志记录器。
func WithAuditLogger(logger AuditLogger) Option {
	return func(o *Options) {
		o.AuditLogger = logger
	}
}

// WithAuditSanitizer 设置审计参数脱敏函数。
// 用于在记录审计日志前对敏感命令参数进行脱敏处理。
// 示例：
//
//	xdbg.WithAuditSanitizer(func(command string, args []string) []string {
//	    if command == "config" && len(args) > 1 {
//	        return xdbg.SanitizeArgs(args) // 脱敏配置参数
//	    }
//	    return args
//	})
func WithAuditSanitizer(sanitizer AuditSanitizer) Option {
	return func(o *Options) {
		o.AuditSanitizer = sanitizer
	}
}

// WithBackgroundMode 设置后台模式。
// 后台模式下不监听信号，仅通过 Enable/Disable 方法控制。
func WithBackgroundMode(enabled bool) Option {
	return func(o *Options) {
		o.BackgroundMode = enabled
	}
}

// WithLeveler 设置日志级别控制器。
// 用于 setlog 命令。
func WithLeveler(leveler Leveler) Option {
	return func(o *Options) {
		o.Leveler = leveler
	}
}

// WithBreakerRegistry 设置熔断器注册表。
// 用于 breaker 命令。
func WithBreakerRegistry(registry BreakerRegistry) Option {
	return func(o *Options) {
		o.BreakerRegistry = registry
	}
}

// WithLimiterRegistry 设置限流器注册表。
// 用于 limit 命令。
func WithLimiterRegistry(registry LimiterRegistry) Option {
	return func(o *Options) {
		o.LimiterRegistry = registry
	}
}

// WithCacheRegistry 设置缓存注册表。
// 用于 cache 命令。
func WithCacheRegistry(registry CacheRegistry) Option {
	return func(o *Options) {
		o.CacheRegistry = registry
	}
}

// WithConfigProvider 设置配置提供者。
// 用于 config 命令。
func WithConfigProvider(provider ConfigProvider) Option {
	return func(o *Options) {
		o.ConfigProvider = provider
	}
}

// WithTransport 设置自定义传输层。
// 用于测试或自定义传输实现。
func WithTransport(t Transport) Option {
	return func(o *Options) {
		o.Transport = t
	}
}

// validateOptions 验证配置选项。
func validateOptions(opts *Options) error {
	if opts.MaxSessions <= 0 {
		return fmt.Errorf("MaxSessions must be positive, got %d", opts.MaxSessions)
	}
	if opts.MaxConcurrentCommands <= 0 {
		return fmt.Errorf("MaxConcurrentCommands must be positive, got %d", opts.MaxConcurrentCommands)
	}
	if opts.MaxOutputSize <= 0 {
		return fmt.Errorf("MaxOutputSize must be positive, got %d", opts.MaxOutputSize)
	}
	if opts.CommandTimeout <= 0 {
		return fmt.Errorf("CommandTimeout must be positive, got %v", opts.CommandTimeout)
	}
	if opts.ShutdownTimeout <= 0 {
		return fmt.Errorf("ShutdownTimeout must be positive, got %v", opts.ShutdownTimeout)
	}
	// 校验 MaxOutputSize 不能超过 MaxPayloadSize，否则响应编码会失败
	// 需要预留 JSONOverhead 空间给 JSON 结构开销
	maxAllowedOutputSize := MaxPayloadSize - JSONOverhead
	if opts.MaxOutputSize > maxAllowedOutputSize {
		return fmt.Errorf("MaxOutputSize (%d) exceeds MaxPayloadSize safety limit (%d)", opts.MaxOutputSize, maxAllowedOutputSize)
	}
	// 校验 Socket 路径安全性
	if err := validateSocketPath(opts.SocketPath); err != nil {
		return fmt.Errorf("invalid socket path: %w", err)
	}
	return nil
}

// validateSocketPath 校验 Socket 路径安全性。
func validateSocketPath(path string) error {
	if path == "" {
		return fmt.Errorf("socket path cannot be empty")
	}

	// 清理路径
	cleanPath := filepath.Clean(path)

	// 检查路径遍历攻击
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("socket path contains path traversal: %s", path)
	}

	// 必须是绝对路径
	if !filepath.IsAbs(cleanPath) {
		return fmt.Errorf("socket path must be absolute: %s", path)
	}

	// 禁止使用敏感路径
	sensitivePatterns := []string{
		"/etc/",
		"/usr/",
		"/bin/",
		"/sbin/",
		"/boot/",
		"/proc/",
		"/sys/",
		"/dev/",
	}
	for _, pattern := range sensitivePatterns {
		if strings.HasPrefix(cleanPath, pattern) {
			return fmt.Errorf("socket path in sensitive directory: %s", path)
		}
	}

	return nil
}
