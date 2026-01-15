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

// TestToRetryIfSimple 测试 ToRetryIfSimple 函数
func TestToRetryIfSimple(t *testing.T) {
	t.Run("BasicUsage", func(t *testing.T) {
		// 简单的错误类型检查：只要不是 context.Canceled 就重试
		retryIf := ToRetryIfSimple(func(err error) bool {
			return !errors.Is(err, context.Canceled)
		})

		assert.True(t, retryIf(errors.New("temporary")))
		assert.True(t, retryIf(errors.New("network error")))
		assert.False(t, retryIf(context.Canceled))
	})

	t.Run("CustomErrorTypes", func(t *testing.T) {
		// 自定义错误类型列表
		permanentErrors := []error{
			errors.New("invalid input"),
			errors.New("permission denied"),
		}

		retryIf := ToRetryIfSimple(func(err error) bool {
			for _, pe := range permanentErrors {
				if err.Error() == pe.Error() {
					return false
				}
			}
			return true
		})

		assert.False(t, retryIf(errors.New("invalid input")))
		assert.False(t, retryIf(errors.New("permission denied")))
		assert.True(t, retryIf(errors.New("timeout")))
	})

	t.Run("IntegrationWithDo", func(t *testing.T) {
		// 集成测试：与 Do 函数一起使用
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
			RetryIf(ToRetryIfSimple(func(err error) bool {
				return err.Error() == "temporary"
			})),
		)

		assert.NoError(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("StopOnPermanentError", func(t *testing.T) {
		// 遇到永久性错误时停止重试
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
			RetryIf(ToRetryIfSimple(func(err error) bool {
				return err.Error() == "temporary"
			})),
		)

		assert.Error(t, err)
		assert.Equal(t, 2, attempts) // 第 2 次返回 permanent 错误后停止
	})
}
