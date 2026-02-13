package xsemaphore

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// =============================================================================
// 模糊测试辅助函数
// =============================================================================

// releasePermitF 模糊测试辅助函数：释放许可（静默忽略错误）
func releasePermitF(_ *testing.T, ctx context.Context, p Permit) {
	if p != nil {
		// fuzz 测试中忽略释放错误，因为边界条件可能导致各种错误状态
		_ = p.Release(ctx) //nolint:errcheck
	}
}

// =============================================================================
// 模糊测试
// =============================================================================

// FuzzTryAcquire 测试 TryAcquire 对各种输入的鲁棒性
func FuzzTryAcquire(f *testing.F) {
	// 添加种子语料
	f.Add("test-resource", 10, 5, "tenant-1", int64(60000))
	f.Add("", 1, 0, "", int64(1000))
	f.Add("resource/with/slashes", 100, 50, "tenant", int64(300000))
	f.Add("资源名称", 1, 1, "租户", int64(5000))
	f.Add("resource:with:colons", 1000, 100, "tenant:id", int64(10000))
	f.Add("resource.with.dots", 5, 2, "tenant.id", int64(30000))

	f.Fuzz(func(t *testing.T, resource string, capacity int, tenantQuota int, tenantID string, ttlMs int64) {
		// 跳过无效输入
		if capacity <= 0 || capacity > 10000 {
			return
		}
		if tenantQuota < 0 || tenantQuota > capacity {
			return
		}
		if ttlMs <= 0 || ttlMs > 600000 {
			return
		}

		mr, err := miniredis.Run()
		if err != nil {
			t.Skip("failed to start miniredis")
		}
		defer mr.Close()

		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer client.Close()

		sem, err := New(client)
		if err != nil {
			t.Skip("failed to create semaphore")
		}
		defer sem.Close(context.Background())

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		opts := []AcquireOption{
			WithCapacity(capacity),
			WithTTL(time.Duration(ttlMs) * time.Millisecond),
		}

		if tenantID != "" && tenantQuota > 0 {
			opts = append(opts, WithTenantID(tenantID), WithTenantQuota(tenantQuota))
		}

		permit, err := sem.TryAcquire(ctx, resource, opts...)
		// 不应 panic
		if err != nil {
			return
		}
		if permit != nil {
			releasePermitF(t, ctx, permit)
		}
	})
}

// FuzzLocalSemaphore 测试本地信号量对各种输入的鲁棒性
func FuzzLocalSemaphore(f *testing.F) {
	f.Add("test-resource", 10, 5, "tenant-1", 1)
	f.Add("", 1, 0, "", 10)
	f.Add("resource/with/slashes", 100, 50, "tenant", 5)
	f.Add("资源名称", 1, 1, "租户", 3)

	f.Fuzz(func(t *testing.T, resource string, capacity int, tenantQuota int, tenantID string, podCount int) {
		if capacity <= 0 || capacity > 10000 {
			return
		}
		if tenantQuota < 0 || tenantQuota > capacity {
			return
		}
		if podCount <= 0 || podCount > 100 {
			return
		}

		opts := defaultOptions()
		opts.podCount = podCount
		sem := newLocalSemaphore(opts)
		defer sem.Close(context.Background())

		ctx := context.Background()

		acquireOpts := []AcquireOption{
			WithCapacity(capacity),
			WithTTL(time.Minute),
		}

		if tenantID != "" && tenantQuota > 0 {
			acquireOpts = append(acquireOpts, WithTenantID(tenantID), WithTenantQuota(tenantQuota))
		}

		permit, err := sem.TryAcquire(ctx, resource, acquireOpts...)
		// 不应 panic
		if err != nil {
			return
		}
		if permit != nil {
			releasePermitF(t, ctx, permit)
		}
	})
}

// FuzzResourceName 测试资源名称的各种边界情况
func FuzzResourceName(f *testing.F) {
	// 添加各种边界情况
	f.Add("")
	f.Add("a")
	f.Add("normal-resource")
	f.Add("resource/with/path")
	f.Add("resource:with:colons")
	f.Add("resource.with.dots")
	f.Add("resource_with_underscores")
	f.Add("UPPERCASE")
	f.Add("MixedCase")
	f.Add("123numeric")
	f.Add("中文资源")
	f.Add("emoji🚀resource")
	f.Add("resource\nwith\nnewlines")
	f.Add("resource\twith\ttabs")
	f.Add("resource with spaces")
	f.Add(string(make([]byte, 1000))) // 长字符串

	f.Fuzz(func(t *testing.T, resource string) {
		mr, err := miniredis.Run()
		if err != nil {
			t.Skip("failed to start miniredis")
		}
		defer mr.Close()

		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer client.Close()

		sem, err := New(client)
		if err != nil {
			t.Skip("failed to create semaphore")
		}
		defer sem.Close(context.Background())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// 测试 TryAcquire
		permit, _ := sem.TryAcquire(ctx, resource, WithCapacity(10)) //nolint:errcheck // fuzz test
		// 不应 panic
		if permit != nil {
			releasePermitF(t, ctx, permit)
		}

		// 测试 Query
		//nolint:errcheck // fuzz test intentionally ignores Query errors
		_, _ = sem.Query(ctx, resource, QueryWithCapacity(10))
	})
}

// FuzzTenantID 测试租户 ID 的各种边界情况
func FuzzTenantID(f *testing.F) {
	f.Add("")
	f.Add("tenant-1")
	f.Add("tenant/with/path")
	f.Add("tenant:with:colons")
	f.Add("tenant.with.dots")
	f.Add("中文租户")
	f.Add("emoji🏢tenant")
	f.Add(string(make([]byte, 500)))

	f.Fuzz(func(t *testing.T, tenantID string) {
		mr, err := miniredis.Run()
		if err != nil {
			t.Skip("failed to start miniredis")
		}
		defer mr.Close()

		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer client.Close()

		sem, err := New(client)
		if err != nil {
			t.Skip("failed to create semaphore")
		}
		defer sem.Close(context.Background())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		permit, _ := sem.TryAcquire(ctx, "test-resource", //nolint:errcheck // fuzz test
			WithCapacity(100),
			WithTenantID(tenantID),
			WithTenantQuota(10),
		)
		// 不应 panic
		if permit != nil {
			releasePermitF(t, ctx, permit)
		}
	})
}

// FuzzOptions 测试选项的各种边界情况
func FuzzOptions(f *testing.F) {
	f.Add(1, 1, int64(1000), 1, int64(100))
	f.Add(100, 50, int64(300000), 10, int64(1000))
	f.Add(0, 0, int64(0), 0, int64(0))
	f.Add(-1, -1, int64(-1), -1, int64(-1))
	f.Add(1000000, 500000, int64(3600000), 100, int64(10000))

	f.Fuzz(func(t *testing.T, capacity, tenantQuota int, ttlMs int64, maxRetries int, retryDelayMs int64) {
		opts := defaultAcquireOptions()

		// 应用选项（不应 panic）
		WithCapacity(capacity)(opts)
		WithTenantQuota(tenantQuota)(opts)
		WithTTL(time.Duration(ttlMs) * time.Millisecond)(opts)
		WithMaxRetries(maxRetries)(opts)
		WithRetryDelay(time.Duration(retryDelayMs) * time.Millisecond)(opts)

		// 验证
		_ = opts.validate() //nolint:errcheck // fuzz test intentionally ignores validation errors
	})
}

// FuzzKeyPrefix 测试键前缀的各种情况
func FuzzKeyPrefix(f *testing.F) {
	f.Add("")
	f.Add("prefix:")
	f.Add("my:app:")
	f.Add("prefix/with/slashes:")
	f.Add("中文前缀:")

	f.Fuzz(func(t *testing.T, prefix string) {
		mr, err := miniredis.Run()
		if err != nil {
			t.Skip("failed to start miniredis")
		}
		defer mr.Close()

		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer client.Close()

		sem, err := New(client, WithKeyPrefix(prefix))
		if err != nil {
			t.Skip("failed to create semaphore")
		}
		defer sem.Close(context.Background())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		permit, _ := sem.TryAcquire(ctx, "test", WithCapacity(10))
		if permit != nil {
			releasePermitF(t, ctx, permit)
		}
	})
}
