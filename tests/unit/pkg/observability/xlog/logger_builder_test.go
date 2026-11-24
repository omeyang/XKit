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

func TestBuilder_SetRotation_DoubleCalled(t *testing.T) {
	tmpDir := t.TempDir()
	file1 := tmpDir + "/first.log"
	file2 := tmpDir + "/second.log"

	// 连续调用两次 SetRotation，第一个 rotator 应被正确关闭
	logger, cleanup, err := xlog.New().
		SetRotation(file1).
		SetRotation(file2). // 应关闭 file1 的 rotator
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	// 写入日志
	logger.Info(context.Background(), "second rotator message")

	// 关闭
	if err := cleanup(); err != nil {
		t.Errorf("cleanup() error: %v", err)
	}

	// 验证第二个文件包含日志
	data, readErr := os.ReadFile(file2)
	if readErr != nil {
		t.Fatalf("ReadFile(%s) error: %v", file2, readErr)
	}
	if !strings.Contains(string(data), "second rotator message") {
		t.Errorf("second log file missing message\ncontent: %s", string(data))
	}
}

func TestBuilder_Build_AlreadyBuilt(t *testing.T) {
	builder := xlog.New()

	// 第一次 Build 应该成功
	_, cleanup, err := builder.Build()
	if err != nil {
		t.Fatalf("first Build() error: %v", err)
	}
	testCleanup(t, cleanup)

	// 第二次 Build 应该返回错误
	_, _, err = builder.Build()
	if err == nil {
		t.Fatal("second Build() should return error")
	}
	if !strings.Contains(err.Error(), "already built") {
		t.Errorf("error should mention 'already built', got: %v", err)
	}
}

// TestBuilder_Build_AlreadyBuilt_RotatorNotClosed 回归：第二次 Build() 不得关闭
// 第一次 Build() 已转交 cleanup 的 rotator，否则第一次 logger 写入将失败。
func TestBuilder_Build_AlreadyBuilt_RotatorNotClosed(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/regress.log"

	b := xlog.New().SetRotation(file)
	logger, cleanup, err := b.Build()
	if err != nil {
		t.Fatalf("first Build() error: %v", err)
	}

	// 第二次 Build 应返回错误但不应关闭第一次 logger 的 rotator
	if _, _, err2 := b.Build(); err2 == nil {
		t.Fatal("second Build() should return error")
	}

	logger.Info(context.Background(), "still alive after second build")

	if cleanupErr := cleanup(); cleanupErr != nil {
		t.Errorf("cleanup() error: %v", cleanupErr)
	}

	data, readErr := os.ReadFile(file)
	if readErr != nil {
		t.Fatalf("ReadFile error: %v", readErr)
	}
	if !strings.Contains(string(data), "still alive after second build") {
		t.Errorf("log file missing message after second Build()\ncontent: %s", string(data))
	}
}

// TestBuilder_SetOutput_ClosesPreviousRotator 回归：SetOutput 覆盖 SetRotation
// 时应关闭并清空 rotator，避免隐藏文件句柄泄漏到 cleanup。
func TestBuilder_SetOutput_ClosesPreviousRotator(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/rot-then-output.log"

	var buf bytes.Buffer
	_, cleanup, err := xlog.New().
		SetRotation(file).
		SetOutput(&buf).
		Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	// cleanup 应是 no-op（rotator 已在 SetOutput 中被关闭并清空）
	if cleanupErr := cleanup(); cleanupErr != nil {
		t.Errorf("cleanup() should not error, got: %v", cleanupErr)
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
			// 使用 t.Setenv 自动在测试结束时恢复环境变量
			if tt.envValue != "" {
				t.Setenv("DEPLOYMENT_TYPE", tt.envValue)
			} else {
				t.Setenv("DEPLOYMENT_TYPE", "")
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
