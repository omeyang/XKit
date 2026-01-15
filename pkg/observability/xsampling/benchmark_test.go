package xsampling

import (
	"context"
	"testing"
)

// benchContextKey 基准测试用的 context key 类型
type benchContextKey string

const benchKeyName benchContextKey = "key"

var (
	benchCtx    = context.Background()
	benchResult bool
)

func BenchmarkAlwaysSampler(b *testing.B) {
	sampler := Always()
	var result bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result = sampler.ShouldSample(benchCtx)
	}

	benchResult = result
}

func BenchmarkNeverSampler(b *testing.B) {
	sampler := Never()
	var result bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result = sampler.ShouldSample(benchCtx)
	}

	benchResult = result
}

func BenchmarkRateSampler(b *testing.B) {
	sampler := NewRateSampler(0.1)
	var result bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result = sampler.ShouldSample(benchCtx)
	}

	benchResult = result
}

func BenchmarkRateSampler_Zero(b *testing.B) {
	sampler := NewRateSampler(0.0)
	var result bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result = sampler.ShouldSample(benchCtx)
	}

	benchResult = result
}

func BenchmarkRateSampler_One(b *testing.B) {
	sampler := NewRateSampler(1.0)
	var result bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result = sampler.ShouldSample(benchCtx)
	}

	benchResult = result
}

func BenchmarkCountSampler(b *testing.B) {
	sampler := NewCountSampler(100)
	var result bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result = sampler.ShouldSample(benchCtx)
	}

	benchResult = result
}

func BenchmarkProbabilitySampler(b *testing.B) {
	sampler := NewProbabilitySampler(0.1)
	var result bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result = sampler.ShouldSample(benchCtx)
	}

	benchResult = result
}

func BenchmarkCompositeSampler_AND_2(b *testing.B) {
	sampler := All(
		NewRateSampler(0.5),
		NewRateSampler(0.5),
	)
	var result bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result = sampler.ShouldSample(benchCtx)
	}

	benchResult = result
}

func BenchmarkCompositeSampler_OR_2(b *testing.B) {
	sampler := Any(
		NewRateSampler(0.1),
		NewRateSampler(0.1),
	)
	var result bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result = sampler.ShouldSample(benchCtx)
	}

	benchResult = result
}

func BenchmarkCompositeSampler_ShortCircuit_AND(b *testing.B) {
	// 第一个 Never 会导致短路
	sampler := All(
		Never(),
		NewRateSampler(0.5),
		NewRateSampler(0.5),
	)
	var result bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result = sampler.ShouldSample(benchCtx)
	}

	benchResult = result
}

func BenchmarkCompositeSampler_ShortCircuit_OR(b *testing.B) {
	// 第一个 Always 会导致短路
	sampler := Any(
		Always(),
		NewRateSampler(0.5),
		NewRateSampler(0.5),
	)
	var result bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result = sampler.ShouldSample(benchCtx)
	}

	benchResult = result
}

func BenchmarkKeyBasedSampler(b *testing.B) {
	ctx := context.WithValue(context.Background(), benchKeyName, "test-key-123")
	sampler := NewKeyBasedSampler(0.1, func(ctx context.Context) string {
		if v, ok := ctx.Value(benchKeyName).(string); ok {
			return v
		}
		return ""
	})
	var result bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result = sampler.ShouldSample(ctx)
	}

	benchResult = result
}

func BenchmarkKeyBasedSampler_EmptyKey(b *testing.B) {
	sampler := NewKeyBasedSampler(0.1, func(ctx context.Context) string {
		return ""
	})
	var result bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result = sampler.ShouldSample(benchCtx)
	}

	benchResult = result
}

func BenchmarkKeyBasedSampler_NilKeyFunc(b *testing.B) {
	sampler := NewKeyBasedSampler(0.1, nil)
	var result bool

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result = sampler.ShouldSample(benchCtx)
	}

	benchResult = result
}

// 并发基准测试
func BenchmarkCountSampler_Parallel(b *testing.B) {
	sampler := NewCountSampler(100)

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sampler.ShouldSample(benchCtx)
		}
	})
}

func BenchmarkRateSampler_Parallel(b *testing.B) {
	sampler := NewRateSampler(0.1)

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sampler.ShouldSample(benchCtx)
		}
	})
}

func BenchmarkKeyBasedSampler_Parallel(b *testing.B) {
	sampler := NewKeyBasedSampler(0.1, func(ctx context.Context) string {
		if v, ok := ctx.Value(benchKeyName).(string); ok {
			return v
		}
		return ""
	})

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		ctx := context.WithValue(context.Background(), benchKeyName, "parallel-test-key")
		for pb.Next() {
			sampler.ShouldSample(ctx)
		}
	})
}
