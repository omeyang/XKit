package xrotate

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

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

	// sizeMBUpperBound 单个日志文件大小上限（10 GB）
	sizeMBUpperBound = 10240

	// backupsUpperBound 备份文件数量上限
	backupsUpperBound = 1024

	// ageDaysUpperBound 备份保留天数上限（约 10 年）
	ageDaysUpperBound = 3650
)

// LumberjackConfig lumberjack 轮转器配置
//
// 基于文件大小的轮转策略，适用于大多数日志场景。
type LumberjackConfig struct {
	// MaxSizeMB 单个日志文件最大大小（MB）
	// 超过此大小时触发轮转
	// 默认值 DefaultMaxSizeMB，必须 > 0
	MaxSizeMB int

	// MaxBackups 保留的备份文件数量
	// 超过此数量时删除最旧的备份
	// 默认值 DefaultMaxBackups，0 表示不限制数量（但仍受 MaxAgeDays 约束）
	MaxBackups int

	// MaxAgeDays 保留备份的天数
	// 超过此天数的备份会被删除
	// 默认值 DefaultMaxAgeDays，0 表示不按天数清理（但仍受 MaxBackups 约束）
	MaxAgeDays int

	// Compress 是否压缩备份文件
	// 启用时备份文件会被 gzip 压缩
	Compress bool

	// LocalTime 备份文件名是否使用本地时间
	// false 时使用 UTC 时间
	LocalTime bool

	// FileMode 日志文件权限
	// 默认为 0，表示使用 lumberjack 默认值 (0600)
	// 设置为非零值时，在首次写入和轮转后调整权限
	// 仅允许权限位（0000~0777），不允许文件类型位或 setuid/setgid
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
	mu       sync.Mutex  // 保护 ensureFileMode 的 Stat+Chmod 操作

	closed atomic.Bool // 标记是否已关闭

	// 设计决策: 使用累计写入字节数检测自动轮转，避免每次 Write 都执行 os.Stat。
	// modeApplied 为 true 时跳过权限检查；当累计写入超过 maxSizeBytes 时
	// （lumberjack 可能已自动轮转）重置标记并重新检查。
	modeApplied  atomic.Bool  // fileMode 已验证，当前文件权限正确
	maxSizeBytes int64        // MaxSizeMB 转换为字节，用于自动轮转检测
	bytesWritten atomic.Int64 // 自上次权限验证以来的累计写入字节数
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
		return nil, ErrEmptyFilename
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
		logger:       l,
		path:         safePath,
		fileMode:     cfg.FileMode,
		maxSizeBytes: int64(cfg.MaxSizeMB) * 1024 * 1024,
	}, nil
}

// validateLumberjackConfig 验证 lumberjack 配置
func validateLumberjackConfig(cfg *LumberjackConfig) error {
	if cfg.MaxSizeMB <= 0 || cfg.MaxSizeMB > sizeMBUpperBound {
		return fmt.Errorf("%w: got %d, want 1~%d", ErrInvalidMaxSize, cfg.MaxSizeMB, sizeMBUpperBound)
	}

	if cfg.MaxBackups < 0 || cfg.MaxBackups > backupsUpperBound {
		return fmt.Errorf("%w: got %d, want 0~%d", ErrInvalidMaxBackups, cfg.MaxBackups, backupsUpperBound)
	}

	if cfg.MaxAgeDays < 0 || cfg.MaxAgeDays > ageDaysUpperBound {
		return fmt.Errorf("%w: got %d, want 0~%d", ErrInvalidMaxAge, cfg.MaxAgeDays, ageDaysUpperBound)
	}

	return validateLumberjackPolicy(cfg)
}

// validateLumberjackPolicy 验证清理策略和文件权限
func validateLumberjackPolicy(cfg *LumberjackConfig) error {
	if cfg.MaxBackups == 0 && cfg.MaxAgeDays == 0 {
		return fmt.Errorf("%w: MaxBackups and MaxAgeDays cannot both be 0", ErrNoCleanupPolicy)
	}

	// FileMode 仅允许权限位（低 9 位），拒绝文件类型位、setuid/setgid 等
	if cfg.FileMode != 0 && cfg.FileMode&^os.FileMode(0o777) != 0 {
		return fmt.Errorf("%w: got %04o, only permission bits (0000~0777) allowed",
			ErrInvalidFileMode, cfg.FileMode)
	}

	return nil
}

// Write 实现 io.Writer 接口
func (r *lumberjackRotator) Write(p []byte) (n int, err error) {
	if r.closed.Load() {
		return 0, ErrClosed
	}

	n, err = r.logger.Write(p)
	if err != nil {
		return n, err
	}

	// 权限调整是尽力而为，不影响日志写入的返回值
	if r.fileMode != 0 {
		needCheck := !r.modeApplied.Load()
		if !needCheck && r.maxSizeBytes > 0 {
			if r.bytesWritten.Add(int64(n)) >= r.maxSizeBytes {
				needCheck = true
			}
		}
		if needCheck {
			r.logFileModeError(r.ensureFileMode())
		}
	}

	return n, nil
}

// ensureFileMode 确保日志文件具有期望的权限。
//
// 通过 Stat 检查实际权限来决定是否需要 Chmod。
// 成功后设置 modeApplied 标记并重置 bytesWritten 计数器，
// 避免后续 Write 重复执行 Stat 系统调用。
func (r *lumberjackRotator) ensureFileMode() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, err := os.Stat(r.path)
	if err != nil {
		// 文件不存在或无法访问，跳过权限检查
		return nil
	}

	currentMode := info.Mode().Perm()
	if currentMode != r.fileMode {
		//#nosec G302 -- 日志文件权限由调用方配置决定
		if err := os.Chmod(r.path, r.fileMode); err != nil {
			return err
		}
	}

	r.modeApplied.Store(true)
	r.bytesWritten.Store(0)
	return nil
}

// logFileModeError 记录权限调整失败的警告日志
func (r *lumberjackRotator) logFileModeError(err error) {
	if err != nil {
		slog.Warn("xrotate: 权限调整失败",
			"path", r.path,
			"mode", fmt.Sprintf("%04o", r.fileMode),
			"error", err,
		)
	}
}

// Close 实现 io.Closer 接口
//
// 关闭后调用 Write 或 Rotate 将返回 [ErrClosed]。
// 重复调用 Close 也返回 [ErrClosed]。
func (r *lumberjackRotator) Close() error {
	if r.closed.Swap(true) {
		return ErrClosed
	}
	return r.logger.Close()
}

// Rotate 手动触发轮转
func (r *lumberjackRotator) Rotate() error {
	if r.closed.Load() {
		return ErrClosed
	}

	if err := r.logger.Rotate(); err != nil {
		return err
	}

	if r.fileMode != 0 {
		// 轮转后新文件使用 lumberjack 默认权限 0600，需要重新调整
		r.modeApplied.Store(false)
		r.bytesWritten.Store(0)
		r.logFileModeError(r.ensureFileMode())
	}
	return nil
}
