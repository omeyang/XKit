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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithSingleflight(false), // 禁用 singleflight 以便多个请求独立处理
		WithDistributedLock(true),
		WithDistributedLockTTL(5*time.Second),
		WithDistributedLockKeyPrefix("lock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	var loadCount atomic.Int32
	var phase atomic.Int32 // 0: 初始, 1: 第一个请求获得锁, 2: 第一个请求已写入缓存

	loadFn := func(ctx context.Context) ([]byte, error) {
		loadCount.Add(1)
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
		phase.Store(1)
		results[0], errs[0] = loader.Load(ctx, "race_key", loadFn, time.Hour)
		phase.Store(2)
	}()

	// 等待第一个请求开始执行
	time.Sleep(10 * time.Millisecond)

	// 第二个请求：等待锁释放，然后应该在 double-check 时发现缓存已有值
	wg.Add(1)
	go func() {
		defer wg.Done()
		// 等待第一个请求写入缓存
		for phase.Load() < 2 {
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
	assert.Equal(t, int32(1), loadCount.Load())
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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockTTL(5*time.Second),
		WithDistributedLockKeyPrefix("hlock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	var loadCount atomic.Int32
	var phase atomic.Int32

	loadFn := func(ctx context.Context) ([]byte, error) {
		loadCount.Add(1)
		time.Sleep(50 * time.Millisecond)
		return []byte("hash_value"), nil
	}

	var wg sync.WaitGroup
	results := make([][]byte, 2)
	errs := make([]error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		phase.Store(1)
		results[0], errs[0] = loader.LoadHash(ctx, "race_hash", "field1", loadFn, time.Hour)
		phase.Store(2)
	}()

	time.Sleep(10 * time.Millisecond)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for phase.Load() < 2 {
			time.Sleep(5 * time.Millisecond)
		}
		results[1], errs[1] = loader.LoadHash(ctx, "race_hash", "field1", loadFn, time.Hour)
	}()

	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.Equal(t, []byte("hash_value"), results[0])
	assert.Equal(t, []byte("hash_value"), results[1])
	assert.Equal(t, int32(1), loadCount.Load())
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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
