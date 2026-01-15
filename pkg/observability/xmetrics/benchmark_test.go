package xmetrics

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ============================================================================
// Attr 创建基准测试
// ============================================================================

func BenchmarkString(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = String("key", "value")
	}
}

func BenchmarkInt(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Int("key", 42)
	}
}

func BenchmarkInt64(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Int64("key", 42)
	}
}

func BenchmarkUint64(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Uint64("key", 42)
	}
}

func BenchmarkFloat64(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Float64("key", 3.14)
	}
}

func BenchmarkBool(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Bool("key", true)
	}
}

func BenchmarkDuration(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Duration("key", 100*time.Millisecond)
	}
}

func BenchmarkAny(b *testing.B) {
	val := map[string]int{"a": 1, "b": 2}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Any("key", val)
	}
}

// ============================================================================
// NoopObserver 基准测试
// ============================================================================

func BenchmarkNoopObserver_Start(b *testing.B) {
	observer := NoopObserver{}
	ctx := context.Background()
	opts := SpanOptions{
		Component: "benchmark",
		Operation: "test",
		Kind:      KindServer,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, span := observer.Start(ctx, opts)
		span.End(Result{})
	}
}

func BenchmarkNoopObserver_StartWithAttrs(b *testing.B) {
	observer := NoopObserver{}
	ctx := context.Background()
	opts := SpanOptions{
		Component: "benchmark",
		Operation: "test",
		Kind:      KindServer,
		Attrs: []Attr{
			String("key1", "value1"),
			String("key2", "value2"),
			Int("key3", 42),
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, span := observer.Start(ctx, opts)
		span.End(Result{Status: StatusOK})
	}
}

func BenchmarkNoopSpan_End(b *testing.B) {
	span := NoopSpan{}
	result := Result{Status: StatusOK}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		span.End(result)
	}
}

func BenchmarkNoopSpan_EndWithError(b *testing.B) {
	span := NoopSpan{}
	err := errors.New("benchmark error")
	result := Result{Status: StatusError, Err: err}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		span.End(result)
	}
}

func BenchmarkNoopSpan_EndWithAttrs(b *testing.B) {
	span := NoopSpan{}
	result := Result{
		Status: StatusOK,
		Attrs: []Attr{
			String("result1", "value1"),
			Int("count", 100),
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		span.End(result)
	}
}

// ============================================================================
// Start 辅助函数基准测试
// ============================================================================

func BenchmarkStart_NilObserver(b *testing.B) {
	ctx := context.Background()
	opts := SpanOptions{
		Component: "benchmark",
		Operation: "test",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, span := Start(ctx, nil, opts)
		span.End(Result{})
	}
}

func BenchmarkStart_NoopObserver(b *testing.B) {
	observer := NoopObserver{}
	ctx := context.Background()
	opts := SpanOptions{
		Component: "benchmark",
		Operation: "test",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, span := Start(ctx, observer, opts)
		span.End(Result{})
	}
}

// ============================================================================
// 并发基准测试
// ============================================================================

func BenchmarkNoopObserver_StartParallel(b *testing.B) {
	observer := NoopObserver{}
	ctx := context.Background()
	opts := SpanOptions{
		Component: "benchmark",
		Operation: "parallel",
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, span := observer.Start(ctx, opts)
			span.End(Result{})
		}
	})
}

func BenchmarkStart_NilObserverParallel(b *testing.B) {
	ctx := context.Background()
	opts := SpanOptions{
		Component: "benchmark",
		Operation: "parallel",
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, span := Start(ctx, nil, opts)
			span.End(Result{})
		}
	})
}

// ============================================================================
// SpanOptions 和 Result 创建基准测试
// ============================================================================

func BenchmarkSpanOptions_Create(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = SpanOptions{
			Component: "test",
			Operation: "benchmark",
			Kind:      KindServer,
		}
	}
}

func BenchmarkSpanOptions_CreateWithAttrs(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = SpanOptions{
			Component: "test",
			Operation: "benchmark",
			Kind:      KindServer,
			Attrs: []Attr{
				{Key: "key1", Value: "value1"},
				{Key: "key2", Value: 42},
				{Key: "key3", Value: true},
			},
		}
	}
}

func BenchmarkResult_Create(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = Result{Status: StatusOK}
	}
}

func BenchmarkResult_CreateWithAttrs(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = Result{
			Status: StatusOK,
			Attrs: []Attr{
				{Key: "key1", Value: "value1"},
				{Key: "key2", Value: 42},
			},
		}
	}
}
