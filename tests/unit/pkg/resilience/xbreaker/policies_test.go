package xbreaker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConsecutiveFailuresPolicy(t *testing.T) {
	t.Run("threshold not reached", func(t *testing.T) {
		policy := NewConsecutiveFailures(5)
		counts := Counts{ConsecutiveFailures: 4}

		assert.False(t, policy.ReadyToTrip(counts))
	})

	t.Run("threshold reached", func(t *testing.T) {
		policy := NewConsecutiveFailures(5)
		counts := Counts{ConsecutiveFailures: 5}

		assert.True(t, policy.ReadyToTrip(counts))
	})

	t.Run("threshold exceeded", func(t *testing.T) {
		policy := NewConsecutiveFailures(5)
		counts := Counts{ConsecutiveFailures: 10}

		assert.True(t, policy.ReadyToTrip(counts))
	})

	t.Run("zero threshold clamped to 1", func(t *testing.T) {
		policy := NewConsecutiveFailures(0)
		// threshold=0 被修正为 1
		assert.Equal(t, uint32(1), policy.Threshold())
		// 0 < 1，不触发熔断
		counts := Counts{ConsecutiveFailures: 0}
		assert.False(t, policy.ReadyToTrip(counts))
		// 1 >= 1，触发熔断
		counts = Counts{ConsecutiveFailures: 1}
		assert.True(t, policy.ReadyToTrip(counts))
	})

	t.Run("threshold getter", func(t *testing.T) {
		policy := NewConsecutiveFailures(5)
		assert.Equal(t, uint32(5), policy.Threshold())
	})
}

func TestFailureRatioPolicy(t *testing.T) {
	t.Run("requests below minimum", func(t *testing.T) {
		policy := NewFailureRatio(0.5, 10)
		counts := Counts{
			Requests:      5,
			TotalFailures: 5, // 100% 失败率，但请求数不足
		}

		assert.False(t, policy.ReadyToTrip(counts))
	})

	t.Run("ratio not reached", func(t *testing.T) {
		policy := NewFailureRatio(0.5, 10)
		counts := Counts{
			Requests:      10,
			TotalFailures: 4, // 40% 失败率
		}

		assert.False(t, policy.ReadyToTrip(counts))
	})

	t.Run("ratio reached", func(t *testing.T) {
		policy := NewFailureRatio(0.5, 10)
		counts := Counts{
			Requests:      10,
			TotalFailures: 5, // 50% 失败率
		}

		assert.True(t, policy.ReadyToTrip(counts))
	})

	t.Run("ratio exceeded", func(t *testing.T) {
		policy := NewFailureRatio(0.5, 10)
		counts := Counts{
			Requests:      10,
			TotalFailures: 8, // 80% 失败率
		}

		assert.True(t, policy.ReadyToTrip(counts))
	})

	t.Run("ratio clamped to valid range", func(t *testing.T) {
		// 负数应该被调整为 0
		policy1 := NewFailureRatio(-0.5, 10)
		assert.Equal(t, float64(0), policy1.Ratio())

		// 大于 1 应该被调整为 1
		policy2 := NewFailureRatio(1.5, 10)
		assert.Equal(t, float64(1), policy2.Ratio())
	})

	t.Run("getters", func(t *testing.T) {
		policy := NewFailureRatio(0.5, 10)
		assert.Equal(t, float64(0.5), policy.Ratio())
		assert.Equal(t, uint32(10), policy.MinRequests())
	})

	t.Run("zero requests should not panic", func(t *testing.T) {
		policy := NewFailureRatio(0.5, 10)
		counts := Counts{
			Requests:      0,
			TotalFailures: 0,
		}
		// 应该返回 false，不应该 panic（除零保护）
		assert.False(t, policy.ReadyToTrip(counts))
	})

	t.Run("minRequests=0 and Requests=0 should not panic", func(t *testing.T) {
		policy := NewFailureRatio(0.5, 0) // minRequests=0 的边界情况
		counts := Counts{
			Requests:      0,
			TotalFailures: 0,
		}
		// 应该返回 false，不应该 panic（除零保护）
		assert.False(t, policy.ReadyToTrip(counts))
	})
}

func TestFailureCountPolicy(t *testing.T) {
	t.Run("threshold not reached", func(t *testing.T) {
		policy := NewFailureCount(10)
		counts := Counts{TotalFailures: 9}

		assert.False(t, policy.ReadyToTrip(counts))
	})

	t.Run("threshold reached", func(t *testing.T) {
		policy := NewFailureCount(10)
		counts := Counts{TotalFailures: 10}

		assert.True(t, policy.ReadyToTrip(counts))
	})

	t.Run("threshold exceeded", func(t *testing.T) {
		policy := NewFailureCount(10)
		counts := Counts{TotalFailures: 15}

		assert.True(t, policy.ReadyToTrip(counts))
	})

	t.Run("threshold getter", func(t *testing.T) {
		policy := NewFailureCount(10)
		assert.Equal(t, uint32(10), policy.Threshold())
	})

	t.Run("zero threshold clamped to 1", func(t *testing.T) {
		policy := NewFailureCount(0)
		// threshold=0 被修正为 1
		assert.Equal(t, uint32(1), policy.Threshold())
		// 0 < 1，不触发熔断
		counts := Counts{TotalFailures: 0}
		assert.False(t, policy.ReadyToTrip(counts))
		// 1 >= 1，触发熔断
		counts = Counts{TotalFailures: 1}
		assert.True(t, policy.ReadyToTrip(counts))
	})
}

func TestCompositePolicy(t *testing.T) {
	t.Run("no policies", func(t *testing.T) {
		policy := NewCompositePolicy()
		counts := Counts{TotalFailures: 100}

		assert.False(t, policy.ReadyToTrip(counts))
	})

	t.Run("empty policies getter", func(t *testing.T) {
		policy := NewCompositePolicy()
		assert.Nil(t, policy.Policies())
	})

	t.Run("first policy triggers", func(t *testing.T) {
		policy := NewCompositePolicy(
			NewConsecutiveFailures(5),
			NewFailureRatio(0.5, 10),
		)
		counts := Counts{
			ConsecutiveFailures: 5,
			Requests:            5,
			TotalFailures:       1,
		}

		assert.True(t, policy.ReadyToTrip(counts))
	})

	t.Run("second policy triggers", func(t *testing.T) {
		policy := NewCompositePolicy(
			NewConsecutiveFailures(10),
			NewFailureRatio(0.5, 10),
		)
		counts := Counts{
			ConsecutiveFailures: 3,
			Requests:            10,
			TotalFailures:       5,
		}

		assert.True(t, policy.ReadyToTrip(counts))
	})

	t.Run("no policy triggers", func(t *testing.T) {
		policy := NewCompositePolicy(
			NewConsecutiveFailures(10),
			NewFailureRatio(0.5, 10),
		)
		counts := Counts{
			ConsecutiveFailures: 3,
			Requests:            10,
			TotalFailures:       3,
		}

		assert.False(t, policy.ReadyToTrip(counts))
	})

	t.Run("policies getter", func(t *testing.T) {
		p1 := NewConsecutiveFailures(5)
		p2 := NewFailureRatio(0.5, 10)
		policy := NewCompositePolicy(p1, p2)

		policies := policy.Policies()
		assert.Len(t, policies, 2)
		assert.Equal(t, p1, policies[0])
		assert.Equal(t, p2, policies[1])
	})
}

func TestNeverTripPolicy(t *testing.T) {
	policy := NewNeverTrip()

	// 无论什么情况都不触发
	testCases := []Counts{
		{},
		{TotalFailures: 1000},
		{ConsecutiveFailures: 1000},
		{Requests: 1000, TotalFailures: 1000},
	}

	for _, counts := range testCases {
		assert.False(t, policy.ReadyToTrip(counts))
	}
}

func TestAlwaysTripPolicy(t *testing.T) {
	policy := NewAlwaysTrip()

	t.Run("no failures", func(t *testing.T) {
		counts := Counts{Requests: 10, TotalSuccesses: 10}
		assert.False(t, policy.ReadyToTrip(counts))
	})

	t.Run("with failures", func(t *testing.T) {
		counts := Counts{TotalFailures: 1}
		assert.True(t, policy.ReadyToTrip(counts))
	})
}

func TestSlowCallRatioPolicy(t *testing.T) {
	t.Run("requests below minimum", func(t *testing.T) {
		policy := NewSlowCallRatio(0.5, 10)
		counts := Counts{
			Requests:      5,
			TotalFailures: 5,
		}

		assert.False(t, policy.ReadyToTrip(counts))
	})

	t.Run("ratio not reached", func(t *testing.T) {
		policy := NewSlowCallRatio(0.5, 10)
		counts := Counts{
			Requests:      10,
			TotalFailures: 4,
		}

		assert.False(t, policy.ReadyToTrip(counts))
	})

	t.Run("ratio reached", func(t *testing.T) {
		policy := NewSlowCallRatio(0.5, 10)
		counts := Counts{
			Requests:      10,
			TotalFailures: 5,
		}

		assert.True(t, policy.ReadyToTrip(counts))
	})

	t.Run("ratio clamped to valid range", func(t *testing.T) {
		policy1 := NewSlowCallRatio(-0.5, 10)
		assert.Equal(t, float64(0), policy1.Ratio())

		policy2 := NewSlowCallRatio(1.5, 10)
		assert.Equal(t, float64(1), policy2.Ratio())
	})

	t.Run("getters", func(t *testing.T) {
		policy := NewSlowCallRatio(0.3, 20)
		assert.Equal(t, float64(0.3), policy.Ratio())
		assert.Equal(t, uint32(20), policy.MinRequests())
	})

	t.Run("zero requests should not panic", func(t *testing.T) {
		policy := NewSlowCallRatio(0.5, 10)
		counts := Counts{
			Requests:      0,
			TotalFailures: 0,
		}
		// 应该返回 false，不应该 panic（除零保护）
		assert.False(t, policy.ReadyToTrip(counts))
	})

	t.Run("minRequests=0 and Requests=0 should not panic", func(t *testing.T) {
		policy := NewSlowCallRatio(0.5, 0) // minRequests=0 的边界情况
		counts := Counts{
			Requests:      0,
			TotalFailures: 0,
		}
		// 应该返回 false，不应该 panic（除零保护）
		assert.False(t, policy.ReadyToTrip(counts))
	})
}

// TestRatioPolicy_ExcludedRequestsDenominator 验证 FG-S1 修复：
// 比率策略使用有效请求数（排除被 ExcludePolicy 排除的请求）作为分母和 minRequests 判定依据。
func TestRatioPolicy_ExcludedRequestsDenominator(t *testing.T) {
	t.Run("excluded requests not counted in denominator", func(t *testing.T) {
		// 场景：3 excluded + 2 failures + 1 success = Requests=6, TotalExclusions=3
		// 有效请求 = 6 - 3 = 3, 失败率 = 2/3 ≈ 0.67 > 0.5 → 应触发熔断
		policy := NewFailureRatio(0.5, 3) // minRequests=3
		counts := Counts{
			Requests:        6,
			TotalFailures:   2,
			TotalSuccesses:  1,
			TotalExclusions: 3,
		}
		assert.True(t, policy.ReadyToTrip(counts),
			"should trip: effective ratio = 2/3 ≈ 0.67 > 0.5")
	})

	t.Run("excluded requests dilute denominator without fix", func(t *testing.T) {
		// 修复前此场景会漏断：Requests=6, TotalFailures=2, ratio=2/6=0.33 < 0.5
		// 修复后：effective=3, ratio=2/3=0.67 > 0.5
		policy := NewFailureRatio(0.5, 3)
		counts := Counts{
			Requests:        6,
			TotalFailures:   2,
			TotalSuccesses:  1,
			TotalExclusions: 3,
		}
		assert.True(t, policy.ReadyToTrip(counts))
	})

	t.Run("excluded requests inflate minRequests check", func(t *testing.T) {
		// 场景：2 excluded + 2 failures = Requests=4, TotalExclusions=2
		// 有效请求 = 4 - 2 = 2, 但 minRequests=4 → 不应触发（有效请求不足）
		policy := NewFailureRatio(0.5, 4) // minRequests=4
		counts := Counts{
			Requests:        4,
			TotalFailures:   2,
			TotalExclusions: 2,
		}
		assert.False(t, policy.ReadyToTrip(counts),
			"should not trip: effective requests (2) < minRequests (4)")
	})

	t.Run("all requests excluded", func(t *testing.T) {
		// 全部被排除：effective = 0 → 不触发
		policy := NewFailureRatio(0.5, 1)
		counts := Counts{
			Requests:        5,
			TotalExclusions: 5,
		}
		assert.False(t, policy.ReadyToTrip(counts),
			"should not trip: all requests excluded, effective = 0")
	})

	t.Run("no exclusions unchanged behavior", func(t *testing.T) {
		// 无排除时行为不变
		policy := NewFailureRatio(0.5, 10)
		counts := Counts{
			Requests:      10,
			TotalFailures: 5,
		}
		assert.True(t, policy.ReadyToTrip(counts),
			"should trip: no exclusions, ratio = 5/10 = 0.5")
	})

	t.Run("SlowCallRatioPolicy also uses effective denominator", func(t *testing.T) {
		// SlowCallRatioPolicy 共享 ratioPolicy.readyToTrip，验证修复一致性
		policy := NewSlowCallRatio(0.5, 3)
		counts := Counts{
			Requests:        6,
			TotalFailures:   2,
			TotalSuccesses:  1,
			TotalExclusions: 3,
		}
		assert.True(t, policy.ReadyToTrip(counts),
			"SlowCallRatioPolicy should also use effective denominator")
	})

	t.Run("TotalExclusions exceeds Requests underflow protection", func(t *testing.T) {
		// 防下溢：TotalExclusions > Requests（理论上不会发生，但防御性编程）
		policy := NewFailureRatio(0.5, 1)
		counts := Counts{
			Requests:        3,
			TotalExclusions: 5, // 异常：排除数大于请求数
			TotalFailures:   1,
		}
		// effective 保持为 Requests=3（不减去，因为 TotalExclusions > Requests）
		// ratio = 1/3 ≈ 0.33 < 0.5 → 不触发
		assert.False(t, policy.ReadyToTrip(counts),
			"underflow protection: effective stays at Requests, ratio = 1/3 < 0.5")
	})
}
