package xlog

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
)

// =============================================================================
// 全局 Logger
//
// 定位：脚手架/小工具/迁移期使用。
// 在服务端推荐依赖注入（显式持有 Logger）。
// =============================================================================

// globalLogger 全局 Logger 实例（并发安全）
var globalLogger atomic.Pointer[LoggerWithLevel]

// globalMu 保护 globalOnce 的重置操作（仅用于测试）
var globalMu sync.Mutex

// globalOnce 确保默认 Logger 只初始化一次
var globalOnce sync.Once

// defaultLogger 创建默认 Logger（惰性初始化）
func defaultLogger() LoggerWithLevel {
	globalMu.Lock()
	once := &globalOnce
	globalMu.Unlock()

	once.Do(func() {
		// 默认配置：输出到 stderr，Info 级别，text 格式，启用 enrich
		// 注：Build() 使用默认参数不应失败，如果失败说明系统状态异常
		logger, _, err := New().Build()
		if err != nil {
			panic("xlog: failed to build default logger: " + err.Error())
		}
		globalLogger.Store(&logger)
	})
	return *globalLogger.Load()
}

// Default 返回全局默认 Logger
//
// 懒初始化：首次调用时创建默认 Logger（stderr，Info 级别，text 格式）。
// 并发安全：使用 sync.Once 和 atomic.Pointer。
//
// 定位说明：
//   - 适用于脚手架、小工具、迁移期代码
//   - 服务端推荐依赖注入（显式持有 Logger）
func Default() LoggerWithLevel {
	if l := globalLogger.Load(); l != nil {
		return *l
	}
	return defaultLogger()
}

// SetDefault 替换全局默认 Logger
//
// 用于测试或自定义配置场景。
// 并发安全：使用 atomic.Pointer。
//
// 注意：如果传入 nil，操作会被忽略（不会修改当前 logger）。
// 要重置为默认 logger，请使用 ResetDefault()。
func SetDefault(l LoggerWithLevel) {
	if l == nil {
		// 拒绝 nil，避免后续全局函数 panic
		return
	}
	globalLogger.Store(&l)
}

// ResetDefault 重置全局 Logger 为未初始化状态（仅用于测试）
//
// 调用后，下次 Default() 会重新初始化默认 Logger。
// 并发安全：使用 mutex 保护 sync.Once 的重置。
func ResetDefault() {
	globalMu.Lock()
	globalLogger.Store(nil)
	globalOnce = sync.Once{}
	globalMu.Unlock()
}

// =============================================================================
// 便利函数：最小集，强制 ctx
// =============================================================================

// globalLog 内部辅助函数，正确处理全局函数的栈帧跳过
// 全局函数比实例方法多一层调用，需要额外跳过 1 帧
func globalLog(l LoggerWithLevel, ctx context.Context, level slog.Level, msg string, attrs []slog.Attr) {
	if xl, ok := l.(*xlogger); ok {
		// 使用内部方法，正确跳过栈帧
		xl.logWithSkip(ctx, level, msg, attrs, 1)
		return
	}
	// fallback：非 xlogger 实现（如用户自定义），使用标准方法
	switch level {
	case slog.LevelDebug:
		l.Debug(ctx, msg, attrs...)
	case slog.LevelInfo:
		l.Info(ctx, msg, attrs...)
	case slog.LevelWarn:
		l.Warn(ctx, msg, attrs...)
	case slog.LevelError:
		l.Error(ctx, msg, attrs...)
	}
}

// Debug 使用全局 Logger 记录 Debug 级别日志
func Debug(ctx context.Context, msg string, attrs ...slog.Attr) {
	globalLog(Default(), ctx, slog.LevelDebug, msg, attrs)
}

// Info 使用全局 Logger 记录 Info 级别日志
func Info(ctx context.Context, msg string, attrs ...slog.Attr) {
	globalLog(Default(), ctx, slog.LevelInfo, msg, attrs)
}

// Warn 使用全局 Logger 记录 Warn 级别日志
func Warn(ctx context.Context, msg string, attrs ...slog.Attr) {
	globalLog(Default(), ctx, slog.LevelWarn, msg, attrs)
}

// Error 使用全局 Logger 记录 Error 级别日志
func Error(ctx context.Context, msg string, attrs ...slog.Attr) {
	globalLog(Default(), ctx, slog.LevelError, msg, attrs)
}

// Stack 使用全局 Logger 记录带堆栈的错误日志
func Stack(ctx context.Context, msg string, attrs ...slog.Attr) {
	Default().Stack(ctx, msg, attrs...)
}
