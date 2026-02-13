package xretry

import (
	"context"
	"errors"
	"testing"
	"time"
)

// BenchmarkRetryerDo 测试 Retryer.Do 性能
func BenchmarkRetryerDo(b *testing.B) {
	b.Run("SuccessFirstAttempt", func(b *testing.B) {
		r := NewRetryer(
			WithRetryPolicy(NewFixedRetry(3)),
			WithBackoffPolicy(NewNoBackoff()),
		)
		ctx := context.Background()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			err := r.Do(ctx, func(ctx context.Context) error {
				return nil
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("SuccessAfterOneRetry", func(b *testing.B) {
		r := NewRetryer(
			WithRetryPolicy(NewFixedRetry(3)),
			WithBackoffPolicy(NewNoBackoff()),
		)
		ctx := context.Background()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			attempt := 0
			err := r.Do(ctx, func(ctx context.Context) error {
				attempt++
				if attempt == 1 {
					return errors.New("retry")
				}
				return nil
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("WithExponentialBackoff", func(b *testing.B) {
		r := NewRetryer(
			WithRetryPolicy(NewFixedRetry(3)),
			WithBackoffPolicy(NewExponentialBackoff(
				WithInitialDelay(time.Nanosecond),
				WithMaxDelay(time.Nanosecond),
				WithJitter(0),
			)),
		)
		ctx := context.Background()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			err := r.Do(ctx, func(ctx context.Context) error {
				return nil
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkDoWithResult 测试 DoWithResult 泛型函数性能
func BenchmarkDoWithResult(b *testing.B) {
	r := NewRetryer(
		WithRetryPolicy(NewFixedRetry(3)),
		WithBackoffPolicy(NewNoBackoff()),
	)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := DoWithResult(ctx, r, func(ctx context.Context) (int, error) {
			return 42, nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWrapperDo 测试 wrapper.Do 性能
func BenchmarkWrapperDo(b *testing.B) {
	b.Run("SuccessFirstAttempt", func(b *testing.B) {
		ctx := context.Background()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			err := Do(ctx, func() error {
				return nil
			}, Attempts(3), Delay(0))
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("SuccessAfterOneRetry", func(b *testing.B) {
		ctx := context.Background()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			attempt := 0
			err := Do(ctx, func() error {
				attempt++
				if attempt == 1 {
					return errors.New("retry")
				}
				return nil
			}, Attempts(3), Delay(0), MaxJitter(0))
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkWrapperDoWithData 测试 wrapper.DoWithData 性能
func BenchmarkWrapperDoWithData(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := DoWithData(ctx, func() (int, error) {
			return 42, nil
		}, Attempts(3), Delay(0))
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkNewRetrier 测试创建 Retrier 的开销
func BenchmarkNewRetrier(b *testing.B) {
	b.Run("Default", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewRetrier()
		}
	})

	b.Run("WithOptions", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewRetrier(Attempts(5), Delay(100*time.Millisecond))
		}
	})
}

// BenchmarkToDelayType 测试 BackoffPolicy 转换性能
func BenchmarkToDelayType(b *testing.B) {
	backoff := NewExponentialBackoff(WithJitter(0))
	delayFunc := ToDelayType(backoff)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// #nosec G115 -- i%10 is always in range [0,9], safe for uint conversion
		_ = delayFunc(uint(i%10), nil, nil)
	}
}

// BenchmarkNewRetryer 测试创建 Retryer 的开销
func BenchmarkNewRetryer(b *testing.B) {
	b.Run("Default", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewRetryer()
		}
	})

	b.Run("WithAllOptions", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewRetryer(
				WithRetryPolicy(NewFixedRetry(5)),
				WithBackoffPolicy(NewExponentialBackoff()),
				WithOnRetry(func(attempt int, err error) {}),
			)
		}
	})
}

// BenchmarkIsRetryable 测试错误分类性能
func BenchmarkIsRetryable(b *testing.B) {
	regularErr := errors.New("regular error")
	permanentErr := NewPermanentError(errors.New("permanent"))
	temporaryErr := NewTemporaryError(errors.New("temporary"))

	b.Run("RegularError", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = IsRetryable(regularErr)
		}
	})

	b.Run("PermanentError", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = IsRetryable(permanentErr)
		}
	})

	b.Run("TemporaryError", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = IsRetryable(temporaryErr)
		}
	})

	b.Run("NilError", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = IsRetryable(nil)
		}
	})
}
