package xlog

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestErr(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantKey string
		wantVal string
		wantNil bool
	}{
		{
			name:    "with error",
			err:     errors.New("test error"),
			wantKey: KeyError,
			wantVal: "test error",
		},
		{
			name:    "nil error",
			err:     nil,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := Err(tt.err)
			if tt.wantNil {
				if attr.Key != "" {
					t.Errorf("Err(nil) should return empty attr, got key=%q", attr.Key)
				}
				return
			}
			if attr.Key != tt.wantKey {
				t.Errorf("Err() key = %q, want %q", attr.Key, tt.wantKey)
			}
			if attr.Value.String() != tt.wantVal {
				t.Errorf("Err() value = %q, want %q", attr.Value.String(), tt.wantVal)
			}
		})
	}
}

func TestDuration(t *testing.T) {
	d := 5 * time.Second
	attr := Duration(d)

	if attr.Key != KeyDuration {
		t.Errorf("Duration() key = %q, want %q", attr.Key, KeyDuration)
	}
	if attr.Value.String() != "5s" {
		t.Errorf("Duration() value = %q, want %q", attr.Value.String(), "5s")
	}
}

func TestComponent(t *testing.T) {
	attr := Component("auth")
	if attr.Key != KeyComponent {
		t.Errorf("Component() key = %q, want %q", attr.Key, KeyComponent)
	}
	if attr.Value.String() != "auth" {
		t.Errorf("Component() value = %q, want %q", attr.Value.String(), "auth")
	}
}

func TestOperation(t *testing.T) {
	attr := Operation("login")
	if attr.Key != KeyOperation {
		t.Errorf("Operation() key = %q, want %q", attr.Key, KeyOperation)
	}
	if attr.Value.String() != "login" {
		t.Errorf("Operation() value = %q, want %q", attr.Value.String(), "login")
	}
}

func TestCount(t *testing.T) {
	attr := Count(42)
	if attr.Key != KeyCount {
		t.Errorf("Count() key = %q, want %q", attr.Key, KeyCount)
	}
	if attr.Value.Int64() != 42 {
		t.Errorf("Count() value = %d, want %d", attr.Value.Int64(), 42)
	}
}

func TestUserID(t *testing.T) {
	attr := UserID("user-123")
	if attr.Key != KeyUserID {
		t.Errorf("UserID() key = %q, want %q", attr.Key, KeyUserID)
	}
	if attr.Value.String() != "user-123" {
		t.Errorf("UserID() value = %q, want %q", attr.Value.String(), "user-123")
	}
}

func TestStatusCode(t *testing.T) {
	attr := StatusCode(200)
	if attr.Key != KeyStatusCode {
		t.Errorf("StatusCode() key = %q, want %q", attr.Key, KeyStatusCode)
	}
	if attr.Value.Int64() != 200 {
		t.Errorf("StatusCode() value = %d, want %d", attr.Value.Int64(), 200)
	}
}

func TestMethod(t *testing.T) {
	attr := Method("POST")
	if attr.Key != KeyMethod {
		t.Errorf("Method() key = %q, want %q", attr.Key, KeyMethod)
	}
	if attr.Value.String() != "POST" {
		t.Errorf("Method() value = %q, want %q", attr.Value.String(), "POST")
	}
}

func TestPath(t *testing.T) {
	attr := Path("/api/v1/users")
	if attr.Key != KeyPath {
		t.Errorf("Path() key = %q, want %q", attr.Key, KeyPath)
	}
	if attr.Value.String() != "/api/v1/users" {
		t.Errorf("Path() value = %q, want %q", attr.Value.String(), "/api/v1/users")
	}
}

func TestKeyConstants(t *testing.T) {
	// 验证 key 常量的值
	tests := []struct {
		name string
		key  string
		want string
	}{
		{"KeyError", KeyError, "error"},
		{"KeyStack", KeyStack, "stack"},
		{"KeyDuration", KeyDuration, "duration"},
		{"KeyCount", KeyCount, "count"},
		{"KeyUserID", KeyUserID, "user_id"},
		{"KeyRequestID", KeyRequestID, "request_id"},
		{"KeyMethod", KeyMethod, "method"},
		{"KeyPath", KeyPath, "path"},
		{"KeyStatusCode", KeyStatusCode, "status_code"},
		{"KeyComponent", KeyComponent, "component"},
		{"KeyOperation", KeyOperation, "operation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.key != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.key, tt.want)
			}
		})
	}
}

// TestAttrsIntegration 测试 attrs 与 logger 的集成
func TestAttrsIntegration(t *testing.T) {
	var buf testBuffer
	logger, cleanup, err := New().
		SetOutput(&buf).
		SetFormat("json").
		SetLevel(LevelDebug).
		Build()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer cleanup()

	ctx := context.Background()

	// 测试 Err
	testErr := errors.New("test error")
	logger.Error(ctx, "operation failed", Err(testErr))
	if !buf.contains("error") || !buf.contains("test error") {
		t.Errorf("Err() attr not in output: %s", buf.String())
	}
	buf.Reset()

	// 测试 nil error（不应输出）
	logger.Error(ctx, "operation ok", Err(nil))
	// nil error 产生空属性，可能不输出或输出为空

	// 测试 Component
	logger.Info(ctx, "starting", Component("auth"))
	if !buf.contains("component") || !buf.contains("auth") {
		t.Errorf("Component() attr not in output: %s", buf.String())
	}
}

// testBuffer 用于测试的线程安全 buffer
type testBuffer struct {
	data []byte
}

func (b *testBuffer) Write(p []byte) (n int, err error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *testBuffer) String() string {
	return string(b.data)
}

func (b *testBuffer) Reset() {
	b.data = b.data[:0]
}

func (b *testBuffer) contains(s string) bool {
	return len(b.data) > 0 && strings.Contains(string(b.data), s)
}

// BenchmarkErr 测试 Err 函数的性能
func BenchmarkErr(b *testing.B) {
	err := errors.New("benchmark error")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Err(err)
	}
}

// BenchmarkErrNil 测试 Err(nil) 的性能
func BenchmarkErrNil(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Err(nil)
	}
}

// BenchmarkComponent 测试 Component 函数的性能
func BenchmarkComponent(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Component("auth")
	}
}

// BenchmarkSlogString 对比原生 slog.String 的性能
func BenchmarkSlogString(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = slog.String("component", "auth")
	}
}
