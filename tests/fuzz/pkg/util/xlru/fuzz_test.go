package xlru

import (
	"testing"
	"time"
)

func FuzzCache(f *testing.F) {
	// 种子语料：覆盖不同操作类型
	f.Add("key1", 100, uint8(0))
	f.Add("", 0, uint8(1))
	f.Add("key2", -1, uint8(2))
	f.Add("key3", 42, uint8(3))
	f.Add("key4", 999, uint8(4))
	f.Add("key5", 0, uint8(5))

	// 设计决策: 共享 Cache 实例（而非每次迭代创建新实例），以测试 Cache 在长期
	// 并发使用下的稳定性。Cache 是并发安全的，Close 后操作安全降级。
	cache, err := New[string, int](Config{Size: 100, TTL: time.Minute})
	if err != nil {
		f.Fatalf("New failed: %v", err)
	}
	f.Cleanup(func() { cache.Close() })

	f.Fuzz(func(t *testing.T, key string, value int, op uint8) {
		switch op % 6 {
		case 0:
			cache.Set(key, value)
		case 1:
			cache.Get(key)
		case 2:
			cache.Delete(key)
		case 3:
			cache.Contains(key)
		case 4:
			cache.Peek(key)
		case 5:
			cache.Len()
		}
	})
}

func FuzzNew(f *testing.F) {
	f.Add(1, int64(time.Minute))
	f.Add(0, int64(0))
	f.Add(-1, int64(-time.Second))
	f.Add(maxSize+1, int64(time.Hour))

	f.Fuzz(func(t *testing.T, size int, ttlNanos int64) {
		ttl := time.Duration(ttlNanos)
		cache, err := New[string, int](Config{Size: size, TTL: ttl})
		if err != nil {
			return
		}
		// 基本操作不应 panic
		cache.Set("k", 1)
		cache.Get("k")
		cache.Peek("k")
		cache.Contains("k")
		cache.Len()
		cache.Keys()
		cache.Delete("k")
		cache.Close()
	})
}
