package xlog

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
)

// =============================================================================
// 错误处理白盒测试
//
// 使用内部 package 测试访问私有字段和类型。
// =============================================================================

// errorHandler 测试用 Handler，总是返回错误
type errorHandler struct {
	slog.Handler
	err error
}

func (h *errorHandler) Handle(_ context.Context, _ slog.Record) error {
	return h.err
}

func (h *errorHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (h *errorHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *errorHandler) WithGroup(_ string) slog.Handler {
	return h
}

// TestXlogger_HandleError 测试 handleError 方法
func TestXlogger_HandleError(t *testing.T) {
	var callbackCount atomic.Int32
	var lastError error

	levelVar := new(slog.LevelVar)
	l := &xlogger{
		handler:  &errorHandler{err: errors.New("test error")},
		levelVar: levelVar,
		onError: func(err error) {
			callbackCount.Add(1)
			lastError = err
		},
		errorCount:     new(atomic.Uint64),
		inErrorHandler: new(atomic.Bool),
	}

	// 调用 log 方法，应该触发错误回调
	l.log(context.Background(), slog.LevelInfo, "test", nil)

	if callbackCount.Load() != 1 {
		t.Errorf("onError callback count = %d, want 1", callbackCount.Load())
	}

	if lastError == nil || lastError.Error() != "test error" {
		t.Errorf("lastError = %v, want 'test error'", lastError)
	}

	// 验证错误计数器
	if l.errorCount.Load() != 1 {
		t.Errorf("errorCount = %d, want 1", l.errorCount.Load())
	}
}

// TestXlogger_ErrorCount 测试多次错误计数
func TestXlogger_ErrorCount(t *testing.T) {
	levelVar := new(slog.LevelVar)
	l := &xlogger{
		handler:        &errorHandler{err: errors.New("repeated error")},
		levelVar:       levelVar,
		errorCount:     new(atomic.Uint64),
		inErrorHandler: new(atomic.Bool),
		// onError 为 nil，只计数不回调
	}

	ctx := context.Background()

	// 多次调用
	for i := 0; i < 10; i++ {
		l.log(ctx, slog.LevelInfo, "test", nil)
	}

	if l.errorCount.Load() != 10 {
		t.Errorf("errorCount = %d, want 10", l.errorCount.Load())
	}
}

// TestXlogger_Stack_HandleError 测试 Stack 方法的错误处理
func TestXlogger_Stack_HandleError(t *testing.T) {
	var callbackCount atomic.Int32

	levelVar := new(slog.LevelVar)
	l := &xlogger{
		handler:  &errorHandler{err: errors.New("stack error")},
		levelVar: levelVar,
		onError: func(_ error) {
			callbackCount.Add(1)
		},
		errorCount:     new(atomic.Uint64),
		inErrorHandler: new(atomic.Bool),
	}

	// 调用 Stack 方法
	l.Stack(context.Background(), "stack test")

	if callbackCount.Load() != 1 {
		t.Errorf("onError callback count = %d, want 1", callbackCount.Load())
	}

	if l.errorCount.Load() != 1 {
		t.Errorf("errorCount = %d, want 1", l.errorCount.Load())
	}
}

// TestXlogger_OnErrorPanic 测试 onError 回调 panic 不扩散到业务代码
func TestXlogger_OnErrorPanic(t *testing.T) {
	levelVar := new(slog.LevelVar)
	l := &xlogger{
		handler:  &errorHandler{err: errors.New("trigger panic")},
		levelVar: levelVar,
		onError: func(_ error) {
			panic("callback panic")
		},
		errorCount:     new(atomic.Uint64),
		inErrorHandler: new(atomic.Bool),
	}

	// 不应 panic，panic 应被 safeOnError 捕获
	l.log(context.Background(), slog.LevelInfo, "test", nil)

	// errorCount: 1 (Handle 错误) + 1 (回调 panic) = 2
	if l.errorCount.Load() != 2 {
		t.Errorf("errorCount = %d, want 2 (handle error + callback panic)", l.errorCount.Load())
	}
}

// TestXlogger_NoCallback 测试 onError 为 nil 时不 panic
func TestXlogger_NoCallback(t *testing.T) {
	levelVar := new(slog.LevelVar)
	l := &xlogger{
		handler:        &errorHandler{err: errors.New("no callback")},
		levelVar:       levelVar,
		onError:        nil, // 没有设置回调
		errorCount:     new(atomic.Uint64),
		inErrorHandler: new(atomic.Bool),
	}

	// 应该不 panic
	l.log(context.Background(), slog.LevelInfo, "test", nil)

	// 错误计数器仍然应该增加
	if l.errorCount.Load() != 1 {
		t.Errorf("errorCount = %d, want 1", l.errorCount.Load())
	}
}

// TestXlogger_With_PreservesOnError 测试 With 是否保留 onError
func TestXlogger_With_PreservesOnError(t *testing.T) {
	var callbackCount atomic.Int32

	levelVar := new(slog.LevelVar)
	l := &xlogger{
		handler:  &errorHandler{err: errors.New("with error")},
		levelVar: levelVar,
		onError: func(_ error) {
			callbackCount.Add(1)
		},
		errorCount:     new(atomic.Uint64),
		inErrorHandler: new(atomic.Bool),
	}

	// 创建派生 logger
	child := l.With(slog.String("key", "value"))

	// 派生的 logger 应该是 xlogger 类型
	childLogger, ok := child.(*xlogger)
	if !ok {
		t.Fatalf("With() should return *xlogger, got %T", child)
	}

	// With() 应该保留 onError 回调
	if childLogger.onError == nil {
		t.Error("With() should preserve onError callback")
	}

	// With() 应该共享 inErrorHandler 指针
	if childLogger.inErrorHandler != l.inErrorHandler {
		t.Error("With() should share inErrorHandler pointer")
	}

	// 使用派生 logger 触发错误
	childLogger.log(context.Background(), slog.LevelInfo, "test", nil)

	if callbackCount.Load() != 1 {
		t.Errorf("child logger onError callback count = %d, want 1", callbackCount.Load())
	}
}

// TestXlogger_StackWithSkip_BufferExpansion 测试堆栈缓冲区扩容路径
func TestXlogger_StackWithSkip_BufferExpansion(t *testing.T) {
	var buf testWriterBuffer
	levelVar := new(slog.LevelVar)
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: levelVar})
	l := &xlogger{
		handler:        handler,
		levelVar:       levelVar,
		errorCount:     new(atomic.Uint64),
		inErrorHandler: new(atomic.Bool),
	}

	// 使用深度递归生成超过 initialStackSize(4096) 字节的堆栈
	var deepCall func(depth int)
	deepCall = func(depth int) {
		if depth > 0 {
			deepCall(depth - 1)
			return
		}
		l.Stack(context.Background(), "deep stack test")
	}
	deepCall(100) // 100 层递归应产生远超 4096 字节的堆栈

	output := buf.String()
	if !containsStr(output, "deep stack test") {
		t.Errorf("output missing message\noutput: %s", output[:min(len(output), 500)])
	}
	if !containsStr(output, "goroutine") {
		t.Errorf("output missing stack trace\noutput: %s", output[:min(len(output), 500)])
	}
}

// testWriterBuffer 简单的非线程安全写缓冲（白盒测试用）
type testWriterBuffer struct {
	data []byte
}

func (b *testWriterBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *testWriterBuffer) String() string {
	return string(b.data)
}

func containsStr(s, substr string) bool {
	return len(s) > 0 && len(s) >= len(substr) && strings.Contains(s, substr)
}

// TestXlogger_WithGroup_PreservesOnError 测试 WithGroup 是否保留 onError
func TestXlogger_WithGroup_PreservesOnError(t *testing.T) {
	var callbackCount atomic.Int32

	levelVar := new(slog.LevelVar)
	l := &xlogger{
		handler:  &errorHandler{err: errors.New("group error")},
		levelVar: levelVar,
		onError: func(_ error) {
			callbackCount.Add(1)
		},
		errorCount:     new(atomic.Uint64),
		inErrorHandler: new(atomic.Bool),
	}

	// 创建分组 logger
	child := l.WithGroup("test-group")

	// 派生的 logger 应该是 xlogger 类型
	childLogger, ok := child.(*xlogger)
	if !ok {
		t.Fatalf("WithGroup() should return *xlogger, got %T", child)
	}

	// WithGroup() 应该保留 onError 回调
	if childLogger.onError == nil {
		t.Error("WithGroup() should preserve onError callback")
	}

	// WithGroup() 应该共享 inErrorHandler 指针
	if childLogger.inErrorHandler != l.inErrorHandler {
		t.Error("WithGroup() should share inErrorHandler pointer")
	}

	// 使用派生 logger 触发错误
	childLogger.log(context.Background(), slog.LevelInfo, "test", nil)

	if callbackCount.Load() != 1 {
		t.Errorf("child logger onError callback count = %d, want 1", callbackCount.Load())
	}
}
