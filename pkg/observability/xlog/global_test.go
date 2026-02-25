package xlog_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/omeyang/xkit/pkg/observability/xlog"
)

// =============================================================================
// Default / SetDefault 测试
// =============================================================================

func TestDefault_LazyInit(t *testing.T) {
	// 重置全局状态
	xlog.ResetDefault()
	defer xlog.ResetDefault()

	// 首次调用应返回有效 Logger
	logger := xlog.Default()
	if logger == nil {
		t.Fatal("Default() should not return nil")
	}

	// 再次调用应返回相同实例
	logger2 := xlog.Default()
	if logger != logger2 {
		t.Error("Default() should return the same instance")
	}
}

func TestSetDefault(t *testing.T) {
	// 重置全局状态
	xlog.ResetDefault()
	defer xlog.ResetDefault()

	var buf bytes.Buffer
	customLogger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetLevel(xlog.LevelDebug).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	defer func() { _ = cleanup() }()

	// 设置自定义 Logger
	xlog.SetDefault(customLogger)

	// 验证 Default() 返回自定义 Logger
	xlog.Info(context.Background(), "custom logger test")

	if !strings.Contains(buf.String(), "custom logger test") {
		t.Errorf("SetDefault did not work, output: %s", buf.String())
	}
}

func TestSetDefault_Nil(t *testing.T) {
	// 重置全局状态
	xlog.ResetDefault()
	defer xlog.ResetDefault()

	// 获取默认 Logger
	original := xlog.Default()
	if original == nil {
		t.Fatal("Default() should not return nil")
	}

	// 设置自定义 Logger
	var buf bytes.Buffer
	customLogger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetLevel(xlog.LevelDebug).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	defer func() { _ = cleanup() }()

	xlog.SetDefault(customLogger)

	// 尝试设置 nil，应该被忽略
	xlog.SetDefault(nil)

	// Default() 应该仍然返回 customLogger（不是 nil）
	current := xlog.Default()
	if current == nil {
		t.Fatal("SetDefault(nil) should be ignored, Default() should not return nil")
	}

	// 验证仍然使用 customLogger
	xlog.Info(context.Background(), "after nil test")
	if !strings.Contains(buf.String(), "after nil test") {
		t.Errorf("SetDefault(nil) should preserve existing logger, output: %s", buf.String())
	}
}

func TestDefault_ConcurrencySafety(t *testing.T) {
	// 重置全局状态
	xlog.ResetDefault()
	defer xlog.ResetDefault()

	var wg sync.WaitGroup
	const goroutines = 100

	loggers := make([]xlog.LoggerWithLevel, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			loggers[idx] = xlog.Default()
		}(i)
	}

	wg.Wait()

	// 所有 goroutine 应该获得相同的 Logger
	first := loggers[0]
	for i, logger := range loggers {
		if logger != first {
			t.Errorf("goroutine %d got different logger", i)
		}
	}
}

// =============================================================================
// 便利函数测试
// =============================================================================

func TestGlobal_ConvenienceFunctions(t *testing.T) {
	// 重置全局状态
	xlog.ResetDefault()
	defer xlog.ResetDefault()

	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetLevel(xlog.LevelDebug).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	defer func() { _ = cleanup() }()

	xlog.SetDefault(logger)
	ctx := context.Background()

	// 测试各级别便利函数
	xlog.Debug(ctx, "debug message")
	xlog.Info(ctx, "info message")
	xlog.Warn(ctx, "warn message")
	xlog.Error(ctx, "error message")

	output := buf.String()

	tests := []string{
		"debug message",
		"info message",
		"warn message",
		"error message",
	}

	for _, want := range tests {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\noutput: %s", want, output)
		}
	}
}

func TestGlobal_Stack(t *testing.T) {
	// 重置全局状态
	xlog.ResetDefault()
	defer xlog.ResetDefault()

	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetLevel(xlog.LevelDebug).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	defer func() { _ = cleanup() }()

	xlog.SetDefault(logger)
	xlog.Stack(context.Background(), "stack test")

	output := buf.String()
	if !strings.Contains(output, "stack test") {
		t.Errorf("output missing message\noutput: %s", output)
	}
	if !strings.Contains(output, "goroutine") {
		t.Errorf("output missing stack trace\noutput: %s", output)
	}
}

func TestGlobal_WithAttrs(t *testing.T) {
	// 重置全局状态
	xlog.ResetDefault()
	defer xlog.ResetDefault()

	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetLevel(xlog.LevelInfo).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	defer func() { _ = cleanup() }()

	xlog.SetDefault(logger)
	xlog.Info(context.Background(), "with attrs", slog.String("key", "value"))

	output := buf.String()
	if !strings.Contains(output, "key") || !strings.Contains(output, "value") {
		t.Errorf("output missing attrs\noutput: %s", output)
	}
}

// =============================================================================
// globalLog 回退路径测试（非 xlogger 实现）
// =============================================================================

// mockLoggerWithLevel 用于测试 globalLog 的 fallback 路径
type mockLoggerWithLevel struct {
	lastLevel slog.Level
	lastMsg   string
}

func (m *mockLoggerWithLevel) Debug(_ context.Context, msg string, _ ...slog.Attr) {
	m.lastLevel = slog.LevelDebug
	m.lastMsg = msg
}

func (m *mockLoggerWithLevel) Info(_ context.Context, msg string, _ ...slog.Attr) {
	m.lastLevel = slog.LevelInfo
	m.lastMsg = msg
}

func (m *mockLoggerWithLevel) Warn(_ context.Context, msg string, _ ...slog.Attr) {
	m.lastLevel = slog.LevelWarn
	m.lastMsg = msg
}

func (m *mockLoggerWithLevel) Error(_ context.Context, msg string, _ ...slog.Attr) {
	m.lastLevel = slog.LevelError
	m.lastMsg = msg
}

func (m *mockLoggerWithLevel) Stack(_ context.Context, msg string, _ ...slog.Attr) {
	m.lastLevel = slog.LevelError
	m.lastMsg = msg
}

func (m *mockLoggerWithLevel) With(_ ...slog.Attr) xlog.Logger { return m }

func (m *mockLoggerWithLevel) WithGroup(_ string) xlog.Logger { return m }

func (m *mockLoggerWithLevel) SetLevel(_ xlog.Level) {}

func (m *mockLoggerWithLevel) GetLevel() xlog.Level { return xlog.LevelDebug }

func (m *mockLoggerWithLevel) Enabled(_ context.Context, _ xlog.Level) bool { return true }

func TestGlobal_FallbackNonXlogger(t *testing.T) {
	xlog.ResetDefault()
	defer xlog.ResetDefault()

	mock := &mockLoggerWithLevel{}
	xlog.SetDefault(mock)

	ctx := context.Background()

	// 测试所有全局便利函数通过 fallback 路径
	xlog.Debug(ctx, "debug fallback")
	if mock.lastLevel != slog.LevelDebug || mock.lastMsg != "debug fallback" {
		t.Errorf("Debug fallback: got level=%v msg=%q", mock.lastLevel, mock.lastMsg)
	}

	xlog.Info(ctx, "info fallback")
	if mock.lastLevel != slog.LevelInfo || mock.lastMsg != "info fallback" {
		t.Errorf("Info fallback: got level=%v msg=%q", mock.lastLevel, mock.lastMsg)
	}

	xlog.Warn(ctx, "warn fallback")
	if mock.lastLevel != slog.LevelWarn || mock.lastMsg != "warn fallback" {
		t.Errorf("Warn fallback: got level=%v msg=%q", mock.lastLevel, mock.lastMsg)
	}

	xlog.Error(ctx, "error fallback")
	if mock.lastLevel != slog.LevelError || mock.lastMsg != "error fallback" {
		t.Errorf("Error fallback: got level=%v msg=%q", mock.lastLevel, mock.lastMsg)
	}
}

func TestGlobal_Stack_FallbackNonXlogger(t *testing.T) {
	xlog.ResetDefault()
	defer xlog.ResetDefault()

	mock := &mockLoggerWithLevel{}
	xlog.SetDefault(mock)

	// 测试 Stack 通过 fallback 路径
	xlog.Stack(context.Background(), "stack fallback")
	if mock.lastLevel != slog.LevelError || mock.lastMsg != "stack fallback" {
		t.Errorf("Stack fallback: got level=%v msg=%q", mock.lastLevel, mock.lastMsg)
	}
}

// =============================================================================
// ResetDefault 测试
// =============================================================================

func TestResetDefault(t *testing.T) {
	// 重置全局状态
	xlog.ResetDefault()

	// 获取默认 Logger
	logger1 := xlog.Default()

	// 重置
	xlog.ResetDefault()

	// 再次获取应该是新的 Logger
	logger2 := xlog.Default()

	// 由于都是默认配置，可能是相同配置但应该是不同实例
	// 注：这个测试主要验证 ResetDefault 不会 panic
	if logger1 == nil || logger2 == nil {
		t.Error("Default() should never return nil")
	}
}

// =============================================================================
// 性能测试
// =============================================================================

func BenchmarkDefault(b *testing.B) {
	xlog.ResetDefault()
	defer xlog.ResetDefault()

	// 预热
	_ = xlog.Default()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = xlog.Default()
	}
}

func BenchmarkGlobal_Info(b *testing.B) {
	xlog.ResetDefault()
	defer xlog.ResetDefault()

	// 设置一个禁用输出的 Logger（避免 I/O 开销）
	logger, cleanup, _ := xlog.New().
		SetLevel(xlog.LevelError). // 禁用 Info
		Build()
	defer func() { _ = cleanup() }()
	xlog.SetDefault(logger)

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		xlog.Info(ctx, "benchmark message")
	}
}
