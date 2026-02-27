package xctx_test

import (
	"context"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

func BenchmarkTraceID(b *testing.B) {
	ctx, _ := xctx.WithTraceID(context.Background(), "trace-123")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = xctx.TraceID(ctx)
	}
}

func BenchmarkGetTrace(b *testing.B) {
	ctx, _ := xctx.WithTraceID(context.Background(), "t1")
	ctx, _ = xctx.WithSpanID(ctx, "s1")
	ctx, _ = xctx.WithRequestID(ctx, "r1")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = xctx.GetTrace(ctx)
	}
}

func BenchmarkGenerateTraceID(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = xctx.GenerateTraceID()
	}
}

func BenchmarkGenerateSpanID(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = xctx.GenerateSpanID()
	}
}

func BenchmarkEnsureTrace(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = xctx.EnsureTrace(ctx)
	}
}

func BenchmarkEnsureTrace_AlreadySet(b *testing.B) {
	ctx, _ := xctx.WithTraceID(context.Background(), "t1")
	ctx, _ = xctx.WithSpanID(ctx, "s1")
	ctx, _ = xctx.WithRequestID(ctx, "r1")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = xctx.EnsureTrace(ctx)
	}
}

func BenchmarkEnsureTrace_PartialSet(b *testing.B) {
	// 仅 TraceID 已设置，需要生成 SpanID 和 RequestID
	ctx, _ := xctx.WithTraceID(context.Background(), "t1")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = xctx.EnsureTrace(ctx)
	}
}
