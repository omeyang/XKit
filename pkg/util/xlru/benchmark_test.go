package xlru

import (
	"fmt"
	"testing"
	"time"
)

// =============================================================================
// 基本操作基准测试
// =============================================================================

func BenchmarkCache_Get(b *testing.B) {
	cache, err := New[string, int](Config{Size: 1000, TTL: time.Minute})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { cache.Close() })

	cache.Set("benchmark_key", 42)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = cache.Get("benchmark_key")
	}
}

func BenchmarkCache_Get_Miss(b *testing.B) {
	cache, err := New[string, int](Config{Size: 1000, TTL: time.Minute})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { cache.Close() })

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = cache.Get("nonexistent")
	}
}

func BenchmarkCache_Set(b *testing.B) {
	cache, err := New[string, int](Config{Size: 10000, TTL: time.Minute})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { cache.Close() })

	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("key_%d", i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		cache.Set(keys[i%1000], i)
	}
}

func BenchmarkCache_Set_Eviction(b *testing.B) {
	cache, err := New[string, int](Config{Size: 100, TTL: time.Minute})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { cache.Close() })

	// 预填充缓存
	for i := range 100 {
		cache.Set(fmt.Sprintf("pre_%d", i), i)
	}

	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("new_%d", i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		cache.Set(keys[i%1000], i)
	}
}

func BenchmarkCache_Contains(b *testing.B) {
	cache, err := New[string, int](Config{Size: 1000, TTL: time.Minute})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { cache.Close() })

	cache.Set("benchmark_key", 42)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = cache.Contains("benchmark_key")
	}
}

func BenchmarkCache_Delete(b *testing.B) {
	cache, err := New[string, int](Config{Size: 10000, TTL: time.Minute})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { cache.Close() })

	cache.Set("del_key", 42)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		cache.Delete("del_key")
		cache.Set("del_key", 42)
	}
}

func BenchmarkCache_Len(b *testing.B) {
	cache, err := New[string, int](Config{Size: 1000, TTL: time.Minute})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { cache.Close() })

	for i := range 500 {
		cache.Set(fmt.Sprintf("key_%d", i), i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = cache.Len()
	}
}

func BenchmarkCache_Keys(b *testing.B) {
	cache, err := New[string, int](Config{Size: 1000, TTL: time.Minute})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { cache.Close() })

	for i := range 100 {
		cache.Set(fmt.Sprintf("key_%d", i), i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = cache.Keys()
	}
}

// =============================================================================
// 并发基准测试
// =============================================================================

func BenchmarkCache_Get_Parallel(b *testing.B) {
	cache, err := New[string, int](Config{Size: 1000, TTL: time.Minute})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { cache.Close() })

	keys := make([]string, 100)
	for i := range keys {
		keys[i] = fmt.Sprintf("key_%d", i)
		cache.Set(keys[i], i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = cache.Get(keys[i%100])
			i++
		}
	})
}

func BenchmarkCache_Set_Parallel(b *testing.B) {
	cache, err := New[string, int](Config{Size: 10000, TTL: time.Minute})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { cache.Close() })

	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("key_%d", i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			cache.Set(keys[i%1000], i)
			i++
		}
	})
}

func BenchmarkCache_SetAndGet_Parallel(b *testing.B) {
	cache, err := New[string, int](Config{Size: 1000, TTL: time.Minute})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { cache.Close() })

	keys := make([]string, 100)
	for i := range keys {
		keys[i] = fmt.Sprintf("key_%d", i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				cache.Set(keys[i%100], i)
			} else {
				_, _ = cache.Get(keys[i%100])
			}
			i++
		}
	})
}

// =============================================================================
// 不同键类型基准测试
// =============================================================================

func BenchmarkCache_IntKey_Get(b *testing.B) {
	cache, err := New[int, int](Config{Size: 1000, TTL: time.Minute})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { cache.Close() })

	cache.Set(42, 100)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = cache.Get(42)
	}
}

func BenchmarkCache_IntKey_Set(b *testing.B) {
	cache, err := New[int, int](Config{Size: 10000, TTL: time.Minute})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { cache.Close() })

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		cache.Set(i%1000, i)
	}
}

// =============================================================================
// 不同值大小基准测试
// =============================================================================

func BenchmarkCache_Set_SmallValue(b *testing.B) {
	benchmarkCacheSetWithSize(b, 100) // 100 bytes
}

func BenchmarkCache_Set_MediumValue(b *testing.B) {
	benchmarkCacheSetWithSize(b, 1024) // 1 KB
}

func BenchmarkCache_Set_LargeValue(b *testing.B) {
	benchmarkCacheSetWithSize(b, 10240) // 10 KB
}

func benchmarkCacheSetWithSize(b *testing.B, size int) {
	cache, err := New[string, []byte](Config{Size: 1000, TTL: time.Minute})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { cache.Close() })

	value := make([]byte, size)
	for i := range value {
		value[i] = byte(i % 256)
	}

	keys := make([]string, 100)
	for i := range keys {
		keys[i] = fmt.Sprintf("key_%d", i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		cache.Set(keys[i%100], value)
	}
}

// =============================================================================
// TTL 相关基准测试
// =============================================================================

func BenchmarkCache_NoTTL_Get(b *testing.B) {
	cache, err := New[string, int](Config{Size: 1000, TTL: 0}) // 无 TTL
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { cache.Close() })

	cache.Set("benchmark_key", 42)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = cache.Get("benchmark_key")
	}
}

func BenchmarkCache_NoTTL_Set(b *testing.B) {
	cache, err := New[string, int](Config{Size: 10000, TTL: 0}) // 无 TTL
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { cache.Close() })

	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("key_%d", i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		cache.Set(keys[i%1000], i)
	}
}
