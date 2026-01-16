package xsampling

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"testing"
)

// testContextKey 测试用的 context key 类型
type testContextKey string

const testKeyName testContextKey = "key"

func TestAlwaysSampler(t *testing.T) {
	sampler := Always()
	ctx := context.Background()

	// 测试多次调用始终返回 true
	for i := 0; i < 100; i++ {
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
	for i := 0; i < 100; i++ {
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
		sampler := NewRateSampler(0.0)
		for i := 0; i < 100; i++ {
			if sampler.ShouldSample(ctx) {
				t.Error("RateSampler with rate=0 should never sample")
			}
		}
	})

	t.Run("rate=1", func(t *testing.T) {
		sampler := NewRateSampler(1.0)
		for i := 0; i < 100; i++ {
			if !sampler.ShouldSample(ctx) {
				t.Error("RateSampler with rate=1 should always sample")
			}
		}
	})

	t.Run("rate negative", func(t *testing.T) {
		sampler := NewRateSampler(-0.5)
		if sampler.Rate() != 0.0 {
			t.Error("Negative rate should be clamped to 0")
		}
	})

	t.Run("rate > 1", func(t *testing.T) {
		sampler := NewRateSampler(1.5)
		if sampler.Rate() != 1.0 {
			t.Error("Rate > 1 should be clamped to 1")
		}
	})

	t.Run("rate=0.5 statistical", func(t *testing.T) {
		sampler := NewRateSampler(0.5)
		sampled := 0
		total := 10000

		for i := 0; i < total; i++ {
			if sampler.ShouldSample(ctx) {
				sampled++
			}
		}

		rate := float64(sampled) / float64(total)
		// 允许 10% 的误差
		if rate < 0.4 || rate > 0.6 {
			t.Errorf("Rate should be around 0.5, got %f", rate)
		}
	})
}

func TestCountSampler(t *testing.T) {
	ctx := context.Background()

	t.Run("n=1", func(t *testing.T) {
		sampler := NewCountSampler(1)
		for i := 0; i < 100; i++ {
			if !sampler.ShouldSample(ctx) {
				t.Error("CountSampler with n=1 should always sample")
			}
		}
	})

	t.Run("n=10", func(t *testing.T) {
		sampler := NewCountSampler(10)
		sampled := 0

		for i := 0; i < 100; i++ {
			if sampler.ShouldSample(ctx) {
				sampled++
			}
		}

		if sampled != 10 {
			t.Errorf("CountSampler with n=10 should sample 10 times in 100 calls, got %d", sampled)
		}
	})

	t.Run("n < 1", func(t *testing.T) {
		sampler := NewCountSampler(0)
		if sampler.N() != 1 {
			t.Error("n < 1 should be set to 1")
		}

		sampler2 := NewCountSampler(-5)
		if sampler2.N() != 1 {
			t.Error("Negative n should be set to 1")
		}
	})

	t.Run("reset", func(t *testing.T) {
		sampler := NewCountSampler(5)

		// 消耗一些计数
		for i := 0; i < 7; i++ {
			sampler.ShouldSample(ctx)
		}

		// 重置
		sampler.Reset()

		// 重置后第一次调用应该返回 true
		if !sampler.ShouldSample(ctx) {
			t.Error("After reset, first call should return true")
		}
	})

	t.Run("sampling pattern", func(t *testing.T) {
		sampler := NewCountSampler(3)

		// 第 1、4、7、10... 个应该被采样
		expected := []bool{true, false, false, true, false, false, true, false, false, true}
		for i, exp := range expected {
			got := sampler.ShouldSample(ctx)
			if got != exp {
				t.Errorf("Call %d: expected %v, got %v", i+1, exp, got)
			}
		}
	})
}

func TestProbabilitySampler(t *testing.T) {
	ctx := context.Background()

	t.Run("probability=0", func(t *testing.T) {
		sampler := NewProbabilitySampler(0.0)
		for i := 0; i < 100; i++ {
			if sampler.ShouldSample(ctx) {
				t.Error("ProbabilitySampler with probability=0 should never sample")
			}
		}
	})

	t.Run("probability=1", func(t *testing.T) {
		sampler := NewProbabilitySampler(1.0)
		for i := 0; i < 100; i++ {
			if !sampler.ShouldSample(ctx) {
				t.Error("ProbabilitySampler with probability=1 should always sample")
			}
		}
	})

	t.Run("probability negative", func(t *testing.T) {
		sampler := NewProbabilitySampler(-0.5)
		if sampler.Probability() != 0.0 {
			t.Error("Negative probability should be clamped to 0")
		}
	})

	t.Run("probability > 1", func(t *testing.T) {
		sampler := NewProbabilitySampler(1.5)
		if sampler.Probability() != 1.0 {
			t.Error("Probability > 1 should be clamped to 1")
		}
	})
}

func TestCompositeSampler_AND(t *testing.T) {
	ctx := context.Background()

	t.Run("all true", func(t *testing.T) {
		sampler := NewCompositeSampler(ModeAND, Always(), Always())
		if !sampler.ShouldSample(ctx) {
			t.Error("AND with all Always should return true")
		}
	})

	t.Run("one false", func(t *testing.T) {
		sampler := NewCompositeSampler(ModeAND, Always(), Never())
		if sampler.ShouldSample(ctx) {
			t.Error("AND with one Never should return false")
		}
	})

	t.Run("all false", func(t *testing.T) {
		sampler := NewCompositeSampler(ModeAND, Never(), Never())
		if sampler.ShouldSample(ctx) {
			t.Error("AND with all Never should return false")
		}
	})

	t.Run("empty", func(t *testing.T) {
		sampler := NewCompositeSampler(ModeAND)
		if !sampler.ShouldSample(ctx) {
			t.Error("AND with empty list should return true (identity element)")
		}
	})
}

func TestCompositeSampler_OR(t *testing.T) {
	ctx := context.Background()

	t.Run("all true", func(t *testing.T) {
		sampler := NewCompositeSampler(ModeOR, Always(), Always())
		if !sampler.ShouldSample(ctx) {
			t.Error("OR with all Always should return true")
		}
	})

	t.Run("one true", func(t *testing.T) {
		sampler := NewCompositeSampler(ModeOR, Never(), Always())
		if !sampler.ShouldSample(ctx) {
			t.Error("OR with one Always should return true")
		}
	})

	t.Run("all false", func(t *testing.T) {
		sampler := NewCompositeSampler(ModeOR, Never(), Never())
		if sampler.ShouldSample(ctx) {
			t.Error("OR with all Never should return false")
		}
	})

	t.Run("empty", func(t *testing.T) {
		sampler := NewCompositeSampler(ModeOR)
		if sampler.ShouldSample(ctx) {
			t.Error("OR with empty list should return false (identity element)")
		}
	})
}

func TestCompositeSampler_Reset(t *testing.T) {
	ctx := context.Background()
	counter := NewCountSampler(5)
	sampler := NewCompositeSampler(ModeAND, counter, Always())

	// 消耗一些计数
	for i := 0; i < 7; i++ {
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
		sampler := All(Always(), Always())
		if sampler.Mode() != ModeAND {
			t.Error("All should create ModeAND sampler")
		}
		if !sampler.ShouldSample(ctx) {
			t.Error("All with Always samplers should return true")
		}
	})

	t.Run("Any", func(t *testing.T) {
		sampler := Any(Never(), Always())
		if sampler.Mode() != ModeOR {
			t.Error("Any should create ModeOR sampler")
		}
		if !sampler.ShouldSample(ctx) {
			t.Error("Any with one Always should return true")
		}
	})
}

func TestCompositeSampler_InvalidMode(t *testing.T) {
	// 测试非法 mode 值会 panic
	t.Run("invalid mode panics", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Error("NewCompositeSampler with invalid mode should panic")
			}
			if msg, ok := r.(string); ok {
				if msg != "xsampling: invalid CompositeMode, must be ModeAND or ModeOR" {
					t.Errorf("unexpected panic message: %s", msg)
				}
			}
		}()

		// 使用非法 mode 值（不是 ModeAND 或 ModeOR）
		NewCompositeSampler(CompositeMode(99), Always())
	})

	t.Run("negative mode panics", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Error("NewCompositeSampler with negative mode should panic")
			}
		}()

		NewCompositeSampler(CompositeMode(-1), Always())
	})
}

func TestCompositeSampler_Samplers(t *testing.T) {
	s1 := Always()
	s2 := Never()
	sampler := NewCompositeSampler(ModeAND, s1, s2)

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

func TestKeyBasedSampler(t *testing.T) {
	t.Run("rate=0", func(t *testing.T) {
		sampler := NewKeyBasedSampler(0.0, func(ctx context.Context) string {
			return "test-key"
		})
		ctx := context.Background()

		for i := 0; i < 100; i++ {
			if sampler.ShouldSample(ctx) {
				t.Error("KeyBasedSampler with rate=0 should never sample")
			}
		}
	})

	t.Run("rate=1", func(t *testing.T) {
		sampler := NewKeyBasedSampler(1.0, func(ctx context.Context) string {
			return "test-key"
		})
		ctx := context.Background()

		for i := 0; i < 100; i++ {
			if !sampler.ShouldSample(ctx) {
				t.Error("KeyBasedSampler with rate=1 should always sample")
			}
		}
	})

	t.Run("consistency", func(t *testing.T) {
		sampler := NewKeyBasedSampler(0.5, func(ctx context.Context) string {
			if v, ok := ctx.Value(testKeyName).(string); ok {
				return v
			}
			return ""
		})

		// 测试相同 key 产生相同结果
		keys := []string{"key1", "key2", "key3", "key4", "key5"}
		for _, key := range keys {
			ctx := context.WithValue(context.Background(), testKeyName, key)
			first := sampler.ShouldSample(ctx)

			// 同一个 key 多次调用应该返回相同结果
			for i := 0; i < 10; i++ {
				if sampler.ShouldSample(ctx) != first {
					t.Errorf("Key %s should produce consistent results", key)
				}
			}
		}
	})

	t.Run("empty key fallback", func(t *testing.T) {
		sampler := NewKeyBasedSampler(0.5, func(ctx context.Context) string {
			return "" // 返回空 key
		})
		ctx := context.Background()

		// 空 key 应该回退到随机采样
		sampled := 0
		total := 10000

		for i := 0; i < total; i++ {
			if sampler.ShouldSample(ctx) {
				sampled++
			}
		}

		rate := float64(sampled) / float64(total)
		// 允许 10% 的误差
		if rate < 0.4 || rate > 0.6 {
			t.Errorf("Empty key fallback rate should be around 0.5, got %f", rate)
		}
	})

	t.Run("nil keyFunc", func(t *testing.T) {
		sampler := NewKeyBasedSampler(0.5, nil)
		ctx := context.Background()

		// nil keyFunc 应该回退到随机采样
		sampled := 0
		total := 10000

		for i := 0; i < total; i++ {
			if sampler.ShouldSample(ctx) {
				sampled++
			}
		}

		rate := float64(sampled) / float64(total)
		// 允许 10% 的误差
		if rate < 0.4 || rate > 0.6 {
			t.Errorf("Nil keyFunc fallback rate should be around 0.5, got %f", rate)
		}
	})

	t.Run("rate clamping", func(t *testing.T) {
		sampler1 := NewKeyBasedSampler(-0.5, nil)
		if sampler1.Rate() != 0.0 {
			t.Error("Negative rate should be clamped to 0")
		}

		sampler2 := NewKeyBasedSampler(1.5, nil)
		if sampler2.Rate() != 1.0 {
			t.Error("Rate > 1 should be clamped to 1")
		}
	})

	t.Run("distribution", func(t *testing.T) {
		sampler := NewKeyBasedSampler(0.1, func(ctx context.Context) string {
			if v, ok := ctx.Value(testKeyName).(string); ok {
				return v
			}
			return ""
		})

		sampled := 0
		total := 10000

		for i := 0; i < total; i++ {
			key := string(rune('a'+i%26)) + string(rune('0'+i/26%10)) + string(rune(i))
			ctx := context.WithValue(context.Background(), testKeyName, key)
			if sampler.ShouldSample(ctx) {
				sampled++
			}
		}

		rate := float64(sampled) / float64(total)
		// 允许 5% 的误差
		if math.Abs(rate-0.1) > 0.05 {
			t.Errorf("Distribution should be around 0.1, got %f", rate)
		}
	})
}

// 并发安全测试
func TestConcurrency(t *testing.T) {
	ctx := context.Background()
	const goroutines = 100
	const iterations = 1000

	t.Run("CountSampler", func(t *testing.T) {
		sampler := NewCountSampler(10)
		var wg sync.WaitGroup
		var sampled atomic.Int64

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					if sampler.ShouldSample(ctx) {
						sampled.Add(1)
					}
				}
			}()
		}

		wg.Wait()

		// 总调用次数 = goroutines * iterations
		// 预期采样次数 = total / 10
		total := goroutines * iterations
		expected := total / 10
		got := sampled.Load()

		if got != int64(expected) {
			t.Errorf("Expected %d samples, got %d", expected, got)
		}
	})

	t.Run("RateSampler", func(t *testing.T) {
		sampler := NewRateSampler(0.1)
		var wg sync.WaitGroup
		var sampled atomic.Int64

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					if sampler.ShouldSample(ctx) {
						sampled.Add(1)
					}
				}
			}()
		}

		wg.Wait()

		total := float64(goroutines * iterations)
		rate := float64(sampled.Load()) / total

		// 允许较大误差，主要验证并发安全
		if rate < 0.05 || rate > 0.15 {
			t.Errorf("Concurrent rate should be around 0.1, got %f", rate)
		}
	})

	t.Run("CompositeSampler", func(t *testing.T) {
		counter := NewCountSampler(10)
		sampler := All(counter, NewRateSampler(1.0))
		var wg sync.WaitGroup

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					sampler.ShouldSample(ctx)
				}
			}()
		}

		wg.Wait()
		// 主要验证没有 panic 或 data race
	})

	t.Run("KeyBasedSampler", func(t *testing.T) {
		sampler := NewKeyBasedSampler(0.5, func(ctx context.Context) string {
			if v, ok := ctx.Value(testKeyName).(string); ok {
				return v
			}
			return ""
		})
		var wg sync.WaitGroup

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					ctx := context.WithValue(context.Background(), testKeyName, string(rune('a'+id%26)))
					sampler.ShouldSample(ctx)
				}
			}(i)
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

	// 验证 Sampler 接口
	samplers := []Sampler{
		Always(),
		Never(),
		NewRateSampler(0.5),
		NewCountSampler(10),
		NewProbabilitySampler(0.5),
		NewCompositeSampler(ModeAND),
		NewKeyBasedSampler(0.5, nil),
	}
	for _, s := range samplers {
		_ = s.ShouldSample(ctx) // 验证方法可调用
	}

	// 验证 ResettableSampler 接口
	resettables := []ResettableSampler{
		NewCountSampler(10),
		NewCompositeSampler(ModeAND),
	}
	for _, r := range resettables {
		r.Reset() // 验证方法可调用
	}
}
