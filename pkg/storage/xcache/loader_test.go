package xcache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// 测试辅助函数
// =============================================================================

// newTestRedis 创建测试用的 Redis 缓存实例。
func newTestRedis(t *testing.T) (Redis, *miniredis.Miniredis) {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr:         mr.Addr(),
		DialTimeout:  100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		WriteTimeout: 100 * time.Millisecond,
		PoolSize:     2,
		MaxRetries:   1,
	})

	cache, err := NewRedis(client)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = cache.Close()
		mr.Close()
	})

	return cache, mr
}

// =============================================================================
// Load 测试
// =============================================================================

func TestLoader_Load_WhenCacheHit_ReturnsFromCache(t *testing.T) {
	// Given
	cache, _ := newTestRedis(t)
	ctx := context.Background()
	err := cache.Client().Set(ctx, "mykey", "cached_value", 0).Err()
	require.NoError(t, err)

	loader := NewLoader(cache)
	loadCount := 0
	loadFn := func(ctx context.Context) ([]byte, error) {
		loadCount++
		return []byte("backend_value"), nil
	}

	// When
	value, err := loader.Load(ctx, "mykey", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("cached_value"), value)
	assert.Equal(t, 0, loadCount) // loadFn 不应该被调用
}

func TestLoader_Load_WhenCacheMiss_LoadsFromBackend(t *testing.T) {
	// Given
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	loader := NewLoader(cache)
	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("backend_value"), nil
	}

	// When
	value, err := loader.Load(ctx, "mykey", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("backend_value"), value)

	// 验证已写入缓存
	cached, err := cache.Client().Get(ctx, "mykey").Bytes()
	require.NoError(t, err)
	assert.Equal(t, []byte("backend_value"), cached)
}

func TestLoader_Load_WhenRedisError_FallsBackToBackend(t *testing.T) {
	// Given
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr:         mr.Addr(),
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
		PoolSize:     1,
		MaxRetries:   0,
	})
	cache, err := NewRedis(client)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	loader := NewLoader(cache)
	mr.Close() // 模拟 Redis 故障

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("backend_value"), nil
	}

	// When
	value, err := loader.Load(context.Background(), "mykey", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("backend_value"), value)
}

func TestLoader_Load_WithNilLoader_ReturnsError(t *testing.T) {
	// Given
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	loader := NewLoader(cache)

	// When
	_, err := loader.Load(ctx, "mykey", nil, time.Hour)

	// Then
	assert.ErrorIs(t, err, ErrNilLoader)
}

func TestLoader_Load_WhenBackendFails_ReturnsError(t *testing.T) {
	// Given
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	loader := NewLoader(cache)
	expectedErr := errors.New("backend error")
	loadFn := func(ctx context.Context) ([]byte, error) {
		return nil, expectedErr
	}

	// When
	_, err := loader.Load(ctx, "mykey", loadFn, time.Hour)

	// Then
	assert.ErrorIs(t, err, expectedErr)
}

func TestLoader_Load_WithSingleflight_PreventsThunderingHerd(t *testing.T) {
	// Given
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	loader := NewLoader(cache, WithSingleflight(true))
	var loadCount int32
	loadFn := func(ctx context.Context) ([]byte, error) {
		atomic.AddInt32(&loadCount, 1)
		time.Sleep(50 * time.Millisecond) // 模拟慢速后端
		return []byte("value"), nil
	}

	// When - 并发请求同一个 key
	var wg sync.WaitGroup
	results := make([][]byte, 10)
	errs := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = loader.Load(ctx, "mykey", loadFn, time.Hour)
		}(i)
	}
	wg.Wait()

	// Then
	for i := 0; i < 10; i++ {
		require.NoError(t, errs[i])
		assert.Equal(t, []byte("value"), results[i])
	}
	// loadFn 应该只被调用一次
	assert.Equal(t, int32(1), atomic.LoadInt32(&loadCount))
}

func TestLoader_Load_WithoutSingleflight_AllowsDuplicateLoads(t *testing.T) {
	// Given
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	loader := NewLoader(cache, WithSingleflight(false))
	var loadCount int32
	loadFn := func(ctx context.Context) ([]byte, error) {
		atomic.AddInt32(&loadCount, 1)
		time.Sleep(50 * time.Millisecond)
		return []byte("value"), nil
	}

	// When - 并发请求同一个 key
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = loader.Load(ctx, "mykey", loadFn, time.Hour)
		}()
	}
	wg.Wait()

	// Then - loadFn 可能被多次调用
	assert.Greater(t, atomic.LoadInt32(&loadCount), int32(1))
}

// =============================================================================
// LoadHash 测试
// =============================================================================

func TestLoader_LoadHash_WhenCacheHit_ReturnsFromCache(t *testing.T) {
	// Given
	cache, _ := newTestRedis(t)
	ctx := context.Background()
	err := cache.Client().HSet(ctx, "myhash", "field1", "cached_value").Err()
	require.NoError(t, err)

	loader := NewLoader(cache)
	loadCount := 0
	loadFn := func(ctx context.Context) ([]byte, error) {
		loadCount++
		return []byte("backend_value"), nil
	}

	// When
	value, err := loader.LoadHash(ctx, "myhash", "field1", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("cached_value"), value)
	assert.Equal(t, 0, loadCount)
}

func TestLoader_LoadHash_WhenCacheMiss_LoadsFromBackend(t *testing.T) {
	// Given
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	loader := NewLoader(cache)
	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("backend_value"), nil
	}

	// When
	value, err := loader.LoadHash(ctx, "myhash", "field1", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("backend_value"), value)

	// 验证已写入缓存
	cached, err := cache.Client().HGet(ctx, "myhash", "field1").Bytes()
	require.NoError(t, err)
	assert.Equal(t, []byte("backend_value"), cached)
}

func TestLoader_LoadHash_WhenRedisError_FallsBackToBackend(t *testing.T) {
	// Given
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr:         mr.Addr(),
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
		PoolSize:     1,
		MaxRetries:   0,
	})
	cache, err := NewRedis(client)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	loader := NewLoader(cache)
	mr.Close() // 模拟 Redis 故障

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("backend_value"), nil
	}

	// When
	value, err := loader.LoadHash(context.Background(), "myhash", "field1", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("backend_value"), value)
}

func TestLoader_LoadHash_WithNilLoader_ReturnsError(t *testing.T) {
	// Given
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	loader := NewLoader(cache)

	// When
	_, err := loader.LoadHash(ctx, "myhash", "field1", nil, time.Hour)

	// Then
	assert.ErrorIs(t, err, ErrNilLoader)
}

func TestLoader_LoadHash_WithSingleflight_PreventsThunderingHerd(t *testing.T) {
	// Given
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	loader := NewLoader(cache, WithSingleflight(true))
	var loadCount int32
	loadFn := func(ctx context.Context) ([]byte, error) {
		atomic.AddInt32(&loadCount, 1)
		time.Sleep(50 * time.Millisecond)
		return []byte("value"), nil
	}

	// When - 并发请求同一个 key:field
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = loader.LoadHash(ctx, "myhash", "field1", loadFn, time.Hour)
		}()
	}
	wg.Wait()

	// Then
	assert.Equal(t, int32(1), atomic.LoadInt32(&loadCount))
}

// =============================================================================
// hashFieldKey 函数测试
// =============================================================================

func TestHashFieldKey_NoCollision(t *testing.T) {
	// 测试可能产生碰撞的 key/field 组合
	testCases := []struct {
		key1, field1 string
		key2, field2 string
	}{
		// 碰撞场景：如果使用简单的 key + ":" + field 拼接，这些会产生相同结果
		{"user", "profile:name", "user:profile", "name"},
		{"a", "b:c", "a:b", "c"},
		{"", "a:b", "a", "b"},
		{"key:", "field", "key", ":field"},
	}

	for _, tc := range testCases {
		sfKey1 := hashFieldKey(tc.key1, tc.field1)
		sfKey2 := hashFieldKey(tc.key2, tc.field2)
		assert.NotEqual(t, sfKey1, sfKey2,
			"hashFieldKey should not collide: (%q,%q) vs (%q,%q)",
			tc.key1, tc.field1, tc.key2, tc.field2)
	}
}

func TestHashFieldKey_Format(t *testing.T) {
	// 验证生成的 key 格式正确
	key := hashFieldKey("user", "profile")
	assert.Equal(t, "4:user:profile", key)

	key = hashFieldKey("user:profile", "name")
	assert.Equal(t, "12:user:profile:name", key)
}
