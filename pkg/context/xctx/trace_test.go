package xctx_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

// =============================================================================
// Trace 操作测试
// =============================================================================

func TestTraceID(t *testing.T) {
	if got := xctx.TraceID(context.Background()); got != "" {
		t.Errorf("TraceID(empty) = %q, want empty", got)
	}

	ctx, err := xctx.WithTraceID(context.Background(), "trace-123")
	if err != nil {
		t.Fatalf("WithTraceID() error = %v", err)
	}
	if got := xctx.TraceID(ctx); got != "trace-123" {
		t.Errorf("TraceID() = %q, want %q", got, "trace-123")
	}

	ctx, err = xctx.WithTraceID(ctx, "new-trace")
	if err != nil {
		t.Fatalf("WithTraceID() error = %v", err)
	}
	if got := xctx.TraceID(ctx); got != "new-trace" {
		t.Errorf("TraceID(overwrite) = %q, want %q", got, "new-trace")
	}

	var nilCtx context.Context
	if got := xctx.TraceID(nilCtx); got != "" {
		t.Errorf("TraceID(nil) = %q, want empty", got)
	}

	// nil context 注入返回 ErrNilContext
	_, err = xctx.WithTraceID(nilCtx, "trace-123")
	if !errors.Is(err, xctx.ErrNilContext) {
		t.Errorf("WithTraceID(nil) error = %v, want %v", err, xctx.ErrNilContext)
	}
}

func TestSpanAndRequestID(t *testing.T) {
	tests := []struct {
		name      string
		testValue string
		setter    func(context.Context, string) (context.Context, error)
		getter    func(context.Context) string
	}{
		{
			name:      "SpanID",
			testValue: "span-456",
			setter:    xctx.WithSpanID,
			getter:    xctx.SpanID,
		},
		{
			name:      "RequestID",
			testValue: "req-789",
			setter:    xctx.WithRequestID,
			getter:    xctx.RequestID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 正常注入和提取
			ctx, err := tt.setter(context.Background(), tt.testValue)
			if err != nil {
				t.Fatalf("%s() error = %v", tt.name, err)
			}
			if got := tt.getter(ctx); got != tt.testValue {
				t.Errorf("%s() = %q, want %q", tt.name, got, tt.testValue)
			}

			// 空 context 返回空字符串
			if got := tt.getter(context.Background()); got != "" {
				t.Errorf("%s(empty) = %q, want empty", tt.name, got)
			}

			// nil context 返回空字符串
			if got := tt.getter(nil); got != "" {
				t.Errorf("%s(nil) = %q, want empty", tt.name, got)
			}

			// nil context 注入返回 ErrNilContext
			_, err = tt.setter(nil, tt.testValue)
			if !errors.Is(err, xctx.ErrNilContext) {
				t.Errorf("With%s(nil) error = %v, want %v", tt.name, err, xctx.ErrNilContext)
			}
		})
	}
}

// =============================================================================
// Trace 结构体测试
// =============================================================================

func TestGetTrace(t *testing.T) {
	t.Run("空context返回空结构体", func(t *testing.T) {
		tr := xctx.GetTrace(context.Background())
		if tr.TraceID != "" || tr.SpanID != "" || tr.RequestID != "" {
			t.Errorf("GetTrace(empty) = %+v, want empty fields", tr)
		}
	})

	t.Run("正常获取", func(t *testing.T) {
		ctx, _ := xctx.WithTraceID(context.Background(), "t1")
		ctx, _ = xctx.WithSpanID(ctx, "s1")
		ctx, _ = xctx.WithRequestID(ctx, "r1")

		tr := xctx.GetTrace(ctx)
		if tr.TraceID != "t1" {
			t.Errorf("TraceID = %q, want %q", tr.TraceID, "t1")
		}
		if tr.SpanID != "s1" {
			t.Errorf("SpanID = %q, want %q", tr.SpanID, "s1")
		}
		if tr.RequestID != "r1" {
			t.Errorf("RequestID = %q, want %q", tr.RequestID, "r1")
		}
	})
}

func TestTrace_IsComplete(t *testing.T) {
	tests := []struct {
		name string
		tr   xctx.Trace
		want bool
	}{
		{"全部存在", xctx.Trace{TraceID: "t1", SpanID: "s1", RequestID: "r1"}, true},
		{"全部为空", xctx.Trace{}, false},
		{"缺少一个", xctx.Trace{TraceID: "t1", SpanID: "s1"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tr.IsComplete(); got != tt.want {
				t.Errorf("IsComplete() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// ID 生成函数测试（W3C Trace Context 规范）
// =============================================================================

// testGenerateID 通用 ID 生成测试辅助函数
func testGenerateID(t *testing.T, name string, wantLen int, generator func() string) {
	t.Helper()

	t.Run("格式正确", func(t *testing.T) {
		id := generator()
		if len(id) != wantLen {
			t.Errorf("%s len = %d, want %d", name, len(id), wantLen)
		}
		for _, c := range id {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("%s contains invalid char: %c", name, c)
			}
		}
	})

	t.Run("每次生成不同", func(t *testing.T) {
		ids := make(map[string]bool)
		for i := 0; i < 1000; i++ {
			id := generator()
			if ids[id] {
				t.Errorf("%s generated duplicate: %s", name, id)
			}
			ids[id] = true
		}
	})
}

func TestGenerateTraceID(t *testing.T) {
	// W3C 规范: 32位小写十六进制
	testGenerateID(t, "TraceID", 32, xctx.GenerateTraceID)
}

func TestGenerateSpanID(t *testing.T) {
	// W3C 规范: 16位小写十六进制
	testGenerateID(t, "SpanID", 16, xctx.GenerateSpanID)
}

func TestGenerateRequestID(t *testing.T) {
	// 与 TraceID 格式一致
	testGenerateID(t, "RequestID", 32, xctx.GenerateRequestID)
}

// =============================================================================
// Ensure 函数测试（自动补全模式）
// =============================================================================

func TestEnsureIDs(t *testing.T) {
	tests := []struct {
		name     string
		wantLen  int // 0 表示不检查长度
		existing string
		setter   func(context.Context, string) (context.Context, error)
		getter   func(context.Context) string
		ensure   func(context.Context) (context.Context, error)
	}{
		{
			name:     "TraceID",
			wantLen:  32,
			existing: "0af7651916cd43dd8448eb211c80319c",
			setter:   xctx.WithTraceID,
			getter:   xctx.TraceID,
			ensure:   xctx.EnsureTraceID,
		},
		{
			name:     "SpanID",
			wantLen:  16,
			existing: "b7ad6b7169203331",
			setter:   xctx.WithSpanID,
			getter:   xctx.SpanID,
			ensure:   xctx.EnsureSpanID,
		},
		{
			name:     "RequestID",
			wantLen:  32,
			existing: "req-existing-123",
			setter:   xctx.WithRequestID,
			getter:   xctx.RequestID,
			ensure:   xctx.EnsureRequestID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("空context自动生成", func(t *testing.T) {
				ctx, err := tt.ensure(context.Background())
				if err != nil {
					t.Fatalf("Ensure%s() error = %v", tt.name, err)
				}
				id := tt.getter(ctx)
				if id == "" {
					t.Errorf("Ensure%s() should generate ID for empty context", tt.name)
				}
				if tt.wantLen > 0 && len(id) != tt.wantLen {
					t.Errorf("Generated %s len = %d, want %d", tt.name, len(id), tt.wantLen)
				}
			})

			t.Run("已有值则沿用", func(t *testing.T) {
				ctx, err := tt.setter(context.Background(), tt.existing)
				if err != nil {
					t.Fatalf("setter() error = %v", err)
				}
				ctx, err = tt.ensure(ctx)
				if err != nil {
					t.Fatalf("Ensure%s() error = %v", tt.name, err)
				}
				if got := tt.getter(ctx); got != tt.existing {
					t.Errorf("Ensure%s() = %q, want existing %q", tt.name, got, tt.existing)
				}
			})

			t.Run("nil context返回ErrNilContext", func(t *testing.T) {
				_, err := tt.ensure(nil)
				if !errors.Is(err, xctx.ErrNilContext) {
					t.Errorf("Ensure%s(nil) error = %v, want %v", tt.name, err, xctx.ErrNilContext)
				}
			})
		})
	}
}

func TestEnsureTrace(t *testing.T) {
	t.Run("空context全部生成", func(t *testing.T) {
		ctx, err := xctx.EnsureTrace(context.Background())
		if err != nil {
			t.Fatalf("EnsureTrace() error = %v", err)
		}
		if xctx.TraceID(ctx) == "" {
			t.Error("EnsureTrace() should generate TraceID")
		}
		if xctx.SpanID(ctx) == "" {
			t.Error("EnsureTrace() should generate SpanID")
		}
		if xctx.RequestID(ctx) == "" {
			t.Error("EnsureTrace() should generate RequestID")
		}
	})

	t.Run("部分存在则部分生成", func(t *testing.T) {
		ctx, _ := xctx.WithTraceID(context.Background(), "existing-trace")
		ctx, err := xctx.EnsureTrace(ctx)
		if err != nil {
			t.Fatalf("EnsureTrace() error = %v", err)
		}

		// TraceID 应保持不变
		if got := xctx.TraceID(ctx); got != "existing-trace" {
			t.Errorf("TraceID should remain %q, got %q", "existing-trace", got)
		}
		// SpanID 和 RequestID 应被生成
		if xctx.SpanID(ctx) == "" {
			t.Error("SpanID should be generated")
		}
		if xctx.RequestID(ctx) == "" {
			t.Error("RequestID should be generated")
		}
	})

	t.Run("全部存在则全部沿用", func(t *testing.T) {
		ctx, _ := xctx.WithTraceID(context.Background(), "t1")
		ctx, _ = xctx.WithSpanID(ctx, "s1")
		ctx, _ = xctx.WithRequestID(ctx, "r1")
		ctx, err := xctx.EnsureTrace(ctx)
		if err != nil {
			t.Fatalf("EnsureTrace() error = %v", err)
		}

		if got := xctx.TraceID(ctx); got != "t1" {
			t.Errorf("TraceID = %q, want %q", got, "t1")
		}
		if got := xctx.SpanID(ctx); got != "s1" {
			t.Errorf("SpanID = %q, want %q", got, "s1")
		}
		if got := xctx.RequestID(ctx); got != "r1" {
			t.Errorf("RequestID = %q, want %q", got, "r1")
		}
	})

	t.Run("nil context返回ErrNilContext", func(t *testing.T) {
		var nilCtx context.Context
		_, err := xctx.EnsureTrace(nilCtx)
		if !errors.Is(err, xctx.ErrNilContext) {
			t.Errorf("EnsureTrace(nil) error = %v, want %v", err, xctx.ErrNilContext)
		}
	})
}

// =============================================================================
// 示例测试
// =============================================================================

func ExampleGetTrace() {
	ctx, _ := xctx.WithTraceID(context.Background(), "trace-001")
	ctx, _ = xctx.WithSpanID(ctx, "span-002")
	ctx, _ = xctx.WithRequestID(ctx, "req-003")

	tr := xctx.GetTrace(ctx)
	fmt.Println("TraceID:", tr.TraceID)
	fmt.Println("SpanID:", tr.SpanID)
	fmt.Println("RequestID:", tr.RequestID)
	// Output:
	// TraceID: trace-001
	// SpanID: span-002
	// RequestID: req-003
}

func ExampleEnsureTrace() {
	// HTTP 中间件入口使用 EnsureTrace 确保追踪信息
	ctx, _ := xctx.EnsureTrace(context.Background())

	// 后续代码可以安全获取追踪信息
	traceID := xctx.TraceID(ctx)
	fmt.Println("TraceID available:", traceID != "")
	// Output:
	// TraceID available: true
}

// =============================================================================
// WithTrace 批量注入测试
// =============================================================================

func TestWithTrace(t *testing.T) {
	t.Run("全部字段非空", func(t *testing.T) {
		tr := xctx.Trace{
			TraceID:   "trace-001",
			SpanID:    "span-002",
			RequestID: "req-003",
		}
		ctx, err := xctx.WithTrace(context.Background(), tr)
		if err != nil {
			t.Fatalf("WithTrace() error = %v", err)
		}

		got := xctx.GetTrace(ctx)
		if got.TraceID != tr.TraceID {
			t.Errorf("TraceID = %q, want %q", got.TraceID, tr.TraceID)
		}
		if got.SpanID != tr.SpanID {
			t.Errorf("SpanID = %q, want %q", got.SpanID, tr.SpanID)
		}
		if got.RequestID != tr.RequestID {
			t.Errorf("RequestID = %q, want %q", got.RequestID, tr.RequestID)
		}
	})

	t.Run("部分字段为空", func(t *testing.T) {
		tr := xctx.Trace{
			TraceID: "trace-001",
			// SpanID 和 RequestID 为空
		}
		ctx, err := xctx.WithTrace(context.Background(), tr)
		if err != nil {
			t.Fatalf("WithTrace() error = %v", err)
		}

		got := xctx.GetTrace(ctx)
		if got.TraceID != tr.TraceID {
			t.Errorf("TraceID = %q, want %q", got.TraceID, tr.TraceID)
		}
		// 空字段应被跳过，保持为空
		if got.SpanID != "" {
			t.Errorf("SpanID = %q, want empty", got.SpanID)
		}
		if got.RequestID != "" {
			t.Errorf("RequestID = %q, want empty", got.RequestID)
		}
	})

	t.Run("全部字段为空", func(t *testing.T) {
		tr := xctx.Trace{}
		ctx, err := xctx.WithTrace(context.Background(), tr)
		if err != nil {
			t.Fatalf("WithTrace() error = %v", err)
		}

		got := xctx.GetTrace(ctx)
		if got.TraceID != "" || got.SpanID != "" || got.RequestID != "" {
			t.Errorf("WithTrace(empty) should not inject any fields, got %+v", got)
		}
	})

	t.Run("nil context返回ErrNilContext", func(t *testing.T) {
		var nilCtx context.Context
		tr := xctx.Trace{TraceID: "t1"}
		_, err := xctx.WithTrace(nilCtx, tr)
		if !errors.Is(err, xctx.ErrNilContext) {
			t.Errorf("WithTrace(nil) error = %v, want %v", err, xctx.ErrNilContext)
		}
	})
}

func ExampleWithTrace() {
	// 从请求头解析追踪信息后批量注入
	tr := xctx.Trace{
		TraceID:   "0af7651916cd43dd8448eb211c80319c",
		SpanID:    "b7ad6b7169203331",
		RequestID: "req-from-upstream",
	}
	ctx, _ := xctx.WithTrace(context.Background(), tr)

	// 验证注入结果
	got := xctx.GetTrace(ctx)
	fmt.Println("TraceID:", got.TraceID)
	fmt.Println("IsComplete:", got.IsComplete())
	// Output:
	// TraceID: 0af7651916cd43dd8448eb211c80319c
	// IsComplete: true
}
