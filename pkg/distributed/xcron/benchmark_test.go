package xcron

import (
	"context"
	"testing"
	"time"
)

// ============================================================================
// Scheduler 创建基准测试
// ============================================================================

func BenchmarkNew_Default(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = New()
	}
}

func BenchmarkNew_WithOptions(b *testing.B) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = New(
			WithLocker(NoopLocker()),
			WithLocation(loc),
			WithSeconds(),
		)
	}
}

// ============================================================================
// AddFunc/AddJob 基准测试
// ============================================================================

func BenchmarkScheduler_AddFunc(b *testing.B) {
	scheduler := New()
	job := func(ctx context.Context) error { return nil }

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = scheduler.AddFunc("@every 1m", job)
	}
}

func BenchmarkScheduler_AddFuncWithOptions(b *testing.B) {
	scheduler := New()
	job := func(ctx context.Context) error { return nil }

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = scheduler.AddFunc("@every 1m", job,
			WithName("test-job"),
			WithTimeout(30*time.Second),
			WithLockTTL(5*time.Minute),
		)
	}
}

func BenchmarkScheduler_AddJob(b *testing.B) {
	scheduler := New()
	job := JobFunc(func(ctx context.Context) error { return nil })

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = scheduler.AddJob("@every 1m", job)
	}
}

// ============================================================================
// JobFunc 适配器基准测试
// ============================================================================

func BenchmarkJobFunc_Create(b *testing.B) {
	fn := func(ctx context.Context) error { return nil }

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = JobFunc(fn)
	}
}

func BenchmarkJobFunc_Run(b *testing.B) {
	job := JobFunc(func(ctx context.Context) error { return nil })
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = job.Run(ctx)
	}
}

// ============================================================================
// Options 创建基准测试
// ============================================================================

func BenchmarkSchedulerOptions(b *testing.B) {
	locker := NoopLocker()
	loc := time.UTC

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		opts := defaultSchedulerOptions()
		WithLocker(locker)(opts)
		WithLocation(loc)(opts)
	}
}

func BenchmarkJobOptions(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		opts := defaultJobOptions()
		WithName("test")(opts)
		WithTimeout(30 * time.Second)(opts)
		WithLockTTL(5 * time.Minute)(opts)
	}
}

// ============================================================================
// NoopLocker 基准测试
// ============================================================================

func BenchmarkNoopLocker_TryLock(b *testing.B) {
	locker := NoopLocker()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = locker.TryLock(ctx, "test-key", 5*time.Minute)
	}
}

func BenchmarkNoopLocker_Unlock(b *testing.B) {
	locker := NoopLocker()
	ctx := context.Background()
	handle, _ := locker.TryLock(ctx, "test-key", 5*time.Minute)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = handle.Unlock(ctx)
	}
}

func BenchmarkNoopLocker_Renew(b *testing.B) {
	locker := NoopLocker()
	ctx := context.Background()
	handle, _ := locker.TryLock(ctx, "test-key", 5*time.Minute)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = handle.Renew(ctx, 5*time.Minute)
	}
}

// ============================================================================
// jobWrapper 创建基准测试
// ============================================================================

func BenchmarkNewJobWrapper(b *testing.B) {
	job := JobFunc(func(ctx context.Context) error { return nil })
	locker := NoopLocker()
	opts := defaultJobOptions()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = newJobWrapper(job, locker, nil, nil, opts)
	}
}

// ============================================================================
// 并发基准测试
// ============================================================================

func BenchmarkNoopLocker_TryLockParallel(b *testing.B) {
	locker := NoopLocker()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = locker.TryLock(ctx, "test-key", 5*time.Minute)
		}
	})
}

func BenchmarkJobFunc_RunParallel(b *testing.B) {
	job := JobFunc(func(ctx context.Context) error { return nil })
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = job.Run(ctx)
		}
	})
}

// ============================================================================
// WithImmediate 基准测试
// ============================================================================

func BenchmarkWithImmediate(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		opts := defaultJobOptions()
		WithImmediate()(opts)
	}
}

func BenchmarkScheduler_AddFuncWithImmediate(b *testing.B) {
	job := func(ctx context.Context) error { return nil }

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		scheduler := New()
		b.StartTimer()

		_, _ = scheduler.AddFunc("@every 1h", job,
			WithName("bench-job"),
			WithImmediate(),
		)

		b.StopTimer()
		// Stop() 已等待所有 immediate 任务完成，无需额外 sleep
		ctx := scheduler.Stop()
		<-ctx.Done()
		b.StartTimer()
	}
}

func BenchmarkScheduler_AddFuncWithoutImmediate(b *testing.B) {
	job := func(ctx context.Context) error { return nil }

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		scheduler := New()
		b.StartTimer()

		_, _ = scheduler.AddFunc("@every 1h", job,
			WithName("bench-job"),
		)

		b.StopTimer()
		scheduler.Stop()
		b.StartTimer()
	}
}

func BenchmarkJobOptions_WithImmediate(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		opts := defaultJobOptions()
		WithName("test")(opts)
		WithTimeout(30 * time.Second)(opts)
		WithLockTTL(5 * time.Minute)(opts)
		WithImmediate()(opts)
	}
}
