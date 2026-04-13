package xsampling

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testContextKey 测试用的 context key 类型
type testContextKey string

const testKeyName testContextKey = "key"

// assertAlwaysSamples 验证采样器在多次调用中始终返回 true
func assertAlwaysSamples(t *testing.T, sampler Sampler, ctx context.Context, msg string) {
	t.Helper()
	for range 100 {
		assert.True(t, sampler.ShouldSample(ctx), msg)
	}
}

// assertNeverSamples 验证采样器在多次调用中始终返回 false
func assertNeverSamples(t *testing.T, sampler Sampler, ctx context.Context, msg string) {
	t.Helper()
	for range 100 {
		assert.False(t, sampler.ShouldSample(ctx), msg)
	}
}

// countSamples 统计采样器在指定次数调用中返回 true 的次数
func countSamples(sampler Sampler, ctx context.Context, total int) int {
	sampled := 0
	for range total {
		if sampler.ShouldSample(ctx) {
			sampled++
		}
	}
	return sampled
}

// assertSamplingRateApprox 验证采样率在预期范围内
func assertSamplingRateApprox(t *testing.T, sampler Sampler, ctx context.Context, expectedRate, tolerance float64) {
	t.Helper()
	total := 10000
	sampled := countSamples(sampler, ctx, total)
	rate := float64(sampled) / float64(total)
	assert.InDelta(t, expectedRate, rate, tolerance,
		"采样率应接近 %f，实际为 %f", expectedRate, rate)
}

// assertConsistentSampling 验证基于 key 的采样器对同一 key 产生一致结果
func assertConsistentSampling(t *testing.T, sampler Sampler, key string) {
	t.Helper()
	ctx := context.WithValue(context.Background(), testKeyName, key)
	first := sampler.ShouldSample(ctx)
	for range 10 {
		assert.Equal(t, first, sampler.ShouldSample(ctx),
			"Key %s should produce consistent results", key)
	}
}

// testKeyFunc 测试用的 context key 提取函数
func testKeyFunc(ctx context.Context) string {
	if v, ok := ctx.Value(testKeyName).(string); ok {
		return v
	}
	return ""
}

// runConcurrentSampling 并发运行采样并返回采样成功次数
func runConcurrentSampling(sampler Sampler, ctx context.Context, goroutines, iterations int) int64 {
	var wg sync.WaitGroup
	var sampled atomic.Int64

	for range goroutines {
		wg.Go(func() {
			for range iterations {
				if sampler.ShouldSample(ctx) {
					sampled.Add(1)
				}
			}
		})
	}

	wg.Wait()
	return sampled.Load()
}

// runConcurrentSamplingOnly 并发运行采样，不统计结果（仅验证并发安全）
func runConcurrentSamplingOnly(sampler Sampler, ctx context.Context, goroutines, iterations int) {
	var wg sync.WaitGroup

	for range goroutines {
		wg.Go(func() {
			for range iterations {
				sampler.ShouldSample(ctx)
			}
		})
	}

	wg.Wait()
}

func TestAlwaysSampler(t *testing.T) {
	sampler := Always()
	ctx := context.Background()

	// 测试多次调用始终返回 true
	for range 100 {
		if !sampler.ShouldSample(ctx) {
			t.Error("AlwaysSampler should always return true")
		}
	}

	// 测试单例
	sampler2 := Always()
	if sampler != sampler2 {
		t.Error("Always() should return the same instance")
	}
}

func TestNeverSampler(t *testing.T) {
	sampler := Never()
	ctx := context.Background()

	// 测试多次调用始终返回 false
	for range 100 {
		if sampler.ShouldSample(ctx) {
			t.Error("NeverSampler should always return false")
		}
	}

	// 测试单例
	sampler2 := Never()
	if sampler != sampler2 {
		t.Error("Never() should return the same instance")
	}
}

func TestRateSampler(t *testing.T) {
	ctx := context.Background()

	t.Run("rate=0", func(t *testing.T) {
		s, err := NewRateSampler(0.0)
		require.NoError(t, err)
		assertNeverSamples(t, s, ctx, "RateSampler with rate=0 should never sample")
	})

	t.Run("rate=1", func(t *testing.T) {
		s, err := NewRateSampler(1.0)
		require.NoError(t, err)
		assertAlwaysSamples(t, s, ctx, "RateSampler with rate=1 should always sample")
	})

	t.Run("rate negative", func(t *testing.T) {
		_, err := NewRateSampler(-0.5)
		assert.ErrorIs(t, err, ErrInvalidRate)
	})

	t.Run("rate > 1", func(t *testing.T) {
		_, err := NewRateSampler(1.5)
		assert.ErrorIs(t, err, ErrInvalidRate)
	})

	t.Run("rate NaN", func(t *testing.T) {
		_, err := NewRateSampler(math.NaN())
		assert.ErrorIs(t, err, ErrInvalidRate)
	})

	t.Run("rate=0.5 statistical", func(t *testing.T) {
		s, err := NewRateSampler(0.5)
		require.NoError(t, err)
		assertSamplingRateApprox(t, s, ctx, 0.5, 0.1)
	})

	t.Run("low rate statistical", func(t *testing.T) {
		// 容差基于二项分布标准差: σ = sqrt(n*p*(1-p))/n ≈ sqrt(p/n)
		// 使用 ~8σ 容差平衡检出能力与 CI 稳定性（flake 概率 < 1e-15）
		tests := []struct {
			rate      float64
			total     int
			tolerance float64
		}{
			{0.01, 100000, 0.003},    // σ≈0.000315, 8σ≈0.0025
			{0.001, 1000000, 0.0003}, // σ≈0.0000316, 8σ≈0.00025
		}
		for _, tt := range tests {
			s, err := NewRateSampler(tt.rate)
			require.NoError(t, err)
			sampled := countSamples(s, ctx, tt.total)
			actualRate := float64(sampled) / float64(tt.total)
			assert.InDelta(t, tt.rate, actualRate, tt.tolerance,
				"rate=%.4f: 采样率应接近 %f，实际为 %f", tt.rate, tt.rate, actualRate)
			assert.Greater(t, sampled, 0,
				"rate=%.4f: 至少应采样到 1 个事件", tt.rate)
		}
	})

	t.Run("rate accessor", func(t *testing.T) {
		s, err := NewRateSampler(0.3)
		require.NoError(t, err)
		assert.Equal(t, 0.3, s.Rate())
	})
}

func TestCountSampler(t *testing.T) {
	ctx := context.Background()

	t.Run("n=1", func(t *testing.T) {
		s, err := NewCountSampler(1)
		require.NoError(t, err)
		assertAlwaysSamples(t, s, ctx, "CountSampler with n=1 should always sample")
	})

	t.Run("n=10", func(t *testing.T) {
		s, err := NewCountSampler(10)
		require.NoError(t, err)
		sampled := countSamples(s, ctx, 100)
		assert.Equal(t, 10, sampled, "CountSampler with n=10 should sample 10 times in 100 calls")
	})

	t.Run("n < 1", func(t *testing.T) {
		_, err := NewCountSampler(0)
		assert.ErrorIs(t, err, ErrInvalidCount)
		_, err = NewCountSampler(-5)
		assert.ErrorIs(t, err, ErrInvalidCount)
	})

	t.Run("reset", func(t *testing.T) {
		sampler, err := NewCountSampler(5)
		require.NoError(t, err)

		// 消耗一些计数
		for range 7 {
			sampler.ShouldSample(ctx)
		}

		// 重置
		sampler.Reset()

		// 重置后第一次调用应该返回 true
		assert.True(t, sampler.ShouldSample(ctx), "After reset, first call should return true")
	})

	t.Run("sampling pattern", func(t *testing.T) {
		sampler, err := NewCountSampler(3)
		require.NoError(t, err)

		// 第 1、4、7、10... 个应该被采样
		expected := []bool{true, false, false, true, false, false, true, false, false, true}
		for i, exp := range expected {
			assert.Equal(t, exp, sampler.ShouldSample(ctx), "Call %d", i+1)
		}
	})

	t.Run("n accessor", func(t *testing.T) {
		s, err := NewCountSampler(42)
		require.NoError(t, err)
		assert.Equal(t, 42, s.N())
	})

	t.Run("zero value safety", func(t *testing.T) {
		// 零值 CountSampler（未经构造函数创建）不应 panic
		var zeroSampler CountSampler
		assert.True(t, zeroSampler.ShouldSample(ctx), "Zero-value CountSampler should always sample")
	})
}

func TestCompositeSampler_AND(t *testing.T) {
	ctx := context.Background()

	t.Run("all true", func(t *testing.T) {
		sampler, err := NewCompositeSampler(ModeAND, Always(), Always())
		require.NoError(t, err)
		if !sampler.ShouldSample(ctx) {
			t.Error("AND with all Always should return true")
		}
	})

	t.Run("one false", func(t *testing.T) {
		sampler, err := NewCompositeSampler(ModeAND, Always(), Never())
		require.NoError(t, err)
		if sampler.ShouldSample(ctx) {
			t.Error("AND with one Never should return false")
		}
	})

	t.Run("all false", func(t *testing.T) {
		sampler, err := NewCompositeSampler(ModeAND, Never(), Never())
		require.NoError(t, err)
		if sampler.ShouldSample(ctx) {
			t.Error("AND with all Never should return false")
		}
	})

	t.Run("empty", func(t *testing.T) {
		sampler, err := NewCompositeSampler(ModeAND)
		require.NoError(t, err)
		if !sampler.ShouldSample(ctx) {
			t.Error("AND with empty list should return true (identity element)")
		}
	})
}

func TestCompositeSampler_OR(t *testing.T) {
	ctx := context.Background()

	t.Run("all true", func(t *testing.T) {
		sampler, err := NewCompositeSampler(ModeOR, Always(), Always())
		require.NoError(t, err)
		if !sampler.ShouldSample(ctx) {
			t.Error("OR with all Always should return true")
		}
	})

	t.Run("one true", func(t *testing.T) {
		sampler, err := NewCompositeSampler(ModeOR, Never(), Always())
		require.NoError(t, err)
		if !sampler.ShouldSample(ctx) {
			t.Error("OR with one Always should return true")
		}
	})

	t.Run("all false", func(t *testing.T) {
		sampler, err := NewCompositeSampler(ModeOR, Never(), Never())
		require.NoError(t, err)
		if sampler.ShouldSample(ctx) {
			t.Error("OR with all Never should return false")
		}
	})

	t.Run("empty", func(t *testing.T) {
		sampler, err := NewCompositeSampler(ModeOR)
		require.NoError(t, err)
		if sampler.ShouldSample(ctx) {
			t.Error("OR with empty list should return false (identity element)")
		}
	})
}

func TestCompositeSampler_Reset(t *testing.T) {
	ctx := context.Background()
	counter, err := NewCountSampler(5)
	require.NoError(t, err)
	sampler, err := NewCompositeSampler(ModeAND, counter, Always())
	require.NoError(t, err)

	// 消耗一些计数
	for range 7 {
		sampler.ShouldSample(ctx)
	}

	// 重置
	sampler.Reset()

	// 重置后第一次调用应该返回 true
	if !sampler.ShouldSample(ctx) {
		t.Error("After reset, CountSampler inside CompositeSampler should be reset")
	}
}

func TestCompositeSampler_All_Any(t *testing.T) {
	ctx := context.Background()

	t.Run("All", func(t *testing.T) {
		sampler, err := All(Always(), Always())
		require.NoError(t, err)
		if sampler.Mode() != ModeAND {
			t.Error("All should create ModeAND sampler")
		}
		if !sampler.ShouldSample(ctx) {
			t.Error("All with Always samplers should return true")
		}
	})

	t.Run("Any", func(t *testing.T) {
		sampler, err := Any(Never(), Always())
		require.NoError(t, err)
		if sampler.Mode() != ModeOR {
			t.Error("Any should create ModeOR sampler")
		}
		if !sampler.ShouldSample(ctx) {
			t.Error("Any with one Always should return true")
		}
	})
}

func TestCompositeSampler_InvalidInput(t *testing.T) {
	t.Run("invalid mode", func(t *testing.T) {
		_, err := NewCompositeSampler(CompositeMode(99), Always())
		assert.ErrorIs(t, err, ErrInvalidMode)
	})

	t.Run("negative mode", func(t *testing.T) {
		_, err := NewCompositeSampler(CompositeMode(-1), Always())
		assert.ErrorIs(t, err, ErrInvalidMode)
	})

	t.Run("nil sampler", func(t *testing.T) {
		_, err := NewCompositeSampler(ModeAND, Always(), nil, Never())
		assert.ErrorIs(t, err, ErrNilSampler)
	})
}

func TestCompositeSampler_Samplers(t *testing.T) {
	s1 := Always()
	s2 := Never()
	sampler, err := NewCompositeSampler(ModeAND, s1, s2)
	require.NoError(t, err)

	samplers := sampler.Samplers()
	if len(samplers) != 2 {
		t.Errorf("Expected 2 samplers, got %d", len(samplers))
	}

	// 修改返回的切片不应影响原采样器
	samplers[0] = Never()
	original := sampler.Samplers()
	if original[0] != s1 {
		t.Error("Samplers() should return a copy")
	}
}

func TestCompositeMode_String(t *testing.T) {
	assert.Equal(t, "AND", ModeAND.String())
	assert.Equal(t, "OR", ModeOR.String())
	assert.Equal(t, "Unknown", CompositeMode(99).String())
}

func TestKeyBasedSampler(t *testing.T) {
	t.Run("rate=0", func(t *testing.T) {
		sampler, err := NewKeyBasedSampler(0.0, func(ctx context.Context) string {
			return "test-key"
		})
		require.NoError(t, err)
		assertNeverSamples(t, sampler, context.Background(),
			"KeyBasedSampler with rate=0 should never sample")
	})

	t.Run("rate=1", func(t *testing.T) {
		sampler, err := NewKeyBasedSampler(1.0, func(ctx context.Context) string {
			return "test-key"
		})
		require.NoError(t, err)
		assertAlwaysSamples(t, sampler, context.Background(),
			"KeyBasedSampler with rate=1 should always sample")
	})

	t.Run("consistency", func(t *testing.T) {
		sampler, err := NewKeyBasedSampler(0.5, testKeyFunc)
		require.NoError(t, err)
		keys := []string{"key1", "key2", "key3", "key4", "key5"}
		for _, key := range keys {
			assertConsistentSampling(t, sampler, key)
		}
	})

	t.Run("empty key fallback", func(t *testing.T) {
		sampler, err := NewKeyBasedSampler(0.5, func(ctx context.Context) string {
			return "" // 返回空 key
		})
		require.NoError(t, err)
		assertSamplingRateApprox(t, sampler, context.Background(), 0.5, 0.1)
	})

	t.Run("empty key callback", func(t *testing.T) {
		var callCount atomic.Int64
		sampler, err := NewKeyBasedSampler(0.5, func(ctx context.Context) string {
			return "" // 返回空 key
		}, WithOnEmptyKey(func() {
			callCount.Add(1)
		}))
		require.NoError(t, err)

		const total = 100
		for range total {
			sampler.ShouldSample(context.Background())
		}
		assert.Equal(t, int64(total), callCount.Load(),
			"OnEmptyKey callback should be called for every empty key")
	})

	t.Run("empty key callback nil", func(t *testing.T) {
		// nil 回调不应 panic
		sampler, err := NewKeyBasedSampler(0.5, func(ctx context.Context) string {
			return ""
		}, WithOnEmptyKey(nil))
		require.NoError(t, err)
		assert.NotPanics(t, func() {
			sampler.ShouldSample(context.Background())
		})
	})

	t.Run("non-empty key no callback", func(t *testing.T) {
		var callCount atomic.Int64
		sampler, err := NewKeyBasedSampler(0.5, testKeyFunc, WithOnEmptyKey(func() {
			callCount.Add(1)
		}))
		require.NoError(t, err)

		ctx := context.WithValue(context.Background(), testKeyName, "has-key")
		for range 100 {
			sampler.ShouldSample(ctx)
		}
		assert.Equal(t, int64(0), callCount.Load(),
			"OnEmptyKey callback should not be called when key is present")
	})

	t.Run("nil ctx fallback", func(t *testing.T) {
		var callCount atomic.Int64
		sampler, err := NewKeyBasedSampler(0.5, testKeyFunc, WithOnEmptyKey(func() {
			callCount.Add(1)
		}))
		require.NoError(t, err)

		// nil ctx 不应 panic，应回退到随机采样并触发 onEmptyKey 回调
		assert.NotPanics(t, func() {
			sampler.ShouldSample(nil) //nolint:staticcheck // 测试 nil ctx 防护
		})
		assert.Equal(t, int64(1), callCount.Load(),
			"OnEmptyKey callback should be called for nil ctx")
	})

	t.Run("nil ctx rate boundary", func(t *testing.T) {
		// rate=0 时 nil ctx 应直接返回 false（短路，不触发 keyFunc）
		s0, err := NewKeyBasedSampler(0.0, testKeyFunc)
		require.NoError(t, err)
		assert.False(t, s0.ShouldSample(nil)) //nolint:staticcheck // 测试 nil ctx 防护

		// rate=1 时 nil ctx 应直接返回 true（短路，不触发 keyFunc）
		s1, err := NewKeyBasedSampler(1.0, testKeyFunc)
		require.NoError(t, err)
		assert.True(t, s1.ShouldSample(nil)) //nolint:staticcheck // 测试 nil ctx 防护
	})

	t.Run("nil keyFunc", func(t *testing.T) {
		_, err := NewKeyBasedSampler(0.5, nil)
		assert.ErrorIs(t, err, ErrNilKeyFunc)
	})

	t.Run("nil option", func(t *testing.T) {
		_, err := NewKeyBasedSampler(0.5, testKeyFunc, nil)
		assert.ErrorIs(t, err, ErrNilOption)
	})

	t.Run("rate clamping negative", func(t *testing.T) {
		_, err := NewKeyBasedSampler(-0.5, testKeyFunc)
		assert.ErrorIs(t, err, ErrInvalidRate)
	})

	t.Run("rate clamping above 1", func(t *testing.T) {
		_, err := NewKeyBasedSampler(1.5, testKeyFunc)
		assert.ErrorIs(t, err, ErrInvalidRate)
	})

	t.Run("rate NaN", func(t *testing.T) {
		_, err := NewKeyBasedSampler(math.NaN(), testKeyFunc)
		assert.ErrorIs(t, err, ErrInvalidRate)
	})

	t.Run("distribution", func(t *testing.T) {
		// 容差基于二项分布标准差: σ = sqrt(n*p*(1-p))/n ≈ sqrt(p/n)
		// 使用 ~8σ 容差平衡检出能力与 CI 稳定性（flake 概率 < 1e-15）
		tests := []struct {
			rate      float64
			total     int
			tolerance float64
		}{
			{0.1, 10000, 0.03},       // σ≈0.003, 8σ≈0.024
			{0.01, 100000, 0.003},    // σ≈0.000315, 8σ≈0.0025
			{0.001, 1000000, 0.0003}, // σ≈0.0000316, 8σ≈0.00025
		}
		for _, tt := range tests {
			sampler, err := NewKeyBasedSampler(tt.rate, testKeyFunc)
			require.NoError(t, err)
			sampled := 0

			for i := range tt.total {
				u := uint64(i) //nolint:gosec // i is always non-negative (loop index)
				key := fmt.Sprintf("%016x%016x", u*0x9e3779b97f4a7c15, u^0xdeadbeefcafebabe)
				ctx := context.WithValue(context.Background(), testKeyName, key)
				if sampler.ShouldSample(ctx) {
					sampled++
				}
			}

			actualRate := float64(sampled) / float64(tt.total)
			assert.InDelta(t, tt.rate, actualRate, tt.tolerance,
				"rate=%.4f: distribution should be around %f, got %f", tt.rate, tt.rate, actualRate)
			assert.Greater(t, sampled, 0,
				"rate=%.4f: should sample at least 1 event", tt.rate)
		}
	})

	t.Run("rate accessor", func(t *testing.T) {
		sampler, err := NewKeyBasedSampler(0.7, testKeyFunc)
		require.NoError(t, err)
		assert.Equal(t, 0.7, sampler.Rate())
	})
}

// 并发安全测试
func TestConcurrency(t *testing.T) {
	ctx := context.Background()
	const goroutines = 100
	const iterations = 1000

	t.Run("CountSampler", func(t *testing.T) {
		cs, err := NewCountSampler(10)
		require.NoError(t, err)
		got := runConcurrentSampling(cs, ctx, goroutines, iterations)
		expected := int64(goroutines * iterations / 10)
		assert.Equal(t, expected, got)
	})

	t.Run("RateSampler", func(t *testing.T) {
		s, err := NewRateSampler(0.1)
		require.NoError(t, err)
		got := runConcurrentSampling(s, ctx, goroutines, iterations)
		total := float64(goroutines * iterations)
		rate := float64(got) / total
		assert.InDelta(t, 0.1, rate, 0.05, "Concurrent rate should be around 0.1")
	})

	t.Run("CompositeSampler", func(t *testing.T) {
		s, err := NewRateSampler(1.0)
		require.NoError(t, err)
		cs, csErr := NewCountSampler(10)
		require.NoError(t, csErr)
		sampler, allErr := All(cs, s)
		require.NoError(t, allErr)
		runConcurrentSamplingOnly(sampler, ctx, goroutines, iterations)
		// 主要验证没有 panic 或 data race
	})

	t.Run("KeyBasedSampler", func(t *testing.T) {
		sampler, err := NewKeyBasedSampler(0.5, testKeyFunc)
		require.NoError(t, err)
		var wg sync.WaitGroup

		for i := range goroutines {
			id := i
			wg.Go(func() {
				for range iterations {
					kctx := context.WithValue(context.Background(), testKeyName, string(rune('a'+id%26)))
					sampler.ShouldSample(kctx)
				}
			})
		}

		wg.Wait()
		// 主要验证没有 panic 或 data race
	})
}

// 接口实现验证
func TestInterfaceImplementation(t *testing.T) {
	// 编译时检查已在各文件中完成
	// 这里再做运行时验证，通过类型断言验证接口实现
	ctx := context.Background()

	rateSampler, err := NewRateSampler(0.5)
	require.NoError(t, err)
	keySampler, err := NewKeyBasedSampler(0.5, testKeyFunc)
	require.NoError(t, err)
	countSampler, err := NewCountSampler(10)
	require.NoError(t, err)
	compositeSampler, err := NewCompositeSampler(ModeAND)
	require.NoError(t, err)

	// 验证 Sampler 接口
	samplers := []Sampler{
		Always(),
		Never(),
		rateSampler,
		countSampler,
		compositeSampler,
		keySampler,
	}
	for _, s := range samplers {
		_ = s.ShouldSample(ctx) // 验证方法可调用
	}

	// 验证 ResettableSampler 接口
	countSampler2, err := NewCountSampler(10)
	require.NoError(t, err)
	compositeSampler2, err := NewCompositeSampler(ModeAND)
	require.NoError(t, err)
	resettables := []ResettableSampler{
		countSampler2,
		compositeSampler2,
	}
	for _, r := range resettables {
		r.Reset() // 验证方法可调用
	}
}
