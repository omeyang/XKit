package xbreaker

import (
	"testing"

	"github.com/sony/gobreaker/v2"
)

// FuzzConsecutiveFailures 模糊测试连续失败策略
func FuzzConsecutiveFailures(f *testing.F) {
	// 添加种子语料
	f.Add(uint32(0))
	f.Add(uint32(1))
	f.Add(uint32(5))
	f.Add(uint32(100))
	f.Add(uint32(1000))
	f.Add(^uint32(0)) // max uint32

	f.Fuzz(func(t *testing.T, threshold uint32) {
		policy := NewConsecutiveFailures(threshold)

		// threshold < 1 被修正为 1
		expectedThreshold := max(threshold, 1)
		if policy.Threshold() != expectedThreshold {
			t.Errorf("Threshold mismatch: got %d, want %d", policy.Threshold(), expectedThreshold)
		}

		// 测试 ReadyToTrip
		counts := gobreaker.Counts{
			ConsecutiveFailures: expectedThreshold,
		}
		result := policy.ReadyToTrip(counts)
		if !result {
			t.Errorf("Expected ReadyToTrip=true for ConsecutiveFailures=%d, threshold=%d",
				counts.ConsecutiveFailures, expectedThreshold)
		}

		// 低于阈值不应触发（expectedThreshold 至少为 1）
		counts.ConsecutiveFailures = expectedThreshold - 1
		if policy.ReadyToTrip(counts) {
			t.Errorf("Expected ReadyToTrip=false for ConsecutiveFailures=%d, threshold=%d",
				counts.ConsecutiveFailures, expectedThreshold)
		}
	})
}

// FuzzFailureRatio 模糊测试失败率策略
func FuzzFailureRatio(f *testing.F) {
	// 添加种子语料
	f.Add(0.0, uint32(10))
	f.Add(0.5, uint32(10))
	f.Add(1.0, uint32(10))
	f.Add(0.5, uint32(0))
	f.Add(0.5, uint32(1))
	f.Add(-0.5, uint32(10))  // 负值
	f.Add(1.5, uint32(10))   // 超出范围
	f.Add(0.333, uint32(30)) // 边界

	f.Fuzz(func(t *testing.T, ratio float64, minRequests uint32) {
		policy := NewFailureRatio(ratio, minRequests)

		// ratio 应该被规范化到 [0, 1]
		normalizedRatio := policy.Ratio()
		if normalizedRatio < 0 || normalizedRatio > 1 {
			t.Errorf("Ratio should be in [0, 1], got %f", normalizedRatio)
		}

		if policy.MinRequests() != minRequests {
			t.Errorf("MinRequests mismatch: got %d, want %d",
				policy.MinRequests(), minRequests)
		}

		// 测试请求数不足时不触发
		if minRequests > 0 {
			counts := gobreaker.Counts{
				Requests:      minRequests - 1,
				TotalFailures: minRequests - 1,
			}
			if policy.ReadyToTrip(counts) {
				t.Errorf("Should not trip when requests < minRequests")
			}
		}
	})
}

// FuzzFailureCount 模糊测试失败次数策略
func FuzzFailureCount(f *testing.F) {
	f.Add(uint32(0))
	f.Add(uint32(1))
	f.Add(uint32(10))
	f.Add(uint32(1000))
	f.Add(^uint32(0))

	f.Fuzz(func(t *testing.T, threshold uint32) {
		policy := NewFailureCount(threshold)

		// threshold < 1 被修正为 1
		expectedThreshold := max(threshold, 1)
		if policy.Threshold() != expectedThreshold {
			t.Errorf("Threshold mismatch: got %d, want %d", policy.Threshold(), expectedThreshold)
		}

		// 达到阈值应触发
		counts := gobreaker.Counts{
			TotalFailures: expectedThreshold,
		}
		result := policy.ReadyToTrip(counts)
		if !result {
			t.Errorf("Expected ReadyToTrip=true for TotalFailures=%d, threshold=%d",
				counts.TotalFailures, expectedThreshold)
		}
	})
}

// FuzzSlowCallRatio 模糊测试慢调用策略
func FuzzSlowCallRatio(f *testing.F) {
	f.Add(0.0, uint32(10))
	f.Add(0.5, uint32(10))
	f.Add(1.0, uint32(10))
	f.Add(-0.5, uint32(10))
	f.Add(1.5, uint32(10))

	f.Fuzz(func(t *testing.T, ratio float64, minRequests uint32) {
		policy := NewSlowCallRatio(ratio, minRequests)

		normalizedRatio := policy.Ratio()
		if normalizedRatio < 0 || normalizedRatio > 1 {
			t.Errorf("Ratio should be in [0, 1], got %f", normalizedRatio)
		}

		if policy.MinRequests() != minRequests {
			t.Errorf("MinRequests mismatch")
		}
	})
}

// FuzzBreakerWithCounts 模糊测试熔断器状态
func FuzzBreakerWithCounts(f *testing.F) {
	f.Add(uint32(10), uint32(5), uint32(3))
	f.Add(uint32(100), uint32(50), uint32(25))
	f.Add(uint32(0), uint32(0), uint32(0))

	f.Fuzz(func(t *testing.T, requests, successes, failures uint32) {
		// 确保统计数据有效
		if successes+failures > requests {
			return
		}

		counts := gobreaker.Counts{
			Requests:       requests,
			TotalSuccesses: successes,
			TotalFailures:  failures,
		}

		// 测试各种策略不会 panic
		p1 := NewConsecutiveFailures(5)
		_ = p1.ReadyToTrip(counts)

		p2 := NewFailureRatio(0.5, 10)
		_ = p2.ReadyToTrip(counts)

		p3 := NewFailureCount(10)
		_ = p3.ReadyToTrip(counts)

		p4 := NewNeverTrip()
		_ = p4.ReadyToTrip(counts)

		p5 := NewAlwaysTrip()
		_ = p5.ReadyToTrip(counts)

		// 组合策略
		p6 := NewCompositePolicy(p1, p2, p3)
		_ = p6.ReadyToTrip(counts)
	})
}
