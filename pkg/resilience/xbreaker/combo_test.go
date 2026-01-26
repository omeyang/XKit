package xbreaker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/resilience/xretry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBreakerRetryer(t *testing.T) {
	t.Run("success on first try", func(t *testing.T) {
		breaker := NewBreaker("test")
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(3)),
		)
		combo := NewBreakerRetryer(breaker, retryer)
		ctx := context.Background()

		var callCount int
		err := combo.DoWithRetry(ctx, func(_ context.Context) error {
			callCount++
			return nil
		})

		assert.NoError(t, err)
		assert.Equal(t, 1, callCount)
	})

	t.Run("success after retry", func(t *testing.T) {
		breaker := NewBreaker("test")
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(3)),
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)
		combo := NewBreakerRetryer(breaker, retryer)
		ctx := context.Background()

		var callCount int
		err := combo.DoWithRetry(ctx, func(_ context.Context) error {
			callCount++
			if callCount < 3 {
				return errTest
			}
			return nil
		})

		assert.NoError(t, err)
		assert.Equal(t, 3, callCount)
	})

	t.Run("all retries fail", func(t *testing.T) {
		breaker := NewBreaker("test",
			WithTripPolicy(NewConsecutiveFailures(5)), // 需要5次连续失败才熔断
		)
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(3)),
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)
		combo := NewBreakerRetryer(breaker, retryer)
		ctx := context.Background()

		var callCount int
		err := combo.DoWithRetry(ctx, func(_ context.Context) error {
			callCount++
			return errTest
		})

		assert.ErrorIs(t, err, errTest)
		assert.Equal(t, 3, callCount)
	})

	t.Run("breaker open", func(t *testing.T) {
		breaker := NewBreaker("test",
			WithTripPolicy(NewConsecutiveFailures(1)),
			WithTimeout(time.Hour),
		)
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(3)),
		)
		combo := NewBreakerRetryer(breaker, retryer)
		ctx := context.Background()

		// 触发熔断
		_ = combo.DoWithRetry(ctx, func(_ context.Context) error {
			return errTest
		})

		// 下一次调用应该直接失败
		var callCount int
		err := combo.DoWithRetry(ctx, func(_ context.Context) error {
			callCount++
			return nil
		})

		assert.True(t, IsOpen(err))
		assert.Equal(t, 0, callCount) // 函数不应该被调用
	})

	t.Run("getters", func(t *testing.T) {
		breaker := NewBreaker("test")
		retryer := xretry.NewRetryer()
		combo := NewBreakerRetryer(breaker, retryer)

		assert.Equal(t, breaker, combo.Breaker())
		assert.Equal(t, retryer, combo.Retryer())
	})
}

func TestBreakerRetryer_DoWithRetrySimple(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		breaker := NewBreaker("test")
		retryer := xretry.NewRetryer()
		combo := NewBreakerRetryer(breaker, retryer)
		ctx := context.Background()

		err := combo.DoWithRetrySimple(ctx, func() error {
			return nil
		})

		assert.NoError(t, err)
	})

	t.Run("failure", func(t *testing.T) {
		breaker := NewBreaker("test")
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(2)),
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)
		combo := NewBreakerRetryer(breaker, retryer)
		ctx := context.Background()

		var callCount int
		err := combo.DoWithRetrySimple(ctx, func() error {
			callCount++
			return errTest
		})

		assert.ErrorIs(t, err, errTest)
		assert.Equal(t, 2, callCount)
	})
}

func TestExecuteWithRetry(t *testing.T) {
	t.Run("success with value", func(t *testing.T) {
		breaker := NewBreaker("test")
		retryer := xretry.NewRetryer()
		combo := NewBreakerRetryer(breaker, retryer)
		ctx := context.Background()

		result, err := ExecuteWithRetry(ctx, combo, func() (string, error) {
			return "hello", nil
		})

		assert.NoError(t, err)
		assert.Equal(t, "hello", result)
	})

	t.Run("success after retry", func(t *testing.T) {
		breaker := NewBreaker("test")
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(3)),
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)
		combo := NewBreakerRetryer(breaker, retryer)
		ctx := context.Background()

		var callCount int32
		result, err := ExecuteWithRetry(ctx, combo, func() (int, error) {
			count := atomic.AddInt32(&callCount, 1)
			if count < 3 {
				return 0, errTest
			}
			return 42, nil
		})

		assert.NoError(t, err)
		assert.Equal(t, 42, result)
		assert.Equal(t, int32(3), callCount)
	})

	t.Run("breaker open", func(t *testing.T) {
		breaker := NewBreaker("test",
			WithTripPolicy(NewConsecutiveFailures(1)),
			WithTimeout(time.Hour),
		)
		retryer := xretry.NewRetryer()
		combo := NewBreakerRetryer(breaker, retryer)
		ctx := context.Background()

		// 触发熔断
		_, _ = ExecuteWithRetry(ctx, combo, func() (string, error) {
			return "", errTest
		})

		// 下一次调用应该直接失败
		result, err := ExecuteWithRetry(ctx, combo, func() (string, error) {
			return "hello", nil
		})

		assert.True(t, IsOpen(err))
		assert.Empty(t, result)
	})
}

func TestRetryThenBreak(t *testing.T) {
	t.Run("success on first try", func(t *testing.T) {
		retryer := xretry.NewRetryer()
		breaker := NewBreaker("test")
		rtb := NewRetryThenBreak(retryer, breaker)
		ctx := context.Background()

		var callCount int
		err := rtb.Do(ctx, func(_ context.Context) error {
			callCount++
			return nil
		})

		assert.NoError(t, err)
		assert.Equal(t, 1, callCount)
		// 成功被记录到 rtb 内部的熔断器
		counts := rtb.Counts()
		assert.Equal(t, uint32(1), counts.TotalSuccesses)
	})

	t.Run("success after retry", func(t *testing.T) {
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(3)),
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)
		breaker := NewBreaker("test")
		rtb := NewRetryThenBreak(retryer, breaker)
		ctx := context.Background()

		var callCount int
		err := rtb.Do(ctx, func(_ context.Context) error {
			callCount++
			if callCount < 3 {
				return errTest
			}
			return nil
		})

		assert.NoError(t, err)
		assert.Equal(t, 3, callCount)
		// 只有最终的成功被记录到 rtb 内部的熔断器
		counts := rtb.Counts()
		assert.Equal(t, uint32(1), counts.TotalSuccesses)
		assert.Equal(t, uint32(0), counts.TotalFailures)
	})

	t.Run("all retries fail", func(t *testing.T) {
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(3)),
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)
		breaker := NewBreaker("test",
			WithTripPolicy(NewConsecutiveFailures(2)),
		)
		rtb := NewRetryThenBreak(retryer, breaker)
		ctx := context.Background()

		// 第一次调用：3次重试都失败 → 记录1次失败
		err := rtb.Do(ctx, func(_ context.Context) error {
			return errTest
		})
		assert.ErrorIs(t, err, errTest)
		assert.Equal(t, StateClosed, rtb.State()) // 只有1次失败

		// 第二次调用：3次重试都失败 → 记录第2次失败 → 触发熔断
		err = rtb.Do(ctx, func(_ context.Context) error {
			return errTest
		})
		assert.ErrorIs(t, err, errTest)
		assert.Equal(t, StateOpen, rtb.State())
	})

	t.Run("getters", func(t *testing.T) {
		retryer := xretry.NewRetryer()
		breaker := NewBreaker("test")
		rtb := NewRetryThenBreak(retryer, breaker)

		assert.Equal(t, breaker, rtb.Breaker())
		assert.Equal(t, retryer, rtb.Retryer())
	})
}

func TestExecuteRetryThenBreak(t *testing.T) {
	t.Run("success with value", func(t *testing.T) {
		retryer := xretry.NewRetryer()
		breaker := NewBreaker("test")
		rtb := NewRetryThenBreak(retryer, breaker)
		ctx := context.Background()

		result, err := ExecuteRetryThenBreak(ctx, rtb, func() (string, error) {
			return "hello", nil
		})

		assert.NoError(t, err)
		assert.Equal(t, "hello", result)
	})

	t.Run("failure", func(t *testing.T) {
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(2)),
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)
		breaker := NewBreaker("test")
		rtb := NewRetryThenBreak(retryer, breaker)
		ctx := context.Background()

		result, err := ExecuteRetryThenBreak(ctx, rtb, func() (int, error) {
			return 0, errTest
		})

		assert.ErrorIs(t, err, errTest)
		assert.Equal(t, 0, result)
	})
}

func TestBreakerRetryer_Integration(t *testing.T) {
	t.Run("realistic scenario", func(t *testing.T) {
		// 模拟一个服务：前几次调用都失败，触发熔断
		breaker := NewBreaker("test-service",
			WithTripPolicy(NewConsecutiveFailures(3)),
			WithTimeout(50*time.Millisecond),
			WithMaxRequests(1),
		)
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(2)),
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)
		combo := NewBreakerRetryer(breaker, retryer)
		ctx := context.Background()

		// 3次调用都失败（每次重试2次也失败），触发熔断
		for i := 0; i < 3; i++ {
			_, _ = ExecuteWithRetry(ctx, combo, func() (string, error) {
				return "", errors.New("service unavailable")
			})
		}

		// 检查熔断器是否打开
		assert.Equal(t, StateOpen, breaker.State())

		// 等待超时，进入半开状态
		time.Sleep(60 * time.Millisecond)
		assert.Equal(t, StateHalfOpen, breaker.State())

		// 服务已恢复，成功的调用会关闭熔断器
		result, err := ExecuteWithRetry(ctx, combo, func() (string, error) {
			return "recovered", nil
		})

		require.NoError(t, err)
		assert.Equal(t, "recovered", result)
		assert.Equal(t, StateClosed, breaker.State())
	})
}

// === 修复验证测试 ===

// TestRetryThenBreak_WithSuccessPolicy 验证问题1的修复：
// RetryThenBreak 应该使用 SuccessPolicy 判断成功，而不是简单的 err == nil
func TestRetryThenBreak_WithSuccessPolicy(t *testing.T) {
	// 定义一个特殊错误，该错误应被视为"成功"
	errExpected := errors.New("expected error - should be treated as success")

	// 自定义成功判定策略：errExpected 被视为成功
	customPolicy := &testSuccessPolicy{
		successErrors: []error{errExpected},
	}

	t.Run("Do with SuccessPolicy", func(t *testing.T) {
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(1)), // 只尝试1次
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)
		breaker := NewBreaker("test",
			WithTripPolicy(NewConsecutiveFailures(2)), // 2次连续失败触发熔断
			WithSuccessPolicy(customPolicy),
		)
		rtb := NewRetryThenBreak(retryer, breaker)
		ctx := context.Background()

		// 返回 errExpected，应被视为成功（不增加失败计数）
		err := rtb.Do(ctx, func(_ context.Context) error {
			return errExpected
		})

		// 函数仍然返回原始错误
		assert.ErrorIs(t, err, errExpected)

		// 但熔断器应该记录为成功
		counts := rtb.Counts()
		assert.Equal(t, uint32(1), counts.TotalSuccesses, "should count as success")
		assert.Equal(t, uint32(0), counts.TotalFailures, "should not count as failure")

		// 再次调用，仍然返回 errExpected
		_ = rtb.Do(ctx, func(_ context.Context) error {
			return errExpected
		})

		// 熔断器应该保持 Closed 状态（因为没有失败）
		assert.Equal(t, StateClosed, rtb.State())
	})

	t.Run("ExecuteRetryThenBreak with SuccessPolicy", func(t *testing.T) {
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(1)),
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)
		breaker := NewBreaker("test",
			WithTripPolicy(NewConsecutiveFailures(2)),
			WithSuccessPolicy(customPolicy),
		)
		rtb := NewRetryThenBreak(retryer, breaker)
		ctx := context.Background()

		// 返回 errExpected，应被视为成功
		// 注意：retry-go 在返回错误时不保留结果值，这是其正常行为
		_, err := ExecuteRetryThenBreak(ctx, rtb, func() (string, error) {
			return "result", errExpected
		})

		assert.ErrorIs(t, err, errExpected)
		// 注：不断言 result，因为 retry-go 在有错误时会丢弃结果

		// 熔断器应该记录为成功（这是测试的核心验证点）
		counts := rtb.Counts()
		assert.Equal(t, uint32(1), counts.TotalSuccesses)
		assert.Equal(t, uint32(0), counts.TotalFailures)
	})
}

// testSuccessPolicy 用于测试的成功判定策略
type testSuccessPolicy struct {
	successErrors []error
}

func (p *testSuccessPolicy) IsSuccessful(err error) bool {
	if err == nil {
		return true
	}
	for _, e := range p.successErrors {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// TestBreakerError_NotRetryable 验证问题2的修复：
// 熔断器错误应该不可重试（Retryable() 返回 false）
func TestBreakerError_NotRetryable(t *testing.T) {
	t.Run("BreakerError implements Retryable", func(t *testing.T) {
		// 创建 BreakerError
		be := &BreakerError{
			Err:   ErrOpenState,
			Name:  "test",
			State: StateOpen,
		}

		// 验证 Retryable() 返回 false
		assert.False(t, be.Retryable(), "BreakerError should not be retryable")

		// 验证 xretry.IsRetryable 也返回 false
		assert.False(t, xretry.IsRetryable(be), "xretry should recognize BreakerError as non-retryable")
	})

	t.Run("breaker open error is not retryable", func(t *testing.T) {
		breaker := NewBreaker("test",
			WithTripPolicy(NewConsecutiveFailures(1)),
			WithTimeout(time.Hour),
		)
		ctx := context.Background()

		// 触发熔断
		_ = breaker.Do(ctx, func() error {
			return errTest
		})
		assert.Equal(t, StateOpen, breaker.State())

		// 下一次调用返回的错误应该是不可重试的
		err := breaker.Do(ctx, func() error {
			return nil
		})

		assert.True(t, IsOpen(err), "should be open state error")
		assert.False(t, xretry.IsRetryable(err), "breaker open error should not be retryable")

		// 验证是 BreakerError 类型
		var be *BreakerError
		assert.True(t, errors.As(err, &be), "should be BreakerError")
		assert.Equal(t, "test", be.Name)
		assert.Equal(t, StateOpen, be.State)
	})

	t.Run("BreakerRetryer stops retrying on breaker open", func(t *testing.T) {
		breaker := NewBreaker("test",
			WithTripPolicy(NewConsecutiveFailures(1)),
			WithTimeout(time.Hour),
		)
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(5)), // 允许5次重试
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)
		combo := NewBreakerRetryer(breaker, retryer)
		ctx := context.Background()

		// 触发熔断
		_ = combo.DoWithRetry(ctx, func(_ context.Context) error {
			return errTest
		})
		assert.Equal(t, StateOpen, breaker.State())

		// 记录开始时间
		start := time.Now()

		// 下一次调用应该立即失败，不进行重试
		var callCount int32
		err := combo.DoWithRetry(ctx, func(_ context.Context) error {
			atomic.AddInt32(&callCount, 1)
			return nil
		})

		elapsed := time.Since(start)

		// 验证：
		// 1. 函数没有被调用（熔断器阻断）
		assert.Equal(t, int32(0), callCount)
		// 2. 返回熔断错误
		assert.True(t, IsOpen(err))
		// 3. 没有进行重试等待（应该立即返回）
		assert.Less(t, elapsed, 100*time.Millisecond, "should return immediately without retry delays")
	})
}

// TestNewRetryThenBreakWithConfig 验证问题3的修复：
// 新增的构造函数应该正常工作
func TestNewRetryThenBreakWithConfig(t *testing.T) {
	t.Run("basic usage", func(t *testing.T) {
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(3)),
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)

		rtb := NewRetryThenBreakWithConfig("test-service", retryer,
			WithTripPolicy(NewConsecutiveFailures(2)),
			WithTimeout(30*time.Second),
		)

		ctx := context.Background()

		// 正常执行
		err := rtb.Do(ctx, func(_ context.Context) error {
			return nil
		})
		assert.NoError(t, err)

		// 验证配置正确应用
		assert.Equal(t, "test-service", rtb.Breaker().Name())
		counts := rtb.Counts()
		assert.Equal(t, uint32(1), counts.TotalSuccesses)
	})

	t.Run("trip policy works", func(t *testing.T) {
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(1)),
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)

		rtb := NewRetryThenBreakWithConfig("test", retryer,
			WithTripPolicy(NewConsecutiveFailures(2)),
		)
		ctx := context.Background()

		// 两次失败触发熔断
		_ = rtb.Do(ctx, func(_ context.Context) error { return errTest })
		assert.Equal(t, StateClosed, rtb.State())

		_ = rtb.Do(ctx, func(_ context.Context) error { return errTest })
		assert.Equal(t, StateOpen, rtb.State())
	})

	t.Run("state independent from passed breaker", func(t *testing.T) {
		// 创建一个已触发熔断的 breaker
		existingBreaker := NewBreaker("existing",
			WithTripPolicy(NewConsecutiveFailures(1)),
			WithTimeout(time.Hour),
		)
		ctx := context.Background()
		_ = existingBreaker.Do(ctx, func() error { return errTest })
		assert.Equal(t, StateOpen, existingBreaker.State())

		// 使用 NewRetryThenBreakWithConfig 创建新实例（不受 existingBreaker 影响）
		retryer := xretry.NewRetryer()
		rtb := NewRetryThenBreakWithConfig("new", retryer,
			WithTripPolicy(NewConsecutiveFailures(5)),
		)

		// 新实例应该从 Closed 状态开始
		assert.Equal(t, StateClosed, rtb.State())

		// 可以正常执行
		err := rtb.Do(ctx, func(_ context.Context) error { return nil })
		assert.NoError(t, err)
	})
}

// TestBreaker_IsSuccessful 测试 Breaker.IsSuccessful 方法
func TestBreaker_IsSuccessful(t *testing.T) {
	t.Run("without custom policy", func(t *testing.T) {
		breaker := NewBreaker("test")

		assert.True(t, breaker.IsSuccessful(nil))
		assert.False(t, breaker.IsSuccessful(errTest))
	})

	t.Run("with custom policy", func(t *testing.T) {
		customPolicy := &testSuccessPolicy{
			successErrors: []error{errTest},
		}
		breaker := NewBreaker("test", WithSuccessPolicy(customPolicy))

		assert.True(t, breaker.IsSuccessful(nil))
		assert.True(t, breaker.IsSuccessful(errTest)) // errTest 被视为成功
		assert.False(t, breaker.IsSuccessful(errors.New("other error")))
	})
}

// TestRetryThenBreak_PanicHandling 验证 panic 场景下熔断器计数正确
// 这个测试验证了修复：panic 应该被记为失败，而不是成功
func TestRetryThenBreak_PanicHandling(t *testing.T) {
	t.Run("Do records failure on panic", func(t *testing.T) {
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(1)), // 不重试
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)
		breaker := NewBreaker("test",
			WithTripPolicy(NewConsecutiveFailures(2)), // 2次连续失败触发熔断
		)
		rtb := NewRetryThenBreak(retryer, breaker)
		ctx := context.Background()

		// 第一次调用：panic
		func() {
			defer func() {
				r := recover()
				require.NotNil(t, r, "should panic")
				assert.Equal(t, "test panic", r)
			}()
			_ = rtb.Do(ctx, func(_ context.Context) error {
				panic("test panic")
			})
		}()

		// 验证：panic 被记为失败
		counts := rtb.Counts()
		assert.Equal(t, uint32(0), counts.TotalSuccesses, "panic should not be counted as success")
		assert.Equal(t, uint32(1), counts.TotalFailures, "panic should be counted as failure")
		assert.Equal(t, uint32(1), counts.ConsecutiveFailures, "consecutive failures should be 1")
		assert.Equal(t, StateClosed, rtb.State(), "should still be closed after 1 failure")

		// 第二次调用：再次 panic，应触发熔断
		func() {
			defer func() {
				r := recover()
				require.NotNil(t, r, "should panic")
			}()
			_ = rtb.Do(ctx, func(_ context.Context) error {
				panic("test panic 2")
			})
		}()

		// 验证：熔断器已打开
		// 注：进入 Open 状态后，gobreaker 会重置计数器，所以不检查 TotalFailures
		assert.Equal(t, StateOpen, rtb.State(), "should be open after 2 consecutive failures")
	})

	t.Run("ExecuteRetryThenBreak records failure on panic", func(t *testing.T) {
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(1)),
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)
		breaker := NewBreaker("test",
			WithTripPolicy(NewConsecutiveFailures(2)),
		)
		rtb := NewRetryThenBreak(retryer, breaker)
		ctx := context.Background()

		// 调用会 panic 的函数
		func() {
			defer func() {
				r := recover()
				require.NotNil(t, r, "should panic")
				assert.Equal(t, "generic panic", r)
			}()
			_, _ = ExecuteRetryThenBreak(ctx, rtb, func() (string, error) {
				panic("generic panic")
			})
		}()

		// 验证：panic 被记为失败
		counts := rtb.Counts()
		assert.Equal(t, uint32(0), counts.TotalSuccesses)
		assert.Equal(t, uint32(1), counts.TotalFailures)
	})

	t.Run("panic value is preserved", func(t *testing.T) {
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(1)),
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)
		breaker := NewBreaker("test")
		rtb := NewRetryThenBreak(retryer, breaker)
		ctx := context.Background()

		// 测试不同类型的 panic 值
		testCases := []struct {
			name       string
			panicValue any
		}{
			{"string panic", "string error"},
			{"error panic", errors.New("error value")},
			{"int panic", 42},
			{"struct panic", struct{ msg string }{"struct error"}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				func() {
					defer func() {
						r := recover()
						require.NotNil(t, r)
						assert.Equal(t, tc.panicValue, r, "panic value should be preserved")
					}()
					_ = rtb.Do(ctx, func(_ context.Context) error {
						panic(tc.panicValue)
					})
				}()
			})
		}
	})

	t.Run("normal error still works after panic fix", func(t *testing.T) {
		// 确保修复没有破坏正常的错误处理
		retryer := xretry.NewRetryer(
			xretry.WithRetryPolicy(xretry.NewFixedRetry(1)),
			xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
		)
		breaker := NewBreaker("test",
			WithTripPolicy(NewConsecutiveFailures(2)),
		)
		rtb := NewRetryThenBreak(retryer, breaker)
		ctx := context.Background()

		// 正常错误
		err := rtb.Do(ctx, func(_ context.Context) error {
			return errTest
		})
		assert.ErrorIs(t, err, errTest)
		counts := rtb.Counts()
		assert.Equal(t, uint32(1), counts.TotalFailures)

		// 成功
		err = rtb.Do(ctx, func(_ context.Context) error {
			return nil
		})
		assert.NoError(t, err)
		counts = rtb.Counts()
		assert.Equal(t, uint32(1), counts.TotalSuccesses)
	})
}
