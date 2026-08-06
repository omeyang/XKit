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
// TraceInfo 方法 Benchmark
// =============================================================================

func BenchmarkTraceInfo_IsEmpty_Empty(b *testing.B) {
	info := xtrace.TraceInfo{}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = info.IsEmpty()
	}
}

func BenchmarkTraceInfo_IsEmpty_NotEmpty(b *testing.B) {
	info := xtrace.TraceInfo{
		TraceID:     "0af7651916cd43dd8448eb211c80319c",
		SpanID:      "b7ad6b7169203331",
		RequestID:   "req-123",
		Traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
		Tracestate:  "congo=t61rcWkgMzE",
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = info.IsEmpty()
	}
}

// =============================================================================
// HTTP Header 提取 Benchmark
// =============================================================================

func BenchmarkExtractFromHTTPHeader_Empty(b *testing.B) {
	h := http.Header{}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xtrace.ExtractFromHTTPHeader(h)
	}
}

func BenchmarkExtractFromHTTPHeader_WithTraceID(b *testing.B) {
	h := http.Header{}
	h.Set(xtrace.HeaderTraceID, "0af7651916cd43dd8448eb211c80319c")
	h.Set(xtrace.HeaderSpanID, "b7ad6b7169203331")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xtrace.ExtractFromHTTPHeader(h)
	}
}

func BenchmarkExtractFromHTTPHeader_WithTraceparent(b *testing.B) {
	h := http.Header{}
	h.Set(xtrace.HeaderTraceparent, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xtrace.ExtractFromHTTPHeader(h)
	}
}

func BenchmarkExtractFromHTTPHeader_Full(b *testing.B) {
	h := http.Header{}
	h.Set(xtrace.HeaderTraceID, "0af7651916cd43dd8448eb211c80319c")
	h.Set(xtrace.HeaderSpanID, "b7ad6b7169203331")
	h.Set(xtrace.HeaderRequestID, "req-123")
	h.Set(xtrace.HeaderTraceparent, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	h.Set(xtrace.HeaderTracestate, "congo=t61rcWkgMzE")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xtrace.ExtractFromHTTPHeader(h)
	}
}

func BenchmarkExtractFromHTTPRequest(b *testing.B) {
	req, err := http.NewRequest("GET", "/api/test", nil)
	if err != nil {
		b.Fatal(err)
	}
	req.Header.Set(xtrace.HeaderTraceID, "0af7651916cd43dd8448eb211c80319c")
	req.Header.Set(xtrace.HeaderSpanID, "b7ad6b7169203331")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xtrace.ExtractFromHTTPRequest(req)
	}
}

// =============================================================================
// gRPC Metadata 提取 Benchmark
// =============================================================================

func BenchmarkExtractFromMetadata_Empty(b *testing.B) {
	md := metadata.MD{}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xtrace.ExtractFromMetadata(md)
	}
}

func BenchmarkExtractFromMetadata_WithTraceID(b *testing.B) {
	md := metadata.Pairs(
		xtrace.MetaTraceID, "0af7651916cd43dd8448eb211c80319c",
		xtrace.MetaSpanID, "b7ad6b7169203331",
	)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xtrace.ExtractFromMetadata(md)
	}
}

func BenchmarkExtractFromMetadata_Full(b *testing.B) {
	md := metadata.Pairs(
		xtrace.MetaTraceID, "0af7651916cd43dd8448eb211c80319c",
		xtrace.MetaSpanID, "b7ad6b7169203331",
		xtrace.MetaRequestID, "req-123",
		xtrace.MetaTraceparent, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
		xtrace.MetaTracestate, "congo=t61rcWkgMzE",
	)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xtrace.ExtractFromMetadata(md)
	}
}

func BenchmarkExtractFromIncomingContext(b *testing.B) {
	md := metadata.Pairs(
		xtrace.MetaTraceID, "0af7651916cd43dd8448eb211c80319c",
		xtrace.MetaSpanID, "b7ad6b7169203331",
	)
	ctx := metadata.NewIncomingContext(context.Background(), md)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xtrace.ExtractFromIncomingContext(ctx)
	}
}

// =============================================================================
// HTTP Header 注入 Benchmark
// =============================================================================

func BenchmarkInjectToRequest(b *testing.B) {
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")
	ctx, _ = xctx.WithRequestID(ctx, "req-123")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, err := http.NewRequest("GET", "/api/test", nil)
		if err != nil {
			b.Fatal(err)
		}
		xtrace.InjectToRequest(ctx, req)
	}
}

func BenchmarkInjectTraceToHeader(b *testing.B) {
	info := xtrace.TraceInfo{
		TraceID:     "0af7651916cd43dd8448eb211c80319c",
		SpanID:      "b7ad6b7169203331",
		RequestID:   "req-123",
		Traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
		Tracestate:  "congo=t61rcWkgMzE",
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		h := http.Header{}
		xtrace.InjectTraceToHeader(h, info)
	}
}

// =============================================================================
// gRPC Metadata 注入 Benchmark
// =============================================================================

func BenchmarkInjectToOutgoingContext(b *testing.B) {
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")
	ctx, _ = xctx.WithRequestID(ctx, "req-123")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xtrace.InjectToOutgoingContext(ctx)
	}
}

func BenchmarkInjectTraceToMetadata(b *testing.B) {
	info := xtrace.TraceInfo{
		TraceID:     "0af7651916cd43dd8448eb211c80319c",
		SpanID:      "b7ad6b7169203331",
		RequestID:   "req-123",
		Traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
		Tracestate:  "congo=t61rcWkgMzE",
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		md := metadata.MD{}
		xtrace.InjectTraceToMetadata(md, info)
	}
}

// =============================================================================
// Context 辅助函数 Benchmark
// =============================================================================

func BenchmarkTraceID(b *testing.B) {
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xtrace.TraceID(ctx)
	}
}

func BenchmarkSpanID(b *testing.B) {
	ctx := context.Background()
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xtrace.SpanID(ctx)
	}
}

func BenchmarkRequestID(b *testing.B) {
	ctx := context.Background()
	ctx, _ = xctx.WithRequestID(ctx, "req-123")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = xtrace.RequestID(ctx)
	}
}

// =============================================================================
// 并发访问 Benchmark
// =============================================================================

func BenchmarkExtractFromHTTPHeader_Parallel(b *testing.B) {
	h := http.Header{}
	h.Set(xtrace.HeaderTraceID, "0af7651916cd43dd8448eb211c80319c")
	h.Set(xtrace.HeaderSpanID, "b7ad6b7169203331")
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = xtrace.ExtractFromHTTPHeader(h)
		}
	})
}

func BenchmarkExtractFromMetadata_Parallel(b *testing.B) {
	md := metadata.Pairs(
		xtrace.MetaTraceID, "0af7651916cd43dd8448eb211c80319c",
		xtrace.MetaSpanID, "b7ad6b7169203331",
	)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = xtrace.ExtractFromMetadata(md)
		}
	})
}

func BenchmarkInjectToOutgoingContext_Parallel(b *testing.B) {
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = xtrace.InjectToOutgoingContext(ctx)
		}
	})
}
