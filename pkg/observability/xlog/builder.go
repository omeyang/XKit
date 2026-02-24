package xlog

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/observability/xrotate"
)

// ReplaceAttrFunc 属性替换函数类型
//
// 用于日志治理场景：字段重命名、敏感信息脱敏、字段过滤等。
// 返回修改后的属性，如果返回空 Key 的 Attr，该属性会被移除。
//
// 参数：
//   - groups: 当前属性所在的分组路径（如 ["request", "headers"]）
//   - a: 原始属性
//
// 示例：
//
//	// 脱敏密码字段
//	func(groups []string, a slog.Attr) slog.Attr {
//	    if a.Key == "password" {
//	        return slog.String("password", "***")
//	    }
//	    return a
//	}
type ReplaceAttrFunc func(groups []string, a slog.Attr) slog.Attr

// Builder 日志配置构建器
//
// Builder 不是并发安全的，应在单个 goroutine 中完成链式调用后 Build。
type Builder struct {
	output         io.Writer
	levelVar       *slog.LevelVar
	format         string
	addSource      bool
	enableEnrich   bool                // 是否启用 context 信息自动注入
	deploymentType xctx.DeploymentType // 部署类型（作为固定属性）
	replaceAttr    ReplaceAttrFunc     // 属性替换函数（用于治理）
	rotator        xrotate.Rotator
	onError        func(error) // 内部错误回调（Handler.Handle 失败时）
	err            error
	built          bool // Build() 已调用，防止重复构建
}

// New 创建配置构建器
func New() *Builder {
	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelInfo)

	return &Builder{
		output:       os.Stderr,
		levelVar:     levelVar,
		format:       "text",
		enableEnrich: true, // 默认启用 context 信息注入
	}
}

// SetOutput 设置日志输出目标
//
// nil 会立即报错（fail-fast，与 SetDeploymentType 行为一致）。
func (b *Builder) SetOutput(w io.Writer) *Builder {
	if b.err != nil {
		return b
	}
	if w == nil {
		b.err = fmt.Errorf("xlog: output writer is nil")
		return b
	}
	b.output = w
	return b
}

// SetLevel 设置日志级别
func (b *Builder) SetLevel(level Level) *Builder {
	if b.err != nil {
		return b
	}
	b.levelVar.Set(slog.Level(level))
	return b
}

// SetLevelString 通过字符串设置日志级别
func (b *Builder) SetLevelString(s string) *Builder {
	if b.err != nil {
		return b
	}
	level, err := ParseLevel(s)
	if err != nil {
		b.err = err
		return b
	}
	return b.SetLevel(level)
}

// SetFormat 设置输出格式：text 或 json
func (b *Builder) SetFormat(format string) *Builder {
	if b.err != nil {
		return b
	}
	normalized := strings.ToLower(strings.TrimSpace(format))
	if normalized == "" {
		// 空值视为使用默认格式，避免误把“没填”变成配置错误。
		b.format = "text"
		return b
	}
	if normalized != "text" && normalized != "json" {
		b.err = fmt.Errorf("xlog: unknown format %q", format)
		return b
	}
	b.format = normalized
	return b
}

// SetAddSource 是否在日志中添加源码位置
func (b *Builder) SetAddSource(enable bool) *Builder {
	if b.err != nil {
		return b
	}
	b.addSource = enable
	return b
}

// SetEnrich 是否启用 context 信息自动注入（trace_id, tenant_id 等）
//
// 默认启用。当启用时，日志会自动从 context 中提取 xctx（trace/identity）信息。
func (b *Builder) SetEnrich(enable bool) *Builder {
	if b.err != nil {
		return b
	}
	b.enableEnrich = enable
	return b
}

// SetRotation 设置日志轮转
//
// 注意：会同时设置 output 为 rotator，覆盖之前的 SetOutput 设置。
// 同理，SetRotation 之后再调用 SetOutput 会覆盖 rotator 的输出。
// Builder 遵循 last-wins 语义：以最后一次设置的输出目标为准。
func (b *Builder) SetRotation(filename string, opts ...xrotate.Option) *Builder {
	if b.err != nil {
		return b
	}
	// 关闭旧 rotator，避免重复调用 SetRotation 时文件句柄泄漏
	if b.rotator != nil {
		if closeErr := b.rotator.Close(); closeErr != nil {
			b.err = fmt.Errorf("xlog: failed to close previous rotator: %w", closeErr)
			return b
		}
	}
	rotator, err := xrotate.NewLumberjack(filename, opts...)
	if err != nil {
		b.err = err
		return b
	}
	b.rotator = rotator
	b.output = rotator
	return b
}

// SetOnError 设置内部错误回调
//
// 当 Handler.Handle() 失败时（如磁盘满、权限问题、writer 异常），
// 会调用此回调。默认策略仍然"不向外返回错误、不 panic"，
// 但允许业务把内部错误接到 metrics/告警系统。
//
// 注意事项：
//   - 回调在热路径同步执行，应保持轻量，复杂逻辑建议使用 channel 异步处理
//   - 内置递归保护：如果回调内部触发日志错误，不会导致无限递归
//   - 内置 panic 隔离：回调 panic 会被捕获并计入错误计数，不会扩散到业务调用链
//   - 回调失败不会影响日志写入的返回值
//
// 示例：
//
//	logger, cleanup, _ := xlog.New().
//		SetOnError(func(err error) {
//			metrics.IncrCounter("log.write.error", 1)
//		}).
//		Build()
func (b *Builder) SetOnError(fn func(error)) *Builder {
	if b.err != nil {
		return b
	}
	b.onError = fn
	return b
}

// SetReplaceAttr 设置属性替换函数（日志治理）
//
// 用于在日志输出前对属性进行处理，支持以下场景：
//   - 字段重命名：统一字段名规范
//   - 敏感信息脱敏：隐藏密码、token 等
//   - 字段过滤：移除不需要的属性
//   - 值格式化：统一时间格式、数值精度等
//
// 示例 - 脱敏密码：
//
//	logger, _, _ := xlog.New().
//		SetReplaceAttr(func(groups []string, a slog.Attr) slog.Attr {
//			if a.Key == "password" || a.Key == "token" {
//				return slog.String(a.Key, "***REDACTED***")
//			}
//			return a
//		}).
//		Build()
//
// 示例 - 移除调试属性：
//
//	logger, _, _ := xlog.New().
//		SetReplaceAttr(func(groups []string, a slog.Attr) slog.Attr {
//			if a.Key == "debug_info" {
//				return slog.Attr{} // 返回空 Key 移除该属性
//			}
//			return a
//		}).
//		Build()
func (b *Builder) SetReplaceAttr(fn ReplaceAttrFunc) *Builder {
	if b.err != nil {
		return b
	}
	b.replaceAttr = fn
	return b
}

// SetDeploymentType 设置部署类型（作为固定属性添加到每条日志）
//
// 部署类型在 Build 时通过 handler.WithAttrs 注入，
// 避免在每条日志的热路径上重复检查。
//
// 支持的值：
//   - xctx.DeploymentLocal ("LOCAL") - 本地/私有化部署
//   - xctx.DeploymentSaaS ("SAAS") - SaaS 云部署
//
// 示例：
//
//	logger, cleanup, _ := xlog.New().
//		SetDeploymentType(xctx.DeploymentSaaS).
//		Build()
func (b *Builder) SetDeploymentType(dt xctx.DeploymentType) *Builder {
	if b.err != nil {
		return b
	}
	if !dt.IsValid() {
		b.err = xctx.ErrInvalidDeploymentType
		return b
	}
	b.deploymentType = dt
	return b
}

// SetDeploymentTypeFromEnv 从环境变量 DEPLOYMENT_TYPE 读取部署类型
//
// 便捷方法，等价于：
//
//	v := os.Getenv("DEPLOYMENT_TYPE")
//	dt, _ := xctx.ParseDeploymentType(v)
//	builder.SetDeploymentType(dt)
func (b *Builder) SetDeploymentTypeFromEnv() *Builder {
	if b.err != nil {
		return b
	}
	v := os.Getenv(xctx.EnvDeploymentType)
	if v == "" {
		b.err = xctx.ErrMissingDeploymentTypeEnv
		return b
	}
	dt, err := xctx.ParseDeploymentType(v)
	if err != nil {
		b.err = err
		return b
	}
	b.deploymentType = dt
	return b
}

// Build 构建 Logger 实例
//
// 返回值：
//   - LoggerWithLevel: 日志实例，同时支持动态级别控制
//   - func() error: 清理函数，用于释放资源（如关闭文件）
//   - error: 配置错误
func (b *Builder) Build() (LoggerWithLevel, func() error, error) {
	if b.err != nil {
		return nil, nil, b.err
	}

	// 设计决策: 禁止重复调用 Build()。当配置了 SetRotation 时，多次 Build() 返回的
	// logger 共享同一个 rotator 实例，第一次 cleanup 关闭 rotator 后会导致第二个 logger
	// 写入失败。使用 built 标志强制一次性使用，避免隐式的资源共享问题。
	if b.built {
		return nil, nil, fmt.Errorf("xlog: builder already built, create a new Builder via New()")
	}
	b.built = true

	if b.output == nil {
		return nil, nil, fmt.Errorf("xlog: output writer is nil")
	}

	// 创建 handler
	opts := &slog.HandlerOptions{
		Level:     b.levelVar,
		AddSource: b.addSource,
	}

	// 设置属性替换函数（日志治理）
	if b.replaceAttr != nil {
		opts.ReplaceAttr = b.replaceAttr
	}

	var handler slog.Handler
	switch b.format {
	case "json":
		handler = slog.NewJSONHandler(b.output, opts)
	default:
		handler = slog.NewTextHandler(b.output, opts)
	}

	// 启用 context 信息注入
	if b.enableEnrich {
		enriched, err := NewEnrichHandler(handler)
		if err != nil {
			return nil, nil, err
		}
		handler = enriched
	}

	// 添加部署类型固定属性（在 Build 时一次性注入，避免热路径检查）
	// 使用 IsValid() 确保只注入有效的部署类型（LOCAL/SAAS）
	if b.deploymentType.IsValid() {
		handler = handler.WithAttrs([]slog.Attr{
			slog.String(xctx.KeyDeploymentType, string(b.deploymentType)),
		})
	}

	// 创建 logger
	// 初始化共享指针，确保派生 logger (With/WithGroup) 能正确共享状态
	logger := &xlogger{
		handler:        handler,
		levelVar:       b.levelVar,
		onError:        b.onError,
		errorCount:     new(atomic.Uint64), // 共享错误计数器
		addSource:      b.addSource,        // 传递源码位置设置，用于热路径优化
		inErrorHandler: new(atomic.Bool),   // 共享递归保护标记
	}

	// 创建 cleanup 函数
	cleanup := b.createCleanup()

	return logger, cleanup, nil
}

// createCleanup 创建清理函数
func (b *Builder) createCleanup() func() error {
	var once sync.Once
	rotator := b.rotator

	return func() error {
		var err error
		once.Do(func() {
			if rotator != nil {
				err = rotator.Close()
			}
		})
		return err
	}
}
