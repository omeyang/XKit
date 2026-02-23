package xlog

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// 编译时接口检查
var (
	_ Logger          = (*xlogger)(nil)
	_ Leveler         = (*xlogger)(nil)
	_ LoggerWithLevel = (*xlogger)(nil)
)

// stackPool 堆栈缓冲区池，避免每次 Stack 调用都分配内存
var stackPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 4096)
		return &buf
	},
}

const (
	// initialStackSize 初始堆栈缓冲区大小
	initialStackSize = 4096
	// maxStackSize 最大堆栈缓冲区大小（64KB）
	maxStackSize = 64 * 1024
)

// xlogger Logger 接口的实现
type xlogger struct {
	handler        slog.Handler
	levelVar       *slog.LevelVar
	onError        func(error)    // 内部错误回调
	errorCount     *atomic.Uint64 // 内部错误计数器（用于监控/测试），派生 logger 共享
	addSource      bool           // 是否记录源码位置（热路径优化）
	inErrorHandler *atomic.Bool   // 防止 onError 递归调用，派生 logger 共享
}

// logWithSkip 通用日志方法，支持额外的栈帧跳过
// extraSkip: 额外需要跳过的栈帧数（用于全局函数等间接调用场景）
//
//go:noinline
func (l *xlogger) logWithSkip(ctx context.Context, level slog.Level, msg string, attrs []slog.Attr, extraSkip int) {
	if !l.handler.Enabled(ctx, level) {
		return
	}

	// 热路径优化：仅在启用 AddSource 时才捕获调用者位置
	// runtime.Callers 有不可忽略的开销，跳过可显著提升性能
	var pc uintptr
	if l.addSource {
		var pcs [1]uintptr
		// 基础 skip=3: Callers(0) → logWithSkip(1) → 直接调用方(2) → 跳到(3)
		// extraSkip 用于跳过额外的中间帧：
		//   实例路径 log() 传 1：Callers → logWithSkip → log → Debug → 业务代码(3+1=4)
		//   全局路径 globalLog 传 1：Callers → logWithSkip → globalLog → xlog.Info → 业务代码(3+1=4)
		runtime.Callers(3+extraSkip, pcs[:])
		pc = pcs[0]
	}

	r := slog.NewRecord(time.Now(), level, msg, pc)
	r.AddAttrs(attrs...)

	if err := l.handler.Handle(ctx, r); err != nil {
		l.handleError(err)
	}
}

// log 通用日志方法，正确捕获调用者位置
// 用于实例方法调用（如 logger.Info()）
// extraSkip=1：跳过 log 自身这一帧（调用链 业务代码 → Debug/Info/… → log → logWithSkip）
//
//go:noinline
func (l *xlogger) log(ctx context.Context, level slog.Level, msg string, attrs []slog.Attr) {
	l.logWithSkip(ctx, level, msg, attrs, 1)
}

// handleError 处理内部错误（Handler.Handle 失败）
// 内置递归保护：如果 onError 回调内部触发日志错误，不会导致无限递归。
// 内置 panic 隔离：回调 panic 不会扩散到业务调用链。
// 递归保护通过 inErrorHandler 指针在派生 logger 间共享，确保 With/WithGroup 创建的
// 派生 logger 也受到保护。
//
// 设计决策: CAS 保护导致并发期间部分错误跳过 onError 回调，这是有意为之。
// errorCount 仍计入所有错误（用于监控），onError 回调定位为 best-effort 通知。
// 异步队列方案会增加复杂度且不符合日志库轻量定位。
func (l *xlogger) handleError(err error) {
	if l.errorCount != nil {
		l.errorCount.Add(1)
	}
	if l.onError != nil && l.inErrorHandler != nil {
		// 递归保护：如果已在 onError 回调中，跳过
		if l.inErrorHandler.CompareAndSwap(false, true) {
			defer l.inErrorHandler.Store(false)
			l.safeOnError(err)
		}
	}
}

// safeOnError 安全执行 onError 回调，隔离 panic 防止扩散到业务代码
//
// 设计决策: 日志子系统遵循"失败不扩散"原则——回调 panic 被捕获并计入错误计数，
// 不会中断业务调用链。
func (l *xlogger) safeOnError(err error) {
	defer func() {
		if r := recover(); r != nil {
			// 回调 panic 计入错误计数，便于监控发现
			if l.errorCount != nil {
				l.errorCount.Add(1)
			}
		}
	}()
	l.onError(err)
}

// Debug 记录 Debug 级别日志
func (l *xlogger) Debug(ctx context.Context, msg string, attrs ...slog.Attr) {
	l.log(ctx, slog.LevelDebug, msg, attrs)
}

// Info 记录 Info 级别日志
func (l *xlogger) Info(ctx context.Context, msg string, attrs ...slog.Attr) {
	l.log(ctx, slog.LevelInfo, msg, attrs)
}

// Warn 记录 Warn 级别日志
func (l *xlogger) Warn(ctx context.Context, msg string, attrs ...slog.Attr) {
	l.log(ctx, slog.LevelWarn, msg, attrs)
}

// Error 记录 Error 级别日志
func (l *xlogger) Error(ctx context.Context, msg string, attrs ...slog.Attr) {
	l.log(ctx, slog.LevelError, msg, attrs)
}

// Stack 记录带完整堆栈的错误日志
//
//go:noinline
func (l *xlogger) Stack(ctx context.Context, msg string, attrs ...slog.Attr) {
	l.stackWithSkip(ctx, msg, attrs, 0)
}

// stackWithSkip 记录带完整堆栈的错误日志，支持额外的栈帧跳过
// extraSkip: 额外需要跳过的栈帧数（用于全局函数等间接调用场景）
//
//go:noinline
func (l *xlogger) stackWithSkip(ctx context.Context, msg string, attrs []slog.Attr, extraSkip int) {
	if !l.handler.Enabled(ctx, slog.LevelError) {
		return
	}

	// 从池中获取初始缓冲区
	bufp, ok := stackPool.Get().(*[]byte)
	if !ok {
		// 类型断言失败，创建新缓冲区（不应发生）
		buf := make([]byte, initialStackSize)
		bufp = &buf
	}

	// 获取堆栈，如果被截断则自动扩展缓冲区
	buf := *bufp
	n := runtime.Stack(buf, false)

	// 如果堆栈填满了缓冲区，可能被截断，尝试扩展
	for n == len(buf) && len(buf) < maxStackSize {
		// 扩展缓冲区（翻倍但不超过上限）
		newSize := min(len(buf)*2, maxStackSize)
		buf = make([]byte, newSize)
		n = runtime.Stack(buf, false)
	}

	// 先将堆栈转为不可变 string，再归还缓冲区。
	// 设计决策: 必须在 Put 前完成 string(buf[:n]) 拷贝，否则未扩展场景下
	// buf 与 *bufp 共享底层数组，另一个 goroutine 的 Get+Stack 会覆盖数据。
	// bufp 始终指向 Get 获取的 initialStackSize 缓冲区，扩展后的大缓冲区
	// (buf) 交给 GC 回收，避免池中积累大量内存。
	stackAttr := slog.String(KeyStack, string(buf[:n]))
	stackPool.Put(bufp)

	// 仅在启用 AddSource 时才捕获调用者位置（与 logWithSkip 行为一致）
	var pc uintptr
	if l.addSource {
		var pcs [1]uintptr
		// skip=3: runtime.Callers -> stackWithSkip -> Stack -> 业务代码
		// extraSkip: 额外跳过的栈帧（如全局函数调用时需要 +1）
		runtime.Callers(3+extraSkip, pcs[:])
		pc = pcs[0]
	}

	r := slog.NewRecord(time.Now(), slog.LevelError, msg, pc)
	r.AddAttrs(attrs...)
	r.AddAttrs(stackAttr)

	if err := l.handler.Handle(ctx, r); err != nil {
		l.handleError(err)
	}
}

// With 返回带额外属性的派生 Logger
func (l *xlogger) With(attrs ...slog.Attr) Logger {
	if len(attrs) == 0 {
		return l
	}
	return &xlogger{
		handler:        l.handler.WithAttrs(attrs),
		levelVar:       l.levelVar,
		onError:        l.onError,        // 保留错误回调
		errorCount:     l.errorCount,     // 共享错误计数器
		addSource:      l.addSource,      // 保留源码位置设置
		inErrorHandler: l.inErrorHandler, // 共享递归保护标记
	}
}

// WithGroup 返回带分组的派生 Logger
func (l *xlogger) WithGroup(name string) Logger {
	if name == "" {
		return l
	}
	return &xlogger{
		handler:        l.handler.WithGroup(name),
		levelVar:       l.levelVar,
		onError:        l.onError,        // 保留错误回调
		errorCount:     l.errorCount,     // 共享错误计数器
		addSource:      l.addSource,      // 保留源码位置设置
		inErrorHandler: l.inErrorHandler, // 共享递归保护标记
	}
}

// SetLevel 动态设置日志级别（实现 Leveler 接口）
func (l *xlogger) SetLevel(level Level) {
	l.levelVar.Set(slog.Level(level))
}

// GetLevel 获取当前日志级别（实现 Leveler 接口）
func (l *xlogger) GetLevel() Level {
	return Level(l.levelVar.Level())
}

// Enabled 检查指定级别是否启用（实现 Leveler 接口）
func (l *xlogger) Enabled(ctx context.Context, level Level) bool {
	return l.handler.Enabled(ctx, slog.Level(level))
}
