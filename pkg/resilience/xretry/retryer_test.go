package xretry

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryer_Do(t *testing.T) {
	t.Run("SuccessOnFirstAttempt", func(t *testing.T) {
		r := NewRetryer()
		var attempts int

		err := r.Do(context.Background(), func(ctx context.Context) error {
			attempts++
			return nil
		})

		assert.NoError(t, err)
		assert.Equal(t, 1, attempts)
	})

	t.Run("SuccessAfterRetry", func(t *testing.T) {
		r := NewRetryer(
			WithRetryPolicy(NewFixedRetry(3)),
			WithBackoffPolicy(NewNoBackoff()),
		)
		var attempts int

		err := r.Do(context.Background(), func(ctx context.Context) error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary error")
			}
			return nil
		})

		assert.NoError(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("FailAfterMaxAttempts", func(t *testing.T) {
		r := NewRetryer(
			WithRetryPolicy(NewFixedRetry(3)),
			WithBackoffPolicy(NewNoBackoff()),
		)
		var attempts int

		err := r.Do(context.Background(), func(ctx context.Context) error {
			attempts++
			return errors.New("persistent error")
		})

		assert.Error(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("PermanentErrorNoRetry", func(t *testing.T) {
		r := NewRetryer(
			WithRetryPolicy(NewFixedRetry(5)),
			WithBackoffPolicy(NewNoBackoff()),
		)
		var attempts int

		err := r.Do(context.Background(), func(ctx context.Context) error {
			attempts++
			return NewPermanentError(errors.New("permanent"))
		})

		assert.Error(t, err)
		assert.Equal(t, 1, attempts) // 只执行一次
	})

	t.Run("UnrecoverableErrorNoRetry", func(t *testing.T) {
		// 测试 retry-go 的 Unrecoverable 错误也能正确停止重试
		r := NewRetryer(
			WithRetryPolicy(NewFixedRetry(5)),
			WithBackoffPolicy(NewNoBackoff()),
		)
		var attempts int

		err := r.Do(context.Background(), func(ctx context.Context) error {
			attempts++
			return Unrecoverable(errors.New("unrecoverable"))
		})

		assert.Error(t, err)
		assert.Equal(t, 1, attempts) // 只执行一次，Unrecoverable 应该立即停止重试
	})

	t.Run("ContextCanceled", func(t *testing.T) {
		r := NewRetryer(
			WithRetryPolicy(NewAlwaysRetry()),
			WithBackoffPolicy(NewFixedBackoff(100*time.Millisecond)),
		)
		var attempts int32

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := r.Do(ctx, func(ctx context.Context) error {
			atomic.AddInt32(&attempts, 1)
			return errors.New("error")
		})

		assert.Error(t, err)
		assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || err.Error() == "error")
	})

	t.Run("OnRetryCallback", func(t *testing.T) {
		var callbacks []int
		r := NewRetryer(
			WithRetryPolicy(NewFixedRetry(3)),
			WithBackoffPolicy(NewNoBackoff()),
			WithOnRetry(func(attempt int, err error) {
				callbacks = append(callbacks, attempt)
			}),
		)
		var attempts int

		_ = r.Do(context.Background(), func(ctx context.Context) error { //nolint:errcheck // 测试回调执行，忽略返回值
			attempts++
			if attempts < 3 {
				return errors.New("error")
			}
			return nil
		})

		assert.Equal(t, []int{1, 2}, callbacks)
	})

	t.Run("BackoffDelay", func(t *testing.T) {
		r := NewRetryer(
			WithRetryPolicy(NewFixedRetry(3)),
			WithBackoffPolicy(NewFixedBackoff(50*time.Millisecond)),
		)
		var attempts int

		start := time.Now()
		_ = r.Do(context.Background(), func(ctx context.Context) error { //nolint:errcheck // 测试退避延迟，忽略返回值
			attempts++
			if attempts < 3 {
				return errors.New("error")
			}
			return nil
		})
		elapsed := time.Since(start)

		// 应该有 2 次退避等待，每次 50ms
		assert.GreaterOrEqual(t, elapsed, 90*time.Millisecond)
		assert.LessOrEqual(t, elapsed, 200*time.Millisecond)
	})
}

func TestDoWithResult(t *testing.T) {
	t.Run("SuccessOnFirstAttempt", func(t *testing.T) {
		r := NewRetryer()

		result, err := DoWithResult(context.Background(), r, func(ctx context.Context) (int, error) {
			return 42, nil
		})

		assert.NoError(t, err)
		assert.Equal(t, 42, result)
	})

	t.Run("SuccessAfterRetry", func(t *testing.T) {
		r := NewRetryer(
			WithRetryPolicy(NewFixedRetry(3)),
			WithBackoffPolicy(NewNoBackoff()),
		)
		var attempts int

		result, err := DoWithResult(context.Background(), r, func(ctx context.Context) (string, error) {
			attempts++
			if attempts < 2 {
				return "", errors.New("error")
			}
			return "success", nil
		})

		assert.NoError(t, err)
		assert.Equal(t, "success", result)
	})

	t.Run("FailAfterMaxAttempts", func(t *testing.T) {
		r := NewRetryer(
			WithRetryPolicy(NewFixedRetry(3)),
			WithBackoffPolicy(NewNoBackoff()),
		)

		result, err := DoWithResult(context.Background(), r, func(ctx context.Context) (int, error) {
			return 0, errors.New("error")
		})

		assert.Error(t, err)
		assert.Equal(t, 0, result)
	})
}

func TestRetryer_Accessors(t *testing.T) {
	retryPolicy := NewFixedRetry(5)
	backoffPolicy := NewFixedBackoff(100 * time.Millisecond)

	r := NewRetryer(
		WithRetryPolicy(retryPolicy),
		WithBackoffPolicy(backoffPolicy),
	)

	assert.Equal(t, retryPolicy, r.RetryPolicy())
	assert.Equal(t, backoffPolicy, r.BackoffPolicy())
}

func TestNewRetryer_NilOptions(t *testing.T) {
	// 测试 nil 选项不会覆盖默认值
	r := NewRetryer(
		WithRetryPolicy(nil),
		WithBackoffPolicy(nil),
	)

	assert.NotNil(t, r.RetryPolicy())
	assert.NotNil(t, r.BackoffPolicy())
}

func TestZeroValueRetryer(t *testing.T) {
	// 零值 Retryer 使用时不应该 panic
	t.Run("zero value Retryer should not panic", func(t *testing.T) {
		var r Retryer // 零值，retryPolicy 和 backoffPolicy 都是 nil
		var attempts int

		// 这里不应该 panic
		err := r.Do(context.Background(), func(ctx context.Context) error {
			attempts++
			if attempts < 2 {
				return errors.New("temporary error")
			}
			return nil
		})

		assert.NoError(t, err)
		assert.Equal(t, 2, attempts)
	})

	t.Run("zero value Retryer DoWithResult should not panic", func(t *testing.T) {
		var r Retryer // 零值
		var attempts int

		result, err := DoWithResult(context.Background(), &r, func(ctx context.Context) (string, error) {
			attempts++
			if attempts < 2 {
				return "", errors.New("temporary error")
			}
			return "success", nil
		})

		assert.NoError(t, err)
		assert.Equal(t, "success", result)
	})
}

// TestBackoffDelayCorrectness 验证 BackoffPolicy.NextDelay 参数正确传递
// 此测试防止 off-by-one 回归（历史问题：retry-go DelayType 的 n 参数从 1 开始，非 0）
func TestBackoffDelayCorrectness(t *testing.T) {
	var delayAttempts []int

	// 创建追踪退避策略
	trackingBackoff := &testTrackingBackoff{
		inner:    NewExponentialBackoff(WithInitialDelay(100*time.Millisecond), WithJitter(0)),
		attempts: &delayAttempts,
	}

	r := NewRetryer(
		WithRetryPolicy(NewFixedRetry(4)),
		WithBackoffPolicy(trackingBackoff),
	)

	var attempts int
	_ = r.Do(context.Background(), func(_ context.Context) error { //nolint:errcheck // 测试延迟参数
		attempts++
		if attempts < 4 {
			return errors.New("fail")
		}
		return nil
	})

	// 验证 NextDelay 被调用时传入的 attempt 参数正确
	// 应该是 [1, 2, 3]（3 次重试，attempt 从 1 开始）
	assert.Equal(t, []int{1, 2, 3}, delayAttempts, "NextDelay should receive 1-based attempt numbers")
}

type testTrackingBackoff struct {
	inner    BackoffPolicy
	attempts *[]int
}

func (t *testTrackingBackoff) NextDelay(attempt int) time.Duration {
	*t.attempts = append(*t.attempts, attempt)
	return t.inner.NextDelay(attempt)
}

// TestRetryer_NegativeMaxAttempts 验证自定义 RetryPolicy 返回负数时的行为
// 此测试防止 uint 溢出导致的无限重试问题
func TestRetryer_NegativeMaxAttempts(t *testing.T) {
	// 自定义策略返回负数
	policy := &negativeMaxAttemptsPolicy{}
	retryer := NewRetryer(
		WithRetryPolicy(policy),
		WithBackoffPolicy(NewNoBackoff()),
	)

	var count int
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 应该作为无限重试处理（直到超时）
	_ = retryer.Do(ctx, func(_ context.Context) error { //nolint:errcheck // 测试负值保护，预期超时退出
		count++
		return errors.New("always fail")
	})

	// 验证确实执行了多次而非 uint 溢出后的巨大次数
	// 如果是 uint(-1) = 18446744073709551615 次，测试会超时或卡死
	// 注意：由于使用 NoBackoff（无延迟），100ms 内在快速机器上可执行数十万次迭代
	// 这里验证的关键是：执行次数远小于 uint 溢出值（~1.8e19）
	assert.Greater(t, count, 1, "should retry multiple times")
	assert.Less(t, count, 1000000, "should not overflow to huge uint value")
}

// negativeMaxAttemptsPolicy 是返回负数 MaxAttempts 的测试策略
type negativeMaxAttemptsPolicy struct{}

func (p *negativeMaxAttemptsPolicy) MaxAttempts() int {
	return -1
}

func (p *negativeMaxAttemptsPolicy) ShouldRetry(_ context.Context, _ int, _ error) bool {
	return true
}
