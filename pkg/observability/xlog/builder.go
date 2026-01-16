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
type Builder struct {
	output         io.Writer
	level          Level
	levelVar       *slog.LevelVar
	format         string
	addSource      bool
	enableEnrich   bool                // 是否启用 context 信息自动注入
	deploymentType xctx.DeploymentType // 部署类型（作为固定属性）
	replaceAttr    ReplaceAttrFunc     // 属性替换函数（用于治理）
	rotator        xrotate.Rotator
	onError        func(error) // 内部错误回调（Handler.Handle 失败时）
	err            error
}

// New 创建配置构建器
func New() *Builder {
	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelInfo)

	return &Builder{
		output:       os.Stderr,
		level:        LevelInfo,
		levelVar:     levelVar,
		format:       "text",
		enableEnrich: true, // 默认启用 context 信息注入
	}
}

// SetOutput 设置日志输出目标
func (b *Builder) SetOutput(w io.Writer) *Builder {
	b.output = w
	return b
}

// SetLevel 设置日志级别
func (b *Builder) SetLevel(level Level) *Builder {
	b.level = level
	b.levelVar.Set(slog.Level(level))
	return b
}

// SetLevelString 通过字符串设置日志级别
func (b *Builder) SetLevelString(s string) *Builder {
	level, err := ParseLevel(s)
	if err != nil {
		b.err = err
		return b
	}
	return b.SetLevel(level)
}

// SetFormat 设置输出格式：text 或 json
func (b *Builder) SetFormat(format string) *Builder {
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
	b.addSource = enable
	return b
}

// SetEnrich 是否启用 context 信息自动注入（trace_id, tenant_id 等）
//
// 默认启用。当启用时，日志会自动从 context 中提取 xctx（trace/identity）信息。
func (b *Builder) SetEnrich(enable bool) *Builder {
	b.enableEnrich = enable
	return b
}

// SetRotation 设置日志轮转
func (b *Builder) SetRotation(filename string, opts ...xrotate.LumberjackOption) *Builder {
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
		handler = NewEnrichHandler(handler)
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
