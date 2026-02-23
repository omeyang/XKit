package xctx_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestTrace_Validate(t *testing.T) {
	t.Run("全部存在", func(t *testing.T) {
		tr := xctx.Trace{TraceID: "t1", SpanID: "s1", RequestID: "r1"}
		if err := tr.Validate(); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("缺少TraceID", func(t *testing.T) {
		tr := xctx.Trace{SpanID: "s1", RequestID: "r1"}
		if err := tr.Validate(); !errors.Is(err, xctx.ErrMissingTraceID) {
			t.Errorf("Validate() error = %v, want %v", err, xctx.ErrMissingTraceID)
		}
	})

	t.Run("缺少SpanID", func(t *testing.T) {
		tr := xctx.Trace{TraceID: "t1", RequestID: "r1"}
		if err := tr.Validate(); !errors.Is(err, xctx.ErrMissingSpanID) {
			t.Errorf("Validate() error = %v, want %v", err, xctx.ErrMissingSpanID)
		}
	})

	t.Run("缺少RequestID", func(t *testing.T) {
		tr := xctx.Trace{TraceID: "t1", SpanID: "s1"}
		if err := tr.Validate(); !errors.Is(err, xctx.ErrMissingRequestID) {
			t.Errorf("Validate() error = %v, want %v", err, xctx.ErrMissingRequestID)
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
// Require 函数测试（强制获取模式）
// =============================================================================

func TestRequireTraceFunctions(t *testing.T) {
	tests := []struct {
		name      string
		testValue string
		wantErr   error
		setter    func(context.Context, string) (context.Context, error)
		require   func(context.Context) (string, error)
	}{
		{
			name:      "TraceID",
			testValue: "trace-123",
			wantErr:   xctx.ErrMissingTraceID,
			setter:    xctx.WithTraceID,
			require:   xctx.RequireTraceID,
		},
		{
			name:      "SpanID",
			testValue: "span-456",
			wantErr:   xctx.ErrMissingSpanID,
			setter:    xctx.WithSpanID,
			require:   xctx.RequireSpanID,
		},
		{
			name:      "RequestID",
			testValue: "req-789",
			wantErr:   xctx.ErrMissingRequestID,
			setter:    xctx.WithRequestID,
			require:   xctx.RequireRequestID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("存在则返回", func(t *testing.T) {
				ctx, err := tt.setter(context.Background(), tt.testValue)
				if err != nil {
					t.Fatalf("setter() error = %v", err)
				}
				got, err := tt.require(ctx)
				if err != nil {
					t.Errorf("Require%s() error = %v", tt.name, err)
				}
				if got != tt.testValue {
					t.Errorf("Require%s() = %q, want %q", tt.name, got, tt.testValue)
				}
			})

			t.Run("不存在则返回错误", func(t *testing.T) {
				_, err := tt.require(context.Background())
				if err == nil {
					t.Errorf("Require%s() should return error for empty context", tt.name)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
			})

			t.Run("nil context返回ErrNilContext", func(t *testing.T) {
				var nilCtx context.Context
				_, err := tt.require(nilCtx)
				if !errors.Is(err, xctx.ErrNilContext) {
					t.Errorf("Require%s(nil) error = %v, want %v", tt.name, err, xctx.ErrNilContext)
				}
			})
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
		require.NoError(t, err, "EnsureTrace()")
		assert.NotEmpty(t, xctx.TraceID(ctx), "EnsureTrace() should generate TraceID")
		assert.NotEmpty(t, xctx.SpanID(ctx), "EnsureTrace() should generate SpanID")
		assert.NotEmpty(t, xctx.RequestID(ctx), "EnsureTrace() should generate RequestID")
	})

	t.Run("部分存在则部分生成", func(t *testing.T) {
		ctx, _ := xctx.WithTraceID(context.Background(), "existing-trace")
		ctx, err := xctx.EnsureTrace(ctx)
		require.NoError(t, err, "EnsureTrace()")

		// TraceID 应保持不变
		assert.Equal(t, "existing-trace", xctx.TraceID(ctx), "TraceID should remain")
		// SpanID 和 RequestID 应被生成
		assert.NotEmpty(t, xctx.SpanID(ctx), "SpanID should be generated")
		assert.NotEmpty(t, xctx.RequestID(ctx), "RequestID should be generated")
	})

	t.Run("全部存在则全部沿用", func(t *testing.T) {
		ctx, _ := xctx.WithTraceID(context.Background(), "t1")
		ctx, _ = xctx.WithSpanID(ctx, "s1")
		ctx, _ = xctx.WithRequestID(ctx, "r1")
		ctx, err := xctx.EnsureTrace(ctx)
		require.NoError(t, err, "EnsureTrace()")

		assert.Equal(t, "t1", xctx.TraceID(ctx), "TraceID")
		assert.Equal(t, "s1", xctx.SpanID(ctx), "SpanID")
		assert.Equal(t, "r1", xctx.RequestID(ctx), "RequestID")
	})

	t.Run("nil context返回ErrNilContext", func(t *testing.T) {
		var nilCtx context.Context
		_, err := xctx.EnsureTrace(nilCtx)
		assert.ErrorIs(t, err, xctx.ErrNilContext, "EnsureTrace(nil)")
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
		require.NoError(t, err, "WithTrace()")

		got := xctx.GetTrace(ctx)
		assert.Equal(t, tr.TraceID, got.TraceID, "TraceID")
		assert.Equal(t, tr.SpanID, got.SpanID, "SpanID")
		assert.Equal(t, tr.RequestID, got.RequestID, "RequestID")
	})

	t.Run("部分字段为空", func(t *testing.T) {
		tr := xctx.Trace{
			TraceID: "trace-001",
			// SpanID 和 RequestID 为空
		}
		ctx, err := xctx.WithTrace(context.Background(), tr)
		require.NoError(t, err, "WithTrace()")

		got := xctx.GetTrace(ctx)
		assert.Equal(t, tr.TraceID, got.TraceID, "TraceID")
		// 空字段应被跳过，保持为空
		assert.Empty(t, got.SpanID, "SpanID should be empty")
		assert.Empty(t, got.RequestID, "RequestID should be empty")
	})

	t.Run("全部字段为空", func(t *testing.T) {
		tr := xctx.Trace{}
		ctx, err := xctx.WithTrace(context.Background(), tr)
		require.NoError(t, err, "WithTrace()")

		got := xctx.GetTrace(ctx)
		assert.Empty(t, got.TraceID, "TraceID should be empty")
		assert.Empty(t, got.SpanID, "SpanID should be empty")
		assert.Empty(t, got.RequestID, "RequestID should be empty")
	})

	t.Run("nil context返回ErrNilContext", func(t *testing.T) {
		var nilCtx context.Context
		tr := xctx.Trace{TraceID: "t1"}
		_, err := xctx.WithTrace(nilCtx, tr)
		assert.ErrorIs(t, err, xctx.ErrNilContext, "WithTrace(nil)")
	})
}

// =============================================================================
// TraceFlags 操作测试
// =============================================================================

func TestTraceFlags(t *testing.T) {
	t.Run("空context返回空字符串", func(t *testing.T) {
		assert.Empty(t, xctx.TraceFlags(context.Background()), "TraceFlags(empty)")
	})

	t.Run("nil context返回空字符串", func(t *testing.T) {
		var nilCtx context.Context
		assert.Empty(t, xctx.TraceFlags(nilCtx), "TraceFlags(nil)")
	})

	t.Run("正常注入和提取", func(t *testing.T) {
		ctx, err := xctx.WithTraceFlags(context.Background(), "01")
		require.NoError(t, err, "WithTraceFlags()")
		assert.Equal(t, "01", xctx.TraceFlags(ctx), "TraceFlags()")
	})

	t.Run("覆盖写入返回新值", func(t *testing.T) {
		ctx, err := xctx.WithTraceFlags(context.Background(), "00")
		require.NoError(t, err, "WithTraceFlags()")
		ctx, err = xctx.WithTraceFlags(ctx, "01")
		require.NoError(t, err, "WithTraceFlags()")
		assert.Equal(t, "01", xctx.TraceFlags(ctx), "TraceFlags(overwrite)")
	})

	t.Run("nil context注入返回ErrNilContext", func(t *testing.T) {
		var nilCtx context.Context
		_, err := xctx.WithTraceFlags(nilCtx, "01")
		assert.ErrorIs(t, err, xctx.ErrNilContext, "WithTraceFlags(nil)")
	})
}

// =============================================================================
// WithTrace 包含 TraceFlags 测试
// =============================================================================

func TestWithTrace_TraceFlags(t *testing.T) {
	t.Run("TraceFlags 随 WithTrace 注入", func(t *testing.T) {
		tr := xctx.Trace{
			TraceID:    "trace-001",
			SpanID:     "span-002",
			RequestID:  "req-003",
			TraceFlags: "01",
		}
		ctx, err := xctx.WithTrace(context.Background(), tr)
		require.NoError(t, err, "WithTrace()")

		got := xctx.GetTrace(ctx)
		assert.Equal(t, "01", got.TraceFlags, "TraceFlags")
	})

	t.Run("空 TraceFlags 不注入", func(t *testing.T) {
		// 先设置 TraceFlags
		ctx, _ := xctx.WithTraceFlags(context.Background(), "01")
		// 用空 TraceFlags 的 Trace 覆盖
		tr := xctx.Trace{TraceID: "t1"}
		ctx, err := xctx.WithTrace(ctx, tr)
		require.NoError(t, err, "WithTrace()")
		// 原 TraceFlags 应保留
		assert.Equal(t, "01", xctx.TraceFlags(ctx), "TraceFlags should keep existing")
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
