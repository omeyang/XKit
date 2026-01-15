package xsemaphore

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// =============================================================================
// 基准测试辅助函数
// =============================================================================

func setupBenchmarkRedis(b *testing.B) (*miniredis.Miniredis, redis.UniversalClient) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	b.Cleanup(func() {
		if err := client.Close(); err != nil {
			b.Logf("redis client close: %v", err)
		}
	})

	return mr, client
}

func setupBenchmarkSemaphore(b *testing.B, opts ...Option) Semaphore {
	_, client := setupBenchmarkRedis(b)
	sem, err := New(client, opts...)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := sem.Close(context.Background()); err != nil {
			b.Logf("semaphore close: %v", err)
		}
	})
	return sem
}

// releasePermitB 基准测试辅助函数：释放许可
func releasePermitB(_ *testing.B, ctx context.Context, p Permit) {
	if p != nil {
		_ = p.Release(ctx) //nolint:errcheck // benchmark ignores release errors
	}
}

// =============================================================================
// TryAcquire 基准测试
// =============================================================================

func BenchmarkTryAcquire_Success(b *testing.B) {
	sem := setupBenchmarkSemaphore(b)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		permit, err := sem.TryAcquire(ctx, "bench-resource",
			WithCapacity(1000000), // 大容量确保不会满
			WithTTL(time.Minute),
		)
		if err != nil {
			b.Fatal(err)
		}
		if permit != nil {
			releasePermitB(b, ctx, permit)
		}
	}
}

func BenchmarkTryAcquire_WithTenant(b *testing.B) {
	sem := setupBenchmarkSemaphore(b)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		permit, err := sem.TryAcquire(ctx, "bench-tenant",
			WithCapacity(1000000),
			WithTenantQuota(100000),
			WithTenantID("tenant-bench"),
			WithTTL(time.Minute),
		)
		if err != nil {
			b.Fatal(err)
		}
		if permit != nil {
			releasePermitB(b, ctx, permit)
		}
	}
}

func BenchmarkTryAcquire_CapacityFull(b *testing.B) {
	sem := setupBenchmarkSemaphore(b)
	ctx := context.Background()

	// 先占满容量
	for i := 0; i < 10; i++ {
		permit, err := sem.TryAcquire(ctx, "bench-full",
			WithCapacity(10),
			WithTTL(time.Hour),
		)
		if err != nil {
			b.Fatal(err)
		}
		if permit == nil {
			break
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		permit, err := sem.TryAcquire(ctx, "bench-full",
			WithCapacity(10),
		)
		if err != nil {
			b.Fatal(err)
		}
		if permit != nil {
			releasePermitB(b, ctx, permit)
		}
	}
}

// =============================================================================
// Release 基准测试
// =============================================================================

func BenchmarkRelease(b *testing.B) {
	sem := setupBenchmarkSemaphore(b)
	ctx := context.Background()

	// 预先获取许可
	permits := make([]Permit, b.N)
	for i := 0; i < b.N; i++ {
		permit, err := sem.TryAcquire(ctx, "bench-release",
			WithCapacity(b.N+100),
			WithTTL(time.Hour),
		)
		if err != nil {
			b.Fatal(err)
		}
		permits[i] = permit
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if permits[i] != nil {
			releasePermitB(b, ctx, permits[i])
		}
	}
}

// =============================================================================
// Extend 基准测试
// =============================================================================

func BenchmarkExtend(b *testing.B) {
	sem := setupBenchmarkSemaphore(b)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "bench-extend",
		WithCapacity(100),
		WithTTL(time.Minute),
	)
	if err != nil {
		b.Fatal(err)
	}
	if permit == nil {
		b.Fatal("failed to acquire permit")
	}
	defer func() { releasePermitB(b, ctx, permit) }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := permit.Extend(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// Query 基准测试
// =============================================================================

func BenchmarkQuery(b *testing.B) {
	sem := setupBenchmarkSemaphore(b)
	ctx := context.Background()

	// 先获取一些许可
	for i := 0; i < 10; i++ {
		permit, _ := sem.TryAcquire(ctx, "bench-query",
			WithCapacity(100),
			WithTenantQuota(50),
			WithTenantID("tenant-query"),
			WithTTL(time.Hour),
		)
		if permit == nil {
			break
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := sem.Query(ctx, "bench-query",
			QueryWithCapacity(100),
			QueryWithTenantQuota(50),
			QueryWithTenantID("tenant-query"),
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// 并发基准测试
// =============================================================================

func BenchmarkConcurrentAcquireRelease(b *testing.B) {
	sem := setupBenchmarkSemaphore(b)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			permit, err := sem.TryAcquire(ctx, "bench-concurrent",
				WithCapacity(1000),
				WithTTL(time.Minute),
			)
			if err != nil {
				continue
			}
			if permit != nil {
				releasePermitB(b, ctx, permit)
			}
		}
	})
}

func BenchmarkConcurrentAcquireRelease_HighContention(b *testing.B) {
	sem := setupBenchmarkSemaphore(b)
	ctx := context.Background()

	// 低容量高竞争
	capacity := 10

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			permit, err := sem.TryAcquire(ctx, "bench-contention",
				WithCapacity(capacity),
				WithTTL(time.Minute),
			)
			if err != nil {
				continue
			}
			if permit != nil {
				releasePermitB(b, ctx, permit)
			}
		}
	})
}

// =============================================================================
// 本地信号量基准测试
// =============================================================================

func BenchmarkLocalSemaphore_TryAcquire(b *testing.B) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		permit, err := sem.TryAcquire(ctx, "bench-local",
			WithCapacity(1000000),
			WithTTL(time.Minute),
		)
		if err != nil {
			b.Fatal(err)
		}
		if permit != nil {
			releasePermitB(b, ctx, permit)
		}
	}
}

func BenchmarkLocalSemaphore_Concurrent(b *testing.B) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			permit, err := sem.TryAcquire(ctx, "bench-local-concurrent",
				WithCapacity(1000),
				WithTTL(time.Minute),
			)
			if err != nil {
				continue
			}
			if permit != nil {
				releasePermitB(b, ctx, permit)
			}
		}
	})
}

// =============================================================================
// 分配基准测试
// =============================================================================

func BenchmarkTryAcquire_Allocs(b *testing.B) {
	sem := setupBenchmarkSemaphore(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		permit, _ := sem.TryAcquire(ctx, "bench-allocs",
			WithCapacity(1000000),
			WithTTL(time.Minute),
		)
		if permit != nil {
			releasePermitB(b, ctx, permit)
		}
	}
}

// =============================================================================
// 吞吐量基准测试
// =============================================================================

func BenchmarkThroughput(b *testing.B) {
	sem := setupBenchmarkSemaphore(b)
	ctx := context.Background()

	const goroutines = 100
	const opsPerGoroutine = 100

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		wg.Add(goroutines)

		for g := 0; g < goroutines; g++ {
			go func() {
				defer wg.Done()
				for op := 0; op < opsPerGoroutine; op++ {
					permit, _ := sem.TryAcquire(ctx, "bench-throughput",
						WithCapacity(10000),
						WithTTL(time.Minute),
					)
					if permit != nil {
						releasePermitB(b, ctx, permit)
					}
				}
			}()
		}

		wg.Wait()
	}
}

// =============================================================================
// 带配置的基准测试（覆盖 unparam lint）
// =============================================================================

func BenchmarkTryAcquire_WithCustomPrefix(b *testing.B) {
	// 使用 opts 参数（覆盖 unparam lint）
	sem := setupBenchmarkSemaphore(b, WithKeyPrefix("bench:custom:"))
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		permit, err := sem.TryAcquire(ctx, "custom-prefix",
			WithCapacity(1000000),
			WithTTL(time.Minute),
		)
		if err != nil {
			b.Fatal(err)
		}
		if permit != nil {
			releasePermitB(b, ctx, permit)
		}
	}
}
