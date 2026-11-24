package xlru

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr error
	}{
		{"valid", Config{Size: 10, TTL: time.Minute}, nil},
		{"zero size", Config{Size: 0}, ErrInvalidSize},
		{"negative size", Config{Size: -1}, ErrInvalidSize},
		{"exceeds max", Config{Size: maxSize + 1}, ErrSizeExceedsMax},
		{"max size", Config{Size: maxSize}, nil},
		{"negative TTL", Config{Size: 1, TTL: -time.Second}, ErrInvalidTTL},
		{"TTL too small", Config{Size: 1, TTL: time.Nanosecond}, ErrTTLTooSmall},
		{"TTL at min boundary", Config{Size: 1, TTL: 100 * time.Nanosecond}, nil},
		{"zero TTL", Config{Size: 1, TTL: 0}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

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
		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range numOperations {
				key := idx*numOperations + j
				cache.Set(key, key*2)
			}
		}()
	}

	// Concurrent reads
	for range numGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range numOperations {
				cache.Get(42)
				cache.Len()
				cache.Contains(42)
			}
		}()
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
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 200 {
				cache.Set(i, i*2)
				cache.Get(i)
			}
		}()
	}

	// 并发关闭
	wg.Add(1)
	go func() {
		defer wg.Done()
		cache.Close()
	}()

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
