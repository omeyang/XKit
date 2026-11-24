package xlog_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/observability/xlog"
)

// testCleanup 测试辅助函数，在测试结束时执行 cleanup
func testCleanup(t *testing.T, cleanup func() error) {
	t.Helper()
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup error: %v", err)
		}
	})
}

// =============================================================================
// Logger 接口测试
// =============================================================================

func TestLogger_BasicLogging(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetLevel(xlog.LevelDebug).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	ctx := context.Background()

	// 测试各级别日志
	logger.Debug(ctx, "debug message")
	logger.Info(ctx, "info message")
	logger.Warn(ctx, "warn message")
	logger.Error(ctx, "error message")

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

func TestLogger_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetLevel(xlog.LevelInfo).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	// 创建带属性的 logger
	childLogger := logger.With(slog.String("service", "test-svc"))
	childLogger.Info(context.Background(), "with attrs")

	output := buf.String()
	if !strings.Contains(output, "service") || !strings.Contains(output, "test-svc") {
		t.Errorf("output missing attrs\noutput: %s", output)
	}
}

func TestLogger_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetLevel(xlog.LevelInfo).
		SetFormat("json").
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	// 创建分组 logger
	groupLogger := logger.WithGroup("request")
	groupLogger.Info(context.Background(), "grouped", slog.String("method", "GET"))

	output := buf.String()
	// JSON 格式下分组会以嵌套形式出现
	if !strings.Contains(output, "request") {
		t.Errorf("output missing group\noutput: %s", output)
	}
}

func TestLogger_Enabled(t *testing.T) {
	logger, cleanup, err := xlog.New().
		SetLevel(xlog.LevelWarn).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	// Build() 返回 LoggerWithLevel，无需类型断言
	ctx := context.Background()

	if logger.Enabled(ctx, xlog.LevelDebug) {
		t.Error("Debug should not be enabled when level is Warn")
	}
	if logger.Enabled(ctx, xlog.LevelInfo) {
		t.Error("Info should not be enabled when level is Warn")
	}
	if !logger.Enabled(ctx, xlog.LevelWarn) {
		t.Error("Warn should be enabled when level is Warn")
	}
	if !logger.Enabled(ctx, xlog.LevelError) {
		t.Error("Error should be enabled when level is Warn")
	}
}

// =============================================================================
// 动态级别控制测试
// =============================================================================

func TestLogger_DynamicLevel(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetLevel(xlog.LevelError).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	// Build() 返回 LoggerWithLevel，无需类型断言
	ctx := context.Background()

	// 初始级别 Error，Info 不应输出
	logger.Info(ctx, "should not appear")
	if strings.Contains(buf.String(), "should not appear") {
		t.Error("Info should not be logged when level is Error")
	}

	// 动态调整到 Info
	logger.SetLevel(xlog.LevelInfo)
	logger.Info(ctx, "should appear")
	if !strings.Contains(buf.String(), "should appear") {
		t.Error("Info should be logged after SetLevel(Info)")
	}

	// 验证 GetLevel
	if logger.GetLevel() != xlog.LevelInfo {
		t.Errorf("GetLevel() = %v, want %v", logger.GetLevel(), xlog.LevelInfo)
	}
}

// =============================================================================
// Stack 追踪测试
// =============================================================================

func TestLogger_Stack(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetLevel(xlog.LevelDebug).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	logger.Stack(context.Background(), "stack trace test")

	output := buf.String()

	// 应该包含消息
	if !strings.Contains(output, "stack trace test") {
		t.Error("output missing message")
	}

	// 应该包含堆栈信息（至少包含 goroutine 或函数调用）
	if !strings.Contains(output, "goroutine") && !strings.Contains(output, "TestLogger_Stack") {
		t.Errorf("output missing stack trace\noutput: %s", output)
	}
}

// =============================================================================
// Builder 配置测试
// =============================================================================

func TestBuilder_SetLevel_String(t *testing.T) {
	logger, cleanup, err := xlog.New().
		SetLevelString("warn").
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	// Build() 返回 LoggerWithLevel，无需类型断言
	if logger.GetLevel() != xlog.LevelWarn {
		t.Errorf("GetLevel() = %v, want %v", logger.GetLevel(), xlog.LevelWarn)
	}
}

func TestBuilder_InvalidLevel(t *testing.T) {
	_, _, err := xlog.New().
		SetLevelString("invalid").
		Build()
	if err == nil {
		t.Error("Build() should return error for invalid level")
	}
}

// TestBuilder_ZeroValue_SetLevel 验证零值 Builder 调用 SetLevel 不 panic
func TestBuilder_ZeroValue_SetLevel(t *testing.T) {
	var b xlog.Builder
	b.SetLevel(xlog.LevelWarn).SetOutput(&bytes.Buffer{})

	logger, cleanup, err := b.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	if logger.GetLevel() != xlog.LevelWarn {
		t.Errorf("GetLevel() = %v, want %v", logger.GetLevel(), xlog.LevelWarn)
	}
}

func TestBuilder_ZeroValue_SetLevel_AfterBuild(t *testing.T) {
	var b xlog.Builder
	b.SetOutput(&bytes.Buffer{})

	logger, cleanup, err := b.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	logger.SetLevel(xlog.LevelWarn)
	if logger.GetLevel() != xlog.LevelWarn {
		t.Errorf("GetLevel() = %v, want %v", logger.GetLevel(), xlog.LevelWarn)
	}
}

func TestBuilder_SetFormat(t *testing.T) {
	tests := []struct {
		format   string
		contains string
	}{
		{"text", "msg="},  // text 格式包含 msg=
		{"json", `"msg"`}, // JSON 格式包含 "msg"
		{"", "msg="},      // 空字符串回退到 text 格式
		{"  ", "msg="},    // 空白字符串回退到 text 格式
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			var buf bytes.Buffer
			logger, cleanup, err := xlog.New().
				SetOutput(&buf).
				SetFormat(tt.format).
				Build()
			if err != nil {
				t.Fatalf("Build() error: %v", err)
			}
			testCleanup(t, cleanup)

			logger.Info(context.Background(), "test")

			if !strings.Contains(buf.String(), tt.contains) {
				t.Errorf("format %s output missing %q\noutput: %s",
					tt.format, tt.contains, buf.String())
			}
		})
	}
}

func TestBuilder_SetFormat_Invalid(t *testing.T) {
	_, _, err := xlog.New().
		SetFormat("yaml").
		Build()
	if err == nil {
		t.Error("Build() should return error for invalid format")
	}
}

func TestBuilder_NilOutput(t *testing.T) {
	_, _, err := xlog.New().
		SetOutput(nil).
		Build()
	if err == nil {
		t.Error("Build() should return error for nil output")
	}
}

func TestBuilder_SetOutput_TypedNil(t *testing.T) {
	var w *bytes.Buffer // typed-nil
	_, _, err := xlog.New().
		SetOutput(w).
		Build()
	if err == nil {
		t.Error("Build() should return error for typed-nil output writer")
	}
}

func TestBuilder_SetAddSource(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetAddSource(true).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	logger.Info(context.Background(), "with source")

	output := buf.String()
	// 应该包含 source 字段
	if !strings.Contains(output, "source=") {
		t.Errorf("output missing source info\noutput: %s", output)
	}
}

func TestBuilder_SetAddSource_Accuracy(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetAddSource(true).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	// 实例方法 source 应指向当前测试文件
	logger.Info(context.Background(), "source accuracy test")
	output := buf.String()
	if !strings.Contains(output, "xlog_test.go") {
		t.Errorf("instance Info source should point to xlog_test.go\noutput: %s", output)
	}

	// Stack 实例方法也应指向当前测试文件
	buf.Reset()
	logger.Stack(context.Background(), "stack source test")
	output = buf.String()
	if !strings.Contains(output, "xlog_test.go") {
		t.Errorf("instance Stack source should point to xlog_test.go\noutput: %s", output)
	}
}

func TestGlobal_SetAddSource_Accuracy(t *testing.T) {
	xlog.ResetDefault()
	defer xlog.ResetDefault()

	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetAddSource(true).
		SetLevel(xlog.LevelDebug).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)
	xlog.SetDefault(logger)

	// 全局便利函数 source 应指向当前测试文件
	xlog.Info(context.Background(), "global source test")
	output := buf.String()
	if !strings.Contains(output, "xlog_test.go") {
		t.Errorf("global Info source should point to xlog_test.go\noutput: %s", output)
	}

	// 全局 Stack source 应指向当前测试文件
	buf.Reset()
	xlog.Stack(context.Background(), "global stack source test")
	output = buf.String()
	if !strings.Contains(output, "xlog_test.go") {
		t.Errorf("global Stack source should point to xlog_test.go\noutput: %s", output)
	}
}

// =============================================================================
// Builder first-error-wins 测试
// =============================================================================

func TestBuilder_FirstErrorWins(t *testing.T) {
	// 第一个错误应该被保留，后续 Set 应被跳过
	_, _, err := xlog.New().
		SetLevelString("invalid_first").     // 第一个错误
		SetFormat("also_invalid").           // 应被跳过
		SetDeploymentType("NOT_VALID_TYPE"). // 应被跳过
		Build()
	if err == nil {
		t.Fatal("Build() should return error")
	}
	// 验证保留的是第一个错误
	if !strings.Contains(err.Error(), "invalid_first") {
		t.Errorf("Build() should return first error, got: %v", err)
	}
}

func TestBuilder_FirstErrorWins_AllSetMethods(t *testing.T) {
	// 表驱动测试：验证每个 Set 方法在 b.err != nil 时跳过执行
	tests := []struct {
		name  string
		apply func(*xlog.Builder) *xlog.Builder
	}{
		{"SetOutput", func(b *xlog.Builder) *xlog.Builder { return b.SetOutput(&bytes.Buffer{}) }},
		{"SetLevel", func(b *xlog.Builder) *xlog.Builder { return b.SetLevel(xlog.LevelDebug) }},
		{"SetLevelString", func(b *xlog.Builder) *xlog.Builder { return b.SetLevelString("debug") }},
		{"SetFormat", func(b *xlog.Builder) *xlog.Builder { return b.SetFormat("json") }},
		{"SetAddSource", func(b *xlog.Builder) *xlog.Builder { return b.SetAddSource(true) }},
		{"SetEnrich", func(b *xlog.Builder) *xlog.Builder { return b.SetEnrich(false) }},
		{"SetOnError", func(b *xlog.Builder) *xlog.Builder { return b.SetOnError(func(error) {}) }},
		{"SetReplaceAttr", func(b *xlog.Builder) *xlog.Builder {
			return b.SetReplaceAttr(func(_ []string, a slog.Attr) slog.Attr { return a })
		}},
		{"SetDeploymentType", func(b *xlog.Builder) *xlog.Builder {
			return b.SetDeploymentType(xctx.DeploymentSaaS)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 先注入错误，然后调用 Set 方法
			builder := xlog.New().SetLevelString("INVALID_SEED_ERROR")
			builder = tt.apply(builder)

			_, _, err := builder.Build()
			if err == nil {
				t.Fatal("Build() should return error")
			}
			// 验证保留的是种子错误，而非 Set 方法的结果
			if !strings.Contains(err.Error(), "INVALID_SEED_ERROR") {
				t.Errorf("error should be seed error, got: %v", err)
			}
		})
	}
}

// =============================================================================
// Cleanup 生命周期测试
// =============================================================================

func TestBuilder_Cleanup(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	// 使用 logger
	logger.Info(context.Background(), "test")

	// 调用 cleanup
	if err := cleanup(); err != nil {
		t.Errorf("cleanup() error: %v", err)
	}

	// 验证不会 panic（重复调用 cleanup 应该安全）
	if err := cleanup(); err != nil {
		t.Errorf("second cleanup() error: %v", err)
	}
}

// =============================================================================
// With/WithGroup 边界测试
// =============================================================================

func TestLogger_With_EmptyAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	// 空属性不应创建新 logger
	childLogger := logger.With()
	if childLogger != logger {
		t.Error("With() with empty attrs should return same logger")
	}
}

func TestLogger_WithGroup_EmptyName(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	// 空组名不应创建新 logger
	childLogger := logger.WithGroup("")
	if childLogger != logger {
		t.Error("WithGroup() with empty name should return same logger")
	}
}

// =============================================================================
// EnrichHandler 集成测试
// =============================================================================

func TestBuilder_EnrichHandler_Integration(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetFormat("json").
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	// 使用空 context，不应包含 trace/identity 信息
	logger.Info(context.Background(), "test message")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("output missing message\noutput: %s", output)
	}
}
