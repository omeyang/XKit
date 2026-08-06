package xcache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Redis 基准测试
// =============================================================================

func BenchmarkRedis_Get(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := NewRedis(client)
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()
	_ = cache.Client().Set(ctx, "benchmark_key", "benchmark_value", 0).Err()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cache.Client().Get(ctx, "benchmark_key").Result()
	}
}

func BenchmarkRedis_Set(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := NewRedis(client)
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()
	value := "benchmark_value"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.Client().Set(ctx, fmt.Sprintf("key_%d", i), value, time.Hour).Err()
	}
}

func BenchmarkRedis_HGet(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := NewRedis(client)
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()
	_ = cache.Client().HSet(ctx, "benchmark_hash", "field", "value").Err()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cache.Client().HGet(ctx, "benchmark_hash", "field").Result()
	}
}

func BenchmarkRedis_Lock(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := NewRedis(client)
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		unlock, err := cache.Lock(ctx, fmt.Sprintf("lock_%d", i), time.Minute)
		if err != nil {
			b.Fatal(err)
		}
		_ = unlock(ctx)
	}
}

// =============================================================================
// Memory 基准测试
// =============================================================================

func BenchmarkMemory_Get(b *testing.B) {
	cache, err := NewMemory()
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	cache.Client().SetWithTTL("benchmark_key", []byte("benchmark_value"), 16, 0)
	cache.Wait()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cache.Client().Get("benchmark_key")
	}
}

func BenchmarkMemory_Set(b *testing.B) {
	cache, err := NewMemory()
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	value := []byte("benchmark_value")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Client().SetWithTTL(fmt.Sprintf("key_%d", i%1000), value, int64(len(value)), time.Hour)
	}
}

func BenchmarkMemory_Get_Parallel(b *testing.B) {
	cache, err := NewMemory()
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	for i := 0; i < 100; i++ {
		cache.Client().SetWithTTL(fmt.Sprintf("key_%d", i), []byte("value"), 5, 0)
	}
	cache.Wait()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = cache.Client().Get(fmt.Sprintf("key_%d", i%100))
			i++
		}
	})
}

// =============================================================================
// Loader 基准测试
// =============================================================================

func BenchmarkLoader_Load_CacheHit(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := NewRedis(client)
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()
	_ = cache.Client().Set(ctx, "benchmark_key", "cached_value", 0).Err()

	loader, err := NewLoader(cache)
	if err != nil {
		b.Fatal(err)
	}
	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("backend_value"), nil
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = loader.Load(ctx, "benchmark_key", loadFn, time.Hour)
	}
}

func BenchmarkLoader_Load_CacheMiss(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := NewRedis(client)
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()

	loader, err := NewLoader(cache)
	if err != nil {
		b.Fatal(err)
	}
	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("backend_value"), nil
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = loader.Load(ctx, fmt.Sprintf("key_%d", i), loadFn, time.Hour)
	}
}

func BenchmarkLoader_LoadHash_CacheHit(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := NewRedis(client)
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()
	_ = cache.Client().HSet(ctx, "benchmark_hash", "field", "cached_value").Err()

	loader, err := NewLoader(cache)
	if err != nil {
		b.Fatal(err)
	}
	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("backend_value"), nil
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = loader.LoadHash(ctx, "benchmark_hash", "field", loadFn, time.Hour)
	}
}

// =============================================================================
// Redis 并发基准测试
// =============================================================================

func BenchmarkRedis_Get_Parallel(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := NewRedis(client)
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()
	for i := 0; i < 100; i++ {
		_ = cache.Client().Set(ctx, fmt.Sprintf("key_%d", i), "value", 0).Err()
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = cache.Client().Get(ctx, fmt.Sprintf("key_%d", i%100)).Result()
			i++
		}
	})
}

func BenchmarkRedis_Set_Parallel(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := NewRedis(client)
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()
	value := "benchmark_value"

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = cache.Client().Set(ctx, fmt.Sprintf("key_%d", i%1000), value, time.Hour).Err()
			i++
		}
	})
}

// =============================================================================
// 不同值大小基准测试
// =============================================================================

func BenchmarkRedis_Set_SmallValue(b *testing.B) {
	benchmarkRedisSetWithSize(b, 100) // 100 bytes
}

func BenchmarkRedis_Set_MediumValue(b *testing.B) {
	benchmarkRedisSetWithSize(b, 1024) // 1 KB
}

func BenchmarkRedis_Set_LargeValue(b *testing.B) {
	benchmarkRedisSetWithSize(b, 10240) // 10 KB
}

func benchmarkRedisSetWithSize(b *testing.B, size int) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := NewRedis(client)
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()
	value := make([]byte, size)
	for i := range value {
		value[i] = byte(i % 256)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.Client().Set(ctx, fmt.Sprintf("key_%d", i%100), value, time.Hour).Err()
	}
}

func BenchmarkMemory_Set_SmallValue(b *testing.B) {
	benchmarkMemorySetWithSize(b, 100)
}

func BenchmarkMemory_Set_MediumValue(b *testing.B) {
	benchmarkMemorySetWithSize(b, 1024)
}

func BenchmarkMemory_Set_LargeValue(b *testing.B) {
	benchmarkMemorySetWithSize(b, 10240)
}

func benchmarkMemorySetWithSize(b *testing.B, size int) {
	cache, err := NewMemory(WithMemoryMaxCost(100 << 20)) // 100MB
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	value := make([]byte, size)
	for i := range value {
		value[i] = byte(i % 256)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Client().SetWithTTL(fmt.Sprintf("key_%d", i%100), value, int64(len(value)), time.Hour)
	}
}

// =============================================================================
// Loader Singleflight 基准测试
// =============================================================================

func BenchmarkLoader_Load_WithSingleflight_Parallel(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := NewRedis(client)
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()

	loader, err := NewLoader(cache, WithSingleflight(true))
	if err != nil {
		b.Fatal(err)
	}
	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("backend_value"), nil
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// 使用少量 key 触发 singleflight 合并
			_, _ = loader.Load(ctx, fmt.Sprintf("key_%d", i%10), loadFn, time.Hour)
			i++
		}
	})
}

func BenchmarkLoader_LoadHash_WithSingleflight_Parallel(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := NewRedis(client)
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()

	loader, err := NewLoader(cache, WithSingleflight(true))
	if err != nil {
		b.Fatal(err)
	}
	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("backend_value"), nil
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = loader.LoadHash(ctx, "hash", fmt.Sprintf("field_%d", i%10), loadFn, time.Hour)
			i++
		}
	})
}

// =============================================================================
// Redis Hash 基准测试
// =============================================================================

func BenchmarkRedis_HSet(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := NewRedis(client)
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()
	value := "benchmark_value"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.Client().HSet(ctx, "benchmark_hash", fmt.Sprintf("field_%d", i%100), value).Err()
	}
}

func BenchmarkRedis_HGetAll(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := NewRedis(client)
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()
	// 预设 100 个 field
	for i := 0; i < 100; i++ {
		_ = cache.Client().HSet(ctx, "benchmark_hash", fmt.Sprintf("field_%d", i), "value").Err()
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cache.Client().HGetAll(ctx, "benchmark_hash").Result()
	}
}

func BenchmarkRedis_Del(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache, err := NewRedis(client)
	if err != nil {
		b.Fatal(err)
	}
	defer cache.Close(context.Background())

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		_ = cache.Client().Set(ctx, "del_key", "value", 0).Err()
		b.StartTimer()
		_ = cache.Client().Del(ctx, "del_key").Err()
	}
}
