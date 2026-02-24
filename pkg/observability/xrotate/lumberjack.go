package xrotate

import (
	"fmt"
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

	// maxSizeMB 单个日志文件大小上限（10 GB）
	maxSizeMB = 10240

	// maxBackups 备份文件数量上限
	maxBackups = 1024

	// maxAgeDays 备份保留天数上限（约 10 年）
	maxAgeDays = 3650
)

// lumberjackConfig lumberjack 轮转器配置
//
// 基于文件大小的轮转策略，适用于大多数日志场景。
type lumberjackConfig struct {
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

	// OnError 可选的错误回调函数
	//
	// 当内部操作（如文件权限调整）失败时调用。默认为 nil（静默忽略）。
	//
	// 安全约束：回调函数不得向同一 Rotator 写入数据，否则会导致递归死锁。
	// 推荐输出到 os.Stderr 或独立的日志通道。
	OnError func(error)
}

// Option lumberjack 配置选项函数
type Option func(*lumberjackConfig)

// WithMaxSize 设置单个日志文件最大大小（MB）
func WithMaxSize(mb int) Option {
	return func(c *lumberjackConfig) {
		c.MaxSizeMB = mb
	}
}

// WithMaxBackups 设置保留的备份文件数量
func WithMaxBackups(n int) Option {
	return func(c *lumberjackConfig) {
		c.MaxBackups = n
	}
}

// WithMaxAge 设置保留备份的天数
func WithMaxAge(days int) Option {
	return func(c *lumberjackConfig) {
		c.MaxAgeDays = days
	}
}

// WithCompress 设置是否压缩备份文件
func WithCompress(compress bool) Option {
	return func(c *lumberjackConfig) {
		c.Compress = compress
	}
}

// WithLocalTime 设置备份文件名是否使用本地时间
func WithLocalTime(local bool) Option {
	return func(c *lumberjackConfig) {
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
func WithFileMode(mode os.FileMode) Option {
	return func(c *lumberjackConfig) {
		c.FileMode = mode
	}
}

// WithOnError 设置错误回调函数
//
// 用于接收内部操作（如文件权限调整）的错误通知。
//
// 设计决策: 不使用 slog 等日志库记录内部错误，避免 Rotator 作为日志输出目标时
// 产生递归写入（写失败 → 打日志 → 再写失败 → 栈溢出/死锁）。
// 回调函数不得向同一 Rotator 写入数据。
func WithOnError(fn func(error)) Option {
	return func(c *lumberjackConfig) {
		c.OnError = fn
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
	onError  func(error) // 错误回调（nil 表示静默忽略）
	mu       sync.Mutex  // 保护 ensureFileMode 的 Stat+Chmod 操作

	closed atomic.Bool // 标记是否已关闭

	// 设计决策: 使用累计写入字节数检测自动轮转，避免每次 Write 都执行 os.Stat。
	// modeApplied 为 true 时跳过权限检查；当累计写入超过 maxSizeBytes 时
	// （lumberjack 可能已自动轮转）重置标记并重新检查。
	modeApplied  atomic.Bool  // fileMode 已验证，当前文件权限正确
	maxSizeBytes int64        // MaxSizeMB 转换为字节，用于自动轮转检测
	bytesWritten atomic.Int64 // 自上次权限验证以来的累计写入字节数

	// 可注入的系统调用（nil 时使用 os 标准库），仅用于测试
	statFn  func(string) (os.FileInfo, error)
	chmodFn func(string, os.FileMode) error
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
func NewLumberjack(filename string, opts ...Option) (Rotator, error) {
	if filename == "" {
		return nil, ErrEmptyFilename
	}

	// 构建配置（使用默认值）
	cfg := lumberjackConfig{
		MaxSizeMB:  DefaultMaxSizeMB,
		MaxBackups: DefaultMaxBackups,
		MaxAgeDays: DefaultMaxAgeDays,
		Compress:   DefaultCompress,
		LocalTime:  DefaultLocalTime,
	}

	// 应用选项
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	// 验证配置
	if err := validatelumberjackConfig(&cfg); err != nil {
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
		onError:      cfg.OnError,
		maxSizeBytes: int64(cfg.MaxSizeMB) * 1024 * 1024,
	}, nil
}

// validatelumberjackConfig 验证 lumberjack 配置
func validatelumberjackConfig(cfg *lumberjackConfig) error {
	if cfg.MaxSizeMB <= 0 || cfg.MaxSizeMB > maxSizeMB {
		return fmt.Errorf("%w: got %d, want 1~%d", ErrInvalidMaxSize, cfg.MaxSizeMB, maxSizeMB)
	}

	if cfg.MaxBackups < 0 || cfg.MaxBackups > maxBackups {
		return fmt.Errorf("%w: got %d, want 0~%d", ErrInvalidMaxBackups, cfg.MaxBackups, maxBackups)
	}

	if cfg.MaxAgeDays < 0 || cfg.MaxAgeDays > maxAgeDays {
		return fmt.Errorf("%w: got %d, want 0~%d", ErrInvalidMaxAge, cfg.MaxAgeDays, maxAgeDays)
	}

	return validateLumberjackPolicy(cfg)
}

// validateLumberjackPolicy 验证清理策略和文件权限
func validateLumberjackPolicy(cfg *lumberjackConfig) error {
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
		// 设计决策: Write 与 Close 存在 TOCTOU 窗口——Write 通过 closed 前置检查后，
		// Close 可能在 logger.Write 执行期间完成。此处后置检查确保调用者始终得到
		// ErrClosed（而非底层 I/O 错误），保持 ErrClosed 契约的可靠性。
		if r.closed.Load() {
			return n, ErrClosed
		}
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
			r.reportError(r.ensureFileMode())
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

	stat := r.statFn
	if stat == nil {
		stat = os.Stat
	}

	info, err := stat(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件尚未创建（lumberjack 延迟创建），跳过权限检查
			return nil
		}
		// 其他错误（权限拒绝、I/O 错误等）需要上报
		return err
	}

	currentMode := info.Mode().Perm()
	if currentMode != r.fileMode {
		chmod := r.chmodFn
		if chmod == nil {
			chmod = os.Chmod
		}
		//#nosec G302 -- 日志文件权限由调用方配置决定
		if err := chmod(r.path, r.fileMode); err != nil {
			return err
		}
	}

	r.modeApplied.Store(true)
	r.bytesWritten.Store(0)
	return nil
}

// reportError 通过回调上报内部错误
//
// 设计决策: 不使用 slog 等日志库，避免 Rotator 作为日志输出目标时产生递归写入。
// 回调 panic 被 recover 隔离，防止日志错误通知反向中断业务主流程。
func (r *lumberjackRotator) reportError(err error) {
	if err != nil && r.onError != nil {
		defer func() { recover() }() //nolint:errcheck // recover 返回值无需检查
		r.onError(err)
	}
}

// Close 实现 io.Closer 接口
//
// 关闭后调用 Write 或 Rotate 将返回 [ErrClosed]。
// 重复调用 Close 也返回 [ErrClosed]。
//
// 设计决策: Close 使用 CAS 原语标记关闭状态，首次 Close 失败后不重置标记。
// 如果底层 Close 返回错误，重试调用会得到 ErrClosed 而非重新尝试关闭。
// 这确保了关闭后不会有新的写入到达底层 logger，避免了并发场景下的状态不一致。
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
		// 设计决策: 与 Write 相同的 TOCTOU 后置检查（见 Write 注释）
		if r.closed.Load() {
			return ErrClosed
		}
		return err
	}

	if r.fileMode != 0 {
		// 轮转后新文件使用 lumberjack 默认权限 0600，需要重新调整
		r.modeApplied.Store(false)
		r.bytesWritten.Store(0)
		r.reportError(r.ensureFileMode())
	}
	return nil
}
