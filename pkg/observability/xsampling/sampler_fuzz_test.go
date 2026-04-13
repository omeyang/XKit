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
			result := rateSampler.ShouldSample(ctx)

			// 不变量: rate=0 永远不采样
			if rate == 0 && result {
				t.Error("RateSampler with rate=0 should never sample")
			}
			// 不变量: rate=1 永远采样
			if rate == 1 && !result {
				t.Error("RateSampler with rate=1 should always sample")
			}
		}

		// CountSampler: 测试有效和无效 n 值
		countSampler, err := NewCountSampler(n)
		if err == nil {
			result := countSampler.ShouldSample(ctx)

			// 不变量: n=1 永远采样（每 1 个采样 1 个）
			if n == 1 && !result {
				t.Error("CountSampler with n=1 should always sample")
			}
		}

		// KeyBasedSampler: 测试有效和无效 rate 值
		keySampler, err := NewKeyBasedSampler(rate, func(context.Context) string {
			return key
		})
		if err == nil {
			result := keySampler.ShouldSample(ctx)

			// 不变量: rate=0 永远不采样
			if rate == 0 && result {
				t.Error("KeyBasedSampler with rate=0 should never sample")
			}
			// 不变量: rate=1 永远采样
			if rate == 1 && !result {
				t.Error("KeyBasedSampler with rate=1 should always sample")
			}

			// 不变量: 非空 key 的一致性——同一 key 多次调用结果应相同
			if key != "" && rate > 0 && rate < 1 {
				for range 5 {
					if keySampler.ShouldSample(ctx) != result {
						t.Errorf("KeyBasedSampler should be consistent for key=%q", key)
					}
				}
			}
		}
	})
}
