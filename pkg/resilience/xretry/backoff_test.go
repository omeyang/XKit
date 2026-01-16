package xretry

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFixedBackoff(t *testing.T) {
	t.Run("Basic", func(t *testing.T) {
		b := NewFixedBackoff(100 * time.Millisecond)

		for i := 1; i <= 10; i++ {
			assert.Equal(t, 100*time.Millisecond, b.NextDelay(i))
		}
	})

	t.Run("NegativeDelay", func(t *testing.T) {
		b := NewFixedBackoff(-100 * time.Millisecond)
		assert.Equal(t, time.Duration(0), b.NextDelay(1))
	})

	t.Run("ZeroDelay", func(t *testing.T) {
		b := NewFixedBackoff(0)
		assert.Equal(t, time.Duration(0), b.NextDelay(1))
	})
}

func TestExponentialBackoff(t *testing.T) {
	t.Run("DefaultValues", func(t *testing.T) {
		b := NewExponentialBackoff()

		// 第一次应该接近 100ms（有 10% 抖动）
		delay := b.NextDelay(1)
		assert.InDelta(t, 100*time.Millisecond, delay, float64(20*time.Millisecond))

		// 第二次应该接近 200ms（有抖动）
		delay = b.NextDelay(2)
		assert.InDelta(t, 200*time.Millisecond, delay, float64(40*time.Millisecond))
	})

	t.Run("CustomValues", func(t *testing.T) {
		b := NewExponentialBackoff(
			WithInitialDelay(50*time.Millisecond),
			WithMaxDelay(1*time.Second),
			WithMultiplier(3.0),
			WithJitter(0), // 无抖动
		)

		assert.Equal(t, 50*time.Millisecond, b.NextDelay(1))
		assert.Equal(t, 150*time.Millisecond, b.NextDelay(2))
		assert.Equal(t, 450*time.Millisecond, b.NextDelay(3))
		assert.Equal(t, 1*time.Second, b.NextDelay(4)) // 达到最大值
	})

	t.Run("MaxDelayLimit", func(t *testing.T) {
		b := NewExponentialBackoff(
			WithInitialDelay(100*time.Millisecond),
			WithMaxDelay(500*time.Millisecond),
			WithMultiplier(2.0),
			WithJitter(0),
		)

		assert.Equal(t, 100*time.Millisecond, b.NextDelay(1))
		assert.Equal(t, 200*time.Millisecond, b.NextDelay(2))
		assert.Equal(t, 400*time.Millisecond, b.NextDelay(3))
		assert.Equal(t, 500*time.Millisecond, b.NextDelay(4)) // 达到最大值
		assert.Equal(t, 500*time.Millisecond, b.NextDelay(100))
	})

	t.Run("JitterRange", func(t *testing.T) {
		b := NewExponentialBackoff(
			WithInitialDelay(100*time.Millisecond),
			WithJitter(0.5), // 50% 抖动
		)

		delays := make([]time.Duration, 100)
		for i := 0; i < 100; i++ {
			delays[i] = b.NextDelay(1)
		}

		// 检查所有延迟都在 50-150ms 范围内
		for _, d := range delays {
			assert.GreaterOrEqual(t, d, 50*time.Millisecond)
			assert.LessOrEqual(t, d, 150*time.Millisecond)
		}
	})

	t.Run("InvalidAttempt", func(t *testing.T) {
		b := NewExponentialBackoff(WithJitter(0))

		// attempt < 1 应该被当作 1
		assert.Equal(t, 100*time.Millisecond, b.NextDelay(0))
		assert.Equal(t, 100*time.Millisecond, b.NextDelay(-1))
	})

	t.Run("Reset", func(t *testing.T) {
		b := NewExponentialBackoff()
		b.Reset() // 不应该 panic
	})

	t.Run("InvalidJitterClamped", func(t *testing.T) {
		// 负抖动应该被设为 0
		b := NewExponentialBackoff(WithJitter(-0.5), WithInitialDelay(100*time.Millisecond))
		delay := b.NextDelay(1)
		assert.Equal(t, 100*time.Millisecond, delay) // 无抖动

		// 超过 1 的抖动应该被设为 1
		b2 := NewExponentialBackoff(WithJitter(1.5), WithInitialDelay(100*time.Millisecond))
		delay2 := b2.NextDelay(1)
		assert.GreaterOrEqual(t, delay2, time.Duration(0))
		assert.LessOrEqual(t, delay2, 200*time.Millisecond)
	})
}

func TestLinearBackoff(t *testing.T) {
	t.Run("Basic", func(t *testing.T) {
		b := NewLinearBackoff(100*time.Millisecond, 50*time.Millisecond, 500*time.Millisecond)

		assert.Equal(t, 100*time.Millisecond, b.NextDelay(1))
		assert.Equal(t, 150*time.Millisecond, b.NextDelay(2))
		assert.Equal(t, 200*time.Millisecond, b.NextDelay(3))
		assert.Equal(t, 250*time.Millisecond, b.NextDelay(4))
	})

	t.Run("MaxDelayLimit", func(t *testing.T) {
		b := NewLinearBackoff(100*time.Millisecond, 100*time.Millisecond, 300*time.Millisecond)

		assert.Equal(t, 100*time.Millisecond, b.NextDelay(1))
		assert.Equal(t, 200*time.Millisecond, b.NextDelay(2))
		assert.Equal(t, 300*time.Millisecond, b.NextDelay(3))
		assert.Equal(t, 300*time.Millisecond, b.NextDelay(4)) // 达到最大值
		assert.Equal(t, 300*time.Millisecond, b.NextDelay(100))
	})

	t.Run("InvalidAttempt", func(t *testing.T) {
		b := NewLinearBackoff(100*time.Millisecond, 50*time.Millisecond, 500*time.Millisecond)

		assert.Equal(t, 100*time.Millisecond, b.NextDelay(0))
		assert.Equal(t, 100*time.Millisecond, b.NextDelay(-1))
	})

	t.Run("NegativeValues", func(t *testing.T) {
		b := NewLinearBackoff(-100*time.Millisecond, -50*time.Millisecond, 500*time.Millisecond)

		assert.Equal(t, time.Duration(0), b.NextDelay(1))
	})

	t.Run("MaxDelayLessThanInitial", func(t *testing.T) {
		// maxDelay < initialDelay 时，maxDelay 应该被设置为 initialDelay
		b := NewLinearBackoff(200*time.Millisecond, 50*time.Millisecond, 100*time.Millisecond)

		// 所有延迟都应该是 initialDelay（因为 maxDelay 被修正为 initialDelay）
		assert.Equal(t, 200*time.Millisecond, b.NextDelay(1))
		assert.Equal(t, 200*time.Millisecond, b.NextDelay(2))
		assert.Equal(t, 200*time.Millisecond, b.NextDelay(10))
	})

	t.Run("ZeroIncrement", func(t *testing.T) {
		b := NewLinearBackoff(100*time.Millisecond, 0, 500*time.Millisecond)

		// 没有增量，所有延迟都应该相同
		assert.Equal(t, 100*time.Millisecond, b.NextDelay(1))
		assert.Equal(t, 100*time.Millisecond, b.NextDelay(5))
		assert.Equal(t, 100*time.Millisecond, b.NextDelay(100))
	})

	t.Run("OverflowProtection", func(t *testing.T) {
		// 创建一个容易溢出的配置：大增量 + 极大的 attempt
		b := NewLinearBackoff(
			time.Second,  // 初始延迟 1s
			time.Hour,    // 每次增加 1 小时
			24*time.Hour, // 最大 24 小时
		)

		// 使用一个非常大的 attempt 值来触发溢出
		// time.Hour * 大数 会导致整数溢出为负数
		veryLargeAttempt := 1 << 60 // 一个非常大的数

		delay := b.NextDelay(veryLargeAttempt)

		// 溢出保护：应该返回 maxDelay，而不是负数或 panic
		assert.Equal(t, 24*time.Hour, delay)
		assert.GreaterOrEqual(t, delay, time.Duration(0), "delay should not be negative due to overflow")
	})
}

func TestNoBackoff(t *testing.T) {
	b := NewNoBackoff()

	for i := 1; i <= 100; i++ {
		assert.Equal(t, time.Duration(0), b.NextDelay(i))
	}
}

// TestLinearBackoff_ExtremeOverflow 验证 LinearBackoff 在极端 attempt 值下的溢出保护
// 此测试防止 attempt-1 溢出成小正数或负数绕过检测的问题
func TestLinearBackoff_ExtremeOverflow(t *testing.T) {
	b := NewLinearBackoff(
		time.Second,  // 初始延迟 1s
		time.Hour,    // 每次增加 1 小时
		24*time.Hour, // 最大 24 小时
	)

	testCases := []int{
		1 << 30,     // 约 10 亿，等于 maxSafeAttempt
		1<<30 + 1,   // 刚超过 maxSafeAttempt
		1 << 40,     // 约 1 万亿
		1 << 60,     // 极端大数
		math.MaxInt, // int 最大值
		math.MaxInt - 1,
	}

	for _, attempt := range testCases {
		t.Run(fmt.Sprintf("attempt_%d", attempt), func(t *testing.T) {
			delay := b.NextDelay(attempt)
			// 所有极端值都应返回 maxDelay
			assert.Equal(t, 24*time.Hour, delay, "should return maxDelay for extreme attempt")
			// 确保不会返回负数（溢出结果）
			assert.GreaterOrEqual(t, delay, time.Duration(0), "delay should never be negative")
		})
	}
}

// TestLinearBackoff_OverflowWrapAround 验证大 increment + 小 attempt 场景下的溢出保护
// 此测试防止 increment * (attempt-1) 溢出后回绕成小正数绕过检测的问题
// 问题场景：increment=51944h, attempt=100, maxDelay=100000h 时，
// 乘法溢出后得到 18404h（小于 maxDelay），旧代码会错误返回 18404h
func TestLinearBackoff_OverflowWrapAround(t *testing.T) {
	t.Run("大 increment 导致溢出回绕", func(t *testing.T) {
		// 这个配置会导致 increment * 99 溢出
		// increment = 1.87e17 ns ≈ 51944 小时
		// increment * 99 的真实值远超 int64 最大值
		// 溢出后会回绕成一个较小的正数（约 18404 小时）
		b := NewLinearBackoff(
			time.Second,            // 初始延迟 1s
			time.Duration(1.87e17), // 约 51944 小时
			100000*time.Hour,       // 最大 100000 小时
		)

		delay := b.NextDelay(100)

		// 正确行为：应返回 maxDelay（因为真实增量远超 maxDelay）
		// 错误行为：返回约 18404 小时（溢出回绕后的值）
		assert.Equal(t, 100000*time.Hour, delay,
			"应返回 maxDelay，而非溢出回绕后的错误值")
	})

	t.Run("边界值：刚好不溢出", func(t *testing.T) {
		// 构造一个刚好不会溢出的场景
		// maxDelay = 100 小时，increment = 10 小时，attempt = 10
		// incrementPart = 10h * 9 = 90h，不溢出
		b := NewLinearBackoff(
			time.Hour,     // 初始延迟 1h
			10*time.Hour,  // 每次增加 10 小时
			100*time.Hour, // 最大 100 小时
		)

		delay := b.NextDelay(10)
		// 1h + 10h * 9 = 91h < 100h
		assert.Equal(t, 91*time.Hour, delay)
	})

	t.Run("边界值：刚好超过 maxDelay", func(t *testing.T) {
		b := NewLinearBackoff(
			time.Hour,     // 初始延迟 1h
			10*time.Hour,  // 每次增加 10 小时
			100*time.Hour, // 最大 100 小时
		)

		delay := b.NextDelay(11)
		// 1h + 10h * 10 = 101h > 100h，应返回 maxDelay
		assert.Equal(t, 100*time.Hour, delay)
	})

	t.Run("零 increment 不应触发溢出检测", func(t *testing.T) {
		b := NewLinearBackoff(
			time.Second,
			0, // 零增量
			time.Hour,
		)

		// 任何 attempt 都应返回 initialDelay
		assert.Equal(t, time.Second, b.NextDelay(1))
		assert.Equal(t, time.Second, b.NextDelay(100))
		assert.Equal(t, time.Second, b.NextDelay(math.MaxInt))
	})
}
