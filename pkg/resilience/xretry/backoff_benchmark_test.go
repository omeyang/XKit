package xretry

import (
	"testing"
	"time"
)

func BenchmarkExponentialBackoff_NextDelay(b *testing.B) {
	backoff := NewExponentialBackoff(
		WithJitter(0.1),
		WithMultiplier(2.0),
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = backoff.NextDelay(5)
	}
}

func BenchmarkLinearBackoff_NextDelay(b *testing.B) {
	backoff := NewLinearBackoff(100*time.Millisecond, 50*time.Millisecond, 10*time.Second)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = backoff.NextDelay(5)
	}
}

func BenchmarkFixedBackoff_NextDelay(b *testing.B) {
	backoff := NewFixedBackoff(100 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = backoff.NextDelay(5)
	}
}
