package xsampling

import (
	"context"
	"testing"
)

func FuzzSamplerInputs(f *testing.F) {
	f.Add(0.1, 10, "user-1")
	f.Add(1.0, 1, "")
	f.Add(0.0, 0, "trace-1")

	f.Fuzz(func(t *testing.T, rate float64, n int, key string) {
		ctx := context.Background()

		rateSampler := NewRateSampler(rate)
		_ = rateSampler.ShouldSample(ctx)

		probSampler := NewProbabilitySampler(rate)
		_ = probSampler.ShouldSample(ctx)

		countSampler := NewCountSampler(n)
		_ = countSampler.ShouldSample(ctx)

		keySampler := NewKeyBasedSampler(rate, func(context.Context) string {
			return key
		})
		_ = keySampler.ShouldSample(ctx)
	})
}
