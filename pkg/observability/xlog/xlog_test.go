package xlog_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
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

func TestBuilder_SetFormat(t *testing.T) {
	tests := []struct {
		format   string
		contains string
	}{
		{"text", "msg="},  // text 格式包含 msg=
		{"json", `"msg"`}, // JSON 格式包含 "msg"
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

func TestBuilder_SetEnrich_Disabled(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetFormat("json").
		SetEnrich(false). // 禁用 enrich
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	logger.Info(context.Background(), "test without enrich")

	output := buf.String()
	if !strings.Contains(output, "test without enrich") {
		t.Errorf("output missing message\noutput: %s", output)
	}
}

func TestBuilder_EnrichHandler_WithContext(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetFormat("json").
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	// 设置 context 包含 trace 和 identity 信息
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "trace-integration-test")
	ctx, _ = xctx.WithTenantID(ctx, "tenant-integration-test")

	logger.Info(ctx, "enriched message")

	output := buf.String()

	// 验证 trace_id 和 tenant_id 被注入
	wantContains := []string{
		"enriched message",
		"trace-integration-test",
		"tenant-integration-test",
	}

	for _, want := range wantContains {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\noutput: %s", want, output)
		}
	}
}

func TestBuilder_EnrichHandler_DisabledNoInjection(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetFormat("json").
		SetEnrich(false). // 禁用 enrich
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	// 设置 context 包含 trace 信息
	ctx, _ := xctx.WithTraceID(context.Background(), "trace-should-not-appear")
	logger.Info(ctx, "no enrich message")

	output := buf.String()

	// 消息应该存在
	if !strings.Contains(output, "no enrich message") {
		t.Errorf("output missing message\noutput: %s", output)
	}

	// trace_id 不应该被注入（因为禁用了 enrich）
	if strings.Contains(output, "trace-should-not-appear") {
		t.Errorf("output should not contain trace_id when enrich disabled\noutput: %s", output)
	}
}

// =============================================================================
// SetRotation 测试
// =============================================================================

func TestBuilder_SetRotation(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()
	logFile := tmpDir + "/test.log"

	logger, cleanup, err := xlog.New().
		SetRotation(logFile).
		SetLevel(xlog.LevelInfo).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	// 写入日志
	logger.Info(context.Background(), "rotation test message")

	// 调用 cleanup 关闭文件
	if err := cleanup(); err != nil {
		t.Errorf("cleanup() error: %v", err)
	}

	// 验证日志文件已创建并包含内容
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	if !strings.Contains(string(data), "rotation test message") {
		t.Errorf("log file missing message\ncontent: %s", string(data))
	}
}

func TestBuilder_SetRotation_WithCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := tmpDir + "/cleanup-test.log"

	logger, cleanup, err := xlog.New().
		SetRotation(logFile).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	logger.Info(context.Background(), "before cleanup")

	// 第一次 cleanup
	if err := cleanup(); err != nil {
		t.Errorf("first cleanup() error: %v", err)
	}

	// 第二次 cleanup 应该安全（sync.Once 保护）
	if err := cleanup(); err != nil {
		t.Errorf("second cleanup() error: %v", err)
	}
}

// =============================================================================
// Stack 级别禁用测试
// =============================================================================

func TestLogger_Stack_Disabled(t *testing.T) {
	var buf bytes.Buffer
	// 设置级别高于 Error，Stack 应该不输出
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetLevel(xlog.Level(100)). // 高于 Error 的自定义级别
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	logger.Stack(context.Background(), "should not appear")

	if buf.Len() > 0 {
		t.Errorf("Stack should not output when level is disabled\noutput: %s", buf.String())
	}
}

func TestBuilder_SetRotation_Error(t *testing.T) {
	// 空文件名应该导致错误
	_, _, err := xlog.New().
		SetRotation("").
		Build()
	if err == nil {
		t.Error("SetRotation with empty filename should return error")
	}
}

// =============================================================================
// DeploymentType 固定属性测试
// =============================================================================

func TestBuilder_SetDeploymentType(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetFormat("json").
		SetDeploymentType(xctx.DeploymentSaaS).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	logger.Info(context.Background(), "deployment test")

	output := buf.String()
	// 验证 deployment_type 字段存在
	if !strings.Contains(output, "deployment_type") {
		t.Errorf("output missing deployment_type\noutput: %s", output)
	}
	if !strings.Contains(output, "SAAS") {
		t.Errorf("output missing SAAS value\noutput: %s", output)
	}
}

func TestBuilder_SetDeploymentType_Local(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetFormat("json").
		SetDeploymentType(xctx.DeploymentLocal).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	logger.Info(context.Background(), "local test")

	output := buf.String()
	if !strings.Contains(output, "LOCAL") {
		t.Errorf("output missing LOCAL value\noutput: %s", output)
	}
}

func TestBuilder_SetDeploymentType_Invalid(t *testing.T) {
	_, _, err := xlog.New().
		SetDeploymentType("INVALID").
		Build()
	if err == nil {
		t.Error("Build() should return error for invalid deployment type")
	}
}

func TestBuilder_SetDeploymentType_NotSet(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetFormat("json").
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	logger.Info(context.Background(), "no deployment type")

	output := buf.String()
	// 未设置时不应包含 deployment_type
	if strings.Contains(output, "deployment_type") {
		t.Errorf("output should not contain deployment_type when not set\noutput: %s", output)
	}
}

// =============================================================================
// OnInternalError 回调测试
// =============================================================================

func TestBuilder_SetOnError(t *testing.T) {
	var buf bytes.Buffer
	var callbackErrors []error

	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetOnError(func(err error) {
			callbackErrors = append(callbackErrors, err)
		}).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	// 正常日志应该不触发 OnError
	logger.Info(context.Background(), "normal message")

	if len(callbackErrors) > 0 {
		t.Errorf("OnError should not be called for normal logging, got %d calls", len(callbackErrors))
	}
}

// =============================================================================
// SetReplaceAttr 治理能力测试
// =============================================================================

func TestBuilder_SetReplaceAttr(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetFormat("json").
		SetReplaceAttr(func(groups []string, a slog.Attr) slog.Attr {
			// 脱敏 password 字段
			if a.Key == "password" {
				return slog.String("password", "***")
			}
			return a
		}).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	logger.Info(context.Background(), "login", slog.String("password", "secret123"))

	output := buf.String()
	if strings.Contains(output, "secret123") {
		t.Errorf("password should be masked\noutput: %s", output)
	}
	if !strings.Contains(output, "***") {
		t.Errorf("output should contain masked password\noutput: %s", output)
	}
}

func TestBuilder_SetReplaceAttr_RemoveField(t *testing.T) {
	var buf bytes.Buffer
	logger, cleanup, err := xlog.New().
		SetOutput(&buf).
		SetFormat("json").
		SetReplaceAttr(func(groups []string, a slog.Attr) slog.Attr {
			// 移除 debug 字段
			if a.Key == "debug" {
				return slog.Attr{} // 空 key 会被移除
			}
			return a
		}).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	logger.Info(context.Background(), "test",
		slog.String("debug", "internal data"),
		slog.String("user", "alice"))

	output := buf.String()
	if strings.Contains(output, "internal data") {
		t.Errorf("debug field should be removed\noutput: %s", output)
	}
	if !strings.Contains(output, "alice") {
		t.Errorf("user field should be present\noutput: %s", output)
	}
}

// =============================================================================
// SetDeploymentTypeFromEnv 测试
// =============================================================================

func TestBuilder_SetDeploymentTypeFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
		wantErr  bool
	}{
		{"SAAS from env", "SAAS", "SAAS", false},
		{"LOCAL from env", "LOCAL", "LOCAL", false},
		{"invalid from env", "INVALID", "", true},
		{"empty env", "", "", true}, // 空环境变量会返回错误
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置环境变量
			if tt.envValue != "" {
				os.Setenv("DEPLOYMENT_TYPE", tt.envValue)
				defer os.Unsetenv("DEPLOYMENT_TYPE")
			} else {
				os.Unsetenv("DEPLOYMENT_TYPE")
			}

			var buf bytes.Buffer
			logger, cleanup, err := xlog.New().
				SetOutput(&buf).
				SetFormat("json").
				SetDeploymentTypeFromEnv().
				Build()

			if tt.wantErr {
				if err == nil {
					t.Error("Build() should return error for invalid/empty env value")
				}
				return
			}

			if err != nil {
				t.Fatalf("Build() error: %v", err)
			}
			testCleanup(t, cleanup)

			logger.Info(context.Background(), "env test")

			output := buf.String()
			if tt.want != "" && !strings.Contains(output, tt.want) {
				t.Errorf("output missing %q\noutput: %s", tt.want, output)
			}
		})
	}
}

// =============================================================================
// handleError 递归保护测试
// =============================================================================

func TestHandleError_RecursionProtection(t *testing.T) {
	var callCount int

	// 使用一个总是失败的 writer 来触发 onError
	failingWriter := &failingWriter{}

	// onError 回调计数
	logger, cleanup, err := xlog.New().
		SetOutput(failingWriter).
		SetOnError(func(err error) {
			callCount++
			// 在 onError 中不要真的再写日志，只是计数
			// 真实场景中如果 onError 内部写日志失败，应该不会递归
		}).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	// 触发一次日志写入，failingWriter 会返回错误
	logger.Info(context.Background(), "test message")

	// 验证 onError 被调用（至少一次）
	if callCount == 0 {
		t.Error("onError should have been called at least once")
	}

	// 再次写日志，验证递归保护状态已重置
	prevCount := callCount
	logger.Info(context.Background(), "another message")
	if callCount == prevCount {
		t.Error("onError should be called again after reset")
	}
}

// failingWriter 是一个总是返回错误的 Writer，用于测试 onError 回调
type failingWriter struct{}

func (w *failingWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("simulated write error")
}
