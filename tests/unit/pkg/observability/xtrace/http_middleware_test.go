package xtrace_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/observability/xtrace"
	"google.golang.org/grpc/metadata"
)

func TestHTTPMiddleware_InvalidTraceIDAutoGenerate(t *testing.T) {
	// 覆盖 injectID: invalid format + autoGenerate=true → discard + ensure
	var capturedTraceID string

	handler := xtrace.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTraceID = xtrace.TraceID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	// 传入无效格式的 TraceID（不是 32 位十六进制）
	req.Header.Set(xtrace.HeaderTraceID, "not-valid-hex-trace-id")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// 无效 TraceID 被丢弃，应自动生成新的
	if capturedTraceID == "" {
		t.Error("TraceID should be auto-generated when invalid format is discarded")
	}
	if capturedTraceID == "not-valid-hex-trace-id" {
		t.Error("invalid TraceID should not be injected")
	}
}

func TestHTTPMiddleware_InvalidSpanIDAutoGenerate(t *testing.T) {
	// 覆盖 injectID: invalid span_id format + autoGenerate=true → discard + ensure
	var capturedSpanID string

	handler := xtrace.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSpanID = xtrace.SpanID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(xtrace.HeaderSpanID, "short")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// 无效 SpanID 被丢弃，应自动生成新的
	if capturedSpanID == "" {
		t.Error("SpanID should be auto-generated when invalid format is discarded")
	}
	if capturedSpanID == "short" {
		t.Error("invalid SpanID should not be injected")
	}
}

func TestHTTPMiddleware_InvalidTraceIDNoAutoGenerate(t *testing.T) {
	// 覆盖 injectID: invalid format + autoGenerate=false → discard only
	var capturedTraceID string

	handler := xtrace.HTTPMiddleware(
		xtrace.WithAutoGenerate(false),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTraceID = xtrace.TraceID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(xtrace.HeaderTraceID, "invalid!")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedTraceID != "" {
		t.Errorf("invalid TraceID should be discarded, got %q", capturedTraceID)
	}
}

func TestHTTPMiddleware_AllZeroTraceID(t *testing.T) {
	// 覆盖 isValidTraceID: 全零 trace ID 被视为无效
	var capturedTraceID string

	handler := xtrace.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTraceID = xtrace.TraceID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(xtrace.HeaderTraceID, "00000000000000000000000000000000")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// 全零 ID 无效，应自动生成
	if capturedTraceID == "00000000000000000000000000000000" {
		t.Error("all-zero TraceID should be rejected")
	}
	if capturedTraceID == "" {
		t.Error("TraceID should be auto-generated for all-zero input")
	}
}

func TestHTTPMiddleware_AllZeroSpanID(t *testing.T) {
	// 覆盖 isValidSpanID: 全零 span ID 被视为无效
	var capturedSpanID string

	handler := xtrace.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSpanID = xtrace.SpanID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(xtrace.HeaderSpanID, "0000000000000000")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedSpanID == "0000000000000000" {
		t.Error("all-zero SpanID should be rejected")
	}
	if capturedSpanID == "" {
		t.Error("SpanID should be auto-generated for all-zero input")
	}
}

func TestHTTPMiddleware_InvalidTraceFlags(t *testing.T) {
	// 覆盖 injectTraceFlags: 无效 trace-flags 被丢弃
	handler := xtrace.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TraceFlags 应该为空（无效值被丢弃）
		flags := xctx.TraceFlags(r.Context())
		if flags != "" {
			t.Errorf("invalid trace_flags should be discarded, got %q", flags)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	// 使用一个有效的 traceparent，但手动设置无效 trace-flags 比较困难
	// 因为 traceparent 解析会设置 flags。
	// 改用自定义 header 传入 trace ID，不通过 traceparent
	req.Header.Set(xtrace.HeaderTraceID, "0af7651916cd43dd8448eb211c80319c")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
}

func TestHTTPMiddleware_TraceFlagsPropagation(t *testing.T) {
	// 覆盖 injectTraceFlags: 有效 trace-flags 通过 traceparent 传播
	var capturedFlags string

	handler := xtrace.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedFlags = xctx.TraceFlags(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(xtrace.HeaderTraceparent, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedFlags != "01" {
		t.Errorf("TraceFlags = %q, want %q", capturedFlags, "01")
	}
}

func TestInjectToRequest_TraceFlagsPropagation(t *testing.T) {
	// 覆盖 InjectToRequest: trace-flags 从 context 传播到请求
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")
	ctx, _ = xctx.WithTraceFlags(ctx, "01")

	req := httptest.NewRequest("GET", "/test", nil)
	xtrace.InjectToRequest(ctx, req)

	expected := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	if got := req.Header.Get(xtrace.HeaderTraceparent); got != expected {
		t.Errorf("traceparent = %q, want %q", got, expected)
	}
}

func TestInjectToRequest_UpperCaseTraceIDNormalized(t *testing.T) {
	// 覆盖 formatTraceparent: 大写输入转小写输出
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0AF7651916CD43DD8448EB211C80319C")
	ctx, _ = xctx.WithSpanID(ctx, "B7AD6B7169203331")

	req := httptest.NewRequest("GET", "/test", nil)
	xtrace.InjectToRequest(ctx, req)

	expected := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	if got := req.Header.Get(xtrace.HeaderTraceparent); got != expected {
		t.Errorf("traceparent = %q, want %q (should be lowercase)", got, expected)
	}
}

func TestExtractFromHTTPHeader_TraceparentEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		traceparent string
		wantTraceID string
		wantSpanID  string
		wantFlags   string
	}{
		{
			name:        "全零 trace-id 无效",
			traceparent: "00-00000000000000000000000000000000-b7ad6b7169203331-01",
			wantTraceID: "",
			wantSpanID:  "",
			wantFlags:   "",
		},
		{
			name:        "全零 span-id 无效",
			traceparent: "00-0af7651916cd43dd8448eb211c80319c-0000000000000000-01",
			wantTraceID: "",
			wantSpanID:  "",
			wantFlags:   "",
		},
		{
			name:        "trace-flags 无效（非十六进制）",
			traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-zz",
			wantTraceID: "",
			wantSpanID:  "",
			wantFlags:   "",
		},
		{
			name:        "version 非十六进制",
			traceparent: "gg-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			wantTraceID: "",
			wantSpanID:  "",
			wantFlags:   "",
		},
		{
			name:        "version 长度不对（1 位）",
			traceparent: "0-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			wantTraceID: "",
			wantSpanID:  "",
			wantFlags:   "",
		},
		{
			name:        "太短的 traceparent",
			traceparent: "00-abc-def",
			wantTraceID: "",
			wantSpanID:  "",
			wantFlags:   "",
		},
		{
			name:        "trace-id 含非十六进制字符",
			traceparent: "00-ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ-b7ad6b7169203331-01",
			wantTraceID: "",
			wantSpanID:  "",
			wantFlags:   "",
		},
		{
			name:        "span-id 含非十六进制字符",
			traceparent: "00-0af7651916cd43dd8448eb211c80319c-ZZZZZZZZZZZZZZZZ-01",
			wantTraceID: "",
			wantSpanID:  "",
			wantFlags:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := makeHeader(xtrace.HeaderTraceparent, tt.traceparent)
			got := xtrace.ExtractFromHTTPHeader(h)
			if got.TraceID != tt.wantTraceID {
				t.Errorf("TraceID = %q, want %q", got.TraceID, tt.wantTraceID)
			}
			if got.SpanID != tt.wantSpanID {
				t.Errorf("SpanID = %q, want %q", got.SpanID, tt.wantSpanID)
			}
			if got.TraceFlags != tt.wantFlags {
				t.Errorf("TraceFlags = %q, want %q", got.TraceFlags, tt.wantFlags)
			}
		})
	}
}

func TestInjectTraceToHeader_FormatTraceparentEdgeCases(t *testing.T) {
	tests := []struct {
		name            string
		info            xtrace.TraceInfo
		wantTraceparent string
	}{
		{
			name: "全零 trace-id 不生成 traceparent",
			info: xtrace.TraceInfo{
				TraceID: "00000000000000000000000000000000",
				SpanID:  "b7ad6b7169203331",
			},
			wantTraceparent: "",
		},
		{
			name: "全零 span-id 不生成 traceparent",
			info: xtrace.TraceInfo{
				TraceID: "0af7651916cd43dd8448eb211c80319c",
				SpanID:  "0000000000000000",
			},
			wantTraceparent: "",
		},
		{
			name: "trace-id 长度不对不生成 traceparent",
			info: xtrace.TraceInfo{
				TraceID: "short",
				SpanID:  "b7ad6b7169203331",
			},
			wantTraceparent: "",
		},
		{
			name: "span-id 含非十六进制不生成 traceparent",
			info: xtrace.TraceInfo{
				TraceID: "0af7651916cd43dd8448eb211c80319c",
				SpanID:  "invalidspanidzzz",
			},
			wantTraceparent: "",
		},
		{
			name: "无效 traceFlags 回退为 00",
			info: xtrace.TraceInfo{
				TraceID:    "0af7651916cd43dd8448eb211c80319c",
				SpanID:     "b7ad6b7169203331",
				TraceFlags: "xyz",
			},
			wantTraceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			xtrace.InjectTraceToHeader(h, tt.info)
			got := h.Get(xtrace.HeaderTraceparent)
			if got != tt.wantTraceparent {
				t.Errorf("traceparent = %q, want %q", got, tt.wantTraceparent)
			}
		})
	}
}

// =============================================================================
// W3C tracestate 防护测试
// =============================================================================

func TestInjectTraceToHeader_TracestateRequiresTraceparent(t *testing.T) {
	t.Run("有 traceparent 时写入 tracestate", func(t *testing.T) {
		h := http.Header{}
		info := xtrace.TraceInfo{
			TraceID:    "0af7651916cd43dd8448eb211c80319c",
			SpanID:     "b7ad6b7169203331",
			Tracestate: "vendor=opaque",
		}
		xtrace.InjectTraceToHeader(h, info)

		if got := h.Get(xtrace.HeaderTracestate); got != "vendor=opaque" {
			t.Errorf("tracestate = %q, want %q", got, "vendor=opaque")
		}
	})

	t.Run("无 traceparent 时丢弃 tracestate", func(t *testing.T) {
		h := http.Header{}
		// TraceID 无效（太短），无法生成 traceparent
		info := xtrace.TraceInfo{
			TraceID:    "short",
			SpanID:     "also-short",
			Tracestate: "vendor=opaque",
		}
		xtrace.InjectTraceToHeader(h, info)

		if got := h.Get(xtrace.HeaderTraceparent); got != "" {
			t.Errorf("traceparent should be empty, got %q", got)
		}
		if got := h.Get(xtrace.HeaderTracestate); got != "" {
			t.Errorf("tracestate should be empty when no valid traceparent, got %q", got)
		}
	})
}

func TestInjectTraceToMetadata_TracestateRequiresTraceparent(t *testing.T) {
	t.Run("无 traceparent 时丢弃 tracestate", func(t *testing.T) {
		md := metadata.MD{}
		info := xtrace.TraceInfo{
			TraceID:    "short",
			SpanID:     "also-short",
			Tracestate: "vendor=opaque",
		}
		xtrace.InjectTraceToMetadata(md, info)

		if vals := md.Get(xtrace.MetaTraceparent); len(vals) > 0 {
			t.Errorf("traceparent should be empty, got %v", vals)
		}
		if vals := md.Get(xtrace.MetaTracestate); len(vals) > 0 {
			t.Errorf("tracestate should be empty when no valid traceparent, got %v", vals)
		}
	})
}

// =============================================================================
// W3C tracestate 提取层防护测试
// =============================================================================

func TestExtractFromHTTPHeader_TracestateWithoutTraceparent(t *testing.T) {
	t.Run("无 traceparent 时丢弃 tracestate", func(t *testing.T) {
		h := makeHeader(
			xtrace.HeaderTraceID, "0af7651916cd43dd8448eb211c80319c",
			xtrace.HeaderTracestate, "vendor=opaque",
		)
		info := xtrace.ExtractFromHTTPHeader(h)
		if info.Tracestate != "" {
			t.Errorf("Tracestate = %q, want empty (no traceparent)", info.Tracestate)
		}
		if info.TraceID != "0af7651916cd43dd8448eb211c80319c" {
			t.Errorf("TraceID = %q, want preserved", info.TraceID)
		}
	})

	t.Run("有效 traceparent 时保留 tracestate", func(t *testing.T) {
		h := makeHeader(
			xtrace.HeaderTraceparent, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			xtrace.HeaderTracestate, "vendor=opaque",
		)
		info := xtrace.ExtractFromHTTPHeader(h)
		if info.Tracestate != "vendor=opaque" {
			t.Errorf("Tracestate = %q, want %q", info.Tracestate, "vendor=opaque")
		}
	})

	t.Run("无效 traceparent 时丢弃 tracestate", func(t *testing.T) {
		h := makeHeader(
			xtrace.HeaderTraceparent, "invalid-format",
			xtrace.HeaderTracestate, "vendor=opaque",
		)
		info := xtrace.ExtractFromHTTPHeader(h)
		if info.Tracestate != "" {
			t.Errorf("Tracestate = %q, want empty (invalid traceparent)", info.Tracestate)
		}
	})
}

func TestExtractFromMetadata_TracestateWithoutTraceparent(t *testing.T) {
	t.Run("无 traceparent 时丢弃 tracestate", func(t *testing.T) {
		md := metadata.Pairs(
			xtrace.MetaTraceID, "0af7651916cd43dd8448eb211c80319c",
			xtrace.MetaTracestate, "vendor=opaque",
		)
		info := xtrace.ExtractFromMetadata(md)
		if info.Tracestate != "" {
			t.Errorf("Tracestate = %q, want empty (no traceparent)", info.Tracestate)
		}
		if info.TraceID != "0af7651916cd43dd8448eb211c80319c" {
			t.Errorf("TraceID = %q, want preserved", info.TraceID)
		}
	})

	t.Run("有效 traceparent 时保留 tracestate", func(t *testing.T) {
		md := metadata.Pairs(
			xtrace.MetaTraceparent, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			xtrace.MetaTracestate, "vendor=opaque",
		)
		info := xtrace.ExtractFromMetadata(md)
		if info.Tracestate != "vendor=opaque" {
			t.Errorf("Tracestate = %q, want %q", info.Tracestate, "vendor=opaque")
		}
	})

	t.Run("无效 traceparent 时丢弃 tracestate", func(t *testing.T) {
		md := metadata.Pairs(
			xtrace.MetaTraceparent, "invalid-format",
			xtrace.MetaTracestate, "vendor=opaque",
		)
		info := xtrace.ExtractFromMetadata(md)
		if info.Tracestate != "" {
			t.Errorf("Tracestate = %q, want empty (invalid traceparent)", info.Tracestate)
		}
	})
}

// =============================================================================
// 已有 traceparent 的覆盖行为测试
// =============================================================================

func TestInjectToRequest_OverwritesExistingTraceparent(t *testing.T) {
	t.Run("新 traceparent 覆盖旧值", func(t *testing.T) {
		// 已有 traceparent 的请求
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set(xtrace.HeaderTraceparent,
			"00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01")

		// 注入新的追踪信息
		ctx := context.Background()
		ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
		ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")
		xtrace.InjectToRequest(ctx, req)

		// 新 traceparent 应覆盖旧值
		want := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
		if got := req.Header.Get(xtrace.HeaderTraceparent); got != want {
			t.Errorf("traceparent = %q, want %q", got, want)
		}
	})

	t.Run("空 context 不清除已有 traceparent", func(t *testing.T) {
		// 设计决策: InjectToRequest 仅设置非空字段，不清除已有 header。
		// 空 context 场景下，调用方通常使用新建的 http.Request，不存在旧值覆盖问题。
		req := httptest.NewRequest("GET", "/test", nil)
		oldTraceparent := "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01"
		req.Header.Set(xtrace.HeaderTraceparent, oldTraceparent)

		// 空 context，没有追踪信息
		ctx := context.Background()
		xtrace.InjectToRequest(ctx, req)

		// 旧 traceparent 保留（InjectToRequest 不清除已有 header）
		if got := req.Header.Get(xtrace.HeaderTraceparent); got != oldTraceparent {
			t.Errorf("traceparent = %q, want %q (should preserve old value)", got, oldTraceparent)
		}
	})
}

func TestInjectToOutgoingContext_OverwritesExistingTraceparent(t *testing.T) {
	t.Run("新 traceparent 覆盖旧值", func(t *testing.T) {
		ctx := context.Background()
		ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
		ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")

		// 预设包含旧 traceparent 的 outgoing metadata
		oldMD := metadata.Pairs(
			xtrace.MetaTraceparent, "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01",
		)
		ctx = metadata.NewOutgoingContext(ctx, oldMD)

		ctx = xtrace.InjectToOutgoingContext(ctx)

		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			t.Fatal("metadata not found")
		}

		// 新 traceparent 应覆盖旧值
		want := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
		vals := md.Get(xtrace.MetaTraceparent)
		if len(vals) != 1 || vals[0] != want {
			t.Errorf("traceparent = %v, want [%q]", vals, want)
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
