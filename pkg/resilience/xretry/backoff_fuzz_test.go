package xretry

import (
	"testing"
	"time"
)

func FuzzExponentialBackoff_NextDelay(f *testing.F) {
	f.Add(int64(100*time.Millisecond), int64(30*time.Second), 2.0, 0.1, 1)

	f.Fuzz(func(t *testing.T, initial, max int64, multiplier, jitter float64, attempt int) {
		initialDelay := clampDuration(initial)
		maxDelay := clampDuration(max)
		attempt = clampAttempt(attempt)

		b := NewExponentialBackoff(
			WithInitialDelay(initialDelay),
			WithMaxDelay(maxDelay),
			WithMultiplier(multiplier),
			WithJitter(jitter),
		)

		if delay := b.NextDelay(attempt); delay < 0 {
			t.Fatalf("negative delay: %v", delay)
		}
	})
}

func FuzzLinearBackoff_NextDelay(f *testing.F) {
	f.Add(int64(100*time.Millisecond), int64(50*time.Millisecond), int64(5*time.Second), 1)

	f.Fuzz(func(t *testing.T, initial, increment, max int64, attempt int) {
		initialDelay := clampDuration(initial)
		incrementDelay := clampDuration(increment)
		maxDelay := clampDuration(max)
		attempt = clampAttempt(attempt)

		b := NewLinearBackoff(initialDelay, incrementDelay, maxDelay)
		if delay := b.NextDelay(attempt); delay < 0 {
			t.Fatalf("negative delay: %v", delay)
		}
	})
}

func FuzzFixedBackoff_NextDelay(f *testing.F) {
	f.Add(int64(100*time.Millisecond), 1)

	f.Fuzz(func(t *testing.T, delay int64, attempt int) {
		backoff := NewFixedBackoff(clampDuration(delay))
		_ = backoff.NextDelay(attempt)
	})
}

func clampDuration(v int64) time.Duration {
	if v < 0 {
		return 0
	}
	if v > int64(time.Hour) {
		return time.Hour
	}
	return time.Duration(v)
}

func clampAttempt(attempt int) int {
	if attempt < 1 {
		return 1
	}
	if attempt > 100 {
		return 100
	}
	return attempt
}
