package xsampling

import (
	"context"
	"testing"
)

// benchContextKey 基准测试用的 context key 类型
type benchContextKey string

const benchKeyName benchContextKey = "key"

var benchCtx = context.Background()

func BenchmarkAlwaysSampler(b *testing.B) {
	sampler := Always()

	b.ReportAllocs()
	for b.Loop() {
		sampler.ShouldSample(benchCtx)
	}
}

func BenchmarkNeverSampler(b *testing.B) {
	sampler := Never()

	b.ReportAllocs()
	for b.Loop() {
		sampler.ShouldSample(benchCtx)
	}
}

func BenchmarkRateSampler(b *testing.B) {
	sampler, err := NewRateSampler(0.1)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		sampler.ShouldSample(benchCtx)
	}
}

func BenchmarkRateSampler_Zero(b *testing.B) {
	sampler, err := NewRateSampler(0.0)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		sampler.ShouldSample(benchCtx)
	}
}

func BenchmarkRateSampler_One(b *testing.B) {
	sampler, err := NewRateSampler(1.0)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		sampler.ShouldSample(benchCtx)
	}
}

func BenchmarkCountSampler(b *testing.B) {
	sampler, err := NewCountSampler(100)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		sampler.ShouldSample(benchCtx)
	}
}

func BenchmarkCompositeSampler_AND_2(b *testing.B) {
	s1, _ := NewRateSampler(0.5)
	s2, _ := NewRateSampler(0.5)
	sampler, err := All(s1, s2)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		sampler.ShouldSample(benchCtx)
	}
}

func BenchmarkCompositeSampler_OR_2(b *testing.B) {
	s1, _ := NewRateSampler(0.1)
	s2, _ := NewRateSampler(0.1)
	sampler, err := Any(s1, s2)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		sampler.ShouldSample(benchCtx)
	}
}

func BenchmarkCompositeSampler_ShortCircuit_AND(b *testing.B) {
	s1, _ := NewRateSampler(0.5)
	s2, _ := NewRateSampler(0.5)
	sampler, err := All(Never(), s1, s2)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		sampler.ShouldSample(benchCtx)
	}
}

func BenchmarkCompositeSampler_ShortCircuit_OR(b *testing.B) {
	s1, _ := NewRateSampler(0.5)
	s2, _ := NewRateSampler(0.5)
	sampler, err := Any(Always(), s1, s2)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		sampler.ShouldSample(benchCtx)
	}
}

func BenchmarkKeyBasedSampler(b *testing.B) {
	ctx := context.WithValue(context.Background(), benchKeyName, "test-key-123")
	sampler, err := NewKeyBasedSampler(0.1, func(ctx context.Context) string {
		if v, ok := ctx.Value(benchKeyName).(string); ok {
			return v
		}
		return ""
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		sampler.ShouldSample(ctx)
	}
}

func BenchmarkKeyBasedSampler_EmptyKey(b *testing.B) {
	sampler, err := NewKeyBasedSampler(0.1, func(ctx context.Context) string {
		return ""
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		sampler.ShouldSample(benchCtx)
	}
}

// 并发基准测试
func BenchmarkCountSampler_Parallel(b *testing.B) {
	sampler, err := NewCountSampler(100)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sampler.ShouldSample(benchCtx)
		}
	})
}

func BenchmarkRateSampler_Parallel(b *testing.B) {
	sampler, err := NewRateSampler(0.1)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sampler.ShouldSample(benchCtx)
		}
	})
}

func BenchmarkKeyBasedSampler_Parallel(b *testing.B) {
	sampler, err := NewKeyBasedSampler(0.1, func(ctx context.Context) string {
		if v, ok := ctx.Value(benchKeyName).(string); ok {
			return v
		}
		return ""
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		ctx := context.WithValue(context.Background(), benchKeyName, "parallel-test-key")
		for pb.Next() {
			sampler.ShouldSample(ctx)
		}
	})
}
