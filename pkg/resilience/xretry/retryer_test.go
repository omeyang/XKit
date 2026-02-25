package xretry

import (
	"context"
	"errors"
	"math"
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

	t.Run("InternalContextCanceledNoRetry", func(t *testing.T) {
		// 函数返回内部 context 的取消错误时，不应重试
		r := NewRetryer(
			WithRetryPolicy(NewFixedRetry(5)),
			WithBackoffPolicy(NewNoBackoff()),
		)
		var attempts int

		err := r.Do(context.Background(), func(_ context.Context) error {
			attempts++
			return context.Canceled
		})

		assert.Error(t, err)
		assert.Equal(t, 1, attempts)
	})

	t.Run("InternalDeadlineExceededNoRetry", func(t *testing.T) {
		r := NewRetryer(
			WithRetryPolicy(NewFixedRetry(5)),
			WithBackoffPolicy(NewNoBackoff()),
		)
		var attempts int

		err := r.Do(context.Background(), func(_ context.Context) error {
			attempts++
			return context.DeadlineExceeded
		})

		assert.Error(t, err)
		assert.Equal(t, 1, attempts)
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

		err := r.Do(context.Background(), func(ctx context.Context) error {
			attempts++
			if attempts < 3 {
				return errors.New("error")
			}
			return nil
		})

		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2}, callbacks)
	})

	t.Run("BackoffDelay", func(t *testing.T) {
		r := NewRetryer(
			WithRetryPolicy(NewFixedRetry(3)),
			WithBackoffPolicy(NewFixedBackoff(50*time.Millisecond)),
		)
		var attempts int

		start := time.Now()
		err := r.Do(context.Background(), func(ctx context.Context) error {
			attempts++
			if attempts < 3 {
				return errors.New("error")
			}
			return nil
		})
		elapsed := time.Since(start)

		assert.NoError(t, err)
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

func TestWithOnRetry_Nil(t *testing.T) {
	// WithOnRetry(nil) 应该被静默忽略，不清除已设置的回调
	var called bool
	r := NewRetryer(
		WithRetryPolicy(NewFixedRetry(2)),
		WithBackoffPolicy(NewNoBackoff()),
		WithOnRetry(func(_ int, _ error) { called = true }),
		WithOnRetry(nil), // 不应清除上面的回调
	)

	var attempts int
	err := r.Do(context.Background(), func(_ context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("error")
		}
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called, "OnRetry callback should not be cleared by WithOnRetry(nil)")
}

// TestRetryer_SequentialReuse 验证 Retryer.Do 可安全复用
// 每次 Do 调用创建独立的 attemptCount 闭包，因此连续调用应互不影响。
func TestRetryer_SequentialReuse(t *testing.T) {
	r := NewRetryer(
		WithRetryPolicy(NewFixedRetry(3)),
		WithBackoffPolicy(NewNoBackoff()),
	)

	// 第一次调用：执行 3 次后成功
	var attempts1 int
	err := r.Do(context.Background(), func(_ context.Context) error {
		attempts1++
		if attempts1 < 3 {
			return errors.New("fail")
		}
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 3, attempts1)

	// 第二次调用：重试次数不应受第一次影响
	var attempts2 int
	err = r.Do(context.Background(), func(_ context.Context) error {
		attempts2++
		if attempts2 < 3 {
			return errors.New("fail")
		}
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 3, attempts2, "second call should have independent retry count")
}

// TestRetryer_ConcurrentUse 验证 Retryer.Do 并发安全（应通过 -race 检测）
func TestRetryer_ConcurrentUse(t *testing.T) {
	r := NewRetryer(
		WithRetryPolicy(NewFixedRetry(3)),
		WithBackoffPolicy(NewNoBackoff()),
	)

	const goroutines = 10
	errs := make(chan error, goroutines)

	for range goroutines {
		go func() {
			var attempts int32
			errs <- r.Do(context.Background(), func(_ context.Context) error {
				n := atomic.AddInt32(&attempts, 1)
				if n < 2 {
					return errors.New("fail")
				}
				return nil
			})
		}()
	}

	for range goroutines {
		assert.NoError(t, <-errs)
	}
}

// TestRetrier_ReuseAccumulatesCount 记录 Retrier() 返回实例的一次性使用特性。
// 设计决策: Retrier() 返回的实例内部 RetryIf 闭包维护 attemptCount 状态，
// 复用同一实例会导致计数累积。此测试明确记录该行为，防止回归。
func TestRetrier_ReuseAccumulatesCount(t *testing.T) {
	r := NewRetryer(
		WithRetryPolicy(NewFixedRetry(5)),
		WithBackoffPolicy(NewNoBackoff()),
	)

	retrier := r.Retrier(context.Background())

	// 第一次调用：执行 2 次后成功
	var attempts1 int
	err := retrier.Do(func() error {
		attempts1++
		if attempts1 < 2 {
			return errors.New("fail")
		}
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, attempts1)

	// 第二次调用同一实例：计数累积导致 ShouldRetry 提前拒绝
	// 这是已知且已记录的行为（一次性使用实例）
	var attempts2 int
	err = retrier.Do(func() error {
		attempts2++
		return errors.New("always fail")
	})
	assert.Error(t, err)
	// 由于 attemptCount 从上次终值继续累积，实际可用重试次数少于 MaxAttempts
	assert.Less(t, attempts2, 5, "reused retrier should have fewer retries due to accumulated count")
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

func TestSafeIntToUint(t *testing.T) {
	assert.Equal(t, uint(0), safeIntToUint(0))
	assert.Equal(t, uint(0), safeIntToUint(-1))
	assert.Equal(t, uint(0), safeIntToUint(-100))
	assert.Equal(t, uint(1), safeIntToUint(1))
	assert.Equal(t, uint(100), safeIntToUint(100))
}

func TestSafeUintToInt(t *testing.T) {
	assert.Equal(t, 0, safeUintToInt(0))
	assert.Equal(t, 1, safeUintToInt(1))
	assert.Equal(t, 100, safeUintToInt(100))
	assert.Equal(t, math.MaxInt, safeUintToInt(math.MaxUint))
	assert.Equal(t, math.MaxInt, safeUintToInt(uint(math.MaxInt)+1))
}

// TestBackoffDelayCorrectness 验证 BackoffPolicy.NextDelay 参数正确传递
// 此测试验证退避延迟参数正确传递，防止 off-by-one 回归
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
	err := r.Do(context.Background(), func(_ context.Context) error {
		attempts++
		if attempts < 4 {
			return errors.New("fail")
		}
		return nil
	})
	assert.NoError(t, err)

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
	err := retryer.Do(ctx, func(_ context.Context) error {
		count++
		return errors.New("always fail")
	})
	assert.Error(t, err)

	// 验证确实执行了多次而非 uint 溢出后的巨大次数
	// 如果是 uint(-1) = 18446744073709551615 次，测试会超时或卡死
	// 注意：由于使用 NoBackoff（无延迟），100ms 内在快速机器上可执行数十万次迭代
	// 这里验证的关键是：执行次数远小于 uint 溢出值（~1.8e19）
	assert.Greater(t, count, 1, "should retry multiple times")
	assert.Less(t, count, 1000000, "should not overflow to huge uint value")
}

// TestRetryer_CustomShouldRetry 验证自定义 ShouldRetry 在 Retryer 路径下真正生效
func TestRetryer_CustomShouldRetry(t *testing.T) {
	t.Run("ShouldRetryReceivesCorrectAttempt", func(t *testing.T) {
		// 自定义策略：记录 ShouldRetry 收到的 attempt 值
		var shouldRetryAttempts []int
		policy := &trackingShouldRetryPolicy{
			maxAttempts: 5,
			attempts:    &shouldRetryAttempts,
		}

		r := NewRetryer(
			WithRetryPolicy(policy),
			WithBackoffPolicy(NewNoBackoff()),
		)

		var execAttempts int
		err := r.Do(context.Background(), func(_ context.Context) error {
			execAttempts++
			if execAttempts < 3 {
				return errors.New("fail")
			}
			return nil
		})

		assert.NoError(t, err)
		assert.Equal(t, 3, execAttempts)
		// ShouldRetry 应该被调用 2 次（第 1 次和第 2 次失败后）
		assert.Equal(t, []int{1, 2}, shouldRetryAttempts)
	})

	t.Run("ShouldRetryCanStopEarly", func(t *testing.T) {
		// 自定义策略：只允许重试特定错误
		policy := &customFilterPolicy{
			maxAttempts:    10,
			retryableError: "retryable",
		}

		r := NewRetryer(
			WithRetryPolicy(policy),
			WithBackoffPolicy(NewNoBackoff()),
		)

		var attempts int
		err := r.Do(context.Background(), func(_ context.Context) error {
			attempts++
			if attempts == 1 {
				return errors.New("retryable")
			}
			return errors.New("fatal")
		})

		assert.Error(t, err)
		assert.Equal(t, 2, attempts) // 第 2 次返回 "fatal"，ShouldRetry 拒绝重试
	})
}

// trackingShouldRetryPolicy 记录 ShouldRetry 调用的策略
type trackingShouldRetryPolicy struct {
	maxAttempts int
	attempts    *[]int
}

func (p *trackingShouldRetryPolicy) MaxAttempts() int { return p.maxAttempts }

func (p *trackingShouldRetryPolicy) ShouldRetry(_ context.Context, attempt int, err error) bool {
	*p.attempts = append(*p.attempts, attempt)
	if attempt >= p.maxAttempts {
		return false
	}
	return IsRetryable(err)
}

// customFilterPolicy 只重试特定错误消息的策略
type customFilterPolicy struct {
	maxAttempts    int
	retryableError string
}

func (p *customFilterPolicy) MaxAttempts() int { return p.maxAttempts }

func (p *customFilterPolicy) ShouldRetry(_ context.Context, attempt int, err error) bool {
	if attempt >= p.maxAttempts {
		return false
	}
	return err != nil && err.Error() == p.retryableError
}

// negativeMaxAttemptsPolicy 是返回负数 MaxAttempts 的测试策略
type negativeMaxAttemptsPolicy struct{}

func (p *negativeMaxAttemptsPolicy) MaxAttempts() int {
	return -1
}

func (p *negativeMaxAttemptsPolicy) ShouldRetry(_ context.Context, _ int, _ error) bool {
	return true
}

func TestNilRetryer(t *testing.T) {
	t.Run("Do", func(t *testing.T) {
		var r *Retryer // nil
		err := r.Do(context.Background(), func(_ context.Context) error {
			t.Fatal("should not be called")
			return nil
		})
		assert.ErrorIs(t, err, ErrNilRetryer)
	})

	t.Run("DoWithResult", func(t *testing.T) {
		result, err := DoWithResult(context.Background(), nil, func(_ context.Context) (int, error) {
			t.Fatal("should not be called")
			return 0, nil
		})
		assert.ErrorIs(t, err, ErrNilRetryer)
		assert.Equal(t, 0, result)
	})

	t.Run("Retrier", func(t *testing.T) {
		var r *Retryer // nil
		retrier := r.Retrier(context.Background())
		assert.NotNil(t, retrier, "nil Retryer should return usable default Retrier")
	})

	t.Run("RetrierWithData", func(t *testing.T) {
		retrier := RetrierWithData[string](context.Background(), nil)
		assert.NotNil(t, retrier, "nil Retryer should return usable default RetrierWithData")
	})
}

func TestRetryer_NilFn(t *testing.T) {
	r := NewRetryer(WithBackoffPolicy(NewNoBackoff()))

	t.Run("Do", func(t *testing.T) {
		err := r.Do(context.Background(), nil)
		assert.ErrorIs(t, err, ErrNilFunc)
	})

	t.Run("DoWithResult", func(t *testing.T) {
		result, err := DoWithResult[int](context.Background(), r, nil)
		assert.ErrorIs(t, err, ErrNilFunc)
		assert.Equal(t, 0, result)
	})
}

func TestNilContext(t *testing.T) {
	r := NewRetryer(WithBackoffPolicy(NewNoBackoff()))

	t.Run("Retryer_Do", func(t *testing.T) {
		var ctx context.Context //nolint:wastedassign // 显式 nil context 用于测试
		err := r.Do(ctx, func(_ context.Context) error {
			t.Fatal("should not be called")
			return nil
		})
		assert.ErrorIs(t, err, ErrNilContext)
	})

	t.Run("DoWithResult", func(t *testing.T) {
		var ctx context.Context //nolint:wastedassign // 显式 nil context 用于测试
		result, err := DoWithResult(ctx, r, func(_ context.Context) (int, error) {
			t.Fatal("should not be called")
			return 0, nil
		})
		assert.ErrorIs(t, err, ErrNilContext)
		assert.Equal(t, 0, result)
	})

	t.Run("Retrier_NilCtx", func(t *testing.T) {
		var ctx context.Context //nolint:wastedassign // 显式 nil context 用于测试
		retrier := r.Retrier(ctx)
		assert.NotNil(t, retrier, "nil ctx should return usable Retrier with context.Background()")
	})

	t.Run("RetrierWithData_NilCtx", func(t *testing.T) {
		var ctx context.Context //nolint:wastedassign // 显式 nil context 用于测试
		retrier := RetrierWithData[string](ctx, r)
		assert.NotNil(t, retrier, "nil ctx should return usable RetrierWithData with context.Background()")
	})
}

func TestNilRetryer_Accessors(t *testing.T) {
	var r *Retryer // nil

	t.Run("RetryPolicy", func(t *testing.T) {
		assert.Nil(t, r.RetryPolicy(), "nil Retryer should return nil RetryPolicy")
	})

	t.Run("BackoffPolicy", func(t *testing.T) {
		assert.Nil(t, r.BackoffPolicy(), "nil Retryer should return nil BackoffPolicy")
	})
}
