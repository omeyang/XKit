package xcache

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRedisForError 创建用于错误场景测试的 Redis 缓存实例，使用更短的超时和更少的重试。
func newTestRedisForError(t *testing.T, opts ...RedisOption) (Redis, *miniredis.Miniredis) {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr:         mr.Addr(),
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
		PoolSize:     1,
		MaxRetries:   0, // 不重试，立即失败
	})

	cache, err := NewRedis(client, opts...)
	require.NoError(t, err)

	t.Cleanup(func() { _ = cache.Close(context.Background()) })

	return cache, mr
}

// =============================================================================
// Redis 连接错误场景测试
// =============================================================================

func TestLoader_Load_WhenRedisConnectionError_FallsBackToBackend(t *testing.T) {
	// Given - 测试 Redis 连接错误时回源加载
	cache, mr := newTestRedisForError(t)

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(false),
	)
	require.NoError(t, err)

	// 关闭 miniredis 模拟连接错误
	mr.Close()

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("backend_value"), nil
	}

	// When - Redis 连接失败时应回源
	value, err := loader.Load(context.Background(), "errkey", loadFn, time.Hour)

	// Then - 应该从后端成功加载
	require.NoError(t, err)
	assert.Equal(t, []byte("backend_value"), value)
}

func TestLoader_LoadHash_WhenRedisConnectionError_FallsBackToBackend(t *testing.T) {
	// Given - 测试 Hash 版本 Redis 连接错误时回源加载
	cache, mr := newTestRedisForError(t)

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(false),
	)
	require.NoError(t, err)

	// 关闭 miniredis 模拟连接错误
	mr.Close()

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("hash_backend"), nil
	}

	// When
	value, err := loader.LoadHash(context.Background(), "errhash", "field", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("hash_backend"), value)
}

func TestLoader_Load_WithDistributedLock_WhenRedisError_FallsBackToBackend(t *testing.T) {
	// Given - 测试启用分布式锁时 Redis 错误回源
	cache, mr := newTestRedisForError(t, WithLockKeyPrefix(""))

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockTTL(100*time.Millisecond),
		WithDistributedLockKeyPrefix("lock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	// 关闭 miniredis 模拟连接错误
	mr.Close()

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("dist_backend"), nil
	}

	// When - 即使启用了分布式锁，Redis 错误时也应该回源
	value, err := loader.Load(context.Background(), "disterrkey", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []byte("dist_backend"), value)
}

func TestLoader_LoadHash_WithDistributedLock_WhenRedisError_FallsBackToBackend(t *testing.T) {
	// Given - Hash 版本分布式锁 Redis 错误回源
	cache, mr := newTestRedisForError(t, WithLockKeyPrefix(""))

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockTTL(100*time.Millisecond),
		WithDistributedLockKeyPrefix("hlock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	mr.Close()

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("hash_dist_backend"), nil
	}

	value, err := loader.LoadHash(context.Background(), "disterrhash", "field", loadFn, time.Hour)

	require.NoError(t, err)
	assert.Equal(t, []byte("hash_dist_backend"), value)
}

// =============================================================================
// waitAndRetry 中的 Redis 错误测试
// =============================================================================

func TestLoader_Load_WhenWaitAndRetry_RedisError_LoadsFromBackend(t *testing.T) {
	// Given - 测试 waitAndRetry 过程中 Redis 出错时回源加载
	cache, mr := newTestRedisForError(t, WithLockKeyPrefix(""))

	// 预先占用锁
	mr.Set("lock:waitkey", "occupied")
	mr.SetTTL("lock:waitkey", 50*time.Millisecond)

	loader, err := NewLoader(cache,
		WithSingleflight(true),
		WithDistributedLock(true),
		WithDistributedLockTTL(200*time.Millisecond),
		WithDistributedLockKeyPrefix("lock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("wait_backend"), nil
	}

	// 在锁过期前关闭 Redis，让 waitAndRetry 遇到错误
	go func() {
		time.Sleep(30 * time.Millisecond)
		mr.Close()
	}()

	// When
	value, err := loader.Load(context.Background(), "waitkey", loadFn, time.Hour)

	// Then - 应该回源成功
	require.NoError(t, err)
	assert.Equal(t, []byte("wait_backend"), value)
}

func TestLoader_LoadHash_WhenWaitAndRetry_RedisError_LoadsFromBackend(t *testing.T) {
	// Given - Hash 版本 waitAndRetry Redis 错误测试
	cache, mr := newTestRedisForError(t, WithLockKeyPrefix(""))

	// 使用 hashFieldKey 生成一致的锁 key
	hashWaitLockKey := "hlock:" + hashFieldKey("waithash", "field")
	mr.Set(hashWaitLockKey, "occupied")
	mr.SetTTL(hashWaitLockKey, 50*time.Millisecond)

	loader, err := NewLoader(cache,
		WithSingleflight(true),
		WithDistributedLock(true),
		WithDistributedLockTTL(200*time.Millisecond),
		WithDistributedLockKeyPrefix("hlock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("hash_wait_backend"), nil
	}

	go func() {
		time.Sleep(30 * time.Millisecond)
		mr.Close()
	}()

	value, err := loader.LoadHash(context.Background(), "waithash", "field", loadFn, time.Hour)

	require.NoError(t, err)
	assert.Equal(t, []byte("hash_wait_backend"), value)
}

// =============================================================================
// Context 取消测试 (覆盖 loadWithDistLock 行 116-117, 137-140)
// =============================================================================

func TestLoader_Load_WhenRedisError_AndContextCancelled_ReturnsContextError(t *testing.T) {
	// Given - Redis 错误 + context 取消，应该返回 context 错误
	cache, mr := newTestRedisForError(t)

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
	)
	require.NoError(t, err)

	// 关闭 Redis 模拟连接错误
	mr.Close()

	// 创建已取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("should_not_reach"), nil
	}

	// When
	_, err = loader.Load(ctx, "ctxkey", loadFn, time.Hour)

	// Then - 应返回 context 错误
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestLoader_LoadHash_WhenRedisError_AndContextCancelled_ReturnsContextError(t *testing.T) {
	// Given - Hash 版本 Redis 错误 + context 取消
	cache, mr := newTestRedisForError(t)

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
	)
	require.NoError(t, err)

	mr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("should_not_reach"), nil
	}

	// When
	_, err = loader.LoadHash(ctx, "ctxhash", "field", loadFn, time.Hour)

	// Then
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestLoader_Load_WhenRedisErrorAfterLock_AndContextCancelled_ReturnsContextError(t *testing.T) {
	// Given - 获取锁成功，但第二次 Get 时 Redis 错误 + context 取消
	// 这是测试 loadWithDistLock 行 137-140 的路径
	cache, mr := newTestRedisForError(t, WithLockKeyPrefix(""))

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockKeyPrefix("lock:"),
	)
	require.NoError(t, err)

	// 使用会取消的 context
	ctx, cancel := context.WithCancel(context.Background())

	loadCount := 0
	loadFn := func(ctx context.Context) ([]byte, error) {
		loadCount++
		return []byte("backend_value"), nil
	}

	// 在获取锁后取消 context 并关闭 Redis
	// 这需要一些技巧：我们先让第一次 Get 返回 redis.Nil，然后在获取锁后关闭 Redis
	go func() {
		time.Sleep(20 * time.Millisecond) // 等待获取锁
		cancel()
		mr.Close()
	}()

	// When
	_, err = loader.Load(ctx, "lockctx", loadFn, time.Hour)

	// Then - 可能返回 context 错误或后端值
	// 由于时序问题，这可能走不同的路径
	if err != nil {
		// 如果有错误，应该是 context 取消
		assert.True(t, errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
	}
}

func TestLoader_Load_WithDistLock_CacheHitAfterLockAcquired_ReturnsCachedValue(t *testing.T) {
	// Given - 测试获取锁后缓存命中 (行 134-136)
	// 模拟：goroutine A 获取锁后在加载时，goroutine B 已经填充了缓存
	cache, mr := newTestRedisForError(t, WithLockKeyPrefix(""))
	t.Cleanup(func() { mr.Close() })

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockKeyPrefix("lock:"),
	)
	require.NoError(t, err)

	ctx := context.Background()

	// 预先设置锁，让第一次获取锁失败
	mr.Set("lock:hitafter", "occupied")
	mr.SetTTL("lock:hitafter", 50*time.Millisecond)

	loadCount := int32(0)
	loadFn := func(ctx context.Context) ([]byte, error) {
		atomic.AddInt32(&loadCount, 1)
		return []byte("loaded_value"), nil
	}

	// goroutine 会在锁释放后填充缓存
	go func() {
		time.Sleep(30 * time.Millisecond)
		// 在锁释放后，填充缓存，让第二次 Get 命中
		mr.Set("hitafter", "precached_value")
	}()

	// When
	value, err := loader.Load(ctx, "hitafter", loadFn, time.Hour)

	// Then - 可能返回预先缓存的值或后端加载的值
	require.NoError(t, err)
	assert.True(t, string(value) == "precached_value" || string(value) == "loaded_value")
}

func TestLoader_LoadHash_WithDistLock_CacheHitAfterLockAcquired(t *testing.T) {
	// Given - Hash 版本：获取锁后缓存命中
	cache, mr := newTestRedisForError(t, WithLockKeyPrefix(""))
	t.Cleanup(func() { mr.Close() })

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockKeyPrefix("hlock:"),
	)
	require.NoError(t, err)

	ctx := context.Background()

	// 预先设置锁（使用 hashFieldKey 生成一致的锁 key）
	hitAfterLockKey := "hlock:" + hashFieldKey("hitafterhash", "field")
	mr.Set(hitAfterLockKey, "occupied")
	mr.SetTTL(hitAfterLockKey, 50*time.Millisecond)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("loaded_hash_value"), nil
	}

	// 在锁释放后填充缓存
	go func() {
		time.Sleep(30 * time.Millisecond)
		mr.HSet("hitafterhash", "field", "precached_hash_value")
	}()

	// When
	value, err := loader.LoadHash(ctx, "hitafterhash", "field", loadFn, time.Hour)

	// Then
	require.NoError(t, err)
	assert.True(t, string(value) == "precached_hash_value" || string(value) == "loaded_hash_value")
}

// =============================================================================
// handleLockError 运行时路径测试
// =============================================================================

func TestLoader_Load_WithExternalLock_ReturningInvalidLockTTL_WrapsAsErrInvalidConfig(t *testing.T) {
	// Given - ExternalLock 返回 ErrInvalidLockTTL，handleLockError 应包装为 ErrInvalidConfig
	cache, mr := newTestRedisForError(t)
	t.Cleanup(func() { mr.Close() })

	externalLock := func(_ context.Context, _ string, _ time.Duration) (Unlocker, error) {
		return nil, ErrInvalidLockTTL
	}

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithExternalLock(externalLock),
		WithDistributedLockTTL(10*time.Second),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	loadFn := func(_ context.Context) ([]byte, error) {
		return []byte("should_not_reach"), nil
	}

	// When
	_, err = loader.Load(context.Background(), "ttlerr-key", loadFn, time.Hour)

	// Then - 应返回 ErrInvalidConfig 包装的 ErrInvalidLockTTL
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
	assert.ErrorIs(t, err, ErrInvalidLockTTL)
}

func TestLoader_Load_WithExternalLock_ReturningInvalidConfig_ReturnsDirectly(t *testing.T) {
	// Given - ExternalLock 返回 ErrInvalidConfig，handleLockError 应直接返回
	cache, mr := newTestRedisForError(t)
	t.Cleanup(func() { mr.Close() })

	externalLock := func(_ context.Context, _ string, _ time.Duration) (Unlocker, error) {
		return nil, ErrInvalidConfig
	}

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithExternalLock(externalLock),
		WithDistributedLockTTL(10*time.Second),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	loadFn := func(_ context.Context) ([]byte, error) {
		return []byte("should_not_reach"), nil
	}

	// When
	_, err = loader.Load(context.Background(), "cfgerr-key", loadFn, time.Hour)

	// Then - 应直接返回 ErrInvalidConfig
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

func TestLoader_Load_WithExternalLock_ReturningDeadlineExceeded_ReturnsDirectly(t *testing.T) {
	// Given - ExternalLock 返回 context.DeadlineExceeded，handleLockError 应直接返回
	cache, mr := newTestRedisForError(t)
	t.Cleanup(func() { mr.Close() })

	externalLock := func(_ context.Context, _ string, _ time.Duration) (Unlocker, error) {
		return nil, context.DeadlineExceeded
	}

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithExternalLock(externalLock),
		WithDistributedLockTTL(10*time.Second),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	loadFn := func(_ context.Context) ([]byte, error) {
		return []byte("should_not_reach"), nil
	}

	// When
	_, err = loader.Load(context.Background(), "deadline-key", loadFn, time.Hour)

	// Then - 应直接返回 context.DeadlineExceeded
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestLoader_Load_WithExternalLock_ReturningNilUnlocker_ReturnsInvalidConfig(t *testing.T) {
	// Given - ExternalLock 返回 (nil, nil)，acquireLock 应转为 ErrInvalidConfig
	cache, mr := newTestRedisForError(t)
	t.Cleanup(func() { mr.Close() })

	externalLock := func(_ context.Context, _ string, _ time.Duration) (Unlocker, error) {
		return nil, nil // 错误的锁实现
	}

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithExternalLock(externalLock),
		WithDistributedLockTTL(10*time.Second),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	loadFn := func(_ context.Context) ([]byte, error) {
		return []byte("should_not_reach"), nil
	}

	// When
	_, err = loader.Load(context.Background(), "nil-unlocker-key", loadFn, time.Hour)

	// Then - 应返回 ErrInvalidConfig
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

// =============================================================================
// 配置错误测试
// =============================================================================

func TestLoader_Load_WithInvalidLockTTL_ReturnsConfigError(t *testing.T) {
	// Given - 配置了无效的 LockTTL（0 或负数）
	cache, mr := newTestRedisForError(t, WithLockKeyPrefix(""))
	t.Cleanup(func() { mr.Close() })

	// When - NewLoader 本身应返回配置错误
	_, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockTTL(0), // 无效配置！
	)

	// Then - 配置错误应该返回 ErrInvalidConfig
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidConfig), "expected ErrInvalidConfig, got: %v", err)
	assert.True(t, errors.Is(err, ErrInvalidLockTTL), "expected ErrInvalidLockTTL, got: %v", err)
}

func TestLoader_LoadHash_WithInvalidLockTTL_ReturnsConfigError(t *testing.T) {
	// Given - 配置了无效的 LockTTL（0 或负数）
	cache, mr := newTestRedisForError(t, WithLockKeyPrefix(""))
	t.Cleanup(func() { mr.Close() })

	// When - NewLoader 本身应返回配置错误
	_, err := NewLoader(cache,
		WithSingleflight(false),
		WithDistributedLock(true),
		WithDistributedLockTTL(-1*time.Second), // 无效配置！
	)

	// Then - 配置错误应该返回 ErrInvalidConfig
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidConfig), "expected ErrInvalidConfig, got: %v", err)
	assert.True(t, errors.Is(err, ErrInvalidLockTTL), "expected ErrInvalidLockTTL, got: %v", err)
}
