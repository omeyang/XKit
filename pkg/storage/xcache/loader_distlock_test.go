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
// 分布式锁测试
// =============================================================================

func TestLoader_Load_WithDistributedLock_AcquiresLock(t *testing.T) {
	// Given
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// 创建无 lock 前缀的缓存，便于测试验证
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithDistributedLock(true),
		WithDistributedLockTTL(10*time.Second),
		WithDistributedLockKeyPrefix("loader:lock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		// 在加载过程中，锁应该存在
		assert.True(t, mr.Exists("loader:lock:mykey"))
		return []byte("value"), nil
	}

	// When
	value, err := loader.Load(ctx, "mykey", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), value)
}

// =============================================================================
// 超时测试
// =============================================================================

func TestLoader_Load_WithTimeout_CancelsOnTimeout(t *testing.T) {
	// Given
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithLoadTimeout(50*time.Millisecond),
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1 * time.Second):
			return []byte("value"), nil
		}
	}

	// When
	_, err = loader.Load(ctx, "mykey", loadFn, time.Hour)

	// Then
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
}

// =============================================================================
// 选项测试
// =============================================================================

func TestWithSingleflight_SetsOption(t *testing.T) {
	opts := defaultLoaderOptions()
	WithSingleflight(false)(opts)
	assert.False(t, opts.EnableSingleflight)
}

func TestWithDistributedLock_SetsOption(t *testing.T) {
	opts := defaultLoaderOptions()
	WithDistributedLock(true)(opts)
	assert.True(t, opts.EnableDistributedLock)
}

func TestWithDistributedLockTTL_SetsOption(t *testing.T) {
	opts := defaultLoaderOptions()
	WithDistributedLockTTL(30 * time.Second)(opts)
	assert.Equal(t, 30*time.Second, opts.DistributedLockTTL)
}

func TestWithLoadTimeout_SetsOption(t *testing.T) {
	opts := defaultLoaderOptions()
	WithLoadTimeout(5 * time.Second)(opts)
	assert.Equal(t, 5*time.Second, opts.LoadTimeout)
}

// =============================================================================
// applyLoadTimeout 测试
// =============================================================================

func TestApplyLoadTimeout_PositiveTimeout(t *testing.T) {
	// Given: 正数超时
	ctx := context.Background()
	timeout := 100 * time.Millisecond

	// When
	newCtx, cancel := applyLoadTimeout(ctx, timeout)
	defer cancel()

	// Then: 应使用指定超时
	deadline, ok := newCtx.Deadline()
	assert.True(t, ok, "应设置 deadline")
	assert.WithinDuration(t, time.Now().Add(timeout), deadline, 10*time.Millisecond)
}

func TestApplyLoadTimeout_ZeroTimeout(t *testing.T) {
	// Given: 零超时（禁用）
	ctx := context.Background()
	var timeout time.Duration = 0

	// When
	newCtx, cancel := applyLoadTimeout(ctx, timeout)
	defer cancel()

	// Then: 应返回原 ctx，无 deadline
	_, ok := newCtx.Deadline()
	assert.False(t, ok, "不应设置 deadline")
	// context.Background() 返回的是同一个实例，可以用 == 比较
	assert.True(t, ctx == newCtx, "应返回原 ctx")
}

func TestApplyLoadTimeout_NegativeTimeout(t *testing.T) {
	// Given: 负数超时（使用默认 30s）
	ctx := context.Background()
	timeout := -1 * time.Second

	// When
	newCtx, cancel := applyLoadTimeout(ctx, timeout)
	defer cancel()

	// Then: 应使用默认超时 (30s)
	deadline, ok := newCtx.Deadline()
	assert.True(t, ok, "应设置 deadline")
	expectedDeadline := time.Now().Add(defaultOperationTimeout)
	assert.WithinDuration(t, expectedDeadline, deadline, 100*time.Millisecond)
}

func TestLoader_Load_WithNegativeTimeout_UsesDefault(t *testing.T) {
	// Given: 集成测试 - 负数超时应使用默认 30s
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	// 使用负数超时
	loader, err := NewLoader(cache,
		WithLoadTimeout(-1*time.Second),
		WithSingleflight(false),
	)
	require.NoError(t, err)

	var loadCtxDeadline time.Time
	loadFn := func(ctx context.Context) ([]byte, error) {
		loadCtxDeadline, _ = ctx.Deadline()
		return []byte("value"), nil
	}

	// When
	_, err = loader.Load(ctx, "mykey", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	expectedDeadline := time.Now().Add(defaultOperationTimeout)
	assert.WithinDuration(t, expectedDeadline, loadCtxDeadline, 100*time.Millisecond,
		"负数超时应使用默认 30s")
}

func TestLoader_Load_WithZeroTimeout_NoDeadline(t *testing.T) {
	// Given: 集成测试 - 零超时应禁用
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	// 使用零超时
	loader, err := NewLoader(cache,
		WithLoadTimeout(0),
		WithSingleflight(false),
	)
	require.NoError(t, err)

	var hasDeadline bool
	loadFn := func(ctx context.Context) ([]byte, error) {
		_, hasDeadline = ctx.Deadline()
		return []byte("value"), nil
	}

	// When
	_, err = loader.Load(ctx, "mykey", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.False(t, hasDeadline, "零超时应禁用 deadline")
}

// =============================================================================
// 分布式锁高级场景测试
// =============================================================================

func TestLoader_Load_WithDistributedLock_WhenLockFails_WaitsAndRetries(t *testing.T) {
	// Given
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithSingleflight(true),
		WithDistributedLock(true),
		WithDistributedLockTTL(10*time.Second),
		WithDistributedLockKeyPrefix("lock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	// 预先占用锁
	mr.Set("lock:testkey", "occupied")
	mr.SetTTL("lock:testkey", 200*time.Millisecond)

	// 在后台设置缓存值（模拟另一个进程完成加载）
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = cache.Client().Set(ctx, "testkey", "loaded_by_other", 0).Err()
	}()

	loadFn := func(ctx context.Context) ([]byte, error) {
		t.Fatal("loadFn should not be called when lock fails and cache becomes available")
		return nil, nil
	}

	// When
	value, err := loader.Load(ctx, "testkey", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("loaded_by_other"), value)
}

func TestLoader_Load_WithDistributedLock_DoubleCheckAfterLock(t *testing.T) {
	// Given - 测试获取锁后的 double-check 逻辑
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithSingleflight(true),
		WithDistributedLock(true),
		WithDistributedLockTTL(10*time.Second),
		WithDistributedLockKeyPrefix("lock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	var loadCount int32
	loadFn := func(ctx context.Context) ([]byte, error) {
		atomic.AddInt32(&loadCount, 1)
		return []byte("from_backend"), nil
	}

	// When - 并发请求
	var wg sync.WaitGroup
	results := make([][]byte, 5)
	errs := make([]error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = loader.Load(ctx, "dcheck_key", loadFn, time.Hour)
		}(i)
	}
	wg.Wait()

	// Then
	for i := 0; i < 5; i++ {
		require.NoError(t, errs[i])
		assert.Equal(t, []byte("from_backend"), results[i])
	}
	// 由于 singleflight + distributed lock double-check，loadFn 只应该调用一次
	assert.Equal(t, int32(1), atomic.LoadInt32(&loadCount))
}

func TestLoader_LoadHash_WithDistributedLock_AcquiresLock(t *testing.T) {
	// Given
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithDistributedLock(true),
		WithDistributedLockTTL(10*time.Second),
		WithDistributedLockKeyPrefix("hlock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		// 在加载过程中，锁应该存在
		// 使用 hashFieldKey 生成一致的锁 key，避免 key/field 中包含 ":" 导致碰撞
		assert.True(t, mr.Exists("hlock:"+hashFieldKey("myhash", "field1")))
		return []byte("hash_value"), nil
	}

	// When
	value, err := loader.LoadHash(ctx, "myhash", "field1", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("hash_value"), value)
}

func TestLoader_LoadHash_WithDistributedLock_WhenLockFails_WaitsAndRetries(t *testing.T) {
	// Given
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithSingleflight(true),
		WithDistributedLock(true),
		WithDistributedLockTTL(10*time.Second),
		WithDistributedLockKeyPrefix("hlock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	// 预先占用锁（使用 hashFieldKey 生成一致的锁 key）
	lockKey := "hlock:" + hashFieldKey("myhash", "myfield")
	mr.Set(lockKey, "occupied")
	mr.SetTTL(lockKey, 200*time.Millisecond)

	// 在后台设置缓存值
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = cache.Client().HSet(ctx, "myhash", "myfield", "loaded_by_other").Err()
	}()

	loadFn := func(ctx context.Context) ([]byte, error) {
		t.Fatal("loadFn should not be called")
		return nil, nil
	}

	// When
	value, err := loader.LoadHash(ctx, "myhash", "myfield", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("loaded_by_other"), value)
}

func TestLoader_Load_WithDistributedLock_WhenContextCancelled_ReturnsError(t *testing.T) {
	// Given
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	// 预先占用锁，让 waitAndRetry 被触发
	mr.Set("lock:ctxkey", "occupied")
	mr.SetTTL("lock:ctxkey", 10*time.Second)

	loader, err := NewLoader(cache,
		WithSingleflight(true),
		WithDistributedLock(true),
		WithDistributedLockTTL(10*time.Second),
		WithDistributedLockKeyPrefix("lock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("value"), nil
	}

	// When
	_, err = loader.Load(ctx, "ctxkey", loadFn, time.Hour)

	// Then - 应该返回 context 超时错误
	assert.Error(t, err)
}

func TestLoader_LoadHash_WhenBackendFails_ReturnsError(t *testing.T) {
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
	_, err = loader.LoadHash(ctx, "myhash", "field1", loadFn, time.Hour)

	// Then
	assert.ErrorIs(t, err, expectedErr)
}

func TestLoader_LoadHash_WithTimeout_CancelsOnTimeout(t *testing.T) {
	// Given
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithLoadTimeout(50*time.Millisecond),
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1 * time.Second):
			return []byte("value"), nil
		}
	}

	// When
	_, err = loader.LoadHash(ctx, "myhash", "field1", loadFn, time.Hour)

	// Then
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
}

func TestLoader_LoadHash_WithDistributedLock_WhenContextCancelled_ReturnsError(t *testing.T) {
	// Given - 测试 waitAndRetryHGet 的 context 取消路径
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	// 预先占用锁，让 waitAndRetryHGet 被触发
	// 使用 hashFieldKey 生成一致的锁 key
	hashLockKey := "hlock:" + hashFieldKey("hash", "field")
	mr.Set(hashLockKey, "occupied")
	mr.SetTTL(hashLockKey, 10*time.Second)

	loader, err := NewLoader(cache,
		WithSingleflight(true),
		WithDistributedLock(true),
		WithDistributedLockTTL(10*time.Second),
		WithDistributedLockKeyPrefix("hlock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("value"), nil
	}

	// When
	_, err = loader.LoadHash(ctx, "hash", "field", loadFn, time.Hour)

	// Then - 应该返回 context 超时错误
	assert.Error(t, err)
}

func TestLoader_Load_WithDistributedLock_WhenCacheStillEmpty_LoadsFromBackend(t *testing.T) {
	// Given - 测试 waitAndRetryGet 缓存仍为空时回源加载
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	// 预先占用锁，但不设置缓存值，让 waitAndRetryGet 等待后仍找不到值
	mr.Set("lock:emptykey", "occupied")
	mr.SetTTL("lock:emptykey", 150*time.Millisecond)

	loader, err := NewLoader(cache,
		WithSingleflight(true),
		WithDistributedLock(true),
		WithDistributedLockTTL(200*time.Millisecond),
		WithDistributedLockKeyPrefix("lock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	called := false
	loadFn := func(ctx context.Context) ([]byte, error) {
		called = true
		return []byte("backend"), nil
	}

	// When
	value, err := loader.Load(context.Background(), "emptykey", loadFn, time.Hour)

	// Then - 等待后缓存仍为空，应该回源并返回结果
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, []byte("backend"), value)
}

func TestLoader_LoadHash_WithDistributedLock_WhenCacheStillEmpty_LoadsFromBackend(t *testing.T) {
	// Given - 测试 waitAndRetryHGet 缓存仍为空时回源加载
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	// 预先占用锁，但不设置 hash field，让 waitAndRetryHGet 等待后仍找不到值
	// 使用 hashFieldKey 生成一致的锁 key
	emptyLockKey := "hlock:" + hashFieldKey("emptyhash", "emptyfield")
	mr.Set(emptyLockKey, "occupied")
	mr.SetTTL(emptyLockKey, 150*time.Millisecond)

	loader, err := NewLoader(cache,
		WithSingleflight(true),
		WithDistributedLock(true),
		WithDistributedLockTTL(200*time.Millisecond),
		WithDistributedLockKeyPrefix("hlock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	called := false
	loadFn := func(ctx context.Context) ([]byte, error) {
		called = true
		return []byte("backend"), nil
	}

	// When
	value, err := loader.LoadHash(context.Background(), "emptyhash", "emptyfield", loadFn, time.Hour)

	// Then - 等待后缓存仍为空，应该回源并返回结果
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, []byte("backend"), value)
}

// =============================================================================
// 分布式锁获取后 double-check 缓存命中测试
// =============================================================================

func TestLoader_Load_WithDistributedLock_CacheHitAfterLockAcquired(t *testing.T) {
	// Given - 测试获取锁后立即发现缓存已被其他进程填充的场景
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithSingleflight(false), // 禁用 singleflight 以便多个请求独立处理
		WithDistributedLock(true),
		WithDistributedLockTTL(5*time.Second),
		WithDistributedLockKeyPrefix("lock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	var loadCount int32
	var phase int32 // 0: 初始, 1: 第一个请求获得锁, 2: 第一个请求已写入缓存

	loadFn := func(ctx context.Context) ([]byte, error) {
		atomic.AddInt32(&loadCount, 1)
		// 模拟耗时的后端加载
		time.Sleep(50 * time.Millisecond)
		return []byte("from_backend"), nil
	}

	var wg sync.WaitGroup
	results := make([][]byte, 2)
	errs := make([]error, 2)

	// 第一个请求：获取锁并加载数据
	wg.Add(1)
	go func() {
		defer wg.Done()
		atomic.StoreInt32(&phase, 1)
		results[0], errs[0] = loader.Load(ctx, "race_key", loadFn, time.Hour)
		atomic.StoreInt32(&phase, 2)
	}()

	// 等待第一个请求开始执行
	time.Sleep(10 * time.Millisecond)

	// 第二个请求：等待锁释放，然后应该在 double-check 时发现缓存已有值
	wg.Add(1)
	go func() {
		defer wg.Done()
		// 等待第一个请求写入缓存
		for atomic.LoadInt32(&phase) < 2 {
			time.Sleep(5 * time.Millisecond)
		}
		results[1], errs[1] = loader.Load(ctx, "race_key", loadFn, time.Hour)
	}()

	wg.Wait()

	// Then - 两个请求都应该成功，但 loadFn 只应该调用一次
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.Equal(t, []byte("from_backend"), results[0])
	assert.Equal(t, []byte("from_backend"), results[1])
	assert.Equal(t, int32(1), atomic.LoadInt32(&loadCount))
}

func TestLoader_LoadHash_WithDistributedLock_CacheHitAfterLockAcquired(t *testing.T) {
	// Given - 测试 Hash 版本获取锁后发现缓存已被填充的场景
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockTTL(5*time.Second),
		WithDistributedLockKeyPrefix("hlock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	var loadCount int32
	var phase int32

	loadFn := func(ctx context.Context) ([]byte, error) {
		atomic.AddInt32(&loadCount, 1)
		time.Sleep(50 * time.Millisecond)
		return []byte("hash_value"), nil
	}

	var wg sync.WaitGroup
	results := make([][]byte, 2)
	errs := make([]error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		atomic.StoreInt32(&phase, 1)
		results[0], errs[0] = loader.LoadHash(ctx, "race_hash", "field1", loadFn, time.Hour)
		atomic.StoreInt32(&phase, 2)
	}()

	time.Sleep(10 * time.Millisecond)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for atomic.LoadInt32(&phase) < 2 {
			time.Sleep(5 * time.Millisecond)
		}
		results[1], errs[1] = loader.LoadHash(ctx, "race_hash", "field1", loadFn, time.Hour)
	}()

	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.Equal(t, []byte("hash_value"), results[0])
	assert.Equal(t, []byte("hash_value"), results[1])
	assert.Equal(t, int32(1), atomic.LoadInt32(&loadCount))
}

// =============================================================================
// waitAndRetry 边界条件测试
// =============================================================================

func TestLoader_Load_WithDistributedLock_WhenDeadlineVeryShort(t *testing.T) {
	// Given - 测试 DistributedLockTTL 非常短的情况
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	// 预先占用锁
	mr.Set("lock:shortkey", "occupied")
	mr.SetTTL("lock:shortkey", 10*time.Millisecond) // 非常短的 TTL

	loader, err := NewLoader(cache,
		WithSingleflight(true),
		WithDistributedLock(true),
		WithDistributedLockTTL(50*time.Millisecond), // 很短的等待时间
		WithDistributedLockKeyPrefix("lock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("loaded"), nil
	}

	// When
	value, err := loader.Load(context.Background(), "shortkey", loadFn, time.Hour)

	// Then - 应该成功（等待后锁过期或直接回源）
	require.NoError(t, err)
	assert.Equal(t, []byte("loaded"), value)
}

func TestLoader_LoadHash_WithDistributedLock_WhenDeadlineVeryShort(t *testing.T) {
	// Given - Hash 版本的短 deadline 测试
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	// 使用 hashFieldKey 生成一致的锁 key
	shortLockKey := "hlock:" + hashFieldKey("shorthash", "field")
	mr.Set(shortLockKey, "occupied")
	mr.SetTTL(shortLockKey, 10*time.Millisecond)

	loader, err := NewLoader(cache,
		WithSingleflight(true),
		WithDistributedLock(true),
		WithDistributedLockTTL(50*time.Millisecond),
		WithDistributedLockKeyPrefix("hlock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("hash_loaded"), nil
	}

	value, err := loader.LoadHash(context.Background(), "shorthash", "field", loadFn, time.Hour)

	require.NoError(t, err)
	assert.Equal(t, []byte("hash_loaded"), value)
}

func TestLoader_Load_WithDistributedLock_WhenLockTTLZero(t *testing.T) {
	// Given - 测试 DistributedLockTTL 为 0 的边界情况
	// TTL=0 是配置错误，应该返回错误而不是静默降级
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	_, err = NewLoader(cache,
		WithSingleflight(true),
		WithDistributedLock(true),
		WithDistributedLockTTL(0), // TTL 为 0 是配置错误
		WithDistributedLockKeyPrefix("lock:"),
	)

	// Then - NewLoader 应该返回配置错误
	assert.ErrorIs(t, err, ErrInvalidConfig)
	assert.ErrorIs(t, err, ErrInvalidLockTTL)
}

// =============================================================================
// loadWithDistLock 首次 double-check 缓存命中测试
// =============================================================================

func TestLoader_Load_WithDistLock_FirstDoubleCheckHit(t *testing.T) {
	// Given - 测试 loadWithDistLock 进入时缓存已有值的情况 (line 113-114)
	// 这种情况发生在 Load 第一次检查 miss，但到 loadWithDistLock 时值已存在
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockKeyPrefix("lock:"),
	)
	require.NoError(t, err)

	// 预先占用锁，迫使第一个请求进入 waitAndRetryGet
	mr.Set("lock:race_first_check", "occupied")
	mr.SetTTL("lock:race_first_check", 100*time.Millisecond)

	loadCalled := int32(0)
	loadFn := func(ctx context.Context) ([]byte, error) {
		atomic.AddInt32(&loadCalled, 1)
		return []byte("loaded"), nil
	}

	// 在另一个 goroutine 中填充缓存（模拟另一个进程完成加载）
	go func() {
		time.Sleep(20 * time.Millisecond)
		mr.Set("race_first_check", "precached")
	}()

	// When
	value, err := loader.Load(ctx, "race_first_check", loadFn, time.Hour)

	// Then - 应该返回预缓存的值，loadFn 不应该被调用
	require.NoError(t, err)
	assert.Equal(t, []byte("precached"), value)
}

func TestLoader_LoadHash_WithDistLock_FirstDoubleCheckHit(t *testing.T) {
	// Given - Hash 版本的首次 double-check 缓存命中测试
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockKeyPrefix("hlock:"),
	)
	require.NoError(t, err)

	// 预先占用锁
	// 使用 hashFieldKey 生成一致的锁 key
	raceLockKey := "hlock:" + hashFieldKey("race_hash", "field")
	mr.Set(raceLockKey, "occupied")
	mr.SetTTL(raceLockKey, 100*time.Millisecond)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("loaded"), nil
	}

	// 在另一个 goroutine 中填充 Hash 缓存
	go func() {
		time.Sleep(20 * time.Millisecond)
		mr.HSet("race_hash", "field", "hash_precached")
	}()

	// When
	value, err := loader.LoadHash(ctx, "race_hash", "field", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("hash_precached"), value)
}

// =============================================================================
// waitAndRetry 中间 wait 时间调整测试
// =============================================================================

func TestLoader_Load_WithDistLock_WaitTimeAdjusted(t *testing.T) {
	// Given - 测试 waitAndRetryGet 中 wait 时间被调整的路径 (line 280-282)
	// 当 remaining < 100ms 时，wait = remaining
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	// 预先占用锁
	mr.Set("lock:waitadjust", "occupied")
	mr.SetTTL("lock:waitadjust", 50*time.Millisecond)

	loader, err := NewLoader(cache,
		WithSingleflight(true),
		WithDistributedLock(true),
		WithDistributedLockTTL(80*time.Millisecond), // 短于 100ms，会触发 wait 时间调整
		WithDistributedLockKeyPrefix("lock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("adjusted"), nil
	}

	// When
	value, err := loader.Load(context.Background(), "waitadjust", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("adjusted"), value)
}

func TestLoader_LoadHash_WithDistLock_WaitTimeAdjusted(t *testing.T) {
	// Given - Hash 版本 wait 时间调整测试
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	// 使用 hashFieldKey 生成一致的锁 key
	waitLockKey := "hlock:" + hashFieldKey("waitadjust", "field")
	mr.Set(waitLockKey, "occupied")
	mr.SetTTL(waitLockKey, 50*time.Millisecond)

	loader, err := NewLoader(cache,
		WithSingleflight(true),
		WithDistributedLock(true),
		WithDistributedLockTTL(80*time.Millisecond),
		WithDistributedLockKeyPrefix("hlock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("hash_adjusted"), nil
	}

	value, err := loader.LoadHash(context.Background(), "waitadjust", "field", loadFn, time.Hour)

	require.NoError(t, err)
	assert.Equal(t, []byte("hash_adjusted"), value)
}

// =============================================================================
// waitAndRetry 循环开始时 context 取消测试
// =============================================================================

func TestLoader_Load_WithDistLock_WaitRetry_ContextCancelledAtLoopStart(t *testing.T) {
	// Given - 测试 waitAndRetryGet 在循环开始时检测 context 取消 (line 263-264)
	// 使用已取消的 context 调用，迫使 waitAndRetry 在第一次循环就退出
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	// 预先占用锁，迫使请求进入 waitAndRetryGet
	mr.Set("lock:ctx_at_start", "occupied")
	mr.SetTTL("lock:ctx_at_start", 10*time.Second) // 长时间占用

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockTTL(5*time.Second),
		WithDistributedLockKeyPrefix("lock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	// 创建一个非常短的超时 context
	// 这会让 Get 操作返回 redis.Nil 后，在下一次循环开始时 context 已经过期
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// 等待 context 超时
	time.Sleep(5 * time.Millisecond)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("should_not_reach"), nil
	}

	// When - context 已取消，waitAndRetryGet 应该在循环开始时检测到
	_, err = loader.Load(ctx, "ctx_at_start", loadFn, time.Hour)

	// Then - 应该返回 context 超时错误
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled))
}

func TestLoader_LoadHash_WithDistLock_WaitRetry_ContextCancelledAtLoopStart(t *testing.T) {
	// Given - Hash 版本测试
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	// 使用 hashFieldKey 生成一致的锁 key
	ctxLockKey := "hlock:" + hashFieldKey("ctx_at_start", "field")
	mr.Set(ctxLockKey, "occupied")
	mr.SetTTL(ctxLockKey, 10*time.Second)

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockTTL(5*time.Second),
		WithDistributedLockKeyPrefix("hlock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("should_not_reach"), nil
	}

	_, err = loader.LoadHash(ctx, "ctx_at_start", "field", loadFn, time.Hour)

	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled))
}

// TestLoader_Load_WithDistLock_UnlockError 测试 unlock 失败时的日志记录路径
func TestLoader_Load_WithDistLock_UnlockError(t *testing.T) {
	// Given - 创建会在 unlock 时失败的场景
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockTTL(5*time.Second),
		WithDistributedLockKeyPrefix("lock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	loadCalled := false
	loadFn := func(ctx context.Context) ([]byte, error) {
		loadCalled = true
		// 在加载函数内关闭 Redis，这样 unlock 会失败
		mr.Close()
		return []byte("value"), nil
	}

	ctx := context.Background()
	// 操作应该成功（即使 unlock 失败，数据仍应返回）
	value, err := loader.Load(ctx, "unlock_error_key", loadFn, time.Hour)

	// 验证加载函数被调用
	assert.True(t, loadCalled)
	// 由于 Redis 关闭，缓存写入可能失败，但加载函数的值应该返回
	// 注意：unlock 错误只是日志记录，不影响返回值
	if err == nil {
		assert.Equal(t, []byte("value"), value)
	}
}

// TestLoader_LoadHash_WithDistLock_UnlockError 测试 Hash 版本 unlock 失败时的日志记录路径
func TestLoader_LoadHash_WithDistLock_UnlockError(t *testing.T) {
	// Given - 创建会在 unlock 时失败的场景
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockTTL(5*time.Second),
		WithDistributedLockKeyPrefix("hlock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	loadCalled := false
	loadFn := func(ctx context.Context) ([]byte, error) {
		loadCalled = true
		// 在加载函数内关闭 Redis，这样 unlock 会失败
		mr.Close()
		return []byte("hash_value"), nil
	}

	ctx := context.Background()
	// 操作应该成功（即使 unlock 失败，数据仍应返回）
	value, err := loader.LoadHash(ctx, "unlock_error_key", "field", loadFn, time.Hour)

	// 验证加载函数被调用
	assert.True(t, loadCalled)
	// 由于 Redis 关闭，缓存写入可能失败，但加载函数的值应该返回
	if err == nil {
		assert.Equal(t, []byte("hash_value"), value)
	}
}

// =============================================================================
// 外部锁 (ExternalLock) 测试
// =============================================================================

func TestLoader_Load_WithExternalLock_NilUnlocker_ReturnsConfigError(t *testing.T) {
	// Given - 外部锁返回 (nil, nil)，应该返回配置错误而非 panic
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	// 模拟错误的外部锁实现：返回 nil unlocker 但无 error
	buggyLock := func(_ context.Context, _ string, _ time.Duration) (Unlocker, error) {
		return nil, nil
	}

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockTTL(5*time.Second),
		WithExternalLock(buggyLock),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	loadFn := func(_ context.Context) ([]byte, error) {
		return []byte("value"), nil
	}

	// When - 不应 panic，应返回配置错误后降级重试
	_, err = loader.Load(context.Background(), "nilunlock", loadFn, time.Hour)

	// Then - handleLockError 将 ErrInvalidConfig 直接返回（不降级）
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

func TestWithExternalLock_SetsOption(t *testing.T) {
	opts := defaultLoaderOptions()
	assert.Nil(t, opts.ExternalLock)

	fn := func(ctx context.Context, key string, ttl time.Duration) (Unlocker, error) {
		return func(ctx context.Context) error { return nil }, nil
	}
	WithExternalLock(fn)(opts)

	assert.NotNil(t, opts.ExternalLock)
	assert.True(t, opts.EnableDistributedLock)
}

func TestWithExternalLock_Nil_OnlyClearsFunction(t *testing.T) {
	opts := defaultLoaderOptions()

	// 先设置外部锁
	fn := func(ctx context.Context, key string, ttl time.Duration) (Unlocker, error) {
		return func(ctx context.Context) error { return nil }, nil
	}
	WithExternalLock(fn)(opts)
	assert.True(t, opts.EnableDistributedLock)

	// 设计决策: 传入 nil 仅清除外部锁函数，不修改 EnableDistributedLock 标志，
	// 避免 WithDistributedLock(true) + WithExternalLock(nil) 意外禁用分布式锁。
	WithExternalLock(nil)(opts)
	assert.Nil(t, opts.ExternalLock)
	assert.True(t, opts.EnableDistributedLock) // 分布式锁标志不受影响
}

func TestLoader_Load_WithExternalLock_UsesExternalLock(t *testing.T) {
	// Given - 使用外部锁替代内置锁
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	ctx := context.Background()

	// 记录外部锁调用
	var lockCalled, unlockCalled bool
	var lockedKey string
	var lockedTTL time.Duration

	externalLock := func(ctx context.Context, key string, ttl time.Duration) (Unlocker, error) {
		lockCalled = true
		lockedKey = key
		lockedTTL = ttl
		return func(ctx context.Context) error {
			unlockCalled = true
			return nil
		}, nil
	}

	loader, err := NewLoader(cache,
		WithDistributedLock(true),
		WithDistributedLockTTL(15*time.Second),
		WithDistributedLockKeyPrefix("ext:lock:"),
		WithLoadTimeout(0),
		WithExternalLock(externalLock),
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("external_value"), nil
	}

	// When
	value, err := loader.Load(ctx, "extkey", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("external_value"), value)
	assert.True(t, lockCalled, "外部锁应该被调用")
	assert.True(t, unlockCalled, "外部锁 unlock 应该被调用")
	assert.Equal(t, "ext:lock:extkey", lockedKey)
	assert.Equal(t, 15*time.Second, lockedTTL)
}

func TestLoader_LoadHash_WithExternalLock_UsesExternalLock(t *testing.T) {
	// Given - Hash 版本使用外部锁
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	ctx := context.Background()

	var lockCalled bool
	var lockedKey string

	externalLock := func(ctx context.Context, key string, ttl time.Duration) (Unlocker, error) {
		lockCalled = true
		lockedKey = key
		return func(ctx context.Context) error { return nil }, nil
	}

	loader, err := NewLoader(cache,
		WithDistributedLock(true),
		WithDistributedLockKeyPrefix("ext:hlock:"),
		WithExternalLock(externalLock),
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("hash_external"), nil
	}

	// When
	value, err := loader.LoadHash(ctx, "exthash", "field1", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("hash_external"), value)
	assert.True(t, lockCalled)
	// 验证锁 key 使用了 hashFieldKey 格式
	assert.Equal(t, "ext:hlock:"+hashFieldKey("exthash", "field1"), lockedKey)
}

func TestLoader_Load_WithExternalLock_WhenLockFails_WaitsAndRetries(t *testing.T) {
	// Given - 外部锁获取失败时的重试逻辑
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	ctx := context.Background()

	// 外部锁总是失败
	externalLock := func(ctx context.Context, key string, ttl time.Duration) (Unlocker, error) {
		return nil, errors.New("external lock failed")
	}

	loader, err := NewLoader(cache,
		WithSingleflight(true),
		WithDistributedLock(true),
		WithDistributedLockTTL(100*time.Millisecond),
		WithLoadTimeout(0),
		WithExternalLock(externalLock),
	)
	require.NoError(t, err)

	// 在后台设置缓存值
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = cache.Client().Set(ctx, "extfailkey", "cached_by_other", 0).Err()
	}()

	loadFn := func(ctx context.Context) ([]byte, error) {
		t.Fatal("loadFn should not be called when cache becomes available")
		return nil, nil
	}

	// When
	value, err := loader.Load(ctx, "extfailkey", loadFn, time.Hour)

	// Then - 应该返回缓存中的值
	require.NoError(t, err)
	assert.Equal(t, []byte("cached_by_other"), value)
}

func TestLoader_Load_WithExternalLock_UnlockError_LogsWarning(t *testing.T) {
	// Given - 外部锁 unlock 失败时应该记录警告日志
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	ctx := context.Background()

	unlockErr := errors.New("unlock failed")
	externalLock := func(ctx context.Context, key string, ttl time.Duration) (Unlocker, error) {
		return func(ctx context.Context) error {
			return unlockErr
		}, nil
	}

	loader, err := NewLoader(cache,
		WithDistributedLock(true),
		WithExternalLock(externalLock),
		WithLogger(nil), // 禁用日志避免测试输出
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("value"), nil
	}

	// When - unlock 失败不影响返回值
	value, err := loader.Load(ctx, "unlockfailkey", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), value)
}

func TestLoader_Load_WithoutExternalLock_UsesBuiltinLock(t *testing.T) {
	// Given - 未设置 ExternalLock 时使用内置锁
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithDistributedLock(true),
		WithDistributedLockKeyPrefix("builtin:"),
		// 不设置 ExternalLock
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		// 验证内置锁存在
		assert.True(t, mr.Exists("builtin:builtinkey"))
		return []byte("builtin_value"), nil
	}

	// When
	value, err := loader.Load(ctx, "builtinkey", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("builtin_value"), value)
}

func TestLoader_Load_WithDistLock_UnlockExpired_LogsInfo(t *testing.T) {
	// Given - 锁在 Redis 中过期后再 unlock，应走 logInfo 路径（ErrLockExpired）
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockTTL(5*time.Second), // 足够长，unlock context 不会超时
		WithDistributedLockKeyPrefix("lock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		// 在 loadFn 内让 miniredis 使锁 key 过期
		mr.FastForward(6 * time.Second)
		return []byte("value"), nil
	}

	// When - unlock 时锁已在 Redis 中过期（Lua 脚本返回 0），触发 ErrLockExpired
	value, err := loader.Load(context.Background(), "expirekey", loadFn, time.Hour)

	// Then - 数据仍应成功返回
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), value)
}

func TestLoader_Load_WithExternalLock_Concurrent(t *testing.T) {
	// Given - 并发场景下外部锁的正确性
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	cache, err := NewRedis(client, WithLockKeyPrefix(""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cache.Close() })

	ctx := context.Background()

	// 使用 mutex 模拟真实的外部锁行为
	var mu sync.Mutex
	var lockCount, unlockCount int32

	externalLock := func(ctx context.Context, key string, ttl time.Duration) (Unlocker, error) {
		mu.Lock()
		atomic.AddInt32(&lockCount, 1)
		return func(ctx context.Context) error {
			atomic.AddInt32(&unlockCount, 1)
			mu.Unlock()
			return nil
		}, nil
	}

	loader, err := NewLoader(cache,
		WithSingleflight(true), // 启用 singleflight
		WithDistributedLock(true),
		WithExternalLock(externalLock),
	)
	require.NoError(t, err)

	var loadCount int32
	loadFn := func(ctx context.Context) ([]byte, error) {
		atomic.AddInt32(&loadCount, 1)
		time.Sleep(10 * time.Millisecond)
		return []byte("concurrent_value"), nil
	}

	// When - 并发请求
	var wg sync.WaitGroup
	results := make([][]byte, 10)
	errs := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = loader.Load(ctx, "concurrentkey", loadFn, time.Hour)
		}(i)
	}
	wg.Wait()

	// Then
	for i := 0; i < 10; i++ {
		require.NoError(t, errs[i])
		assert.Equal(t, []byte("concurrent_value"), results[i])
	}
	// 由于 singleflight，loadFn 只应该调用一次
	assert.Equal(t, int32(1), atomic.LoadInt32(&loadCount))
	// lock/unlock 应该各调用一次（singleflight 合并了请求）
	assert.Equal(t, int32(1), atomic.LoadInt32(&lockCount))
	assert.Equal(t, int32(1), atomic.LoadInt32(&unlockCount))
}
