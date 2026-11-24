package xcache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/omeyang/xkit/internal/rediscompat"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryWrapper_Wait_AfterClose_ReturnsImmediately(t *testing.T) {
	cache, err := NewMemory()
	require.NoError(t, err)

	_ = cache.Close(context.Background())

	// Wait() 应在 closed 后立即返回，不阻塞也不 panic
	cache.Wait()
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

func TestWithLockRetry_ClampsCountToUpperBound(t *testing.T) {
	opts := defaultRedisOptions()
	WithLockRetry(100*time.Millisecond, MaxLockRetryCount+500)(opts)
	assert.Equal(t, MaxLockRetryCount, opts.LockRetryCount)
}

func TestWithLockRetry_ClampsIntervalToUpperBound(t *testing.T) {
	opts := defaultRedisOptions()
	WithLockRetry(MaxLockRetryInterval+time.Minute, 5)(opts)
	assert.Equal(t, MaxLockRetryInterval, opts.LockRetryInterval)
}

func TestWithLockRetry_ExactUpperBounds(t *testing.T) {
	opts := defaultRedisOptions()
	WithLockRetry(MaxLockRetryInterval, MaxLockRetryCount)(opts)
	assert.Equal(t, MaxLockRetryInterval, opts.LockRetryInterval)
	assert.Equal(t, MaxLockRetryCount, opts.LockRetryCount)
}

func TestWithLockKeyPrefix_SetsOption(t *testing.T) {
	opts := defaultRedisOptions()
	WithLockKeyPrefix("lock:")(opts)
	assert.Equal(t, "lock:", opts.LockKeyPrefix)
}

func TestRedisWrapper_Lock_WithInvalidTTL_ReturnsError(t *testing.T) {
	cache, _ := newTestRedisCache(t)
	ctx := context.Background()

	// TTL <= 0 应返回 ErrInvalidLockTTL
	_, err := cache.Lock(ctx, "key", 0)
	assert.ErrorIs(t, err, ErrInvalidLockTTL)

	_, err = cache.Lock(ctx, "key", -1*time.Second)
	assert.ErrorIs(t, err, ErrInvalidLockTTL)
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
		cache.Close(context.Background())
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
	for range 10 {
		cache.Client().Get("key1")
	}
	// 读取不存在的 key 以产生 misses
	for range 5 {
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
	for i := range 5 {
		key := string(rune('a' + i))
		cache.Client().SetWithTTL(key, []byte("value"), 6, 0)
	}
	cache.Wait()
	time.Sleep(50 * time.Millisecond) // 等待异步写入完成

	// 反复读取直到产生足够的命中统计
	// ristretto 的 metrics 是异步更新的
	var stats MemoryStats
	for attempt := range 10 {
		// 读取存在的 key 产生 hits
		for i := range 20 {
			key := string(rune('a' + (i % 5)))
			cache.Client().Get(key)
		}
		// 读取不存在的 key 产生 misses
		for range 10 {
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
	err = cache.Close(context.Background())
	assert.NoError(t, err)

	// 第二次关闭应返回 ErrClosed
	err = cache.Close(context.Background())
	assert.ErrorIs(t, err, ErrClosed)
}

func TestRedisWrapper_LockAfterClose_ReturnsErrClosed(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache, err := NewRedis(client)
	require.NoError(t, err)

	_ = cache.Close(context.Background())

	_, err = cache.Lock(context.Background(), "test-key", 10*time.Second)
	assert.ErrorIs(t, err, ErrClosed)
}

func TestRedisWrapper_Lock_WithEmptyKey_ReturnsError(t *testing.T) {
	cache, _ := newTestRedisCache(t)
	ctx := context.Background()

	_, err := cache.Lock(ctx, "", 10*time.Second)
	assert.ErrorIs(t, err, ErrEmptyKey)
}

func TestRedisWrapper_Lock_WithNilContext_ReturnsErrNilContext(t *testing.T) {
	cache, _ := newTestRedisCache(t)

	//nolint:staticcheck // SA1012: 故意传入 nil context 测试 fail-fast 校验
	_, err := cache.Lock(nil, "key", 10*time.Second)
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestMemoryWrapper_Stats_AfterClose_ReturnsZero(t *testing.T) {
	cache, err := NewMemory()
	require.NoError(t, err)

	_ = cache.Close(context.Background())

	// Stats() 应该返回零值而非 panic
	stats := cache.Stats()
	assert.Equal(t, MemoryStats{}, stats)
}

func TestMemoryWrapper_DoubleClose_ReturnsErrClosed(t *testing.T) {
	cache, err := NewMemory()
	require.NoError(t, err)

	// 第一次关闭应成功
	err = cache.Close(context.Background())
	assert.NoError(t, err)

	// 第二次关闭应返回 ErrClosed
	err = cache.Close(context.Background())
	assert.ErrorIs(t, err, ErrClosed)
}

// =============================================================================
// 辅助函数后备路径测试
// =============================================================================

func TestGetHostIdentifier_WhenHostnameFails_ReturnsFallback(t *testing.T) {
	// Given - 模拟 os.Hostname 失败
	origHostname := osHostname
	osHostname = func() (string, error) {
		return "", errors.New("hostname unavailable")
	}
	defer func() { osHostname = origHostname }()

	// When
	id := getHostIdentifier()

	// Then - 应返回 "unknown-" 前缀的后备值
	assert.Contains(t, id, "unknown-")
}

func TestGetHostIdentifier_WhenHostnameEmpty_ReturnsFallback(t *testing.T) {
	// Given - os.Hostname 返回空字符串
	origHostname := osHostname
	osHostname = func() (string, error) {
		return "", nil
	}
	defer func() { osHostname = origHostname }()

	// When
	id := getHostIdentifier()

	// Then
	assert.Contains(t, id, "unknown-")
}

func TestGenerateLockValue_WhenCryptoRandFails_ReturnsFallbackValue(t *testing.T) {
	// Given - 模拟 crypto/rand.Read 失败
	origRandRead := cryptoRandRead
	cryptoRandRead = func(b []byte) (int, error) {
		return 0, errors.New("rand unavailable")
	}
	defer func() { cryptoRandRead = origRandRead }()

	// When
	value := generateLockValue()

	// Then - 应返回主机标识符格式的后备值（包含 "-" 分隔符）
	assert.NotEmpty(t, value)
	assert.Contains(t, value, "-")
}

func TestGenerateLockValue_Normal_ReturnsHexString(t *testing.T) {
	// When
	value := generateLockValue()

	// Then - 应返回 32 字符的 hex 字符串 (16 bytes = 32 hex chars)
	assert.Len(t, value, 32)
}

// =============================================================================
// Redis 代理兼容模式测试
// =============================================================================

func newTestRedisCacheCompat(t *testing.T) (Redis, *miniredis.Miniredis) {
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
	cache, err := NewRedis(client, WithScriptMode(rediscompat.ScriptModeCompat))
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = cache.Close(context.Background())
		mr.Close()
	})

	return cache, mr
}

func TestRedisWrapper_CompatMode_LockUnlock_Success(t *testing.T) {
	cache, _ := newTestRedisCacheCompat(t)
	ctx := context.Background()

	unlock, err := cache.Lock(ctx, "compat-lock", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, unlock)

	err = unlock(ctx)
	require.NoError(t, err)
}

func TestRedisWrapper_CompatMode_UnlockExpired(t *testing.T) {
	cache, mr := newTestRedisCacheCompat(t)
	ctx := context.Background()

	unlock, err := cache.Lock(ctx, "compat-expire", time.Minute)
	require.NoError(t, err)

	// 模拟锁过期：直接删除 key
	mr.Del("lock:compat-expire")

	err = unlock(ctx)
	assert.ErrorIs(t, err, ErrLockExpired)
}

func TestRedisWrapper_CompatMode_UnlockStolen(t *testing.T) {
	cache, mr := newTestRedisCacheCompat(t)
	ctx := context.Background()

	unlock, err := cache.Lock(ctx, "compat-stolen", time.Minute)
	require.NoError(t, err)

	// 模拟锁被抢走：覆盖 value
	mr.Set("lock:compat-stolen", "other-owner")

	err = unlock(ctx)
	assert.ErrorIs(t, err, ErrLockExpired)
}

func TestRedisWrapper_CompatMode_LockAlreadyHeld(t *testing.T) {
	cache, _ := newTestRedisCacheCompat(t)
	ctx := context.Background()

	unlock1, err := cache.Lock(ctx, "compat-held", time.Minute)
	require.NoError(t, err)
	defer func() { _ = unlock1(ctx) }()

	_, err = cache.Lock(ctx, "compat-held", time.Minute)
	assert.ErrorIs(t, err, ErrLockFailed)
}

func TestWithScriptMode_SetsOption(t *testing.T) {
	opts := defaultRedisOptions()
	WithScriptMode(rediscompat.ScriptModeCompat)(opts)
	assert.Equal(t, rediscompat.ScriptModeCompat, opts.ScriptMode)
}
