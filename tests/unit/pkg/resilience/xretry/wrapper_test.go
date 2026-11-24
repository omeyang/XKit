package xretry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDo(t *testing.T) {
	t.Run("SuccessOnFirstAttempt", func(t *testing.T) {
		var attempts int
		err := Do(context.Background(), func() error {
			attempts++
			return nil
		}, Attempts(3))

		assert.NoError(t, err)
		assert.Equal(t, 1, attempts)
	})

	t.Run("SuccessAfterRetry", func(t *testing.T) {
		var attempts int
		err := Do(context.Background(), func() error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary error")
			}
			return nil
		}, Attempts(5), Delay(time.Millisecond))

		assert.NoError(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("FailAfterMaxAttempts", func(t *testing.T) {
		var attempts int
		err := Do(context.Background(), func() error {
			attempts++
			return errors.New("persistent error")
		}, Attempts(3), Delay(time.Millisecond))

		assert.Error(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("PermanentErrorNoRetry", func(t *testing.T) {
		var attempts int
		err := Do(context.Background(), func() error {
			attempts++
			return NewPermanentError(errors.New("permanent"))
		}, Attempts(5))

		assert.Error(t, err)
		assert.Equal(t, 1, attempts)
	})

	t.Run("ContextCanceled", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := Do(ctx, func() error {
			return errors.New("error")
		}, UntilSucceeded(), Delay(100*time.Millisecond))

		assert.Error(t, err)
	})

	t.Run("InternalContextCanceledNoRetry", func(t *testing.T) {
		// wrapper 路径：函数返回内部 context 取消错误时，不应重试
		var attempts int
		err := Do(context.Background(), func() error {
			attempts++
			return context.Canceled
		}, Attempts(5), Delay(time.Millisecond))

		assert.Error(t, err)
		assert.Equal(t, 1, attempts)
	})

	t.Run("InternalDeadlineExceededNoRetry", func(t *testing.T) {
		var attempts int
		err := Do(context.Background(), func() error {
			attempts++
			return context.DeadlineExceeded
		}, Attempts(5), Delay(time.Millisecond))

		assert.Error(t, err)
		assert.Equal(t, 1, attempts)
	})
}

func TestDoWithData(t *testing.T) {
	t.Run("SuccessOnFirstAttempt", func(t *testing.T) {
		result, err := DoWithData(context.Background(), func() (int, error) {
			return 42, nil
		}, Attempts(3))

		assert.NoError(t, err)
		assert.Equal(t, 42, result)
	})

	t.Run("SuccessAfterRetry", func(t *testing.T) {
		var attempts int
		result, err := DoWithData(context.Background(), func() (string, error) {
			attempts++
			if attempts < 2 {
				return "", errors.New("error")
			}
			return "success", nil
		}, Attempts(3), Delay(time.Millisecond))

		assert.NoError(t, err)
		assert.Equal(t, "success", result)
	})

	t.Run("FailAfterMaxAttempts", func(t *testing.T) {
		result, err := DoWithData(context.Background(), func() (int, error) {
			return 0, errors.New("error")
		}, Attempts(3), Delay(time.Millisecond))

		assert.Error(t, err)
		assert.Equal(t, 0, result)
	})

	t.Run("UnrecoverableNoRetry", func(t *testing.T) {
		var attempts int
		result, err := DoWithData(context.Background(), func() (int, error) {
			attempts++
			return 0, Unrecoverable(errors.New("unrecoverable"))
		}, Attempts(5))

		assert.Error(t, err)
		assert.Equal(t, 0, result)
		assert.Equal(t, 1, attempts)
	})

	t.Run("PermanentErrorNoRetry", func(t *testing.T) {
		var attempts int
		result, err := DoWithData(context.Background(), func() (string, error) {
			attempts++
			return "", NewPermanentError(errors.New("permanent"))
		}, Attempts(5))

		assert.Error(t, err)
		assert.Equal(t, "", result)
		assert.Equal(t, 1, attempts)
	})
}

func TestNewRetrier(t *testing.T) {
	t.Run("BasicUsage", func(t *testing.T) {
		retrier := NewRetrier(
			Attempts(3),
			Delay(time.Millisecond),
		)

		var attempts int
		err := retrier.Do(func() error {
			attempts++
			if attempts < 2 {
				return errors.New("error")
			}
			return nil
		})

		assert.NoError(t, err)
		assert.Equal(t, 2, attempts)
	})

	t.Run("WithOnRetry", func(t *testing.T) {
		var callbacks []uint
		retrier := NewRetrier(
			Attempts(3),
			Delay(time.Millisecond),
			OnRetry(func(n uint, err error) {
				callbacks = append(callbacks, n)
			}),
		)

		var attempts int
		err := retrier.Do(func() error {
			attempts++
			if attempts < 3 {
				return errors.New("error")
			}
			return nil
		})

		assert.NoError(t, err)
		// retry-go 的 OnRetry 从 0 开始计数
		assert.Equal(t, []uint{0, 1}, callbacks)
	})
}

func TestNewRetrierWithData(t *testing.T) {
	t.Run("BasicUsage", func(t *testing.T) {
		retrier := NewRetrierWithData[string](
			Attempts(3),
			Delay(time.Millisecond),
		)

		var attempts int
		result, err := retrier.Do(func() (string, error) {
			attempts++
			if attempts < 2 {
				return "", errors.New("error")
			}
			return "success", nil
		})

		assert.NoError(t, err)
		assert.Equal(t, "success", result)
	})
}

func TestToDelayType(t *testing.T) {
	t.Run("FixedBackoff", func(t *testing.T) {
		backoff := NewFixedBackoff(100 * time.Millisecond)
		delayFunc := ToDelayType(backoff)

		// 固定退避每次返回相同值
		assert.Equal(t, 100*time.Millisecond, delayFunc(0, nil, nil))
		assert.Equal(t, 100*time.Millisecond, delayFunc(1, nil, nil))
		assert.Equal(t, 100*time.Millisecond, delayFunc(2, nil, nil))
	})

	t.Run("IntegrationWithRetrier", func(t *testing.T) {
		backoff := NewFixedBackoff(10 * time.Millisecond)
		retrier := NewRetrier(
			Attempts(3),
			DelayType(ToDelayType(backoff)),
		)

		var attempts int
		start := time.Now()
		err := retrier.Do(func() error {
			attempts++
			if attempts < 3 {
				return errors.New("error")
			}
			return nil
		})
		elapsed := time.Since(start)

		assert.NoError(t, err)
		assert.Equal(t, 3, attempts)
		// 应该有约 20ms 的延迟（2 次退避 × 10ms）
		assert.GreaterOrEqual(t, elapsed, 15*time.Millisecond)
	})
}

func TestToDelayType_NilPolicy(t *testing.T) {
	delayFunc := ToDelayType(nil)
	assert.Equal(t, time.Duration(0), delayFunc(1, nil, nil))
	assert.Equal(t, time.Duration(0), delayFunc(5, nil, nil))
}

func TestUnrecoverableCompat(t *testing.T) {
	t.Run("RetryGoUnrecoverable", func(t *testing.T) {
		// 使用 retry-go 原生的 Unrecoverable
		var attempts int
		err := Do(context.Background(), func() error {
			attempts++
			return Unrecoverable(errors.New("unrecoverable"))
		}, Attempts(5))

		assert.Error(t, err)
		assert.Equal(t, 1, attempts)
	})

	t.Run("XretryPermanentError", func(t *testing.T) {
		// 使用 xretry 的 PermanentError
		var attempts int
		err := Do(context.Background(), func() error {
			attempts++
			return NewPermanentError(errors.New("permanent"))
		}, Attempts(5))

		assert.Error(t, err)
		assert.Equal(t, 1, attempts)
	})
}

func TestRetrier_Retrier(t *testing.T) {
	r := NewRetryer(
		WithRetryPolicy(NewFixedRetry(3)),
		WithBackoffPolicy(NewNoBackoff()),
	)

	// 获取底层 retry.Retrier
	retrier := r.Retrier(context.Background())
	assert.NotNil(t, retrier)

	var attempts int
	err := retrier.Do(func() error {
		attempts++
		if attempts < 2 {
			return errors.New("error")
		}
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestRetrierWithData_Function(t *testing.T) {
	r := NewRetryer(
		WithRetryPolicy(NewFixedRetry(3)),
		WithBackoffPolicy(NewNoBackoff()),
	)

	// 使用 RetrierWithData 函数获取底层 retry.RetrierWithData
	retrier := RetrierWithData[string](context.Background(), r)
	assert.NotNil(t, retrier)

	var attempts int
	result, err := retrier.Do(func() (string, error) {
		attempts++
		if attempts < 2 {
			return "", errors.New("error")
		}
		return "success", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", result)
}

func TestDo_NilFn(t *testing.T) {
	err := Do(context.Background(), nil, Attempts(1))
	assert.ErrorIs(t, err, ErrNilFunc)
}

func TestDoWithData_NilFn(t *testing.T) {
	result, err := DoWithData[int](context.Background(), nil, Attempts(1))
	assert.ErrorIs(t, err, ErrNilFunc)
	assert.Equal(t, 0, result)
}

func TestDo_NilContext(t *testing.T) {
	var ctx context.Context //nolint:wastedassign // 显式 nil context 用于测试
	err := Do(ctx, func() error {
		t.Fatal("should not be called")
		return nil
	}, Attempts(1))
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestDoWithData_NilContext(t *testing.T) {
	var ctx context.Context //nolint:wastedassign // 显式 nil context 用于测试
	result, err := DoWithData[int](ctx, nil, Attempts(1))
	assert.ErrorIs(t, err, ErrNilContext)
	assert.Equal(t, 0, result)
}

// TestContextPriority 验证函数参数 ctx 始终优先于 opts 中的 Context()
func TestContextPriority(t *testing.T) {
	shortCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	// 即使 opts 中传了 context.Background()（无超时），
	// 函数参数 ctx 的 20ms 超时仍应生效
	err := Do(shortCtx, func() error {
		return errors.New("always fail")
	}, UntilSucceeded(), Delay(10*time.Millisecond), Context(context.Background()))

	assert.Error(t, err)
	// 如果 ctx 参数被 opts 中的 Context 覆盖，会无限重试不超时
	// 20ms 超时说明 ctx 参数优先
}

// TestCustomRetryIf 测试自定义 RetryIf 与 Do 函数的集成
func TestCustomRetryIf(t *testing.T) {
	t.Run("IntegrationWithDo", func(t *testing.T) {
		var attempts int
		err := Do(context.Background(), func() error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary")
			}
			return nil
		},
			Attempts(5),
			Delay(time.Millisecond),
			RetryIf(func(err error) bool {
				return err.Error() == "temporary"
			}),
		)

		assert.NoError(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("StopOnPermanentError", func(t *testing.T) {
		var attempts int
		err := Do(context.Background(), func() error {
			attempts++
			if attempts >= 2 {
				return errors.New("permanent")
			}
			return errors.New("temporary")
		},
			Attempts(5),
			Delay(time.Millisecond),
			RetryIf(func(err error) bool {
				return err.Error() == "temporary"
			}),
		)

		assert.Error(t, err)
		assert.Equal(t, 2, attempts)
	})
}
