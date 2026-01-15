package xtrace_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/observability/xtrace"

	"google.golang.org/grpc/metadata"
)

// =============================================================================
// TraceInfo Fuzz 测试
// =============================================================================

func FuzzTraceInfo_IsEmpty(f *testing.F) {
	// 添加种子语料
	f.Add("", "", "", "", "")
	f.Add("trace-123", "", "", "", "")
	f.Add("", "span-456", "", "", "")
	f.Add("", "", "req-789", "", "")
	f.Add("trace-123", "span-456", "req-789", "", "")
	f.Add("0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331", "req-123",
		"00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", "congo=t61rcWkgMzE")

	f.Fuzz(func(t *testing.T, traceID, spanID, requestID, traceparent, tracestate string) {
		info := xtrace.TraceInfo{
			TraceID:     traceID,
			SpanID:      spanID,
			RequestID:   requestID,
			Traceparent: traceparent,
			Tracestate:  tracestate,
		}

		// 不应该 panic
		isEmpty := info.IsEmpty()

		// 验证逻辑一致性
		allEmpty := traceID == "" && spanID == "" && requestID == "" &&
			traceparent == "" && tracestate == ""
		if isEmpty != allEmpty {
			t.Errorf("IsEmpty() = %v, but expected %v", isEmpty, allEmpty)
		}
	})
}

// =============================================================================
// HTTP Header 提取 Fuzz 测试
// =============================================================================

func FuzzExtractFromHTTPHeader(f *testing.F) {
	f.Add("trace-123", "span-456", "req-789")
	f.Add("", "", "")
	f.Add("  spaced  ", "  spaced  ", "  spaced  ")
	f.Add("0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331", "req-123")
	f.Add("中文TraceID", "中文SpanID", "中文RequestID")

	f.Fuzz(func(t *testing.T, traceID, spanID, requestID string) {
		h := http.Header{}
		if traceID != "" {
			h.Set(xtrace.HeaderTraceID, traceID)
		}
		if spanID != "" {
			h.Set(xtrace.HeaderSpanID, spanID)
		}
		if requestID != "" {
			h.Set(xtrace.HeaderRequestID, requestID)
		}

		// 不应该 panic
		info := xtrace.ExtractFromHTTPHeader(h)
		_ = info.IsEmpty()
	})
}

func FuzzExtractFromHTTPHeader_Traceparent(f *testing.F) {
	// 标准 W3C traceparent 格式
	f.Add("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	// 无效格式
	f.Add("")
	f.Add("invalid")
	f.Add("00-short-id-01")
	f.Add("01-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01") // 版本错误
	f.Add("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331")    // 缺少部分
	f.Add("00-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx-yyyyyyyyyyyyyyyy-01")

	f.Fuzz(func(t *testing.T, traceparent string) {
		h := http.Header{}
		h.Set(xtrace.HeaderTraceparent, traceparent)

		// 不应该 panic
		info := xtrace.ExtractFromHTTPHeader(h)
		_ = info.IsEmpty()
	})
}

// =============================================================================
// gRPC Metadata 提取 Fuzz 测试
// =============================================================================

func FuzzExtractFromMetadata(f *testing.F) {
	f.Add("trace-123", "span-456", "req-789")
	f.Add("", "", "")
	f.Add("  spaced  ", "  spaced  ", "  spaced  ")
	f.Add("0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331", "req-123")

	f.Fuzz(func(t *testing.T, traceID, spanID, requestID string) {
		pairs := []string{}
		if traceID != "" {
			pairs = append(pairs, xtrace.MetaTraceID, traceID)
		}
		if spanID != "" {
			pairs = append(pairs, xtrace.MetaSpanID, spanID)
		}
		if requestID != "" {
			pairs = append(pairs, xtrace.MetaRequestID, requestID)
		}

		var md metadata.MD
		if len(pairs) > 0 {
			md = metadata.Pairs(pairs...)
		}

		// 不应该 panic
		info := xtrace.ExtractFromMetadata(md)
		_ = info.IsEmpty()
	})
}

func FuzzExtractFromMetadata_Full(f *testing.F) {
	f.Add("trace", "span", "req", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", "congo=t61rcWkgMzE")
	f.Add("", "", "", "", "")
	f.Add("t1", "s1", "r1", "invalid-traceparent", "state")

	f.Fuzz(func(t *testing.T, traceID, spanID, requestID, traceparent, tracestate string) {
		pairs := []string{}
		if traceID != "" {
			pairs = append(pairs, xtrace.MetaTraceID, traceID)
		}
		if spanID != "" {
			pairs = append(pairs, xtrace.MetaSpanID, spanID)
		}
		if requestID != "" {
			pairs = append(pairs, xtrace.MetaRequestID, requestID)
		}
		if traceparent != "" {
			pairs = append(pairs, xtrace.MetaTraceparent, traceparent)
		}
		if tracestate != "" {
			pairs = append(pairs, xtrace.MetaTracestate, tracestate)
		}

		var md metadata.MD
		if len(pairs) > 0 {
			md = metadata.Pairs(pairs...)
		}

		// 不应该 panic
		info := xtrace.ExtractFromMetadata(md)
		_ = info.IsEmpty()
	})
}

// =============================================================================
// HTTP 注入 Fuzz 测试
// =============================================================================

func FuzzInjectToRequest(f *testing.F) {
	f.Add("trace-123", "span-456", "req-789")
	f.Add("", "", "")
	f.Add("0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331", "req-123")

	f.Fuzz(func(t *testing.T, traceID, spanID, requestID string) {
		ctx := context.Background()
		if traceID != "" {
			ctx, _ = xctx.WithTraceID(ctx, traceID)
		}
		if spanID != "" {
			ctx, _ = xctx.WithSpanID(ctx, spanID)
		}
		if requestID != "" {
			ctx, _ = xctx.WithRequestID(ctx, requestID)
		}

		req, err := http.NewRequest("GET", "/api/test", nil)
		if err != nil {
			t.Fatalf("NewRequest() error = %v", err)
		}

		// 不应该 panic
		xtrace.InjectToRequest(ctx, req)
	})
}

func FuzzInjectTraceToHeader(f *testing.F) {
	f.Add("trace-123", "span-456", "req-789", "", "")
	f.Add("", "", "", "", "")
	f.Add("0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331", "req-123",
		"00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", "congo=t61rcWkgMzE")

	f.Fuzz(func(t *testing.T, traceID, spanID, requestID, traceparent, tracestate string) {
		info := xtrace.TraceInfo{
			TraceID:     traceID,
			SpanID:      spanID,
			RequestID:   requestID,
			Traceparent: traceparent,
			Tracestate:  tracestate,
		}

		h := http.Header{}

		// 不应该 panic
		xtrace.InjectTraceToHeader(h, info)
	})
}

// =============================================================================
// gRPC 注入 Fuzz 测试
// =============================================================================

func FuzzInjectToOutgoingContext(f *testing.F) {
	f.Add("trace-123", "span-456", "req-789")
	f.Add("", "", "")
	f.Add("0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331", "req-123")

	f.Fuzz(func(t *testing.T, traceID, spanID, requestID string) {
		ctx := context.Background()
		if traceID != "" {
			ctx, _ = xctx.WithTraceID(ctx, traceID)
		}
		if spanID != "" {
			ctx, _ = xctx.WithSpanID(ctx, spanID)
		}
		if requestID != "" {
			ctx, _ = xctx.WithRequestID(ctx, requestID)
		}

		// 不应该 panic
		newCtx := xtrace.InjectToOutgoingContext(ctx)
		_ = newCtx
	})
}

func FuzzInjectTraceToMetadata(f *testing.F) {
	f.Add("trace-123", "span-456", "req-789", "", "")
	f.Add("", "", "", "", "")
	f.Add("0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331", "req-123",
		"00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", "congo=t61rcWkgMzE")

	f.Fuzz(func(t *testing.T, traceID, spanID, requestID, traceparent, tracestate string) {
		info := xtrace.TraceInfo{
			TraceID:     traceID,
			SpanID:      spanID,
			RequestID:   requestID,
			Traceparent: traceparent,
			Tracestate:  tracestate,
		}

		md := metadata.MD{}

		// 不应该 panic
		xtrace.InjectTraceToMetadata(md, info)
	})
}

// =============================================================================
// Context 辅助函数 Fuzz 测试
// =============================================================================

func FuzzContextHelpers(f *testing.F) {
	f.Add("trace-123", "span-456", "req-789")
	f.Add("", "", "")
	f.Add("中文ID", "特殊字符!@#", "空格 ID")

	f.Fuzz(func(t *testing.T, traceID, spanID, requestID string) {
		ctx := context.Background()
		if traceID != "" {
			ctx, _ = xctx.WithTraceID(ctx, traceID)
		}
		if spanID != "" {
			ctx, _ = xctx.WithSpanID(ctx, spanID)
		}
		if requestID != "" {
			ctx, _ = xctx.WithRequestID(ctx, requestID)
		}

		// 不应该 panic
		gotTraceID := xtrace.TraceID(ctx)
		gotSpanID := xtrace.SpanID(ctx)
		gotRequestID := xtrace.RequestID(ctx)

		// 验证设置的值能正确读取
		if traceID != "" && gotTraceID != traceID {
			t.Errorf("TraceID = %q, want %q", gotTraceID, traceID)
		}
		if spanID != "" && gotSpanID != spanID {
			t.Errorf("SpanID = %q, want %q", gotSpanID, spanID)
		}
		if requestID != "" && gotRequestID != requestID {
			t.Errorf("RequestID = %q, want %q", gotRequestID, requestID)
		}
	})
}
