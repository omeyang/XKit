package xretry_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/omeyang/xkit/pkg/resilience/xretry"
)

func ExampleNewRetryer() {
	r := xretry.NewRetryer(
		xretry.WithRetryPolicy(xretry.NewFixedRetry(3)),
		xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
	)

	var attempts int
	err := r.Do(context.Background(), func(_ context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	fmt.Println("error:", err)
	fmt.Println("attempts:", attempts)
	// Output:
	// error: <nil>
	// attempts: 3
}

func ExampleDo() {
	var attempts int
	err := xretry.Do(context.Background(), func() error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		return nil
	}, xretry.Attempts(3), xretry.Delay(time.Millisecond))

	fmt.Println("error:", err)
	fmt.Println("attempts:", attempts)
	// Output:
	// error: <nil>
	// attempts: 2
}

func ExampleDoWithData() {
	result, err := xretry.DoWithData(context.Background(), func() (string, error) {
		return "hello", nil
	}, xretry.Attempts(3))

	fmt.Println("result:", result)
	fmt.Println("error:", err)
	// Output:
	// result: hello
	// error: <nil>
}

func ExampleNewExponentialBackoff() {
	backoff := xretry.NewExponentialBackoff(
		xretry.WithInitialDelay(100*time.Millisecond),
		xretry.WithMaxDelay(5*time.Second),
		xretry.WithMultiplier(2.0),
		xretry.WithJitter(0), // 无抖动，确定性输出
	)

	fmt.Println("attempt 1:", backoff.NextDelay(1))
	fmt.Println("attempt 2:", backoff.NextDelay(2))
	fmt.Println("attempt 3:", backoff.NextDelay(3))
	// Output:
	// attempt 1: 100ms
	// attempt 2: 200ms
	// attempt 3: 400ms
}

func ExampleNewLinearBackoff() {
	backoff := xretry.NewLinearBackoff(
		100*time.Millisecond, // 初始延迟
		50*time.Millisecond,  // 每次增加
		500*time.Millisecond, // 最大延迟
	)

	fmt.Println("attempt 1:", backoff.NextDelay(1))
	fmt.Println("attempt 2:", backoff.NextDelay(2))
	fmt.Println("attempt 3:", backoff.NextDelay(3))
	fmt.Println("attempt 9:", backoff.NextDelay(9)) // 达到最大值
	// Output:
	// attempt 1: 100ms
	// attempt 2: 150ms
	// attempt 3: 200ms
	// attempt 9: 500ms
}

func ExampleNewAlwaysRetry() {
	r := xretry.NewRetryer(
		xretry.WithRetryPolicy(xretry.NewAlwaysRetry()),
		xretry.WithBackoffPolicy(xretry.NewNoBackoff()),
	)

	var attempts int
	err := r.Do(context.Background(), func(_ context.Context) error {
		attempts++
		if attempts < 5 {
			return errors.New("temporary error")
		}
		return nil
	})

	fmt.Println("error:", err)
	fmt.Println("attempts:", attempts)
	// Output:
	// error: <nil>
	// attempts: 5
}

func ExampleNewPermanentError() {
	var attempts int
	err := xretry.Do(context.Background(), func() error {
		attempts++
		return xretry.NewPermanentError(errors.New("invalid input"))
	}, xretry.Attempts(5))

	fmt.Println("attempts:", attempts)
	fmt.Println("is retryable:", xretry.IsRetryable(err))
	// Output:
	// attempts: 1
	// is retryable: false
}
