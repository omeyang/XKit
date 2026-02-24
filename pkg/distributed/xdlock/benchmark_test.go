package xdlock_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/omeyang/xkit/pkg/distributed/xdlock"
)

// =============================================================================
// 基准测试辅助函数
// =============================================================================

// setupMiniredis 创建 miniredis 实例和 Redis 客户端。
func setupMiniredis(b *testing.B) (*miniredis.Miniredis, redis.UniversalClient) {
	b.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return mr, client
}

// =============================================================================
// Redis 工厂基准测试
// =============================================================================

// BenchmarkNewRedisFactory 测试创建 Redis 工厂的性能。
func BenchmarkNewRedisFactory(b *testing.B) {
	mr, client := setupMiniredis(b)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		factory, err := xdlock.NewRedisFactory(client)
		if err != nil {
			b.Fatal(err)
		}
		_ = factory.Close(context.Background())
	}
}

// BenchmarkRedisFactory_Health 测试健康检查性能。
func BenchmarkRedisFactory_Health(b *testing.B) {
	mr, client := setupMiniredis(b)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := factory.Health(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// Redis 锁操作基准测试（使用 Handle API）
// =============================================================================

// BenchmarkRedisFactory_Lock 测试阻塞式获取锁性能。
func BenchmarkRedisFactory_Lock(b *testing.B) {
	mr, client := setupMiniredis(b)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// 每次使用不同的 key 避免锁冲突
		handle, err := factory.Lock(ctx, fmt.Sprintf("bench-lock-%d", i), xdlock.WithTries(1))
		if err != nil {
			b.Fatal(err)
		}
		if err := handle.Unlock(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRedisFactory_TryLock 测试非阻塞式获取锁性能。
func BenchmarkRedisFactory_TryLock(b *testing.B) {
	mr, client := setupMiniredis(b)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		handle, err := factory.TryLock(ctx, fmt.Sprintf("bench-trylock-%d", i))
		if err != nil {
			b.Fatal(err)
		}
		if handle == nil {
			b.Fatal("expected handle, got nil")
		}
		if err := handle.Unlock(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRedisFactory_Unlock 测试释放锁性能。
func BenchmarkRedisFactory_Unlock(b *testing.B) {
	mr, client := setupMiniredis(b)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	// 预先创建锁并获取
	handles := make([]xdlock.LockHandle, b.N)
	for i := 0; i < b.N; i++ {
		handle, err := factory.TryLock(ctx, fmt.Sprintf("bench-unlock-%d", i))
		if err != nil {
			b.Fatal(err)
		}
		if handle == nil {
			b.Fatalf("expected handle for key bench-unlock-%d", i)
		}
		handles[i] = handle
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := handles[i].Unlock(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRedisLockHandle_Extend 测试续期锁性能。
func BenchmarkRedisLockHandle_Extend(b *testing.B) {
	mr, client := setupMiniredis(b)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	// 创建并获取锁
	handle, err := factory.TryLock(ctx, "bench-extend", xdlock.WithExpiry(30*time.Second))
	if err != nil {
		b.Fatal(err)
	}
	if handle == nil {
		b.Fatal("expected handle, got nil")
	}
	defer func() { _ = handle.Unlock(ctx) }()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := handle.Extend(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRedisFactory_LockUnlock_Cycle 测试完整的锁获取-释放周期。
func BenchmarkRedisFactory_LockUnlock_Cycle(b *testing.B) {
	mr, client := setupMiniredis(b)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// 使用单个 key，模拟实际场景中的锁竞争
		handle, err := factory.Lock(ctx, "bench-cycle",
			xdlock.WithExpiry(time.Second),
			xdlock.WithTries(1),
		)
		if err != nil {
			b.Fatal(err)
		}
		if err := handle.Unlock(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// 并发基准测试
// =============================================================================

// BenchmarkRedisFactory_Lock_Parallel 测试并发获取锁性能。
func BenchmarkRedisFactory_Lock_Parallel(b *testing.B) {
	mr, client := setupMiniredis(b)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()
	var counter atomic.Int64

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// 每个操作使用全局唯一 key，避免跨 goroutine 冲突
			seq := counter.Add(1)
			key := fmt.Sprintf("parallel-lock-%d", seq)
			handle, err := factory.TryLock(ctx, key)
			if err != nil {
				b.Error(err)
				return
			}
			if handle == nil {
				b.Error("expected handle, got nil")
				return
			}
			if err := handle.Unlock(ctx); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

// BenchmarkRedisFactory_Lock_Contention 测试锁竞争场景性能。
func BenchmarkRedisFactory_Lock_Contention(b *testing.B) {
	mr, client := setupMiniredis(b)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	// 使用有限数量的 key，增加竞争
	numKeys := 10

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("contention-%d", i%numKeys)
			handle, err := factory.Lock(ctx, key,
				xdlock.WithExpiry(100*time.Millisecond),
				xdlock.WithTries(10),
				xdlock.WithRetryDelay(10*time.Millisecond),
			)

			if err != nil {
				// 竞争激烈时可能获取失败，跳过
				i++
				continue
			}
			// 模拟短暂的临界区操作
			_ = handle.Unlock(ctx)
			i++
		}
	})
}

// BenchmarkRedisFactory_Health_Parallel 测试并发健康检查性能。
func BenchmarkRedisFactory_Health_Parallel(b *testing.B) {
	mr, client := setupMiniredis(b)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := factory.Health(ctx); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

// =============================================================================
// 选项函数基准测试
// =============================================================================

// BenchmarkMutexOptions 测试各种选项函数的性能。
func BenchmarkMutexOptions(b *testing.B) {
	b.Run("WithKeyPrefix", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = xdlock.WithKeyPrefix("myapp:")
		}
	})

	b.Run("WithExpiry", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = xdlock.WithExpiry(10 * time.Second)
		}
	})

	b.Run("WithTries", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = xdlock.WithTries(5)
		}
	})

	b.Run("WithRetryDelay", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = xdlock.WithRetryDelay(100 * time.Millisecond)
		}
	})

	b.Run("WithDriftFactor", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = xdlock.WithDriftFactor(0.02)
		}
	})

	b.Run("WithTimeoutFactor", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = xdlock.WithTimeoutFactor(0.1)
		}
	})

	b.Run("WithFailFast", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = xdlock.WithFailFast(true)
		}
	})

	b.Run("WithShufflePools", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = xdlock.WithShufflePools(true)
		}
	})

	b.Run("AllOptions", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = xdlock.WithKeyPrefix("myapp:")
			_ = xdlock.WithExpiry(10 * time.Second)
			_ = xdlock.WithTries(5)
			_ = xdlock.WithRetryDelay(100 * time.Millisecond)
			_ = xdlock.WithDriftFactor(0.02)
			_ = xdlock.WithTimeoutFactor(0.1)
			_ = xdlock.WithFailFast(true)
			_ = xdlock.WithShufflePools(true)
		}
	})
}

// =============================================================================
// 多节点 Redlock 基准测试
// =============================================================================

// BenchmarkRedlock_ThreeNodes 测试三节点 Redlock 性能。
func BenchmarkRedlock_ThreeNodes(b *testing.B) {
	// 创建三个 miniredis 实例
	mr1, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr1.Close()

	mr2, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr2.Close()

	mr3, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr3.Close()

	client1 := redis.NewClient(&redis.Options{Addr: mr1.Addr()})
	defer func() { _ = client1.Close() }()
	client2 := redis.NewClient(&redis.Options{Addr: mr2.Addr()})
	defer func() { _ = client2.Close() }()
	client3 := redis.NewClient(&redis.Options{Addr: mr3.Addr()})
	defer func() { _ = client3.Close() }()

	factory, err := xdlock.NewRedisFactory(client1, client2, client3)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		handle, err := factory.TryLock(ctx, fmt.Sprintf("redlock-%d", i))
		if err != nil {
			b.Fatal(err)
		}
		if handle == nil {
			b.Fatal("expected handle, got nil")
		}
		if err := handle.Unlock(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// 内存和 GC 压力测试
// =============================================================================

// BenchmarkRedisFactory_HighVolume 测试高频创建锁实例的内存压力。
func BenchmarkRedisFactory_HighVolume(b *testing.B) {
	mr, client := setupMiniredis(b)
	defer mr.Close()
	defer func() { _ = client.Close() }()

	factory, err := xdlock.NewRedisFactory(client)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = factory.Close(context.Background()) }()

	ctx := context.Background()
	opts := []xdlock.MutexOption{
		xdlock.WithExpiry(100 * time.Millisecond),
		xdlock.WithTries(1),
	}

	b.ResetTimer()
	b.ReportAllocs()

	var wg sync.WaitGroup
	numGoroutines := 10

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < b.N/numGoroutines; i++ {
				key := fmt.Sprintf("highvol-%d-%d", gid, i)
				handle, err := factory.TryLock(ctx, key, opts...)
				if err == nil && handle != nil {
					_ = handle.Unlock(ctx)
				}
			}
		}(g)
	}

	wg.Wait()
}
