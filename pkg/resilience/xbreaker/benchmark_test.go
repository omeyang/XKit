package xbreaker

import (
	"context"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
)

// ============================================================================
// Breaker 创建基准测试
// ============================================================================

func BenchmarkNewBreaker_Default(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewBreaker("test")
	}
}

func BenchmarkNewBreaker_WithOptions(b *testing.B) {
	policy := NewConsecutiveFailures(10)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = NewBreaker("test",
			WithTripPolicy(policy),
			WithTimeout(30*time.Second),
			WithMaxRequests(5),
		)
	}
}

// ============================================================================
// Breaker.Do 基准测试
// ============================================================================

func BenchmarkBreaker_Do_Success(b *testing.B) {
	breaker := NewBreaker("test")
	ctx := context.Background()
	fn := func() error { return nil }

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = breaker.Do(ctx, fn)
	}
}

func BenchmarkBreaker_Do_SuccessParallel(b *testing.B) {
	breaker := NewBreaker("test")
	ctx := context.Background()
	fn := func() error { return nil }

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = breaker.Do(ctx, fn)
		}
	})
}

// ============================================================================
// Execute 泛型版本基准测试
// ============================================================================

func BenchmarkExecute_Success(b *testing.B) {
	breaker := NewBreaker("test")
	ctx := context.Background()
	fn := func() (int, error) { return 42, nil }

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = Execute(ctx, breaker, fn)
	}
}

func BenchmarkExecute_SuccessParallel(b *testing.B) {
	breaker := NewBreaker("test")
	ctx := context.Background()
	fn := func() (int, error) { return 42, nil }

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = Execute(ctx, breaker, fn)
		}
	})
}

// ============================================================================
// ManagedBreaker 基准测试
// ============================================================================

func BenchmarkManagedBreaker_Execute(b *testing.B) {
	breaker := NewBreaker("test")
	managed, err := NewManagedBreaker[int](breaker)
	if err != nil {
		b.Fatal(err)
	}
	fn := func() (int, error) { return 42, nil }

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = managed.Execute(fn)
	}
}

func BenchmarkManagedBreaker_ExecuteParallel(b *testing.B) {
	breaker := NewBreaker("test")
	managed, err := NewManagedBreaker[int](breaker)
	if err != nil {
		b.Fatal(err)
	}
	fn := func() (int, error) { return 42, nil }

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = managed.Execute(fn)
		}
	})
}

// ============================================================================
// TripPolicy 基准测试
// ============================================================================

func BenchmarkConsecutiveFailures_ReadyToTrip(b *testing.B) {
	policy := NewConsecutiveFailures(5)
	counts := gobreaker.Counts{
		Requests:             100,
		TotalSuccesses:       95,
		TotalFailures:        5,
		ConsecutiveSuccesses: 0,
		ConsecutiveFailures:  5,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = policy.ReadyToTrip(counts)
	}
}

func BenchmarkFailureRatio_ReadyToTrip(b *testing.B) {
	policy := NewFailureRatio(0.5, 10)
	counts := gobreaker.Counts{
		Requests:       100,
		TotalSuccesses: 45,
		TotalFailures:  55,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = policy.ReadyToTrip(counts)
	}
}

func BenchmarkFailureCount_ReadyToTrip(b *testing.B) {
	policy := NewFailureCount(10)
	counts := gobreaker.Counts{
		Requests:       100,
		TotalSuccesses: 90,
		TotalFailures:  10,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = policy.ReadyToTrip(counts)
	}
}

func BenchmarkCompositePolicy_ReadyToTrip(b *testing.B) {
	policy := NewCompositePolicy(
		NewConsecutiveFailures(5),
		NewFailureRatio(0.5, 10),
		NewFailureCount(20),
	)
	counts := gobreaker.Counts{
		Requests:            100,
		TotalSuccesses:      90,
		TotalFailures:       10,
		ConsecutiveFailures: 3,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = policy.ReadyToTrip(counts)
	}
}

func BenchmarkNeverTrip_ReadyToTrip(b *testing.B) {
	policy := NewNeverTrip()
	counts := gobreaker.Counts{
		Requests:            100,
		TotalFailures:       100,
		ConsecutiveFailures: 100,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = policy.ReadyToTrip(counts)
	}
}

func BenchmarkAlwaysTrip_ReadyToTrip(b *testing.B) {
	policy := NewAlwaysTrip()
	counts := gobreaker.Counts{
		Requests:      100,
		TotalFailures: 1,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = policy.ReadyToTrip(counts)
	}
}

// ============================================================================
// State 查询基准测试
// ============================================================================

func BenchmarkBreaker_State(b *testing.B) {
	breaker := NewBreaker("test")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = breaker.State()
	}
}

func BenchmarkBreaker_Counts(b *testing.B) {
	breaker := NewBreaker("test")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = breaker.Counts()
	}
}

// ============================================================================
// Policy 创建基准测试
// ============================================================================

func BenchmarkNewConsecutiveFailures(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewConsecutiveFailures(5)
	}
}

func BenchmarkNewFailureRatio(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewFailureRatio(0.5, 10)
	}
}

func BenchmarkNewCompositePolicy(b *testing.B) {
	p1 := NewConsecutiveFailures(5)
	p2 := NewFailureRatio(0.5, 10)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = NewCompositePolicy(p1, p2)
	}
}
