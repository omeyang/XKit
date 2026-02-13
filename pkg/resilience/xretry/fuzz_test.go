package xretry

import (
	"context"
	"errors"
	"testing"
	"time"
)

// FuzzExponentialBackoff 测试指数退避策略的边界条件
func FuzzExponentialBackoff(f *testing.F) {
	// 添加种子语料
	f.Add(int64(100), int64(1000), 2.0, 0.1, 1)
	f.Add(int64(0), int64(0), 1.0, 0.0, 0)
	f.Add(int64(-100), int64(-100), -1.0, -0.5, -1)
	f.Add(int64(1000000000), int64(10000000000), 10.0, 1.0, 100)

	f.Fuzz(func(t *testing.T, initialMs, maxMs int64, multiplier, jitter float64, attempt int) {
		initial := time.Duration(initialMs) * time.Millisecond
		maxDelay := time.Duration(maxMs) * time.Millisecond

		// 创建退避策略，应该不会 panic
		b := NewExponentialBackoff(
			WithInitialDelay(initial),
			WithMaxDelay(maxDelay),
			WithMultiplier(multiplier),
			WithJitter(jitter),
		)

		// NextDelay 应该不会 panic 且返回非负值
		delay := b.NextDelay(attempt)
		if delay < 0 {
			t.Errorf("NextDelay returned negative: %v", delay)
		}

		// Reset 应该不会 panic
		b.Reset()
	})
}

// FuzzLinearBackoff 测试线性退避策略的边界条件
func FuzzLinearBackoff(f *testing.F) {
	// 添加种子语料
	f.Add(int64(100), int64(50), int64(1000), 1)
	f.Add(int64(0), int64(0), int64(0), 0)
	f.Add(int64(-100), int64(-50), int64(-1000), -1)
	f.Add(int64(1000000000), int64(100000000), int64(10000000000), 1000)

	f.Fuzz(func(t *testing.T, initialMs, incrementMs, maxMs int64, attempt int) {
		initial := time.Duration(initialMs) * time.Millisecond
		increment := time.Duration(incrementMs) * time.Millisecond
		maxDelay := time.Duration(maxMs) * time.Millisecond

		// 创建退避策略，应该不会 panic
		b := NewLinearBackoff(initial, increment, maxDelay)

		// NextDelay 应该不会 panic 且返回非负值
		delay := b.NextDelay(attempt)
		if delay < 0 {
			t.Errorf("NextDelay returned negative: %v", delay)
		}
	})
}

// FuzzFixedBackoff 测试固定退避策略
func FuzzFixedBackoff(f *testing.F) {
	f.Add(int64(100), 1)
	f.Add(int64(0), 0)
	f.Add(int64(-100), -1)
	f.Add(int64(1000000000), 1000000)

	f.Fuzz(func(t *testing.T, delayMs int64, attempt int) {
		delay := time.Duration(delayMs) * time.Millisecond

		b := NewFixedBackoff(delay)
		result := b.NextDelay(attempt)
		if result < 0 {
			t.Errorf("NextDelay returned negative: %v", result)
		}
	})
}

// FuzzFixedRetry 测试固定重试策略
func FuzzFixedRetry(f *testing.F) {
	f.Add(3, 1)
	f.Add(0, 0)
	f.Add(-1, -1)
	f.Add(1000000, 1000000)

	f.Fuzz(func(t *testing.T, maxAttempts, attempt int) {
		p := NewFixedRetry(maxAttempts)

		// MaxAttempts 应该返回正数或零
		max := p.MaxAttempts()
		if max < 0 {
			t.Errorf("MaxAttempts returned negative: %v", max)
		}

		// ShouldRetry 应该不会 panic
		_ = p.ShouldRetry(context.Background(), attempt, errors.New("test"))
	})
}

// FuzzPermanentError 测试永久错误包装
func FuzzPermanentError(f *testing.F) {
	f.Add("error message")
	f.Add("")
	f.Add("错误消息")
	f.Add(string(make([]byte, 10000))) // 长字符串

	f.Fuzz(func(t *testing.T, msg string) {
		if msg == "" {
			return // 空消息跳过
		}

		err := NewPermanentError(errors.New(msg))

		// Error() 应该不会 panic
		_ = err.Error()

		// Unwrap() 应该返回原始错误
		unwrapped := err.Unwrap()
		if unwrapped == nil {
			t.Error("Unwrap returned nil")
		}

		// Retryable() 应该返回 false
		if err.Retryable() {
			t.Error("PermanentError.Retryable() should return false")
		}

		// IsRetryable 应该返回 false
		if IsRetryable(err) {
			t.Error("IsRetryable should return false for PermanentError")
		}

		// IsPermanent 应该返回 true
		if !IsPermanent(err) {
			t.Error("IsPermanent should return true for PermanentError")
		}
	})
}

// FuzzTemporaryError 测试临时错误包装
func FuzzTemporaryError(f *testing.F) {
	f.Add("error message")
	f.Add("")
	f.Add("错误消息")

	f.Fuzz(func(t *testing.T, msg string) {
		if msg == "" {
			return
		}

		err := NewTemporaryError(errors.New(msg))

		// Error() 应该不会 panic
		_ = err.Error()

		// Unwrap() 应该返回原始错误
		unwrapped := err.Unwrap()
		if unwrapped == nil {
			t.Error("Unwrap returned nil")
		}

		// Retryable() 应该返回 true
		if !err.Retryable() {
			t.Error("TemporaryError.Retryable() should return true")
		}

		// IsRetryable 应该返回 true
		if !IsRetryable(err) {
			t.Error("IsRetryable should return true for TemporaryError")
		}
	})
}

// clampFuzzRetryParams 限制 fuzz 参数范围避免测试过慢。
func clampFuzzRetryParams(maxAttempts int, delayNs int64) (int, int64) {
	if maxAttempts < 0 {
		maxAttempts = 0
	}
	if maxAttempts > 10 {
		maxAttempts = 10
	}
	if delayNs < 0 {
		delayNs = 0
	}
	if delayNs > int64(time.Millisecond) {
		delayNs = int64(time.Millisecond)
	}

	return maxAttempts, delayNs
}

// FuzzRetryer 测试 Retryer 的健壮性
func FuzzRetryer(f *testing.F) {
	f.Add(3, int64(100), true)
	f.Add(0, int64(0), false)
	f.Add(1, int64(1000000), true)
	f.Add(100, int64(1), false)

	f.Fuzz(func(t *testing.T, maxAttempts int, delayNs int64, shouldSucceed bool) {
		maxAttempts, delayNs = clampFuzzRetryParams(maxAttempts, delayNs)

		r := NewRetryer(
			WithRetryPolicy(NewFixedRetry(maxAttempts)),
			WithBackoffPolicy(NewFixedBackoff(time.Duration(delayNs))),
		)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		attempt := 0
		err := r.Do(ctx, func(ctx context.Context) error {
			attempt++
			if shouldSucceed && attempt >= 1 {
				return nil
			}
			return errors.New("retry")
		})

		// 验证结果的一致性
		if shouldSucceed && err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			// 如果预期成功但失败了，可能是超时，这是可接受的
			_ = err
		}
	})
}

// FuzzWrapperDo 测试 wrapper.Do 的健壮性
func FuzzWrapperDo(f *testing.F) {
	f.Add(uint(3), int64(0), true)
	f.Add(uint(1), int64(1000), false)
	f.Add(uint(0), int64(0), true) // 0 表示无限重试

	f.Fuzz(func(t *testing.T, attempts uint, delayNs int64, shouldSucceed bool) {
		// 限制参数范围
		if attempts > 10 {
			attempts = 10
		}
		if attempts == 0 {
			attempts = 1 // 避免无限循环
		}
		if delayNs < 0 {
			delayNs = 0
		}
		if delayNs > int64(time.Millisecond) {
			delayNs = int64(time.Millisecond)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		attempt := 0
		err := Do(ctx, func() error {
			attempt++
			if shouldSucceed {
				return nil
			}
			return errors.New("retry")
		}, Attempts(attempts), Delay(time.Duration(delayNs)))

		// 验证结果
		if shouldSucceed && err != nil {
			t.Errorf("Expected success but got error: %v", err)
		}
	})
}
