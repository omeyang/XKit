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

// 配置上界常量，防止不合理的配置导致内存问题。
const (
	// maxSessions 最大并发会话数上界。
	maxSessions = 1 << 8 // 256

	// maxConcurrentCommands 最大并发命令数上界。
	maxConcurrentCommands = 1 << 10 // 1024
)

// options 服务器配置选项（非导出，仅通过 Option 函数式选项设置）。
//
// 设计决策: options 使用非导出类型，防止用户绕过 validateOptions 直接构造。
// 所有配置通过 WithXxx 函数式选项暴露，与 xsemaphore、xpool 等包一致。
type options struct {
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

	// CommandWhitelist 命令白名单。
	// nil 表示不启用白名单（允许所有命令），空切片 []string{} 表示仅允许必要命令（help, exit）。
	// 设计决策: 默认 nil（允许所有命令）。调试服务需要显式激活（信号触发或 API 调用），
	// 且受 Unix Socket 权限（0600）和自动关闭定时器保护。默认允许全部命令简化了
	// 开发环境使用，生产环境应通过 WithCommandWhitelist 显式收敛命令集。
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
type Option func(*options)

// defaultOptions 返回默认配置。
func defaultOptions() *options {
	return &options{
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
	return func(o *options) {
		o.SocketPath = path
	}
}

// WithSocketPerm 设置 Unix Socket 文件权限。
func WithSocketPerm(perm uint32) Option {
	return func(o *options) {
		o.SocketPerm = perm
	}
}

// WithAutoShutdown 设置自动关闭时间。
// 设置为 0 表示不自动关闭。
func WithAutoShutdown(d time.Duration) Option {
	return func(o *options) {
		o.AutoShutdown = d
	}
}

// WithMaxSessions 设置最大并发会话数。
func WithMaxSessions(n int) Option {
	return func(o *options) {
		o.MaxSessions = n
	}
}

// WithMaxConcurrentCommands 设置最大并发命令数。
func WithMaxConcurrentCommands(n int) Option {
	return func(o *options) {
		o.MaxConcurrentCommands = n
	}
}

// WithCommandTimeout 设置命令执行超时。
func WithCommandTimeout(d time.Duration) Option {
	return func(o *options) {
		o.CommandTimeout = d
	}
}

// WithShutdownTimeout 设置优雅关闭超时。
func WithShutdownTimeout(d time.Duration) Option {
	return func(o *options) {
		o.ShutdownTimeout = d
	}
}

// WithMaxOutputSize 设置最大输出大小。
func WithMaxOutputSize(size int) Option {
	return func(o *options) {
		o.MaxOutputSize = size
	}
}

// WithSessionReadTimeout 设置会话读超时。
// 防止恶意客户端连接后不发送数据占用会话槽。
func WithSessionReadTimeout(d time.Duration) Option {
	return func(o *options) {
		o.SessionReadTimeout = d
	}
}

// WithSessionWriteTimeout 设置会话写超时。
// 防止恶意客户端连接后不读取数据阻塞服务端 goroutine。
func WithSessionWriteTimeout(d time.Duration) Option {
	return func(o *options) {
		o.SessionWriteTimeout = d
	}
}

// WithCommandWhitelist 设置命令白名单。
// 设置为 nil 表示允许所有命令，空切片 []string{} 表示仅允许必要命令（help, exit）。
func WithCommandWhitelist(whitelist []string) Option {
	return func(o *options) {
		o.CommandWhitelist = whitelist
	}
}

// WithAuditLogger 设置审计日志记录器。
func WithAuditLogger(logger AuditLogger) Option {
	return func(o *options) {
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
	return func(o *options) {
		o.AuditSanitizer = sanitizer
	}
}

// WithBackgroundMode 设置后台模式。
// 后台模式下不监听信号，仅通过 Enable/Disable 方法控制。
func WithBackgroundMode(enabled bool) Option {
	return func(o *options) {
		o.BackgroundMode = enabled
	}
}

// WithLeveler 设置日志级别控制器。
// 用于 setlog 命令。
func WithLeveler(leveler Leveler) Option {
	return func(o *options) {
		o.Leveler = leveler
	}
}

// WithBreakerRegistry 设置熔断器注册表。
// 用于 breaker 命令。
func WithBreakerRegistry(registry BreakerRegistry) Option {
	return func(o *options) {
		o.BreakerRegistry = registry
	}
}

// WithLimiterRegistry 设置限流器注册表。
// 用于 limit 命令。
func WithLimiterRegistry(registry LimiterRegistry) Option {
	return func(o *options) {
		o.LimiterRegistry = registry
	}
}

// WithCacheRegistry 设置缓存注册表。
// 用于 cache 命令。
func WithCacheRegistry(registry CacheRegistry) Option {
	return func(o *options) {
		o.CacheRegistry = registry
	}
}

// WithConfigProvider 设置配置提供者。
// 用于 config 命令。
func WithConfigProvider(provider ConfigProvider) Option {
	return func(o *options) {
		o.ConfigProvider = provider
	}
}

// WithTransport 设置自定义传输层。
// 用于测试或自定义传输实现。
func WithTransport(t Transport) Option {
	return func(o *options) {
		o.Transport = t
	}
}

// validateOptions 验证配置选项。
func validateOptions(opts *options) error {
	if err := validateNumericOptions(opts); err != nil {
		return err
	}
	if err := validateTimeoutOptions(opts); err != nil {
		return err
	}
	// 校验 Socket 路径安全性
	if err := validateSocketPath(opts.SocketPath); err != nil {
		return fmt.Errorf("invalid socket path: %w", err)
	}
	// 校验 Socket 文件权限安全性
	if err := validateSocketPerm(opts.SocketPerm); err != nil {
		return err
	}
	return nil
}

// validateSocketPerm 校验 Socket 文件权限安全性。
//
// 设计决策: 拒绝 "other" 有任何权限（0o007 掩码），因为调试 Socket 允许执行
// 高风险命令（pprof、config、setlog）。允许 "group" 权限以支持通过组策略
// 管理访问（如 Kubernetes Pod 安全上下文）。
func validateSocketPerm(perm uint32) error {
	if perm == 0 {
		return fmt.Errorf("SocketPerm must be non-zero")
	}
	if perm&0o007 != 0 {
		return fmt.Errorf("SocketPerm must not grant 'other' access (got %04o): "+
			"debug socket allows high-risk commands, restrict to owner/group only", perm)
	}
	return nil
}

// validateNumericOptions 验证数值型配置选项（范围和上界）。
func validateNumericOptions(opts *options) error {
	if opts.MaxSessions <= 0 {
		return fmt.Errorf("MaxSessions must be positive, got %d", opts.MaxSessions)
	}
	if opts.MaxSessions > maxSessions {
		return fmt.Errorf("MaxSessions exceeds upper bound (%d), got %d", maxSessions, opts.MaxSessions)
	}
	if opts.MaxConcurrentCommands <= 0 {
		return fmt.Errorf("MaxConcurrentCommands must be positive, got %d", opts.MaxConcurrentCommands)
	}
	if opts.MaxConcurrentCommands > maxConcurrentCommands {
		return fmt.Errorf("MaxConcurrentCommands exceeds upper bound (%d), got %d", maxConcurrentCommands, opts.MaxConcurrentCommands)
	}
	if opts.MaxOutputSize <= 0 {
		return fmt.Errorf("MaxOutputSize must be positive, got %d", opts.MaxOutputSize)
	}
	// 校验 MaxOutputSize 不能超过 MaxPayloadSize，否则响应编码会失败
	// 需要预留 JSONOverhead 空间给 JSON 结构开销
	maxAllowedOutputSize := MaxPayloadSize - JSONOverhead
	if opts.MaxOutputSize > maxAllowedOutputSize {
		return fmt.Errorf("MaxOutputSize (%d) exceeds MaxPayloadSize safety limit (%d)", opts.MaxOutputSize, maxAllowedOutputSize)
	}
	return nil
}

// validateTimeoutOptions 验证超时类配置选项。
func validateTimeoutOptions(opts *options) error {
	if opts.CommandTimeout <= 0 {
		return fmt.Errorf("CommandTimeout must be positive, got %v", opts.CommandTimeout)
	}
	if opts.ShutdownTimeout <= 0 {
		return fmt.Errorf("ShutdownTimeout must be positive, got %v", opts.ShutdownTimeout)
	}
	if opts.SessionReadTimeout < 0 {
		return fmt.Errorf("SessionReadTimeout must be non-negative, got %v", opts.SessionReadTimeout)
	}
	if opts.SessionWriteTimeout < 0 {
		return fmt.Errorf("SessionWriteTimeout must be non-negative, got %v", opts.SessionWriteTimeout)
	}
	return nil
}

// validateSocketPath 校验 Socket 路径安全性。
func validateSocketPath(path string) error {
	if path == "" {
		return fmt.Errorf("socket path cannot be empty")
	}

	// 设计决策: 先检查原始输入中的路径遍历片段，再清理路径。
	// filepath.Clean 会将 "/tmp/../etc/foo" 解析为 "/etc/foo"，使得清理后的路径
	// 不再包含 ".."。如果仅对清理后路径检查，带遍历语义的输入会被遗漏。
	if strings.Contains(path, "..") {
		return fmt.Errorf("socket path contains path traversal: %s", path)
	}

	// 清理路径（用于后续敏感目录检查）
	cleanPath := filepath.Clean(path)

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
