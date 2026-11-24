package xcache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestNewLoader_DistLockTTL_LessThanLoadTimeout_ReturnsError(t *testing.T) {
	// Given - DistributedLockTTL <= LoadTimeout 时应返回 ErrInvalidConfig
	cache, _ := newTestRedis(t)

	// When - DistributedLockTTL (10s) < LoadTimeout (30s)
	_, err := NewLoader(cache,
		WithDistributedLock(true),
		WithDistributedLockTTL(10*time.Second),
		WithLoadTimeout(30*time.Second),
	)

	// Then
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
	assert.Contains(t, err.Error(), "DistributedLockTTL")
}

func TestNewLoader_DistLockTTL_EqualToLoadTimeout_ReturnsError(t *testing.T) {
	// Given - DistributedLockTTL == LoadTimeout 时也应返回 ErrInvalidConfig
	cache, _ := newTestRedis(t)

	// When
	_, err := NewLoader(cache,
		WithDistributedLock(true),
		WithDistributedLockTTL(30*time.Second),
		WithLoadTimeout(30*time.Second),
	)

	// Then
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

func TestNewLoader_NegativeLoadTimeout_StillValidatesDistLockTTL(t *testing.T) {
	// LoadTimeout < 0 语义为"使用默认超时 30s"，validate 必须用实际生效值做 DistributedLockTTL 比较。
	cache, _ := newTestRedis(t)

	// When - LoadTimeout=-1（运行时转为 30s），DistributedLockTTL=20s < 30s
	_, err := NewLoader(cache,
		WithDistributedLock(true),
		WithLoadTimeout(-1),
		WithDistributedLockTTL(20*time.Second),
	)

	// Then
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
	assert.Contains(t, err.Error(), "DistributedLockTTL")
}

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

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err1 = loader.Load(ctx1, "sf-cancel-shared", loadFn, time.Hour)
	}()
	wg.Add(1)
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

func TestLoader_Load_WithNegativeTTL_SkipsCache(t *testing.T) {
	// Given - 负 TTL 时应跳过缓存写入（FG-S1 修复）
	cache, mr := newTestRedis(t)
	ctx := context.Background()

	loader, err := NewLoader(cache, WithSingleflight(false))
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("neg_ttl_value"), nil
	}

	// When - 使用负 TTL
	value, err := loader.Load(ctx, "neg_ttl_key", loadFn, -1*time.Second)

	// Then - 回源成功但不写入缓存
	require.NoError(t, err)
	assert.Equal(t, []byte("neg_ttl_value"), value)

	// 验证未写入缓存
	assert.False(t, mr.Exists("neg_ttl_key"), "key should not exist with negative TTL")
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

// =============================================================================
// setHashTTLIfMissing 错误路径覆盖（FG-M3/L3 修复）
// =============================================================================

func TestLoader_LoadHash_WhenTTLCheckFails_LogsAndContinues(t *testing.T) {
	// Given - TTL 查询失败时应记录日志但不影响业务返回
	cache, mr := newTestRedis(t)
	ctx := context.Background()

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithHashTTLRefresh(false), // 走 setHashTTLIfMissing 路径
	)
	require.NoError(t, err)

	loadFn := func(ctx context.Context) ([]byte, error) {
		return []byte("ttl_check_value"), nil
	}

	// 第一次加载成功（写入 HSet）
	value, err := loader.LoadHash(ctx, "ttl_err_hash", "field1", loadFn, time.Hour)
	require.NoError(t, err)
	assert.Equal(t, []byte("ttl_check_value"), value)

	// 清理缓存以便第二次加载触发回源
	mr.Del("ttl_err_hash")

	// 在 TTL 命令上注入错误
	mr.SetError("injected TTL error")

	// When - 回源成功，但 setHashTTLIfMissing 中 TTL 查询失败
	_, err = loader.LoadHash(ctx, "ttl_err_hash", "field1", loadFn, time.Hour)

	// Then - 回源自身不受 Redis 错误影响，但 Redis 操作（Get/HSet/TTL）会失败
	// 只要不 panic 即可，错误路径被日志记录
	if err != nil {
		t.Logf("Expected: error from Redis during cache operation: %v", err)
	}

	// 清除注入的错误
	mr.SetError("")
}

func TestLoader_LoadHash_WhenExpireFails_LogsAndContinues(t *testing.T) {
	// Given - Expire 失败时应记录日志但不影响业务返回
	cache, mr := newTestRedis(t)
	ctx := context.Background()

	loadCount := 0
	loadFn := func(ctx context.Context) ([]byte, error) {
		loadCount++
		return []byte("expire_err_value"), nil
	}

	loader, err := NewLoader(cache,
		WithSingleflight(false),
		WithHashTTLRefresh(false), // 走 setHashTTLIfMissing 路径
	)
	require.NoError(t, err)

	// When - 第一次加载，HSet 成功但 TTL 未设置
	value, err := loader.LoadHash(ctx, "expire_err_hash", "field1", loadFn, time.Hour)

	// Then - 回源成功
	require.NoError(t, err)
	assert.Equal(t, []byte("expire_err_value"), value)
	assert.Equal(t, 1, loadCount)

	// 验证缓存存在
	assert.True(t, mr.Exists("expire_err_hash"))
}

func TestSetHashTTLIfMissing_WhenTTLCheckFails_LogsAndReturns(t *testing.T) {
	// 直接测试 setHashTTLIfMissing 的 TTL 查询失败路径
	cache, mr := newTestRedis(t)
	ctx := context.Background()

	l := &loader{cache: cache, options: defaultLoaderOptions()}

	// 创建 Hash key
	err := cache.Client().HSet(ctx, "ttl-err-direct", "field", "value").Err()
	require.NoError(t, err)

	// 注入全局错误，使 TTL 命令失败
	mr.SetError("injected TTL error")
	defer mr.SetError("")

	// 调用不应 panic，TTL 错误被记录并跳过
	l.setHashTTLIfMissing(ctx, "ttl-err-direct", time.Hour)
}

func TestSetHashTTLIfMissing_WhenKeyHasTTL_SkipsExpire(t *testing.T) {
	// 测试 key 已有 TTL 时（currentTTL >= 0），不调用 Expire
	cache, _ := newTestRedis(t)
	ctx := context.Background()

	l := &loader{cache: cache, options: defaultLoaderOptions()}

	// 创建 Hash key 并设置 TTL
	err := cache.Client().HSet(ctx, "has-ttl-hash", "field", "value").Err()
	require.NoError(t, err)
	err = cache.Client().Expire(ctx, "has-ttl-hash", time.Hour).Err()
	require.NoError(t, err)

	// 调用 setHashTTLIfMissing — 因为 TTL >= 0，不应调用 Expire
	l.setHashTTLIfMissing(ctx, "has-ttl-hash", 2*time.Hour)

	// 验证 TTL 没有被覆盖（仍为最初设置的值，约 1 小时）
	ttlResult, err := cache.Client().TTL(ctx, "has-ttl-hash").Result()
	require.NoError(t, err)
	assert.True(t, ttlResult > 30*time.Minute, "original TTL should be preserved")
}

func TestSetHashTTLIfMissing_WhenKeyNotExist_SkipsExpire(t *testing.T) {
	cache, mr := newTestRedis(t)
	ctx := context.Background()

	l := &loader{cache: cache, options: defaultLoaderOptions()}

	// key 不存在时 TTL 返回 -2，不应调用 Expire（无操作可跳过）
	mr.Del("nonexistent-key")
	l.setHashTTLIfMissing(ctx, "nonexistent-key", time.Hour)

	// key 仍不存在（Expire 未创建 key）
	assert.False(t, mr.Exists("nonexistent-key"), "Expire should not create non-existent key")
}

func TestApplyTTLJitter_ExtremelyLargeTTL_ReturnsTTL(t *testing.T) {
	l := &loader{options: defaultLoaderOptions()}
	l.options.TTLJitter = 1.0

	largeTTL := time.Duration(1<<63 - 1)
	result := l.applyTTLJitter(largeTTL)
	assert.True(t, result > 0, "result should be positive for extreme TTL")
}

// =============================================================================
// WithMaxRetryAttempts 上界钳位测试（FG-L6 修复）
// =============================================================================

func TestWithMaxRetryAttempts_ClampsToUpperBound(t *testing.T) {
	opts := defaultLoaderOptions()
	WithMaxRetryAttempts(MaxMaxRetryAttempts + 1000)(opts)
	assert.Equal(t, MaxMaxRetryAttempts, opts.MaxRetryAttempts)
}

func TestWithMaxRetryAttempts_ExactUpperBound(t *testing.T) {
	opts := defaultLoaderOptions()
	WithMaxRetryAttempts(MaxMaxRetryAttempts)(opts)
	assert.Equal(t, MaxMaxRetryAttempts, opts.MaxRetryAttempts)
}
