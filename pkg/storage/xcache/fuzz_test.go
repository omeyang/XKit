package xcache

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Redis Fuzz 测试
// =============================================================================

func FuzzRedis_SetGet(f *testing.F) {
	// 添加种子语料
	f.Add("key1", []byte("value1"))
	f.Add("", []byte("empty_key"))
	f.Add("key:with:colons", []byte("value:with:colons"))
	f.Add("中文key", []byte("中文value"))
	f.Add("key\x00null", []byte("value\x00null"))
	f.Add("long_key_"+string(make([]byte, 100)), []byte("long_value"))

	mr, err := miniredis.Run()
	require.NoError(f, err)
	f.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache, err := NewRedis(client)
	require.NoError(f, err)
	f.Cleanup(func() { _ = cache.Close() })

	f.Fuzz(func(t *testing.T, key string, value []byte) {
		if key == "" {
			return // 跳过空 key
		}

		ctx := context.Background()

		// Set - 使用底层客户端
		err := cache.Client().Set(ctx, key, value, time.Hour).Err()
		if err != nil {
			return // 忽略 set 错误
		}

		// Get - 使用底层客户端
		got, err := cache.Client().Get(ctx, key).Bytes()
		if err != nil {
			t.Errorf("Get failed after Set: %v", err)
			return
		}

		// 验证值相等
		if string(got) != string(value) {
			t.Errorf("value mismatch: got %q, want %q", got, value)
		}

		// Del - 使用底层客户端
		err = cache.Client().Del(ctx, key).Err()
		if err != nil {
			t.Errorf("Del failed: %v", err)
		}
	})
}

func FuzzRedis_HSetHGet(f *testing.F) {
	// 添加种子语料
	f.Add("hash1", "field1", []byte("value1"))
	f.Add("hash:with:colons", "field:with:colons", []byte("value"))
	f.Add("中文hash", "中文field", []byte("中文value"))

	mr, err := miniredis.Run()
	require.NoError(f, err)
	f.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache, err := NewRedis(client)
	require.NoError(f, err)
	f.Cleanup(func() { _ = cache.Close() })

	f.Fuzz(func(t *testing.T, key, field string, value []byte) {
		if key == "" || field == "" {
			return
		}

		ctx := context.Background()

		// HSet - 使用底层客户端
		err := cache.Client().HSet(ctx, key, field, value).Err()
		if err != nil {
			return
		}

		// HGet - 使用底层客户端
		got, err := cache.Client().HGet(ctx, key, field).Bytes()
		if err != nil {
			t.Errorf("HGet failed after HSet: %v", err)
			return
		}

		if string(got) != string(value) {
			t.Errorf("value mismatch: got %q, want %q", got, value)
		}

		// HDel - 使用底层客户端
		err = cache.Client().HDel(ctx, key, field).Err()
		if err != nil {
			t.Errorf("HDel failed: %v", err)
		}
	})
}

// =============================================================================
// Memory Fuzz 测试
// =============================================================================

func FuzzMemory_SetGet(f *testing.F) {
	// 添加种子语料
	f.Add("key1", []byte("value1"))
	f.Add("", []byte("empty_key"))
	f.Add("key:with:colons", []byte("value:with:colons"))
	f.Add("中文key", []byte("中文value"))

	cache, err := NewMemory(WithMemoryMaxCost(1 << 20))
	require.NoError(f, err)
	f.Cleanup(func() { cache.Close() })

	f.Fuzz(func(t *testing.T, key string, value []byte) {
		if key == "" {
			return
		}

		// Set - 使用底层客户端
		cache.Client().SetWithTTL(key, value, int64(len(value)), time.Hour)

		// Wait for ristretto to process
		cache.Wait()

		// Get - ristretto 可能因容量限制而丢弃某些条目
		got, found := cache.Client().Get(key)
		if !found {
			// 可能被驱逐，这是正常的
			return
		}

		if string(got) != string(value) {
			t.Errorf("value mismatch: got %q, want %q", got, value)
		}
	})
}

// =============================================================================
// Loader Fuzz 测试
// =============================================================================

func FuzzLoader_Load(f *testing.F) {
	// 添加种子语料
	f.Add("key1", []byte("value1"))
	f.Add("key:with:colons", []byte("value"))
	f.Add("中文key", []byte("中文value"))

	mr, err := miniredis.Run()
	require.NoError(f, err)
	f.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache, err := NewRedis(client)
	require.NoError(f, err)
	f.Cleanup(func() { _ = cache.Close() })

	loader := NewLoader(cache, WithSingleflight(true))

	// 使用计数器确保每次 fuzz 迭代使用唯一的 key
	var counter int64

	f.Fuzz(func(t *testing.T, key string, value []byte) {
		if key == "" {
			return
		}

		// 生成唯一 key，避免跨迭代缓存干扰
		uniqueKey := fmt.Sprintf("%s:%d", key, atomic.AddInt64(&counter, 1))

		ctx := context.Background()

		loadFn := func(ctx context.Context) ([]byte, error) {
			return value, nil
		}

		got, err := loader.Load(ctx, uniqueKey, loadFn, time.Hour)
		if err != nil {
			t.Errorf("Load failed: %v", err)
			return
		}

		if string(got) != string(value) {
			t.Errorf("value mismatch: got %q, want %q", got, value)
		}
	})
}

func FuzzLoader_LoadHash(f *testing.F) {
	// 添加种子语料
	f.Add("hash1", "field1", []byte("value1"))
	f.Add("hash:colons", "field:colons", []byte("value"))

	mr, err := miniredis.Run()
	require.NoError(f, err)
	f.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache, err := NewRedis(client)
	require.NoError(f, err)
	f.Cleanup(func() { _ = cache.Close() })

	loader := NewLoader(cache, WithSingleflight(true))

	// 使用计数器确保每次 fuzz 迭代使用唯一的 key
	var counter int64

	f.Fuzz(func(t *testing.T, key, field string, value []byte) {
		if key == "" || field == "" {
			return
		}

		// 生成唯一 key，避免跨迭代缓存干扰
		uniqueKey := fmt.Sprintf("%s:%d", key, atomic.AddInt64(&counter, 1))

		ctx := context.Background()

		loadFn := func(ctx context.Context) ([]byte, error) {
			return value, nil
		}

		got, err := loader.LoadHash(ctx, uniqueKey, field, loadFn, time.Hour)
		if err != nil {
			t.Errorf("LoadHash failed: %v", err)
			return
		}

		if string(got) != string(value) {
			t.Errorf("value mismatch: got %q, want %q", got, value)
		}
	})
}
