package xlru

import (
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"go.uber.org/goleak"
)

func TestNew(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cache, err := New[string, int](Config{Size: 10, TTL: time.Minute})
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}
		defer cache.Close()
		if cache == nil {
			t.Fatal("cache should not be nil")
		}
	})

	t.Run("zero size", func(t *testing.T) {
		_, err := New[string, int](Config{Size: 0})
		if !errors.Is(err, ErrInvalidSize) {
			t.Errorf("expected ErrInvalidSize, got %v", err)
		}
	})

	t.Run("negative size", func(t *testing.T) {
		_, err := New[string, int](Config{Size: -1})
		if !errors.Is(err, ErrInvalidSize) {
			t.Errorf("expected ErrInvalidSize, got %v", err)
		}
	})

	t.Run("TTL too small", func(t *testing.T) {
		_, err := New[string, int](Config{Size: 10, TTL: 1 * time.Nanosecond})
		if !errors.Is(err, ErrTTLTooSmall) {
			t.Errorf("expected ErrTTLTooSmall, got %v", err)
		}
	})

	t.Run("TTL at minimum boundary", func(t *testing.T) {
		cache, err := New[string, int](Config{Size: 10, TTL: 100 * time.Nanosecond})
		if err != nil {
			t.Fatalf("New with minTTL should succeed: %v", err)
		}
		defer cache.Close()
		if cache == nil {
			t.Fatal("cache should not be nil")
		}
	})

	t.Run("zero TTL (no expiration)", func(t *testing.T) {
		cache, err := New[string, int](Config{Size: 10, TTL: 0})
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}
		defer cache.Close()
		if cache == nil {
			t.Fatal("cache should not be nil")
		}
	})
}

func TestCache_SetAndGet(t *testing.T) {
	cache, err := New[string, int](Config{Size: 10, TTL: time.Minute})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	t.Run("set and get", func(t *testing.T) {
		cache.Set("key1", 100)

		val, ok := cache.Get("key1")
		if !ok {
			t.Fatal("expected key to exist")
		}
		if val != 100 {
			t.Errorf("val = %d, expected 100", val)
		}
	})

	t.Run("get nonexistent", func(t *testing.T) {
		val, ok := cache.Get("nonexistent")
		if ok {
			t.Error("expected key to not exist")
		}
		if val != 0 {
			t.Errorf("val = %d, expected zero value", val)
		}
	})

	t.Run("overwrite", func(t *testing.T) {
		cache.Set("key2", 200)
		cache.Set("key2", 300)

		val, ok := cache.Get("key2")
		if !ok {
			t.Fatal("expected key to exist")
		}
		if val != 300 {
			t.Errorf("val = %d, expected 300", val)
		}
	})
}

func TestCache_Delete(t *testing.T) {
	cache, err := New[string, int](Config{Size: 10, TTL: time.Minute})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	cache.Set("key1", 100)

	t.Run("delete existing", func(t *testing.T) {
		deleted := cache.Delete("key1")
		if !deleted {
			t.Error("expected delete to return true")
		}

		_, ok := cache.Get("key1")
		if ok {
			t.Error("key should not exist after delete")
		}
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		deleted := cache.Delete("nonexistent")
		if deleted {
			t.Error("expected delete to return false for nonexistent key")
		}
	})
}

func TestCache_Clear(t *testing.T) {
	cache, err := New[string, int](Config{Size: 10, TTL: time.Minute})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	cache.Set("key1", 100)
	cache.Set("key2", 200)
	cache.Set("key3", 300)

	if cache.Len() != 3 {
		t.Errorf("len = %d, expected 3", cache.Len())
	}

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("len = %d, expected 0 after clear", cache.Len())
	}

	_, ok := cache.Get("key1")
	if ok {
		t.Error("key1 should not exist after clear")
	}
}

func TestCache_Len(t *testing.T) {
	cache, err := New[string, int](Config{Size: 10, TTL: time.Minute})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	if cache.Len() != 0 {
		t.Errorf("len = %d, expected 0", cache.Len())
	}

	cache.Set("key1", 100)
	if cache.Len() != 1 {
		t.Errorf("len = %d, expected 1", cache.Len())
	}

	cache.Set("key2", 200)
	if cache.Len() != 2 {
		t.Errorf("len = %d, expected 2", cache.Len())
	}

	cache.Delete("key1")
	if cache.Len() != 1 {
		t.Errorf("len = %d, expected 1", cache.Len())
	}
}

func TestCache_Contains(t *testing.T) {
	cache, err := New[string, int](Config{Size: 10, TTL: time.Minute})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	cache.Set("key1", 100)

	if !cache.Contains("key1") {
		t.Error("expected Contains to return true for existing key")
	}

	if cache.Contains("nonexistent") {
		t.Error("expected Contains to return false for nonexistent key")
	}
}

func TestCache_Keys(t *testing.T) {
	cache, err := New[string, int](Config{Size: 10, TTL: time.Minute})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	cache.Set("key1", 100)
	cache.Set("key2", 200)
	cache.Set("key3", 300)

	keys := cache.Keys()
	if len(keys) != 3 {
		t.Errorf("len(keys) = %d, expected 3", len(keys))
	}

	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}

	if !keySet["key1"] || !keySet["key2"] || !keySet["key3"] {
		t.Error("missing expected keys")
	}
}

func TestCache_TTLExpiration(t *testing.T) {
	cache, err := New[string, int](Config{Size: 10, TTL: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	cache.Set("key1", 100)

	// Immediately accessible
	val, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected key to exist immediately after set")
	}
	if val != 100 {
		t.Errorf("val = %d, expected 100", val)
	}

	// Wait for expiration (3x margin for CI stability)
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	_, ok = cache.Get("key1")
	if ok {
		t.Error("expected key to be expired")
	}
}

func TestCache_LRUEviction(t *testing.T) {
	var evictedKeys []string
	cache, err := New(Config{Size: 3, TTL: time.Minute},
		WithOnEvicted(func(key string, _ int) {
			evictedKeys = append(evictedKeys, key)
		}))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	// Fill cache
	cache.Set("key1", 100)
	cache.Set("key2", 200)
	cache.Set("key3", 300)

	// Access key1 to make it recently used
	cache.Get("key1")

	// Add new key, should evict least recently used (key2)
	cache.Set("key4", 400)

	if cache.Len() != 3 {
		t.Errorf("len = %d, expected 3", cache.Len())
	}

	// key2 should be evicted (least recently used)
	if cache.Contains("key2") {
		t.Error("key2 should have been evicted")
	}

	// key1, key3, key4 should still exist
	if !cache.Contains("key1") {
		t.Error("key1 should exist")
	}
	if !cache.Contains("key3") {
		t.Error("key3 should exist")
	}
	if !cache.Contains("key4") {
		t.Error("key4 should exist")
	}

	// Check eviction callback was called
	if len(evictedKeys) != 1 || evictedKeys[0] != "key2" {
		t.Errorf("evictedKeys = %v, expected [key2]", evictedKeys)
	}
}

func TestCache_WithOnEvicted(t *testing.T) {
	var evictCount atomic.Int32
	cache, err := New(Config{Size: 2, TTL: time.Minute},
		WithOnEvicted(func(_ string, _ int) {
			evictCount.Add(1)
		}))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	cache.Set("key1", 100)
	cache.Set("key2", 200)
	cache.Set("key3", 300) // Should trigger eviction

	if evictCount.Load() != 1 {
		t.Errorf("evictCount = %d, expected 1", evictCount.Load())
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	cache, err := New[int, int](Config{Size: 1000, TTL: time.Minute})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	var wg sync.WaitGroup
	numGoroutines := 100
	numOperations := 1000

	// Concurrent writes
	for i := range numGoroutines {
		wg.Go(func() {
			for j := range numOperations {
				key := i*numOperations + j
				cache.Set(key, key*2)
			}
		})
	}

	// Concurrent reads
	for range numGoroutines {
		wg.Go(func() {
			for range numOperations {
				cache.Get(42)
				cache.Len()
				cache.Contains(42)
			}
		})
	}

	wg.Wait()

	// Should complete without race conditions or panics
	// The test passes if we get here without issues
}

func TestCache_Close_ConcurrentSetGet(t *testing.T) {
	cache, err := New[int, int](Config{Size: 1000, TTL: time.Minute})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// 预填充
	for i := range 100 {
		cache.Set(i, i)
	}

	var wg sync.WaitGroup

	// 并发读写
	for range 50 {
		wg.Go(func() {
			for i := range 200 {
				cache.Set(i, i*2)
				cache.Get(i)
			}
		})
	}

	// 并发关闭
	wg.Go(func() {
		cache.Close()
	})

	wg.Wait()

	// Close 后所有操作应安全降级
	if cache.Len() != 0 {
		t.Errorf("Len after Close = %d, expected 0", cache.Len())
	}
	val, ok := cache.Get(1)
	if ok {
		t.Error("Get after Close should return false")
	}
	if val != 0 {
		t.Errorf("Get after Close should return zero value, got %d", val)
	}
}

func TestCache_ZeroTTL(t *testing.T) {
	cache, err := New[string, int](Config{Size: 10, TTL: 0})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	cache.Set("key1", 100)

	// With zero TTL, items should never expire
	time.Sleep(50 * time.Millisecond)

	val, ok := cache.Get("key1")
	if !ok {
		t.Error("expected key to still exist with zero TTL")
	}
	if val != 100 {
		t.Errorf("val = %d, expected 100", val)
	}
}

func TestCache_SetReturnValue(t *testing.T) {
	cache, err := New[string, int](Config{Size: 2, TTL: time.Minute})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	// First set should not evict
	evicted := cache.Set("key1", 100)
	if evicted {
		t.Error("first set should not cause eviction")
	}

	// Second set should not evict
	evicted = cache.Set("key2", 200)
	if evicted {
		t.Error("second set should not cause eviction")
	}

	// Third set should cause eviction (cache is full)
	evicted = cache.Set("key3", 300)
	if !evicted {
		t.Error("third set should cause eviction")
	}

	// Update existing key does not trigger eviction callback
	evicted = cache.Set("key3", 350)
	if evicted {
		t.Error("update existing key should not indicate eviction")
	}
}

func TestCache_PointerValues(t *testing.T) {
	type Data struct {
		Name  string
		Value int
	}

	cache, err := New[string, *Data](Config{Size: 10, TTL: time.Minute})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	data := &Data{Name: "test", Value: 42}
	cache.Set("key1", data)

	retrieved, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected key to exist")
	}
	if retrieved.Name != "test" || retrieved.Value != 42 {
		t.Errorf("retrieved = %+v, expected {Name: test, Value: 42}", retrieved)
	}

	// Verify it's the same pointer
	if retrieved != data {
		t.Error("expected same pointer")
	}
}

func TestCache_IntKeys(t *testing.T) {
	cache, err := New[int, string](Config{Size: 10, TTL: time.Minute})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	cache.Set(1, "one")
	cache.Set(2, "two")
	cache.Set(3, "three")

	val, ok := cache.Get(2)
	if !ok {
		t.Fatal("expected key to exist")
	}
	if val != "two" {
		t.Errorf("val = %q, expected 'two'", val)
	}
}

func TestCache_EmptyKeys(t *testing.T) {
	cache, err := New[string, int](Config{Size: 10, TTL: time.Minute})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	keys := cache.Keys()
	if len(keys) != 0 {
		t.Errorf("len(keys) = %d, expected 0", len(keys))
	}
}

func TestNew_NilOption(t *testing.T) {
	// nil Option 不应导致 panic
	cache, err := New[string, int](Config{Size: 10, TTL: time.Minute}, nil)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if cache == nil {
		t.Fatal("cache should not be nil")
	}
	cache.Close()

	// 多个 Option 包含 nil
	var called bool
	cache, err = New(Config{Size: 10, TTL: time.Minute},
		nil,
		WithOnEvicted(func(_ string, _ int) { called = true }),
		nil,
	)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// 验证非 nil Option 仍正常工作
	cache.Set("key1", 1)
	cache.Set("key2", 2)
	cache.Set("key3", 3) // Size=10，不会触发淘汰
	cache.Close()

	// 创建小缓存验证回调
	cache, err = New(Config{Size: 2, TTL: time.Minute},
		nil,
		WithOnEvicted(func(_ string, _ int) { called = true }),
	)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	cache.Set("a", 1)
	cache.Set("b", 2)
	cache.Set("c", 3) // 触发淘汰

	if !called {
		t.Error("OnEvicted callback should have been called")
	}
}

func TestCache_Close(t *testing.T) {
	cache, err := New[string, int](Config{Size: 10, TTL: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	cache.Set("key1", 100)
	cache.Set("key2", 200)

	// Close should purge and stop goroutine
	cache.Close()

	if cache.Len() != 0 {
		t.Errorf("len = %d, expected 0 after close", cache.Len())
	}
}

func TestCache_Close_Idempotent(t *testing.T) {
	cache, err := New[string, int](Config{Size: 10, TTL: time.Minute})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// 多次 Close 不应 panic
	cache.Close()
	cache.Close()
	cache.Close()
}

func TestCache_Close_ThenUse(t *testing.T) {
	// 验证 Close 后所有操作安全降级：读返回零值/false，写静默忽略
	cache, err := New[string, int](Config{Size: 10, TTL: time.Minute})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	cache.Set("key1", 100)
	cache.Close()

	// Get 返回零值和 false
	val, ok := cache.Get("key1")
	if ok {
		t.Error("Get after Close should return false")
	}
	if val != 0 {
		t.Errorf("Get after Close should return zero value, got %d", val)
	}

	// Set 静默忽略
	evicted := cache.Set("key2", 200)
	if evicted {
		t.Error("Set after Close should return false")
	}

	// Delete 返回 false
	if cache.Delete("key1") {
		t.Error("Delete after Close should return false")
	}

	// Contains 返回 false
	if cache.Contains("key1") {
		t.Error("Contains after Close should return false")
	}

	// Len 返回 0
	if cache.Len() != 0 {
		t.Errorf("Len after Close should return 0, got %d", cache.Len())
	}

	// Keys 返回 nil
	if cache.Keys() != nil {
		t.Error("Keys after Close should return nil")
	}

	// Peek 返回零值和 false
	val, ok = cache.Peek("key1")
	if ok {
		t.Error("Peek after Close should return false")
	}
	if val != 0 {
		t.Errorf("Peek after Close should return zero value, got %d", val)
	}

	// Clear 不应 panic
	cache.Clear()
}

func TestCache_Close_ZeroTTL(t *testing.T) {
	// TTL=0 时无清理 goroutine，Close 仍应正常工作
	cache, err := New[string, int](Config{Size: 10, TTL: 0})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	cache.Set("key1", 100)
	cache.Close()

	if cache.Len() != 0 {
		t.Errorf("len = %d, expected 0 after close", cache.Len())
	}
}

func TestNew_NegativeTTL(t *testing.T) {
	_, err := New[string, int](Config{Size: 10, TTL: -1 * time.Second})
	if !errors.Is(err, ErrInvalidTTL) {
		t.Errorf("expected ErrInvalidTTL, got %v", err)
	}
}

func TestNew_SizeExceedsMax(t *testing.T) {
	_, err := New[string, int](Config{Size: maxSize + 1, TTL: time.Minute})
	if !errors.Is(err, ErrSizeExceedsMax) {
		t.Errorf("expected ErrSizeExceedsMax, got %v", err)
	}

	// 恰好等于上限应该成功
	cache, err := New[string, int](Config{Size: maxSize, TTL: 0})
	if err != nil {
		t.Fatalf("New with maxSize should succeed: %v", err)
	}
	if cache == nil {
		t.Fatal("cache should not be nil")
	}
	cache.Close()
}

func TestCache_Peek(t *testing.T) {
	cache, err := New[string, int](Config{Size: 3, TTL: time.Minute})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	cache.Set("key1", 100)
	cache.Set("key2", 200)
	cache.Set("key3", 300)

	// Peek 应返回正确的值
	val, ok := cache.Peek("key1")
	if !ok {
		t.Fatal("expected key to exist")
	}
	if val != 100 {
		t.Errorf("val = %d, expected 100", val)
	}

	// Peek 不应更新 LRU 顺序：key1 仍然是最久未通过 Get 访问的
	// 添加新条目应淘汰 key1（因为 Peek 不提升优先级）
	cache.Set("key4", 400)

	if cache.Contains("key1") {
		t.Error("key1 should have been evicted (Peek does not update LRU order)")
	}

	// Peek 不存在的键
	val, ok = cache.Peek("nonexistent")
	if ok {
		t.Error("expected Peek to return false for nonexistent key")
	}
	if val != 0 {
		t.Errorf("val = %d, expected zero value", val)
	}
}

func TestCache_TTLExpiration_SemanticDifference(t *testing.T) {
	// 验证 TTL 过期语义：Get/Peek/Contains 过滤过期条目，Len/Keys 不过滤（延迟清理）。
	cache, err := New[string, int](Config{Size: 10, TTL: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	cache.Set("key1", 100)

	// 等待过期（但后台清理可能尚未执行）
	time.Sleep(80 * time.Millisecond)

	// Get 应返回 miss（过滤过期）
	_, ok := cache.Get("key1")
	if ok {
		t.Error("Get should return false for expired key")
	}

	// Peek 也应返回 miss（过滤过期）
	_, ok = cache.Peek("key1")
	if ok {
		t.Error("Peek should return false for expired key")
	}

	// Contains 也应返回 false（内部使用 Peek，过滤过期）
	if cache.Contains("key1") {
		t.Error("Contains should return false for expired key")
	}

	// Len/Keys 可能仍包含已过期条目（延迟清理语义）——
	// 这是底层库的已知行为，不视为 bug。
	// 不做断言，因为后台清理时机不确定。
}

func TestCache_SetRefreshesTTL(t *testing.T) {
	cache, err := New[string, int](Config{Size: 10, TTL: 80 * time.Millisecond})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cache.Close()

	cache.Set("key1", 100)

	// 等待 50ms，然后重新 Set 刷新 TTL
	time.Sleep(50 * time.Millisecond)
	cache.Set("key1", 200)

	// 再等 50ms（距第一次 Set 已 100ms，超过 80ms TTL）
	time.Sleep(50 * time.Millisecond)

	// 由于 Set 刷新了 TTL，key1 仍应存在（距第二次 Set 仅 50ms < 80ms）
	val, ok := cache.Get("key1")
	if !ok {
		t.Error("expected key1 to exist (Set should refresh TTL)")
	}
	if val != 200 {
		t.Errorf("val = %d, expected 200", val)
	}
}

func TestCache_Close_StopsCleanupGoroutine(t *testing.T) {
	// 使用 goleak 验证 Close 后清理 goroutine 确实退出。
	// 所有测试均已正确调用 Close()，无需 IgnoreTopFunction 过滤。
	defer goleak.VerifyNone(t)

	var evictCount atomic.Int32
	cache, err := New(Config{Size: 100, TTL: 50 * time.Millisecond},
		WithOnEvicted(func(_ string, _ int) {
			evictCount.Add(1)
		}))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	cache.Set("key1", 100)
	cache.Close()

	// Close 会 Purge 所有条目（触发 onEvicted）
	if evictCount.Load() != 1 {
		t.Errorf("expected 1 eviction from Close/Purge, got %d", evictCount.Load())
	}
}

func TestCache_OnEvicted_AsyncPattern(t *testing.T) {
	// 验证 OnEvicted 回调的推荐异步模式：
	// 回调将事件发送到 channel，由外部消费者处理。
	// 注意：回调中严禁调用 Cache 自身方法（会死锁）。
	type evictEvent struct {
		key   string
		value int
	}

	evictCh := make(chan evictEvent, 10)
	cache, err := New(Config{Size: 2, TTL: time.Minute},
		WithOnEvicted(func(key string, value int) {
			evictCh <- evictEvent{key, value}
		}))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	cache.Set("a", 1)
	cache.Set("b", 2)
	cache.Set("c", 3) // 淘汰 "a"

	select {
	case ev := <-evictCh:
		if ev.key != "a" || ev.value != 1 {
			t.Errorf("expected eviction of a=1, got %s=%d", ev.key, ev.value)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for eviction event")
	}

	cache.Close()
}

func TestStopCleanupGoroutine_EdgeCases(t *testing.T) {
	// nil 输入不应 panic，应返回 false
	if stopCleanupGoroutine(nil) {
		t.Error("nil input should return false")
	}

	// 非指针输入不应 panic，应返回 false
	if stopCleanupGoroutine(42) {
		t.Error("non-pointer input should return false")
	}

	// 无 done 字段的结构体不应 panic，应返回 false
	type noDone struct{ Name string }
	if stopCleanupGoroutine(&noDone{Name: "test"}) {
		t.Error("struct without done field should return false")
	}

	// done 字段类型不匹配（非 nilable 类型）不应 panic，应返回 false
	type wrongDone struct{ done int }
	if stopCleanupGoroutine(&wrongDone{done: 1}) {
		t.Error("struct with wrong done type should return false")
	}

	// done 字段类型不匹配（nilable 但非 chan struct{}），应经过类型检查返回 false
	type wrongChanDone struct{ done chan int }
	if stopCleanupGoroutine(&wrongChanDone{done: make(chan int)}) {
		t.Error("struct with chan int done should return false")
	}

	// done 字段类型正确（chan struct{}）但为 nil，应返回 false
	// 复用 hasDone 类型（下方定义），零值即 done == nil
	if stopCleanupGoroutine(&struct{ done chan struct{} }{}) {
		t.Error("struct with nil chan struct{} done should return false")
	}

	// 二次调用触发 recover（done 通道已关闭）：应返回 false 而非 panic
	type hasDone struct{ done chan struct{} }
	s := &hasDone{done: make(chan struct{})}
	if !stopCleanupGoroutine(s) {
		t.Error("first call should return true")
	}
	if stopCleanupGoroutine(s) {
		t.Error("second call (double close) should return false via recover")
	}
}

func TestStopCleanupGoroutine_UpstreamStructAssert(t *testing.T) {
	// 维护须知: 此测试验证上游 expirable.LRU 的内部结构未发生变化。
	// 如果此测试失败，说明上游升级改变了内部布局，需要更新 stopCleanupGoroutine。
	lru := expirable.NewLRU[string, int](10, nil, time.Minute)
	defer func() {
		// 停止清理 goroutine
		stopCleanupGoroutine(lru)
	}()

	v := reflect.ValueOf(lru)
	if v.Kind() != reflect.Pointer {
		t.Fatalf("expirable.NewLRU should return pointer, got %s", v.Kind())
	}

	doneField := v.Elem().FieldByName("done")
	if !doneField.IsValid() {
		t.Fatal("upstream expirable.LRU no longer has 'done' field; stopCleanupGoroutine needs update")
	}

	expectedType := reflect.TypeOf(make(chan struct{}))
	if doneField.Type() != expectedType {
		t.Fatalf("upstream 'done' field type changed from chan struct{} to %v; stopCleanupGoroutine needs update",
			doneField.Type())
	}
}

func TestCache_Size1_Semantics(t *testing.T) {
	// 验证 Size=1 边界条件下的 LRU 语义
	t.Run("basic set and get", func(t *testing.T) {
		cache, err := New[string, int](Config{Size: 1, TTL: time.Minute})
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}
		defer cache.Close()

		cache.Set("a", 1)
		val, ok := cache.Get("a")
		if !ok || val != 1 {
			t.Errorf("Get(a) = (%d, %v), expected (1, true)", val, ok)
		}
	})

	t.Run("set evicts previous entry", func(t *testing.T) {
		var evictedKey string
		cache, err := New(Config{Size: 1, TTL: time.Minute},
			WithOnEvicted(func(key string, _ int) {
				evictedKey = key
			}))
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}
		defer cache.Close()

		cache.Set("a", 1)
		evicted := cache.Set("b", 2)

		if !evicted {
			t.Error("Set should report eviction when cache is full")
		}
		if evictedKey != "a" {
			t.Errorf("evictedKey = %q, expected 'a'", evictedKey)
		}
		if cache.Contains("a") {
			t.Error("a should have been evicted")
		}
		val, ok := cache.Get("b")
		if !ok || val != 2 {
			t.Errorf("Get(b) = (%d, %v), expected (2, true)", val, ok)
		}
	})

	t.Run("overwrite refreshes TTL without eviction", func(t *testing.T) {
		cache, err := New[string, int](Config{Size: 1, TTL: 80 * time.Millisecond})
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}
		defer cache.Close()

		cache.Set("a", 1)
		time.Sleep(50 * time.Millisecond)
		evicted := cache.Set("a", 2) // 刷新 TTL
		if evicted {
			t.Error("overwrite should not indicate eviction")
		}

		time.Sleep(50 * time.Millisecond) // 距首次 Set 已 100ms > 80ms TTL
		val, ok := cache.Get("a")
		if !ok || val != 2 {
			t.Errorf("Get(a) = (%d, %v), expected (2, true) after TTL refresh", val, ok)
		}
	})

	t.Run("peek does not promote", func(t *testing.T) {
		cache, err := New[string, int](Config{Size: 1, TTL: time.Minute})
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}
		defer cache.Close()

		cache.Set("a", 1)
		val, ok := cache.Peek("a")
		if !ok || val != 1 {
			t.Errorf("Peek(a) = (%d, %v), expected (1, true)", val, ok)
		}

		// Peek 后再 Set 新 key，应淘汰 a（Peek 不提升优先级）
		cache.Set("b", 2)
		if cache.Contains("a") {
			t.Error("a should have been evicted (Peek does not promote)")
		}
	})
}
