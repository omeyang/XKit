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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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

	var loadCount atomic.Int32
	loadFn := func(ctx context.Context) ([]byte, error) {
		loadCount.Add(1)
		time.Sleep(10 * time.Millisecond)
		return []byte("concurrent_value"), nil
	}

	// When - 并发请求
	var wg sync.WaitGroup
	results := make([][]byte, 10)
	errs := make([]error, 10)

	for i := range 10 {
		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[idx], errs[idx] = loader.Load(ctx, "concurrentkey", loadFn, time.Hour)
		}()
	}
	wg.Wait()

	// Then
	for i := range 10 {
		require.NoError(t, errs[i])
		assert.Equal(t, []byte("concurrent_value"), results[i])
	}
	// 由于 singleflight，loadFn 只应该调用一次
	assert.Equal(t, int32(1), loadCount.Load())
	// lock/unlock 应该各调用一次（singleflight 合并了请求）
	assert.Equal(t, int32(1), atomic.LoadInt32(&lockCount))
	assert.Equal(t, int32(1), atomic.LoadInt32(&unlockCount))
}
