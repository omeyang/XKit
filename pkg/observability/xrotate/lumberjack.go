package xrotate

import (
	"errors"
	"os"
	"sync"

	"github.com/omeyang/xkit/pkg/util/xfile"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Lumberjack 默认配置值
const (
	// DefaultMaxSizeMB 默认单个日志文件最大大小（MB）
	DefaultMaxSizeMB = 500

	// DefaultMaxBackups 默认保留的备份文件数量
	DefaultMaxBackups = 7

	// DefaultMaxAgeDays 默认保留备份的天数
	DefaultMaxAgeDays = 30

	// DefaultCompress 默认是否压缩备份
	DefaultCompress = true

	// DefaultLocalTime 默认是否使用本地时间（false 表示 UTC）
	DefaultLocalTime = false
)

// LumberjackConfig lumberjack 轮转器配置
//
// 基于文件大小的轮转策略，适用于大多数日志场景。
type LumberjackConfig struct {
	// MaxSizeMB 单个日志文件最大大小（MB）
	// 超过此大小时触发轮转
	// 零值使用默认值 DefaultMaxSizeMB
	MaxSizeMB int

	// MaxBackups 保留的备份文件数量
	// 超过此数量时删除最旧的备份
	// 零值表示不限制数量（但仍受 MaxAgeDays 约束）
	MaxBackups int

	// MaxAgeDays 保留备份的天数
	// 超过此天数的备份会被删除
	// 零值表示不按天数清理（但仍受 MaxBackups 约束）
	MaxAgeDays int

	// Compress 是否压缩备份文件
	// 启用时备份文件会被 gzip 压缩
	Compress bool

	// LocalTime 备份文件名是否使用本地时间
	// false 时使用 UTC 时间
	LocalTime bool

	// FileMode 日志文件权限
	// 默认为 0，表示使用 lumberjack 默认值 (0600)
	// 设置为非零值时，会在每次写入后调整权限
	//
	// 注意：lumberjack v2.2+ 内部使用 0600 创建文件。如需更宽松的
	// 权限（如 0644），可使用此选项调整。
	//
	// 安全说明：由于 lumberjack 不暴露权限配置，此选项通过
	// chmod 方式调整权限，存在短暂时间窗口权限为 0600。
	FileMode os.FileMode
}

// LumberjackOption lumberjack 配置选项函数
type LumberjackOption func(*LumberjackConfig)

// WithMaxSize 设置单个日志文件最大大小（MB）
func WithMaxSize(mb int) LumberjackOption {
	return func(c *LumberjackConfig) {
		c.MaxSizeMB = mb
	}
}

// WithMaxBackups 设置保留的备份文件数量
func WithMaxBackups(n int) LumberjackOption {
	return func(c *LumberjackConfig) {
		c.MaxBackups = n
	}
}

// WithMaxAge 设置保留备份的天数
func WithMaxAge(days int) LumberjackOption {
	return func(c *LumberjackConfig) {
		c.MaxAgeDays = days
	}
}

// WithCompress 设置是否压缩备份文件
func WithCompress(compress bool) LumberjackOption {
	return func(c *LumberjackConfig) {
		c.Compress = compress
	}
}

// WithLocalTime 设置备份文件名是否使用本地时间
func WithLocalTime(local bool) LumberjackOption {
	return func(c *LumberjackConfig) {
		c.LocalTime = local
	}
}

// WithFileMode 设置日志文件权限
//
// lumberjack v2.2+ 默认使用 0600 权限创建日志文件。使用此选项可以
// 设置不同的权限（如 0644）。
//
// 注意：权限调整在文件创建/写入后通过 chmod 实现，
// 存在短暂时间窗口文件权限为 lumberjack 默认值 0600。
func WithFileMode(mode os.FileMode) LumberjackOption {
	return func(c *LumberjackConfig) {
		c.FileMode = mode
	}
}

// lumberjackRotator 基于 lumberjack 的 Rotator 实现
//
// lumberjack 是一个成熟的日志轮转库，提供：
//   - 按大小自动轮转
//   - 备份文件管理（数量和天数）
//   - 可选的 gzip 压缩
//   - 并发安全的写入
type lumberjackRotator struct {
	logger   *lumberjack.Logger
	path     string      // 日志文件路径（用于 chmod）
	fileMode os.FileMode // 目标文件权限（0 表示不调整）
	mu       sync.Mutex  // 保护 chmod 操作
}

// NewLumberjack 创建基于 lumberjack 的日志轮转器
//
// 参数:
//   - filename: 日志文件路径（必需）
//   - opts: 可选配置项
//
// 安全说明:
//   - 会对文件路径进行规范化和安全检查
//   - 自动创建不存在的父目录（权限 0750）
func NewLumberjack(filename string, opts ...LumberjackOption) (Rotator, error) {
	if filename == "" {
		return nil, errors.New("xrotate: filename is required")
	}

	// 构建配置（使用默认值）
	cfg := LumberjackConfig{
		MaxSizeMB:  DefaultMaxSizeMB,
		MaxBackups: DefaultMaxBackups,
		MaxAgeDays: DefaultMaxAgeDays,
		Compress:   DefaultCompress,
		LocalTime:  DefaultLocalTime,
	}

	// 应用选项
	for _, opt := range opts {
		opt(&cfg)
	}

	// 验证配置
	if err := validateLumberjackConfig(&cfg); err != nil {
		return nil, err
	}

	// 安全检查和路径规范化
	safePath, err := xfile.SanitizePath(filename)
	if err != nil {
		return nil, err
	}

	// 确保目录存在
	if err := xfile.EnsureDir(safePath); err != nil {
		return nil, err
	}

	// 创建 lumberjack 实例
	l := &lumberjack.Logger{
		Filename:   safePath,
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAgeDays,
		Compress:   cfg.Compress,
		LocalTime:  cfg.LocalTime,
	}

	return &lumberjackRotator{
		logger:   l,
		path:     safePath,
		fileMode: cfg.FileMode,
	}, nil
}

// validateLumberjackConfig 验证 lumberjack 配置
func validateLumberjackConfig(cfg *LumberjackConfig) error {
	// 零值替换为默认值
	if cfg.MaxSizeMB == 0 {
		cfg.MaxSizeMB = DefaultMaxSizeMB
	}

	// 验证数值范围
	if cfg.MaxSizeMB < 0 {
		return errors.New("xrotate: MaxSizeMB must be > 0")
	}

	if cfg.MaxBackups < 0 {
		return errors.New("xrotate: MaxBackups must be >= 0")
	}

	if cfg.MaxAgeDays < 0 {
		return errors.New("xrotate: MaxAgeDays must be >= 0")
	}

	return nil
}

// Write 实现 io.Writer 接口
func (r *lumberjackRotator) Write(p []byte) (n int, err error) {
	n, err = r.logger.Write(p)
	if err != nil {
		return n, err
	}

	// 如果设置了 FileMode，检查当前文件权限并在必要时调整
	// 这种方式能正确处理 lumberjack 的自动轮转（新文件权限可能不同）
	// 注意：chmod 失败不影响日志写入结果，权限调整是尽力而为
	if r.fileMode != 0 {
		// chmod 失败是非关键性错误：写入已成功，权限调整仅为尽力而为
		// 此处显式忽略错误，避免因权限问题影响日志写入的返回值
		_ = r.ensureFileMode()
	}

	return n, nil
}

// ensureFileMode 确保日志文件具有期望的权限。
// 通过检查实际权限来决定是否需要 chmod，能正确处理：
//   - lumberjack 自动轮转创建的新文件
//   - 外部权限变更
//   - 首次文件创建
//
// 返回 nil 表示权限已正确，返回 error 表示 chmod 失败（但不影响日志写入）。
func (r *lumberjackRotator) ensureFileMode() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, err := os.Stat(r.path)
	if err != nil {
		// 文件不存在或无法访问，跳过权限检查
		return nil
	}

	// 只比较权限位（去除文件类型位）
	currentMode := info.Mode().Perm()
	if currentMode != r.fileMode {
		//#nosec G302 -- 日志文件权限由调用方配置决定
		return os.Chmod(r.path, r.fileMode)
	}
	return nil
}

// Close 实现 io.Closer 接口
func (r *lumberjackRotator) Close() error {
	return r.logger.Close()
}

// Rotate 手动触发轮转
func (r *lumberjackRotator) Rotate() error {
	if err := r.logger.Rotate(); err != nil {
		return err
	}

	// 如果配置了 FileMode，修正新文件权限
	// lumberjack 创建新文件使用默认权限 0600，需要调整
	if r.fileMode != 0 {
		// 权限调整是尽力而为，不影响 rotate 结果
		_ = r.ensureFileMode()
	}
	return nil
}
