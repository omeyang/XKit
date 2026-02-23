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

		// RateSampler: 测试有效和无效 rate 值
		rateSampler, err := NewRateSampler(rate)
		if err == nil {
			_ = rateSampler.ShouldSample(ctx)
		}

		countSampler, err := NewCountSampler(n)
		if err == nil {
			_ = countSampler.ShouldSample(ctx)
		}

		// KeyBasedSampler: 测试有效和无效 rate 值
		keySampler, err := NewKeyBasedSampler(rate, func(context.Context) string {
			return key
		})
		if err == nil {
			_ = keySampler.ShouldSample(ctx)
		}
	})
}
