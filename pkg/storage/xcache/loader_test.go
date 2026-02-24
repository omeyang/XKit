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

	loader, err := NewLoader(cache)
	require.NoError(t, err)
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

	loader, err := NewLoader(cache)
	require.NoError(t, err)
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

	loader, err := NewLoader(cache)
	require.NoError(t, err)
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

	loader, err := NewLoader(cache)
	require.NoError(t, err)

	// When
	_, err = loader.Load(ctx, "mykey", nil, time.Hour)

	// Then
	assert.ErrorIs(t, err, ErrNilLoader)
}

func TestLoader_Load_WhenBackendFails_ReturnsError(t *testing.T) {
	// Given
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	loader, err := NewLoader(cache)
	require.NoError(t, err)
	expectedErr := errors.New("backend error")
	loadFn := func(ctx context.Context) ([]byte, error) {
		return nil, expectedErr
	}

	// When
	_, err = loader.Load(ctx, "mykey", loadFn, time.Hour)

	// Then
	assert.ErrorIs(t, err, expectedErr)
}

func TestLoader_Load_WithSingleflight_PreventsThunderingHerd(t *testing.T) {
	// Given
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	loader, err := NewLoader(cache, WithSingleflight(true))
	require.NoError(t, err)
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

	loader, err := NewLoader(cache, WithSingleflight(false))
	require.NoError(t, err)
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

	loader, err := NewLoader(cache)
	require.NoError(t, err)
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

	loader, err := NewLoader(cache)
	require.NoError(t, err)
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

	loader, err := NewLoader(cache)
	require.NoError(t, err)
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

	loader, err := NewLoader(cache)
	require.NoError(t, err)

	// When
	_, err = loader.LoadHash(ctx, "myhash", "field1", nil, time.Hour)

	// Then
	assert.ErrorIs(t, err, ErrNilLoader)
}

func TestLoader_LoadHash_WithSingleflight_PreventsThunderingHerd(t *testing.T) {
	// Given
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	loader, err := NewLoader(cache, WithSingleflight(true))
	require.NoError(t, err)
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

// =============================================================================
// Loader 配置选项覆盖测试
// =============================================================================

func TestWithMaxRetryAttempts_SetsOption(t *testing.T) {
	opts := defaultLoaderOptions()
	WithMaxRetryAttempts(5)(opts)
	assert.Equal(t, 5, opts.MaxRetryAttempts)
}

func TestWithMaxRetryAttempts_IgnoresNonPositive(t *testing.T) {
	opts := defaultLoaderOptions()
	original := opts.MaxRetryAttempts
	WithMaxRetryAttempts(0)(opts)
	assert.Equal(t, original, opts.MaxRetryAttempts)
	WithMaxRetryAttempts(-1)(opts)
	assert.Equal(t, original, opts.MaxRetryAttempts)
}

func TestWithTTLJitter_NegativeValue_ClampsToZero(t *testing.T) {
	opts := defaultLoaderOptions()
	WithTTLJitter(-0.5)(opts)
	assert.Equal(t, 0.0, opts.TTLJitter)
}

func TestWithTTLJitter_OverOneValue_ClampsToOne(t *testing.T) {
	opts := defaultLoaderOptions()
	WithTTLJitter(2.0)(opts)
	assert.Equal(t, 1.0, opts.TTLJitter)
}

func TestWithHashTTLRefresh_SetsOption(t *testing.T) {
	opts := defaultLoaderOptions()
	assert.True(t, opts.HashTTLRefresh)
	WithHashTTLRefresh(false)(opts)
	assert.False(t, opts.HashTTLRefresh)
}

func TestWithOnCacheSetError_SetsOption(t *testing.T) {
	opts := defaultLoaderOptions()
	assert.Nil(t, opts.OnCacheSetError)

	called := false
	hook := func(ctx context.Context, key string, err error) {
		called = true
	}
	WithOnCacheSetError(hook)(opts)
	assert.NotNil(t, opts.OnCacheSetError)

	// 验证 hook 可被调用
	opts.OnCacheSetError(context.Background(), "key", errors.New("err"))
	assert.True(t, called)
}

func TestLoader_Load_WithOnCacheSetError_CallsHookOnSetFailure(t *testing.T) {
	// Given - Redis 关闭后写入失败，应触发 hook
	cache, mr := newTestRedis(t)
	ctx := context.Background()

	var hookKey string
	var hookErr error
	hook := func(ctx context.Context, key string, err error) {
		hookKey = key
		hookErr = err
	}

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithOnCacheSetError(hook),
	)
	require.NoError(t, err)

	// 关闭 Redis，使缓存写入失败
	mr.Close()

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("value"), nil
	}

	// When
	value, err := loader.Load(ctx, "hookkey", loadFn, time.Hour)

	// Then - 加载应成功，hook 应被调用
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), value)
	assert.Equal(t, "hookkey", hookKey)
	assert.Error(t, hookErr)
}

func TestLoader_LoadHash_WithHashTTLRefreshFalse_OnlySetsFirstTTL(t *testing.T) {
	// Given - 测试 HashTTLRefresh=false 时仅首次设置 TTL
	cache, mr := newTestRedis(t)
	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithHashTTLRefresh(false),
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("value1"), nil
	}

	// When - 第一次写入：key 不存在，TTL < 0，应设置 TTL
	value, err := loader.LoadHash(ctx, "ttl_hash", "field1", loadFn, time.Hour)
	require.NoError(t, err)
	assert.Equal(t, []byte("value1"), value)

	// 验证 TTL 被设置
	ttl := mr.TTL("ttl_hash")
	assert.True(t, ttl > 0, "TTL should be set on first write")

	// When - 第二次写入：key 已有 TTL，不应刷新
	loadFn2 := func(ctx context.Context) ([]byte, error) {
		return []byte("value2"), nil
	}
	value, err = loader.LoadHash(ctx, "ttl_hash", "field2", loadFn2, 2*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, []byte("value2"), value)
}

func TestLoader_LoadHash_WithZeroTTL_SkipsExpire(t *testing.T) {
	// Given - TTL 为 0 时不设置过期时间
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	loader, err := NewLoader(cache, WithSingleflight(false))
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("no_ttl_value"), nil
	}

	// When - 使用 TTL=0
	value, err := loader.LoadHash(ctx, "no_ttl_hash", "field1", loadFn, 0)

	// Then - 应成功，不设置 TTL
	require.NoError(t, err)
	assert.Equal(t, []byte("no_ttl_value"), value)
}

func TestLoader_LoadHash_WithNegativeTTL_SkipsCache(t *testing.T) {
	// Given - 负 TTL 时应跳过缓存写入（与 Load 行为一致）
	cache, mr := newTestRedis(t)
	ctx := context.Background()

	loader, err := NewLoader(cache, WithSingleflight(false))
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("neg_ttl_value"), nil
	}

	// When - 使用负 TTL
	value, err := loader.LoadHash(ctx, "neg_ttl_hash", "field1", loadFn, -1*time.Second)

	// Then - 回源成功但不写入缓存
	require.NoError(t, err)
	assert.Equal(t, []byte("neg_ttl_value"), value)

	// 验证未写入缓存
	assert.False(t, mr.Exists("neg_ttl_hash"), "hash key should not exist with negative TTL")
}

// =============================================================================
// 内部函数覆盖测试
// =============================================================================

func TestDetachedCtx_IsFullyDetached(t *testing.T) {
	// detachedCtx 应完全脱离原始 context 的取消链
	origCtx, cancel := context.WithCancel(context.Background())
	cancel()

	detached := contextDetached(origCtx)

	// Err 返回 nil（不继承取消）
	assert.Nil(t, detached.Err())
	// Done 返回 nil channel（不继承取消）
	assert.Nil(t, detached.Done())
	// Deadline 返回零值（不继承截止时间）
	deadline, ok := detached.Deadline()
	assert.False(t, ok)
	assert.True(t, deadline.IsZero())
}

func TestLoader_Load_WithNilCache_ReturnsError(t *testing.T) {
	// 测试 loader.cache == nil 的路径
	l := &loader{cache: nil, options: defaultLoaderOptions()}
	_, err := l.Load(context.Background(), "key", func(ctx context.Context) ([]byte, error) {
		return nil, nil
	}, time.Hour)
	assert.ErrorIs(t, err, ErrNilClient)
}

func TestLoader_LoadHash_WithNilCache_ReturnsError(t *testing.T) {
	l := &loader{cache: nil, options: defaultLoaderOptions()}
	_, err := l.LoadHash(context.Background(), "key", "field", func(ctx context.Context) ([]byte, error) {
		return nil, nil
	}, time.Hour)
	assert.ErrorIs(t, err, ErrNilClient)
}

// =============================================================================
// nil context 测试（FG-S1 修复）
// =============================================================================

func TestLoader_Load_WithNilContext_ReturnsErrNilContext(t *testing.T) {
	cache, _ := newTestRedis(t)
	loader, err := NewLoader(cache)
	require.NoError(t, err)

	//nolint:staticcheck // SA1012: 故意传入 nil context 测试 fail-fast 校验
	_, err = loader.Load(nil, "key", func(ctx context.Context) ([]byte, error) {
		return nil, nil
	}, time.Hour)
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestLoader_LoadHash_WithNilContext_ReturnsErrNilContext(t *testing.T) {
	cache, _ := newTestRedis(t)
	loader, err := NewLoader(cache)
	require.NoError(t, err)

	//nolint:staticcheck // SA1012: 故意传入 nil context 测试 fail-fast 校验
	_, err = loader.LoadHash(nil, "key", "field", func(ctx context.Context) ([]byte, error) {
		return nil, nil
	}, time.Hour)
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestBackoffWithJitter_OverflowProtection(t *testing.T) {
	// 测试 attempt 超过安全位移范围时使用 maxBackoff
	result := backoffWithJitter(100) // 远超 maxSafeShift=30
	assert.True(t, result > 0, "backoff should be positive")
	assert.LessOrEqual(t, result, maxBackoff+time.Duration(float64(maxBackoff)*jitterFraction))
}

func TestContextWithIndependentTimeout_ZeroTimeout(t *testing.T) {
	// timeout == 0 表示禁用超时
	ctx := context.Background()
	newCtx, cancel := contextWithIndependentTimeout(ctx, 0)
	defer cancel()

	_, ok := newCtx.Deadline()
	assert.False(t, ok, "zero timeout should not set deadline")
}

func TestContextWithIndependentTimeout_NegativeTimeout(t *testing.T) {
	// timeout < 0 表示使用默认超时 (30s)
	ctx := context.Background()
	newCtx, cancel := contextWithIndependentTimeout(ctx, -1*time.Second)
	defer cancel()

	deadline, ok := newCtx.Deadline()
	assert.True(t, ok, "negative timeout should set deadline")
	expected := time.Now().Add(defaultOperationTimeout)
	assert.WithinDuration(t, expected, deadline, 100*time.Millisecond)
}

func TestLoader_Load_WithoutSingleflight_WithoutDistLock_DirectLoad(t *testing.T) {
	// 测试禁用 singleflight 和 dist lock 时的直接加载路径
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(false),
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("direct"), nil
	}

	value, err := loader.Load(ctx, "directkey", loadFn, time.Hour)
	require.NoError(t, err)
	assert.Equal(t, []byte("direct"), value)
}

func TestLoader_LoadHash_WithoutSingleflight_WithoutDistLock_DirectLoad(t *testing.T) {
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(false),
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("direct_hash"), nil
	}

	value, err := loader.LoadHash(ctx, "directhash", "field", loadFn, time.Hour)
	require.NoError(t, err)
	assert.Equal(t, []byte("direct_hash"), value)
}

func TestWithMemoryMaxCost_MinCostClamp(t *testing.T) {
	// 测试 cost < MinMemoryMaxCost 时被钳位到最小值
	opts := defaultMemoryOptions()
	WithMemoryMaxCost(100)(opts) // 100 bytes << MinMemoryMaxCost (1MB)
	assert.Equal(t, int64(MinMemoryMaxCost), opts.MaxCost)
}

func TestLoader_Load_WithSingleflight_ContextCancelled_BeforeResult(t *testing.T) {
	// 测试 singleflight 场景下 context 取消的路径
	cache, _ := newTestRedis(t)

	loader, err := NewLoader(cache, WithSingleflight(true))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	loadFn := func(ctx context.Context) ([]byte, error) {
		time.Sleep(200 * time.Millisecond)
		return []byte("value"), nil
	}

	// 立即取消 context
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err = loader.Load(ctx, "sf_cancel_key", loadFn, time.Hour)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestLoader_LoadHash_WithSingleflight_ContextCancelled_BeforeResult(t *testing.T) {
	cache, _ := newTestRedis(t)

	loader, err := NewLoader(cache, WithSingleflight(true))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	loadFn := func(ctx context.Context) ([]byte, error) {
		time.Sleep(200 * time.Millisecond)
		return []byte("value"), nil
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err = loader.LoadHash(ctx, "sf_cancel_hash", "field", loadFn, time.Hour)
	assert.ErrorIs(t, err, context.Canceled)
}

// =============================================================================
// NewLoader 构造验证测试
// =============================================================================

func TestNewLoader_NilCache_ReturnsError(t *testing.T) {
	_, err := NewLoader(nil)
	assert.ErrorIs(t, err, ErrNilClient)
}

func TestNewLoader_ExternalLockWithoutEnable_ReturnsError(t *testing.T) {
	cache, mr := newTestRedis(t)
	t.Cleanup(func() { mr.Close() })

	// WithExternalLock(fn) 自动设置 EnableDistributedLock=true，
	// 之后显式关闭，形成矛盾配置
	_, err := NewLoader(cache,
		WithExternalLock(func(_ context.Context, _ string, _ time.Duration) (Unlocker, error) {
			return nil, nil
		}),
		WithDistributedLock(false),
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

func TestWithExternalLock_NilDoesNotDisableDistLock(t *testing.T) {
	// FG-M7: WithDistributedLock(true) + WithExternalLock(nil) 不应意外禁用分布式锁
	cache, mr := newTestRedis(t)
	t.Cleanup(func() { mr.Close() })

	loader, err := NewLoader(cache,
		WithDistributedLock(true),
		WithExternalLock(nil), // 不应禁用分布式锁
	)
	require.NoError(t, err)
	assert.NotNil(t, loader)
}

// =============================================================================
// safeLoadFn panic 恢复测试
// =============================================================================

func TestLoader_Load_WhenLoadFnPanics_ReturnsErrLoadPanic(t *testing.T) {
	cache, mr := newTestRedis(t)
	t.Cleanup(func() { mr.Close() })

	loader, err := NewLoader(cache, WithSingleflight(false))
	require.NoError(t, err)

	_, err = loader.Load(context.Background(), "panic-key", func(_ context.Context) ([]byte, error) {
		panic("test panic in loadFn")
	}, time.Hour)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLoadPanic)
	assert.Contains(t, err.Error(), "test panic in loadFn")
}

func TestLoader_LoadHash_WhenLoadFnPanics_ReturnsErrLoadPanic(t *testing.T) {
	cache, mr := newTestRedis(t)
	t.Cleanup(func() { mr.Close() })

	loader, err := NewLoader(cache, WithSingleflight(false))
	require.NoError(t, err)

	_, err = loader.LoadHash(context.Background(), "panic-hash", "field", func(_ context.Context) ([]byte, error) {
		panic("hash panic test")
	}, time.Hour)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLoadPanic)
	assert.Contains(t, err.Error(), "hash panic test")
}

// TestLoader_Load_WithSingleflight_FirstCallerCancel_DoesNotAffectOthers 验证 FG-S1 修复：
// 当首个调用者取消 context 时，其他共享 singleflight 的调用者仍能正常获得结果。
func TestLoader_Load_WithSingleflight_FirstCallerCancel_DoesNotAffectOthers(t *testing.T) {
	cache, _ := newTestRedis(t)

	loader, err := NewLoader(cache, WithSingleflight(true), WithLoadTimeout(0))
	require.NoError(t, err)

	var loadCount atomic.Int32
	loadFn := func(_ context.Context) ([]byte, error) {
		loadCount.Add(1)
		time.Sleep(200 * time.Millisecond)
		return []byte("shared-value"), nil
	}

	// caller1: 短 context，会在 loadFn 执行期间被取消
	ctx1, cancel1 := context.WithCancel(context.Background())

	// caller2: 正常 context，应该收到结果
	ctx2 := context.Background()

	var wg sync.WaitGroup
	var err1, err2 error
	var val2 []byte

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err1 = loader.Load(ctx1, "sf-cancel-shared", loadFn, time.Hour)
	}()
	go func() {
		defer wg.Done()
		// 确保 caller2 在 caller1 之后进入 singleflight
		time.Sleep(10 * time.Millisecond)
		val2, err2 = loader.Load(ctx2, "sf-cancel-shared", loadFn, time.Hour)
	}()

	// 在 loadFn 执行期间取消 caller1
	time.Sleep(50 * time.Millisecond)
	cancel1()

	wg.Wait()

	// caller1 应被取消
	assert.ErrorIs(t, err1, context.Canceled)
	// caller2 应正常获得结果（FG-S1 修复前会收到 context.Canceled）
	require.NoError(t, err2)
	assert.Equal(t, []byte("shared-value"), val2)
	// loadFn 只应调用一次（singleflight 去重）
	assert.Equal(t, int32(1), loadCount.Load())
}

func TestLoader_Load_WithSingleflight_WhenLoadFnPanics_ReturnsErrLoadPanic(t *testing.T) {
	cache, mr := newTestRedis(t)
	t.Cleanup(func() { mr.Close() })

	loader, err := NewLoader(cache, WithSingleflight(true))
	require.NoError(t, err)

	_, err = loader.Load(context.Background(), "sf-panic-key", func(_ context.Context) ([]byte, error) {
		panic("singleflight panic")
	}, time.Hour)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLoadPanic)
}

// =============================================================================
// TTL 抖动测试
// =============================================================================

func TestLoader_Load_WithTTLJitter_WritesJitteredTTL(t *testing.T) {
	cache, mr := newTestRedis(t)

	loader, err := NewLoader(cache, WithTTLJitter(0.5))
	require.NoError(t, err)

	ctx := context.Background()
	baseTTL := 10 * time.Minute

	_, err = loader.Load(ctx, "jitter-key", func(_ context.Context) ([]byte, error) {
		return []byte("value"), nil
	}, baseTTL)
	require.NoError(t, err)

	// 验证 TTL 被设置且在合理范围内（baseTTL ± 25%）
	actualTTL := mr.TTL("jitter-key")
	minTTL := time.Duration(float64(baseTTL) * 0.7)
	maxTTL := time.Duration(float64(baseTTL) * 1.3)
	assert.True(t, actualTTL >= minTTL && actualTTL <= maxTTL,
		"TTL %v should be between %v and %v", actualTTL, minTTL, maxTTL)
}

func TestLoader_Load_WithZeroTTLJitter_WritesExactTTL(t *testing.T) {
	cache, mr := newTestRedis(t)

	// TTLJitter=0 (default) should write exact TTL
	loader, err := NewLoader(cache)
	require.NoError(t, err)

	ctx := context.Background()
	baseTTL := 10 * time.Minute

	_, err = loader.Load(ctx, "exact-ttl-key", func(_ context.Context) ([]byte, error) {
		return []byte("value"), nil
	}, baseTTL)
	require.NoError(t, err)

	actualTTL := mr.TTL("exact-ttl-key")
	assert.Equal(t, baseTTL, actualTTL)
}
