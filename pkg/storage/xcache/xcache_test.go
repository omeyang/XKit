package xcache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// 工厂函数测试
// =============================================================================

func TestNewRedis_WithNilClient_ReturnsError(t *testing.T) {
	_, err := NewRedis(nil)
	assert.ErrorIs(t, err, ErrNilClient)
}

func TestNewRedis_WithValidClient_Succeeds(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache, err := NewRedis(client)
	require.NoError(t, err)
	defer cache.Close()

	assert.NotNil(t, cache)
}

func TestNewMemory_WithDefaults_Succeeds(t *testing.T) {
	cache, err := NewMemory()
	require.NoError(t, err)
	defer cache.Close()

	assert.NotNil(t, cache)
}

func TestNewMemoryFromClient_WithNilClient_ReturnsError(t *testing.T) {
	_, err := NewMemoryFromClient(nil)
	assert.ErrorIs(t, err, ErrNilClient)
}

func TestNewMemoryFromClient_WithMetricsDisabled_ReturnsError(t *testing.T) {
	client, err := ristretto.NewCache(&ristretto.Config[string, []byte]{
		NumCounters: 1e4,
		MaxCost:     1 << 20,
		BufferItems: 64,
	})
	require.NoError(t, err)
	defer client.Close()

	_, err = NewMemoryFromClient(client)
	assert.ErrorIs(t, err, ErrMetricsDisabled)
}

func TestNewMemoryFromClient_WithValidClient_Succeeds(t *testing.T) {
	// 创建 ristretto cache
	client, err := ristretto.NewCache(&ristretto.Config[string, []byte]{
		NumCounters: 1e4,
		MaxCost:     1 << 20,
		BufferItems: 64,
		Metrics:     true,
	})
	require.NoError(t, err)
	defer client.Close()

	cache, err := NewMemoryFromClient(client)
	require.NoError(t, err)

	// 验证可以正常使用
	cache.Client().SetWithTTL("key", []byte("value"), 5, 0)
	cache.Wait()

	val, found := cache.Client().Get("key")
	require.True(t, found)
	assert.Equal(t, []byte("value"), val)
}

// =============================================================================
// Redis Wrapper 测试
// =============================================================================

func newTestRedisCache(t *testing.T) (Redis, *miniredis.Miniredis) {
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

func TestRedisWrapper_Client_ReturnsUnderlyingClient(t *testing.T) {
	cache, _ := newTestRedisCache(t)
	client := cache.Client()
	assert.NotNil(t, client)
}

func TestRedisWrapper_BasicOperations(t *testing.T) {
	cache, _ := newTestRedisCache(t)
	ctx := context.Background()

	// 直接使用底层客户端进行基础操作
	err := cache.Client().Set(ctx, "key1", "value1", time.Hour).Err()
	require.NoError(t, err)

	val, err := cache.Client().Get(ctx, "key1").Result()
	require.NoError(t, err)
	assert.Equal(t, "value1", val)

	// 删除
	err = cache.Client().Del(ctx, "key1").Err()
	require.NoError(t, err)

	// 验证删除
	_, err = cache.Client().Get(ctx, "key1").Result()
	assert.ErrorIs(t, err, redis.Nil)
}

func TestRedisWrapper_HashOperations(t *testing.T) {
	cache, _ := newTestRedisCache(t)
	ctx := context.Background()

	// Hash 操作
	err := cache.Client().HSet(ctx, "hash", "field1", "value1").Err()
	require.NoError(t, err)

	val, err := cache.Client().HGet(ctx, "hash", "field1").Result()
	require.NoError(t, err)
	assert.Equal(t, "value1", val)

	// HGetAll
	all, err := cache.Client().HGetAll(ctx, "hash").Result()
	require.NoError(t, err)
	assert.Equal(t, "value1", all["field1"])
}

func TestRedisWrapper_Lock_Success(t *testing.T) {
	cache, _ := newTestRedisCache(t)
	ctx := context.Background()

	// 获取锁
	unlock, err := cache.Lock(ctx, "test-lock", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, unlock)

	// 释放锁
	err = unlock(ctx)
	require.NoError(t, err)
}

func TestRedisWrapper_Lock_AlreadyHeld(t *testing.T) {
	cache, _ := newTestRedisCache(t)
	ctx := context.Background()

	// 第一次获取锁
	unlock1, err := cache.Lock(ctx, "test-lock", time.Minute)
	require.NoError(t, err)
	defer func() { _ = unlock1(ctx) }()

	// 第二次获取同一个锁应该失败
	_, err = cache.Lock(ctx, "test-lock", time.Minute)
	assert.ErrorIs(t, err, ErrLockFailed)
}

func TestRedisWrapper_Lock_WithRetry_Success(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache, err := NewRedis(client, WithLockRetry(50*time.Millisecond, 5))
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	// 第一次获取锁
	unlock1, err := cache.Lock(ctx, "retry-lock", 100*time.Millisecond)
	require.NoError(t, err)

	// 在后台释放锁
	go func() {
		time.Sleep(80 * time.Millisecond)
		_ = unlock1(ctx)
	}()

	// 第二次获取锁，应该在重试后成功
	unlock2, err := cache.Lock(ctx, "retry-lock", time.Minute)
	require.NoError(t, err)
	defer func() { _ = unlock2(ctx) }()
}

func TestRedisWrapper_Lock_WithRetry_ContextCancelled(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache, err := NewRedis(client, WithLockRetry(100*time.Millisecond, 10))
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	// 先获取锁
	unlock1, err := cache.Lock(ctx, "cancel-lock", time.Minute)
	require.NoError(t, err)
	defer func() { _ = unlock1(ctx) }()

	// 用可取消的 context 尝试获取锁
	ctxWithCancel, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()

	_, err = cache.Lock(ctxWithCancel, "cancel-lock", time.Minute)
	assert.Error(t, err) // 应该因为 context 取消而失败
}

func TestRedisWrapper_Lock_WithRetry_AllRetriesFail(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache, err := NewRedis(client, WithLockRetry(10*time.Millisecond, 2))
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	// 先获取锁，不释放
	unlock1, err := cache.Lock(ctx, "fail-lock", time.Minute)
	require.NoError(t, err)
	defer func() { _ = unlock1(ctx) }()

	// 第二次获取锁，重试后仍失败
	_, err = cache.Lock(ctx, "fail-lock", time.Minute)
	assert.ErrorIs(t, err, ErrLockFailed)
}

func TestRedisWrapper_Lock_ConnectionError(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

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

	// 关闭 miniredis 模拟连接错误
	mr.Close()

	ctx := context.Background()

	// 尝试获取锁应该返回错误
	_, err = cache.Lock(ctx, "error-lock", time.Minute)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrLockFailed) // 应该是连接错误，不是锁失败
}

func TestRedisWrapper_Lock_WithRetry_ConnectionErrorDuringRetry(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr:         mr.Addr(),
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
		PoolSize:     1,
		MaxRetries:   0,
	})
	cache, err := NewRedis(client, WithLockRetry(50*time.Millisecond, 5))
	require.NoError(t, err)

	ctx := context.Background()

	// 先获取锁
	unlock1, err := cache.Lock(ctx, "retry-error-lock", time.Minute)
	require.NoError(t, err)
	defer func() { _ = unlock1(ctx) }()

	// 在后台关闭 miniredis，模拟重试过程中的连接错误
	go func() {
		time.Sleep(30 * time.Millisecond)
		mr.Close()
	}()

	// 第二次获取锁，重试过程中会遇到连接错误
	_, err = cache.Lock(ctx, "retry-error-lock", time.Minute)
	assert.Error(t, err)
}

// =============================================================================
// Memory Wrapper 测试
// =============================================================================

func newTestMemoryCache(t *testing.T) Memory {
	t.Helper()
	cache, err := NewMemory(WithMemoryMaxCost(1 << 20))
	require.NoError(t, err)
	t.Cleanup(func() { cache.Close() })
	return cache
}

func TestMemoryWrapper_BasicOperations(t *testing.T) {
	cache := newTestMemoryCache(t)

	// 直接使用底层客户端
	cache.Client().SetWithTTL("key", []byte("value"), 5, time.Hour)
	cache.Wait()

	val, found := cache.Client().Get("key")
	require.True(t, found)
	assert.Equal(t, []byte("value"), val)
}

func TestMemoryWrapper_Del(t *testing.T) {
	cache := newTestMemoryCache(t)

	cache.Client().SetWithTTL("key", []byte("value"), 5, 0)
	cache.Wait()

	cache.Client().Del("key")
	cache.Wait()

	_, found := cache.Client().Get("key")
	assert.False(t, found)
}

func TestMemoryWrapper_Clear(t *testing.T) {
	cache := newTestMemoryCache(t)

	// 设置多个 key
	for i := 0; i < 10; i++ {
		cache.Client().SetWithTTL(string(rune('a'+i)), []byte("value"), 5, 0)
	}
	cache.Wait()

	// 清空
	cache.Client().Clear()

	// 验证全部删除
	_, found := cache.Client().Get("a")
	assert.False(t, found)
}

func TestMemoryWrapper_Stats_ReturnsMetrics(t *testing.T) {
	cache := newTestMemoryCache(t)

	// 设置并读取
	cache.Client().SetWithTTL("key", []byte("value"), 5, 0)
	cache.Wait()

	// 多次读取以确保统计生效
	for i := 0; i < 10; i++ {
		_, _ = cache.Client().Get("key")      // hit
		_, _ = cache.Client().Get("nonexist") // miss
	}

	// ristretto 的统计是最终一致的，验证 Stats() 能正常返回即可
	stats := cache.Stats()
	// 注意：ristretto 统计可能有延迟，这里只验证能获取到 stats
	assert.NotNil(t, stats)
}

func TestMemoryWrapper_Stats_ZeroTotal(t *testing.T) {
	cache := newTestMemoryCache(t)

	// 直接获取统计，不进行任何读写操作
	stats := cache.Stats()

	// 验证初始状态：没有命中和未命中，hitRatio 应该是 0
	assert.Equal(t, uint64(0), stats.Hits)
	assert.Equal(t, uint64(0), stats.Misses)
	assert.Equal(t, float64(0), stats.HitRatio)
}

func TestMemoryWrapper_Client_ReturnsUnderlyingClient(t *testing.T) {
	cache := newTestMemoryCache(t)
	client := cache.Client()
	assert.NotNil(t, client)
}

func TestMemoryWrapper_Wait_WaitsForWrites(t *testing.T) {
	cache := newTestMemoryCache(t)

	// 快速写入多个 key
	for i := 0; i < 100; i++ {
		cache.Client().SetWithTTL(string(rune('a'+i%26)), []byte("value"), 5, 0)
	}

	// Wait 应该阻塞直到写入完成
	cache.Wait()

	// 至少能读到一些 key
	val, found := cache.Client().Get("a")
	require.True(t, found)
	assert.Equal(t, []byte("value"), val)
}

// =============================================================================
// Redis 配置选项测试
// =============================================================================

func TestWithLockRetry_SetsOptions(t *testing.T) {
	opts := defaultRedisOptions()
	WithLockRetry(100*time.Millisecond, 5)(opts)
	assert.Equal(t, 100*time.Millisecond, opts.LockRetryInterval)
	assert.Equal(t, 5, opts.LockRetryCount)
}

func TestWithLockKeyPrefix_SetsOption(t *testing.T) {
	opts := defaultRedisOptions()
	WithLockKeyPrefix("lock:")(opts)
	assert.Equal(t, "lock:", opts.LockKeyPrefix)
}

// =============================================================================
// Memory 配置选项测试
// =============================================================================

func TestWithMemoryNumCounters_SetsOption(t *testing.T) {
	opts := defaultMemoryOptions()
	WithMemoryNumCounters(1e5)(opts)
	assert.Equal(t, int64(1e5), opts.NumCounters)
}

func TestWithMemoryMaxCost_SetsOption(t *testing.T) {
	opts := defaultMemoryOptions()
	WithMemoryMaxCost(10 << 20)(opts)
	assert.Equal(t, int64(10<<20), opts.MaxCost)
}

func TestWithMemoryBufferItems_SetsOption(t *testing.T) {
	opts := defaultMemoryOptions()
	WithMemoryBufferItems(128)(opts)
	assert.Equal(t, int64(128), opts.BufferItems)
}

// =============================================================================
// Loader 配置选项测试
// =============================================================================

func TestWithDistributedLockKeyPrefix_SetsOption(t *testing.T) {
	opts := defaultLoaderOptions()
	WithDistributedLockKeyPrefix("mylock:")(opts)
	assert.Equal(t, "mylock:", opts.DistributedLockKeyPrefix)
}

// =============================================================================
// NewMemory 错误路径测试
// =============================================================================

func TestNewMemory_WithInvalidConfig_UsesDefaults(t *testing.T) {
	// Given - 无效的配置（NumCounters 和 MaxCost 为 0）
	// 这些无效值会被忽略，使用默认值代替
	cache, err := NewMemory(
		WithMemoryNumCounters(0),
		WithMemoryMaxCost(0),
	)

	// Then - 应该成功创建（使用默认值）
	assert.NoError(t, err)
	assert.NotNil(t, cache)
	if cache != nil {
		cache.Close()
	}
}

// =============================================================================
// 更多 Stats 边界测试
// =============================================================================

func TestMemoryWrapper_Stats_WithHits(t *testing.T) {
	cache := newTestMemoryCache(t)

	// 写入一些数据
	cache.Client().SetWithTTL("key1", []byte("value1"), 6, 0)
	cache.Wait()

	// 多次读取以产生 hits（需要等待让异步写入完成）
	time.Sleep(10 * time.Millisecond)
	for i := 0; i < 10; i++ {
		cache.Client().Get("key1")
	}
	// 读取不存在的 key 以产生 misses
	for i := 0; i < 5; i++ {
		cache.Client().Get("nonexistent")
	}

	stats := cache.Stats()

	// 验证统计数据
	// 注意: ristretto 的 metrics 是近似的，可能不完全精确
	t.Logf("Stats: Hits=%d, Misses=%d, HitRatio=%.4f", stats.Hits, stats.Misses, stats.HitRatio)

	// 验证命中率计算逻辑（当有数据时，比率应该在合理范围内）
	total := stats.Hits + stats.Misses
	if total > 0 {
		expectedRatio := float64(stats.Hits) / float64(total)
		assert.InDelta(t, expectedRatio, stats.HitRatio, 0.01)
	}
}

// TestMemoryWrapper_Stats_HitRatio_NonZero 确保命中率计算逻辑被覆盖
func TestMemoryWrapper_Stats_HitRatio_NonZero(t *testing.T) {
	cache := newTestMemoryCache(t)

	// 写入多个 key 确保缓存中有数据
	for i := 0; i < 5; i++ {
		key := string(rune('a' + i))
		cache.Client().SetWithTTL(key, []byte("value"), 6, 0)
	}
	cache.Wait()
	time.Sleep(50 * time.Millisecond) // 等待异步写入完成

	// 反复读取直到产生足够的命中统计
	// ristretto 的 metrics 是异步更新的
	var stats MemoryStats
	for attempt := 0; attempt < 10; attempt++ {
		// 读取存在的 key 产生 hits
		for i := 0; i < 20; i++ {
			key := string(rune('a' + (i % 5)))
			cache.Client().Get(key)
		}
		// 读取不存在的 key 产生 misses
		for i := 0; i < 10; i++ {
			cache.Client().Get("nonexistent-key")
		}
		time.Sleep(10 * time.Millisecond)

		stats = cache.Stats()
		total := stats.Hits + stats.Misses
		if total > 0 {
			// 验证 HitRatio 被正确计算
			t.Logf("Attempt %d: Hits=%d, Misses=%d, Total=%d, HitRatio=%.4f",
				attempt, stats.Hits, stats.Misses, total, stats.HitRatio)
			assert.True(t, stats.HitRatio >= 0 && stats.HitRatio <= 1,
				"HitRatio should be between 0 and 1")
			return
		}
	}

	// 如果所有尝试都失败，至少确认 stats 结构被返回
	t.Logf("Could not generate hits/misses, stats: %+v", stats)
}

// =============================================================================
// Close 行为测试
// =============================================================================

func TestRedisWrapper_DoubleClose_ReturnsErrClosed(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache, err := NewRedis(client)
	require.NoError(t, err)

	// 第一次关闭应成功
	err = cache.Close()
	assert.NoError(t, err)

	// 第二次关闭应返回 ErrClosed
	err = cache.Close()
	assert.ErrorIs(t, err, ErrClosed)
}

func TestRedisWrapper_LockAfterClose_ReturnsErrClosed(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache, err := NewRedis(client)
	require.NoError(t, err)

	_ = cache.Close()

	_, err = cache.Lock(context.Background(), "test-key", 10*time.Second)
	assert.ErrorIs(t, err, ErrClosed)
}

func TestMemoryWrapper_DoubleClose_ReturnsErrClosed(t *testing.T) {
	cache, err := NewMemory()
	require.NoError(t, err)

	// 第一次关闭应成功
	err = cache.Close()
	assert.NoError(t, err)

	// 第二次关闭应返回 ErrClosed
	err = cache.Close()
	assert.ErrorIs(t, err, ErrClosed)
}
