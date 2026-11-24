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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithSingleflight(true),
		WithDistributedLock(true),
		WithDistributedLockTTL(10*time.Second),
		WithDistributedLockKeyPrefix("lock:"),
		WithLoadTimeout(0),
	)
	require.NoError(t, err)

	var loadCount atomic.Int32
	loadFn := func(ctx context.Context) ([]byte, error) {
		loadCount.Add(1)
		return []byte("from_backend"), nil
	}

	// When - 并发请求
	var wg sync.WaitGroup
	results := make([][]byte, 5)
	errs := make([]error, 5)

	for i := range 5 {
		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[idx], errs[idx] = loader.Load(ctx, "dcheck_key", loadFn, time.Hour)
		}()
	}
	wg.Wait()

	// Then
	for i := range 5 {
		require.NoError(t, errs[i])
		assert.Equal(t, []byte("from_backend"), results[i])
	}
	// 由于 singleflight + distributed lock double-check，loadFn 只应该调用一次
	assert.Equal(t, int32(1), loadCount.Load())
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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
	t.Cleanup(func() { _ = cache.Close(context.Background()) })

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
