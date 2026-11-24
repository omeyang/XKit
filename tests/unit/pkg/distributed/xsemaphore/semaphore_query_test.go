package xsemaphore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// 过期测试
// =============================================================================

func TestPermitExpiry(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	// 获取短 TTL 许可
	permit, err := sem.TryAcquire(ctx, "expiry",
		WithCapacity(1),
		WithTTL(50*time.Millisecond),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	// 容量已满
	permit2, err := sem.TryAcquire(ctx, "expiry",
		WithCapacity(1),
	)
	assert.NoError(t, err)
	assert.Nil(t, permit2)

	// 等待许可过期（使用真实时间，因为 Lua 脚本使用我们传入的时间戳）
	time.Sleep(100 * time.Millisecond)

	// 现在应该可以获取新许可
	permit3, err := sem.TryAcquire(ctx, "expiry",
		WithCapacity(1),
	)
	require.NoError(t, err)
	assert.NotNil(t, permit3, "should acquire after expiry")

	releasePermit(t, ctx, permit3)
}

// =============================================================================
// Close 和 Health 测试
// =============================================================================

func TestClose(t *testing.T) {
	_, client := setupRedis(t)
	sem, err := New(client)
	require.NoError(t, err)

	err = sem.Close(context.Background())
	assert.NoError(t, err)

	// 关闭后操作应返回错误
	ctx := context.Background()
	_, err = sem.TryAcquire(ctx, "after-close",
		WithCapacity(10),
	)
	assert.ErrorIs(t, err, ErrSemaphoreClosed)
}

func TestHealth(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	err := sem.Health(ctx)
	assert.NoError(t, err)

	closeSemaphore(t, sem)

	err = sem.Health(ctx)
	assert.ErrorIs(t, err, ErrSemaphoreClosed)
}

// =============================================================================
// 本地信号量测试
// =============================================================================

func TestLocalSemaphore_Basic(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 2
	sem := newLocalSemaphore(opts)
	ctx := context.Background()

	// 本地容量 = 10 / 2 = 5
	permits := make([]Permit, 0, 6)
	for i := range 5 {
		permit, err := sem.TryAcquire(ctx, "local-test",
			WithCapacity(10),
		)
		require.NoError(t, err)
		require.NotNil(t, permit, "should acquire permit %d", i+1)
		permits = append(permits, permit)
	}

	// 容量满
	permit, err := sem.TryAcquire(ctx, "local-test",
		WithCapacity(10),
	)
	assert.NoError(t, err)
	assert.Nil(t, permit)

	// 清理
	for _, p := range permits {
		releasePermit(t, ctx, p)
	}
	closeSemaphore(t, sem)
}

// =============================================================================
// 降级测试
// =============================================================================

func TestFallbackSemaphore_Local(t *testing.T) {
	_, client := setupRedis(t)

	sem, err := New(client,
		WithFallback(FallbackLocal),
		WithPodCount(2),
	)
	require.NoError(t, err)

	ctx := context.Background()
	permit, err := sem.TryAcquire(ctx, "fallback-test",
		WithCapacity(10),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	releasePermit(t, ctx, permit)
	closeSemaphore(t, sem)
}

// =============================================================================
// 错误检查函数测试
// =============================================================================

func TestErrorChecks(t *testing.T) {
	assert.True(t, IsCapacityFull(ErrCapacityFull))
	assert.False(t, IsCapacityFull(ErrPermitNotHeld))

	assert.True(t, IsTenantQuotaExceeded(ErrTenantQuotaExceeded))
	assert.False(t, IsTenantQuotaExceeded(ErrCapacityFull))

	assert.True(t, IsPermitNotHeld(ErrPermitNotHeld))
	assert.False(t, IsPermitNotHeld(ErrCapacityFull))
}

func TestAcquireFailReason(t *testing.T) {
	assert.Equal(t, "capacity_full", ReasonCapacityFull.String())
	assert.Equal(t, "tenant_quota_exceeded", ReasonTenantQuotaExceeded.String())
	assert.Equal(t, "unknown", ReasonUnknown.String())

	assert.Equal(t, ErrCapacityFull, ReasonCapacityFull.Error())
	assert.Equal(t, ErrTenantQuotaExceeded, ReasonTenantQuotaExceeded.Error())
	assert.Nil(t, ReasonUnknown.Error())
}

// =============================================================================
// Options 测试
// =============================================================================

func TestFallbackStrategy_IsValid(t *testing.T) {
	assert.True(t, FallbackLocal.IsValid())
	assert.True(t, FallbackOpen.IsValid())
	assert.True(t, FallbackClose.IsValid())
	assert.True(t, FallbackStrategy("").IsValid())
	assert.False(t, FallbackStrategy("invalid").IsValid())
}

func TestAcquireOptions_Defaults(t *testing.T) {
	opts := defaultAcquireOptions()

	assert.Equal(t, 1, opts.capacity)
	assert.Equal(t, 5*time.Minute, opts.ttl)
	assert.Equal(t, 10, opts.maxRetries)
	assert.Equal(t, 100*time.Millisecond, opts.retryDelay)
}

func TestAcquireOptions_WithFunctions(t *testing.T) {
	opts := defaultAcquireOptions()

	WithCapacity(100)(opts)
	assert.Equal(t, 100, opts.capacity)

	WithTenantID("tenant-1")(opts)
	assert.Equal(t, "tenant-1", opts.tenantID)

	WithTenantQuota(5)(opts)
	assert.Equal(t, 5, opts.tenantQuota)

	WithTTL(10 * time.Minute)(opts)
	assert.Equal(t, 10*time.Minute, opts.ttl)

	WithMaxRetries(20)(opts)
	assert.Equal(t, 20, opts.maxRetries)

	WithRetryDelay(500 * time.Millisecond)(opts)
	assert.Equal(t, 500*time.Millisecond, opts.retryDelay)
}

func TestAcquireOptions_InvalidValues(t *testing.T) {
	// Fail-fast 设计：setter 直接设置值，validate() 捕获非法值
	opts := defaultAcquireOptions()

	WithCapacity(0)(opts)
	assert.Equal(t, 0, opts.capacity)
	assert.ErrorIs(t, opts.validate(), ErrInvalidCapacity)

	// 恢复有效 capacity 以测试其他字段
	opts = defaultAcquireOptions()
	WithTTL(0)(opts)
	assert.Equal(t, time.Duration(0), opts.ttl)
	assert.ErrorIs(t, opts.validate(), ErrInvalidTTL)

	opts = defaultAcquireOptions()
	WithMaxRetries(0)(opts)
	assert.Equal(t, 0, opts.maxRetries)
	// maxRetries 校验已移至 validateRetryParams（仅 Acquire 调用）
	assert.NoError(t, opts.validate())
	assert.ErrorIs(t, opts.validateRetryParams(), ErrInvalidMaxRetries)
}

// =============================================================================
// 空资源名校验测试（修复 #8）
// =============================================================================

func TestTryAcquire_EmptyResource(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	// 空资源名应返回错误
	permit, err := sem.TryAcquire(ctx, "", WithCapacity(10))
	assert.ErrorIs(t, err, ErrInvalidResource)
	assert.Nil(t, permit)
}

func TestAcquire_EmptyResource(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	// 空资源名应返回错误
	permit, err := sem.Acquire(ctx, "", WithCapacity(10), WithMaxRetries(1))
	assert.ErrorIs(t, err, ErrInvalidResource)
	assert.Nil(t, permit)
}

func TestLocalSemaphore_EmptyResource(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)

	ctx := context.Background()

	// TryAcquire 空资源名
	permit, err := sem.TryAcquire(ctx, "", WithCapacity(10))
	assert.ErrorIs(t, err, ErrInvalidResource)
	assert.Nil(t, permit)

	// Acquire 空资源名
	permit, err = sem.Acquire(ctx, "", WithCapacity(10), WithMaxRetries(1))
	assert.ErrorIs(t, err, ErrInvalidResource)
	assert.Nil(t, permit)
}

// =============================================================================
// 自动续租单次启动测试（修复 #2）
// =============================================================================

func TestAutoExtend_SingleStart(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "auto-extend-single",
		WithCapacity(10),
		WithTTL(time.Minute),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	// 多次调用 StartAutoExtend 应该返回相同的 stop 函数（单次启动策略）
	stop1 := permit.StartAutoExtend(100 * time.Millisecond)
	stop2 := permit.StartAutoExtend(100 * time.Millisecond) // 再次调用

	// 两次调用都不应 panic，且都返回有效的 stop 函数
	assert.NotNil(t, stop1)
	assert.NotNil(t, stop2)

	// 调用 stop 应该安全
	stop1()
	stop2() // 重复调用也应该安全

	releasePermit(t, ctx, permit)
}

// =============================================================================
// 无租户键时的操作测试（修复 #1 - Redis Cluster 兼容）
// =============================================================================

func TestTryAcquire_WithoutTenant(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	// 不设置租户，只使用全局容量
	permit, err := sem.TryAcquire(ctx, "no-tenant-resource",
		WithCapacity(10),
		WithTTL(time.Minute),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	// 验证租户 ID 为空
	assert.Empty(t, permit.TenantID())

	// 续期应该成功
	err = permit.Extend(ctx)
	assert.NoError(t, err)

	// 释放应该成功
	err = permit.Release(ctx)
	assert.NoError(t, err)
}

func TestQuery_WithoutTenant(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	// 先获取一些许可（不设置租户）
	permits := make([]Permit, 0, 3)
	for range 3 {
		permit, err := sem.TryAcquire(ctx, "query-no-tenant",
			WithCapacity(10),
		)
		require.NoError(t, err)
		require.NotNil(t, permit)
		permits = append(permits, permit)
	}

	// 查询不带租户
	info, err := sem.Query(ctx, "query-no-tenant",
		QueryWithCapacity(10),
	)
	require.NoError(t, err)

	assert.Equal(t, "query-no-tenant", info.Resource)
	assert.Equal(t, 10, info.GlobalCapacity)
	assert.Equal(t, 3, info.GlobalUsed)
	assert.Equal(t, 7, info.GlobalAvailable)
	assert.Empty(t, info.TenantID)
	assert.Equal(t, 0, info.TenantUsed)

	// 清理
	for _, p := range permits {
		releasePermit(t, ctx, p)
	}
}

// =============================================================================
// nil option 防护测试
// =============================================================================

func TestNilOption_NoPanic(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	// nil option 应该被静默忽略
	permit, err := sem.TryAcquire(ctx, "nil-opt-test",
		WithCapacity(10),
		nil, // 显式传入 nil
		WithTTL(time.Minute),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)
	releasePermit(t, ctx, permit)
}

func TestNilOption_Query(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	// nil QueryOption 应该被静默忽略
	info, err := sem.Query(ctx, "nil-query-opt-test",
		QueryWithCapacity(10),
		nil, // 显式传入 nil
		QueryWithTenantQuota(5),
	)
	require.NoError(t, err)
	assert.Equal(t, "nil-query-opt-test", info.Resource)
}

func TestNilOption_New(t *testing.T) {
	_, client := setupRedis(t)

	// nil Option 应该被静默忽略
	sem, err := New(client,
		WithKeyPrefix("test:"),
		nil, // 显式传入 nil
		WithPodCount(3),
	)
	require.NoError(t, err)
	assert.NotNil(t, sem)
	closeSemaphore(t, sem)
}

// =============================================================================
// Query 租户统计一致性测试
// =============================================================================

func TestQuery_TenantUsedConsistency(t *testing.T) {
	sem, _ := setupSemaphore(t)
	ctx := context.Background()

	// 获取一些许可（设置 tenantID 但不设置 tenantQuota）
	permit, err := sem.TryAcquire(ctx, "consistency-test",
		WithCapacity(10),
		WithTenantID("tenant-X"),
		// 不设置 WithTenantQuota
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	// Query 不带 tenantQuota，tenantUsed 应该为 0（与 Redis 保持一致）
	info, err := sem.Query(ctx, "consistency-test",
		QueryWithCapacity(10),
		QueryWithTenantID("tenant-X"),
		// 不设置 QueryWithTenantQuota
	)
	require.NoError(t, err)
	assert.Equal(t, 0, info.TenantUsed, "tenantUsed should be 0 when tenantQuota not set")

	releasePermit(t, ctx, permit)
}

func TestQuery_TenantUsedConsistency_Local(t *testing.T) {
	opts := defaultOptions()
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)
	ctx := context.Background()

	// 获取一些许可（设置 tenantID 但不设置 tenantQuota）
	permit, err := sem.TryAcquire(ctx, "consistency-test-local",
		WithCapacity(10),
		WithTenantID("tenant-Y"),
		// 不设置 WithTenantQuota
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	// Query 不带 tenantQuota，tenantUsed 应该为 0（与 Redis 保持一致）
	info, err := sem.Query(ctx, "consistency-test-local",
		QueryWithCapacity(10),
		QueryWithTenantID("tenant-Y"),
		// 不设置 QueryWithTenantQuota
	)
	require.NoError(t, err)
	assert.Equal(t, 0, info.TenantUsed, "local tenantUsed should be 0 when tenantQuota not set")

	releasePermit(t, ctx, permit)
}
