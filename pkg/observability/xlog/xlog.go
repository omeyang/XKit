// xlog.go 定义核心接口：Logger、Leveler、LoggerWithLevel
//
// 设计理念：
//   - 强制 context 传递，确保追踪信息传播
//   - 动态级别控制，支持运行时调整
//   - Handler 装饰链，自动注入 xctx（trace/identity）字段
//   - 生命周期管理，Build() 返回 cleanup 函数
//   - 类型安全，方法签名只接受 slog.Attr
package xlog

import (
	"context"
	"log/slog"
)

// Logger 日志接口
//
// 所有方法都需要 context.Context 参数，确保追踪信息正确传播。
// 方法签名只接受 slog.Attr，保证类型安全，避免隐式 key-value 转换开销。
type Logger interface {
	// Debug 记录 Debug 级别日志
	Debug(ctx context.Context, msg string, attrs ...slog.Attr)

	// Info 记录 Info 级别日志
	Info(ctx context.Context, msg string, attrs ...slog.Attr)

	// Warn 记录 Warn 级别日志
	Warn(ctx context.Context, msg string, attrs ...slog.Attr)

	// Error 记录 Error 级别日志
	Error(ctx context.Context, msg string, attrs ...slog.Attr)

	// Stack 记录带完整堆栈的错误日志
	// 用于问题诊断，输出当前 goroutine 的调用栈
	Stack(ctx context.Context, msg string, attrs ...slog.Attr)

	// With 返回带额外属性的派生 Logger
	//
	// 设计决策: 返回 Logger 而非 LoggerWithLevel，保持接口最小化。
	// 底层实现（xlogger）同时实现 LoggerWithLevel，可通过类型断言获取 Leveler 能力。
	// 派生 logger 共享父级的 LevelVar，动态级别变更会同步生效。
	With(attrs ...slog.Attr) Logger

	// WithGroup 返回带分组的派生 Logger
	// 后续 With 添加的属性会在此分组下
	//
	// 设计决策: 返回 Logger 而非 LoggerWithLevel，与 With 保持一致。
	WithGroup(name string) Logger
}

// Leveler 级别控制接口
//
// 与 Logger 分离，避免污染核心日志接口。
// 通过类型断言检查具体实现是否支持动态级别控制。
type Leveler interface {
	// SetLevel 动态设置日志级别
	// 运行时生效，无需重启服务
	SetLevel(level Level)

	// GetLevel 获取当前日志级别
	GetLevel() Level

	// Enabled 检查指定级别是否启用
	// 用于 Lazy 求值优化：在构造昂贵的日志参数前先检查级别
	Enabled(ctx context.Context, level Level) bool
}

// LoggerWithLevel 组合接口：Logger + Leveler
//
// Build() 返回此接口，避免业务代码频繁类型断言。
// 常用路径显式化：构建的 Logger 通常都需要动态级别控制。
type LoggerWithLevel interface {
	Logger
	Leveler
}
