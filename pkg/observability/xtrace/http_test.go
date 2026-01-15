package xtrace_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/observability/xtrace"
)

// =============================================================================
// HTTP Header 提取测试
// =============================================================================

// makeHeader 创建 HTTP Header 并正确设置值
func makeHeader(kvs ...string) http.Header {
	h := make(http.Header)
	for i := 0; i < len(kvs)-1; i += 2 {
		h.Set(kvs[i], kvs[i+1])
	}
	return h
}

func TestExtractFromHTTPHeader(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   xtrace.TraceInfo
	}{
		{
			name:   "nil Header",
			header: nil,
			want:   xtrace.TraceInfo{},
		},
		{
			name:   "空 Header",
			header: http.Header{},
			want:   xtrace.TraceInfo{},
		},
		{
			name: "完整 Header",
			header: makeHeader(
				xtrace.HeaderTraceID, "0af7651916cd43dd8448eb211c80319c",
				xtrace.HeaderSpanID, "b7ad6b7169203331",
				xtrace.HeaderRequestID, "req-123",
			),
			want: xtrace.TraceInfo{
				TraceID:   "0af7651916cd43dd8448eb211c80319c",
				SpanID:    "b7ad6b7169203331",
				RequestID: "req-123",
			},
		},
		{
			name: "只有 TraceID",
			header: makeHeader(
				xtrace.HeaderTraceID, "abc123",
			),
			want: xtrace.TraceInfo{
				TraceID: "abc123",
			},
		},
		{
			name: "带空白的值会被 trim",
			header: makeHeader(
				xtrace.HeaderTraceID, "  trace123  ",
				xtrace.HeaderRequestID, "  req456  ",
			),
			want: xtrace.TraceInfo{
				TraceID:   "trace123",
				RequestID: "req456",
			},
		},
		{
			name: "W3C traceparent 解析",
			header: makeHeader(
				xtrace.HeaderTraceparent, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			),
			want: xtrace.TraceInfo{
				Traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
				TraceID:     "0af7651916cd43dd8448eb211c80319c",
				SpanID:      "b7ad6b7169203331",
			},
		},
		{
			name: "W3C traceparent 优先于自定义 Header",
			header: makeHeader(
				xtrace.HeaderTraceID, "custom-trace-id",
				xtrace.HeaderTraceparent, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			),
			want: xtrace.TraceInfo{
				TraceID:     "0af7651916cd43dd8448eb211c80319c", // W3C traceparent 覆盖自定义值
				SpanID:      "b7ad6b7169203331",
				Traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			},
		},
		{
			name: "无效 traceparent 被忽略",
			header: makeHeader(
				xtrace.HeaderTraceparent, "invalid-format",
			),
			want: xtrace.TraceInfo{
				Traceparent: "invalid-format",
			},
		},
		{
			name: "W3C 版本前向兼容 - 未知版本按 00 格式解析",
			header: makeHeader(
				// 版本 01 是未知的，但应该按 00 格式解析
				xtrace.HeaderTraceparent, "01-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			),
			want: xtrace.TraceInfo{
				Traceparent: "01-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
				TraceID:     "0af7651916cd43dd8448eb211c80319c",
				SpanID:      "b7ad6b7169203331",
				TraceFlags:  "01",
			},
		},
		{
			name: "W3C 版本前向兼容 - 版本 ff 保留为无效",
			header: makeHeader(
				// 版本 ff 始终无效
				xtrace.HeaderTraceparent, "ff-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			),
			want: xtrace.TraceInfo{
				Traceparent: "ff-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
				// TraceID 和 SpanID 应为空（因为 ff 版本无效）
			},
		},
		{
			name: "W3C 版本前向兼容 - 未来版本包含额外字段",
			header: makeHeader(
				// 未来版本可能包含额外字段，应忽略
				xtrace.HeaderTraceparent, "02-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01-extra-field",
			),
			want: xtrace.TraceInfo{
				Traceparent: "02-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01-extra-field",
				TraceID:     "0af7651916cd43dd8448eb211c80319c",
				SpanID:      "b7ad6b7169203331",
				TraceFlags:  "01",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := xtrace.ExtractFromHTTPHeader(tt.header)
			if got.TraceID != tt.want.TraceID {
				t.Errorf("TraceID = %q, want %q", got.TraceID, tt.want.TraceID)
			}
			if got.SpanID != tt.want.SpanID {
				t.Errorf("SpanID = %q, want %q", got.SpanID, tt.want.SpanID)
			}
			if got.RequestID != tt.want.RequestID {
				t.Errorf("RequestID = %q, want %q", got.RequestID, tt.want.RequestID)
			}
			if got.Traceparent != tt.want.Traceparent {
				t.Errorf("Traceparent = %q, want %q", got.Traceparent, tt.want.Traceparent)
			}
		})
	}
}

func TestExtractFromHTTPRequest(t *testing.T) {
	t.Run("nil Request", func(t *testing.T) {
		info := xtrace.ExtractFromHTTPRequest(nil)
		if !info.IsEmpty() {
			t.Errorf("expected empty TraceInfo, got %+v", info)
		}
	})

	t.Run("有 Header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set(xtrace.HeaderTraceID, "trace123")
		req.Header.Set(xtrace.HeaderRequestID, "req456")

		info := xtrace.ExtractFromHTTPRequest(req)
		if info.TraceID != "trace123" {
			t.Errorf("TraceID = %q, want %q", info.TraceID, "trace123")
		}
		if info.RequestID != "req456" {
			t.Errorf("RequestID = %q, want %q", info.RequestID, "req456")
		}
	})
}

// =============================================================================
// TraceInfo 测试
// =============================================================================

func TestTraceInfo_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		info xtrace.TraceInfo
		want bool
	}{
		{
			name: "空结构体",
			info: xtrace.TraceInfo{},
			want: true,
		},
		{
			name: "有 TraceID",
			info: xtrace.TraceInfo{TraceID: "t1"},
			want: false,
		},
		{
			name: "有 SpanID",
			info: xtrace.TraceInfo{SpanID: "s1"},
			want: false,
		},
		{
			name: "有 RequestID",
			info: xtrace.TraceInfo{RequestID: "r1"},
			want: false,
		},
		{
			name: "有 Traceparent",
			info: xtrace.TraceInfo{Traceparent: "00-abc-def-01"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// HTTP 中间件测试
// =============================================================================

func TestHTTPMiddleware(t *testing.T) {
	t.Run("提取追踪信息", func(t *testing.T) {
		var capturedTraceID, capturedRequestID string

		handler := xtrace.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedTraceID = xtrace.TraceID(r.Context())
			capturedRequestID = xtrace.RequestID(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set(xtrace.HeaderTraceID, "0af7651916cd43dd8448eb211c80319c")
		req.Header.Set(xtrace.HeaderRequestID, "req-123")

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if capturedTraceID != "0af7651916cd43dd8448eb211c80319c" {
			t.Errorf("TraceID = %q, want %q", capturedTraceID, "0af7651916cd43dd8448eb211c80319c")
		}
		if capturedRequestID != "req-123" {
			t.Errorf("RequestID = %q, want %q", capturedRequestID, "req-123")
		}
	})

	t.Run("自动生成追踪信息", func(t *testing.T) {
		var capturedTraceID, capturedSpanID, capturedRequestID string

		handler := xtrace.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedTraceID = xtrace.TraceID(r.Context())
			capturedSpanID = xtrace.SpanID(r.Context())
			capturedRequestID = xtrace.RequestID(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		// 不设置任何追踪 Header

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		// 应该自动生成
		if capturedTraceID == "" {
			t.Error("TraceID should be auto-generated")
		}
		if capturedSpanID == "" {
			t.Error("SpanID should be auto-generated")
		}
		if capturedRequestID == "" {
			t.Error("RequestID should be auto-generated")
		}
	})

	t.Run("禁用自动生成", func(t *testing.T) {
		var capturedTraceID string

		handler := xtrace.HTTPMiddlewareWithOptions(
			xtrace.WithAutoGenerate(false),
		)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedTraceID = xtrace.TraceID(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		// 不设置任何追踪 Header

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		// 不应该自动生成
		if capturedTraceID != "" {
			t.Errorf("TraceID should be empty when auto-generate is disabled, got %q", capturedTraceID)
		}
	})

	t.Run("W3C traceparent 解析", func(t *testing.T) {
		var capturedTraceID, capturedSpanID string

		handler := xtrace.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedTraceID = xtrace.TraceID(r.Context())
			capturedSpanID = xtrace.SpanID(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set(xtrace.HeaderTraceparent, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if capturedTraceID != "0af7651916cd43dd8448eb211c80319c" {
			t.Errorf("TraceID = %q, want %q", capturedTraceID, "0af7651916cd43dd8448eb211c80319c")
		}
		if capturedSpanID != "b7ad6b7169203331" {
			t.Errorf("SpanID = %q, want %q", capturedSpanID, "b7ad6b7169203331")
		}
	})
}

// =============================================================================
// HTTP Header 注入测试
// =============================================================================

func TestInjectToRequest(t *testing.T) {
	t.Run("注入追踪信息", func(t *testing.T) {
		ctx := context.Background()
		ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
		ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")
		ctx, _ = xctx.WithRequestID(ctx, "req-123")

		req := httptest.NewRequest("GET", "/test", nil)
		xtrace.InjectToRequest(ctx, req)

		if got := req.Header.Get(xtrace.HeaderTraceID); got != "0af7651916cd43dd8448eb211c80319c" {
			t.Errorf("X-Trace-ID = %q, want %q", got, "0af7651916cd43dd8448eb211c80319c")
		}
		if got := req.Header.Get(xtrace.HeaderSpanID); got != "b7ad6b7169203331" {
			t.Errorf("X-Span-ID = %q, want %q", got, "b7ad6b7169203331")
		}
		if got := req.Header.Get(xtrace.HeaderRequestID); got != "req-123" {
			t.Errorf("X-Request-ID = %q, want %q", got, "req-123")
		}

		// 验证 traceparent 格式（-00 表示未采样，因为无法确定实际采样决策）
		traceparent := req.Header.Get(xtrace.HeaderTraceparent)
		expected := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00"
		if traceparent != expected {
			t.Errorf("traceparent = %q, want %q", traceparent, expected)
		}
	})

	t.Run("nil Request 不 panic", func(t *testing.T) {
		ctx := context.Background()
		ctx, _ = xctx.WithTraceID(ctx, "trace123")
		xtrace.InjectToRequest(ctx, nil) // 不应该 panic
	})

	t.Run("nil Header 自动初始化", func(t *testing.T) {
		ctx := context.Background()
		ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
		ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")

		// 构造一个 Header 为 nil 的 Request（模拟 &http.Request{} 场景）
		req := &http.Request{}
		if req.Header != nil {
			t.Fatal("test setup: Header should be nil")
		}

		// 不应该 panic，且应该能正确注入
		xtrace.InjectToRequest(ctx, req)

		// Header 应该被自动初始化
		if req.Header == nil {
			t.Error("Header should be initialized")
		}
		if got := req.Header.Get(xtrace.HeaderTraceID); got != "0af7651916cd43dd8448eb211c80319c" {
			t.Errorf("X-Trace-ID = %q, want %q", got, "0af7651916cd43dd8448eb211c80319c")
		}
	})

	t.Run("空 context 不添加 Header", func(t *testing.T) {
		ctx := context.Background()
		req := httptest.NewRequest("GET", "/test", nil)

		xtrace.InjectToRequest(ctx, req)

		if got := req.Header.Get(xtrace.HeaderTraceID); got != "" {
			t.Errorf("X-Trace-ID should be empty, got %q", got)
		}
	})
}

func TestInjectTraceToHeader(t *testing.T) {
	t.Run("nil Header 不 panic", func(t *testing.T) {
		info := xtrace.TraceInfo{TraceID: "t1"}
		xtrace.InjectTraceToHeader(nil, info) // 不应该 panic
	})

	t.Run("注入非空字段", func(t *testing.T) {
		h := http.Header{}
		// 使用有效的 W3C traceparent 格式
		validTraceparent := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
		info := xtrace.TraceInfo{
			TraceID:     "0af7651916cd43dd8448eb211c80319c",
			SpanID:      "b7ad6b7169203331",
			RequestID:   "r1",
			Traceparent: validTraceparent,
			Tracestate:  "vendor=value",
		}
		xtrace.InjectTraceToHeader(h, info)

		if got := h.Get(xtrace.HeaderTraceID); got != "0af7651916cd43dd8448eb211c80319c" {
			t.Errorf("X-Trace-ID = %q, want %q", got, "0af7651916cd43dd8448eb211c80319c")
		}
		if got := h.Get(xtrace.HeaderSpanID); got != "b7ad6b7169203331" {
			t.Errorf("X-Span-ID = %q, want %q", got, "b7ad6b7169203331")
		}
		if got := h.Get(xtrace.HeaderRequestID); got != "r1" {
			t.Errorf("X-Request-ID = %q, want %q", got, "r1")
		}
		if got := h.Get(xtrace.HeaderTraceparent); got != validTraceparent {
			t.Errorf("traceparent = %q, want %q", got, validTraceparent)
		}
		if got := h.Get(xtrace.HeaderTracestate); got != "vendor=value" {
			t.Errorf("tracestate = %q, want %q", got, "vendor=value")
		}
	})

	t.Run("无效 traceparent 被丢弃", func(t *testing.T) {
		h := http.Header{}
		// 无效的 traceparent（trace-id 和 span-id 长度不对）
		info := xtrace.TraceInfo{
			TraceID:     "0af7651916cd43dd8448eb211c80319c",
			SpanID:      "b7ad6b7169203331",
			Traceparent: "00-abc-def-01", // 无效格式
		}
		xtrace.InjectTraceToHeader(h, info)

		// 无效 traceparent 应被丢弃，但会从 TraceID/SpanID 生成有效的
		got := h.Get(xtrace.HeaderTraceparent)
		if got == "00-abc-def-01" {
			t.Error("invalid traceparent should not be forwarded")
		}
		// 应该从 TraceID 和 SpanID 生成有效的 traceparent
		want := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00"
		if got != want {
			t.Errorf("traceparent = %q, want generated %q", got, want)
		}
	})

	t.Run("空字段不注入", func(t *testing.T) {
		h := http.Header{}
		info := xtrace.TraceInfo{TraceID: "t1"} // 只有 TraceID
		xtrace.InjectTraceToHeader(h, info)

		if got := h.Get(xtrace.HeaderSpanID); got != "" {
			t.Errorf("X-Span-ID should be empty, got %q", got)
		}
	})
}

// =============================================================================
// Header 常量测试
// =============================================================================

func TestHeaderConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"HeaderTraceID", xtrace.HeaderTraceID, "X-Trace-ID"},
		{"HeaderSpanID", xtrace.HeaderSpanID, "X-Span-ID"},
		{"HeaderRequestID", xtrace.HeaderRequestID, "X-Request-ID"},
		{"HeaderTraceparent", xtrace.HeaderTraceparent, "traceparent"},
		{"HeaderTracestate", xtrace.HeaderTracestate, "tracestate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}
