package xsemaphore

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// buildOpenQueryInfo 测试
// =============================================================================

func TestBuildOpenQueryInfo(t *testing.T) {
	opts := defaultOptions()
	f := &fallbackSemaphore{
		strategy: FallbackOpen,
		opts:     opts,
	}

	info := f.buildOpenQueryInfo(context.Background(), "test-resource", []QueryOption{
		QueryWithCapacity(100),
		QueryWithTenantQuota(10),
	})

	if info.Resource != "test-resource" {
		t.Errorf("expected resource 'test-resource', got %s", info.Resource)
	}
	if info.GlobalCapacity != 100 {
		t.Errorf("expected GlobalCapacity 100, got %d", info.GlobalCapacity)
	}
	if info.GlobalUsed != 0 {
		t.Errorf("expected GlobalUsed 0, got %d", info.GlobalUsed)
	}
	if info.GlobalAvailable != 100 {
		t.Errorf("expected GlobalAvailable 100, got %d", info.GlobalAvailable)
	}
	if info.TenantQuota != 10 {
		t.Errorf("expected TenantQuota 10, got %d", info.TenantQuota)
	}
}

// =============================================================================
// acquireWithRetry 更多分支测试
// =============================================================================

func TestAcquireWithRetry_Branches(t *testing.T) {
	t.Run("context already canceled", func(t *testing.T) {
		sem := &redisSemaphore{opts: defaultOptions()}
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		cfg := &acquireOptions{
			maxRetries: 3,
			retryDelay: 10 * time.Millisecond,
		}

		permit, reason, retryCount, err := sem.acquireWithRetry(ctx, "resource", "tenant", cfg)

		if permit != nil {
			t.Error("expected nil permit")
		}
		if reason != ReasonUnknown {
			t.Errorf("expected ReasonUnknown, got %v", reason)
		}
		if retryCount != 0 {
			t.Errorf("expected retryCount 0, got %d", retryCount)
		}
		if err == nil {
			t.Error("expected error due to context cancellation")
		}
	})
}

// =============================================================================
// waitForRetry 测试
// =============================================================================

func TestWaitForRetry(t *testing.T) {
	t.Run("waits for delay", func(t *testing.T) {
		ctx := context.Background()
		start := time.Now()
		err := waitForRetry(ctx, 50*time.Millisecond)
		duration := time.Since(start)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if duration < 40*time.Millisecond {
			t.Error("expected to wait at least 40ms")
		}
	})

	t.Run("returns early on context cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		err := waitForRetry(ctx, 1*time.Second)
		duration := time.Since(start)

		if err == nil {
			t.Error("expected error due to context cancellation")
		}
		if duration > 500*time.Millisecond {
			t.Error("should return early on cancel")
		}
	})
}

// =============================================================================
// prepareAcquireCommon 和 prepareQueryCommon 测试
// =============================================================================

func TestPrepareAcquireCommon_Closed(t *testing.T) {
	_, _, err := prepareAcquireCommon(context.Background(), "resource", nil, true)
	if err != ErrSemaphoreClosed {
		t.Errorf("expected ErrSemaphoreClosed, got %v", err)
	}
}

func TestPrepareQueryCommon_Closed(t *testing.T) {
	_, _, err := prepareQueryCommon(context.Background(), "resource", nil, true)
	if err != ErrSemaphoreClosed {
		t.Errorf("expected ErrSemaphoreClosed, got %v", err)
	}
}

// =============================================================================
// resolveTenantID 测试
// =============================================================================

func TestResolveTenantID(t *testing.T) {
	t.Run("explicit ID takes precedence", func(t *testing.T) {
		result := resolveTenantID(context.Background(), "explicit")
		if result != "explicit" {
			t.Errorf("expected 'explicit', got %s", result)
		}
	})

	t.Run("empty explicit ID falls back to context", func(t *testing.T) {
		result := resolveTenantID(context.Background(), "")
		// Without xtenant set in context, should return empty
		if result != "" {
			t.Errorf("expected empty string, got %s", result)
		}
	})
}

// =============================================================================
// Option 应用测试
// =============================================================================

func TestApplyAcquireOptions(t *testing.T) {
	cfg := applyAcquireOptions([]AcquireOption{
		WithCapacity(100),
		WithTTL(10 * time.Minute),
		WithTenantID("tenant1"),
		WithTenantQuota(50),
		nil, // nil option should be ignored
	})

	if cfg.capacity != 100 {
		t.Errorf("expected capacity 100, got %d", cfg.capacity)
	}
	if cfg.ttl != 10*time.Minute {
		t.Errorf("expected ttl 10m, got %v", cfg.ttl)
	}
	if cfg.tenantID != "tenant1" {
		t.Errorf("expected tenantID 'tenant1', got %s", cfg.tenantID)
	}
	if cfg.tenantQuota != 50 {
		t.Errorf("expected tenantQuota 50, got %d", cfg.tenantQuota)
	}
}

func TestApplyQueryOptions(t *testing.T) {
	cfg := applyQueryOptions([]QueryOption{
		QueryWithCapacity(100),
		QueryWithTenantID("tenant1"),
		QueryWithTenantQuota(50),
		nil, // nil option should be ignored
	})

	if cfg.capacity != 100 {
		t.Errorf("expected capacity 100, got %d", cfg.capacity)
	}
	if cfg.tenantID != "tenant1" {
		t.Errorf("expected tenantID 'tenant1', got %s", cfg.tenantID)
	}
	if cfg.tenantQuota != 50 {
		t.Errorf("expected tenantQuota 50, got %d", cfg.tenantQuota)
	}
}

// =============================================================================
// Option 边界测试
// =============================================================================

func TestOptionEdgeCases(t *testing.T) {
	// Fail-fast 设计：setter 直接接受值，validate() 捕获非法值

	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{"WithCapacity sets zero and validate catches it", func(t *testing.T) {
			cfg := defaultAcquireOptions()
			WithCapacity(0)(cfg)
			assert.Equal(t, 0, cfg.capacity, "zero capacity should be set")
			assert.Error(t, cfg.validate(), "validate should catch zero capacity")
		}},
		{"WithCapacity sets negative and validate catches it", func(t *testing.T) {
			cfg := defaultAcquireOptions()
			WithCapacity(-1)(cfg)
			assert.Equal(t, -1, cfg.capacity, "negative capacity should be set")
			assert.Error(t, cfg.validate(), "validate should catch negative capacity")
		}},
		{"WithTTL sets zero and validate catches it", func(t *testing.T) {
			cfg := defaultAcquireOptions()
			WithTTL(0)(cfg)
			assert.Equal(t, time.Duration(0), cfg.ttl, "zero TTL should be set")
			assert.Error(t, cfg.validate(), "validate should catch zero TTL")
		}},
		{"WithTenantQuota allows zero", func(t *testing.T) {
			cfg := defaultAcquireOptions()
			cfg.tenantQuota = 10
			WithTenantQuota(0)(cfg)
			assert.Equal(t, 0, cfg.tenantQuota, "zero quota should be accepted (means no tenant quota)")
		}},
		{"WithTenantQuota sets negative and validate catches it", func(t *testing.T) {
			cfg := defaultAcquireOptions()
			WithTenantQuota(-1)(cfg)
			assert.Equal(t, -1, cfg.tenantQuota, "negative quota should be set")
			assert.Error(t, cfg.validate(), "validate should catch negative quota")
		}},
		{"WithMaxRetries sets zero and validateRetryParams catches it", func(t *testing.T) {
			cfg := defaultAcquireOptions()
			WithMaxRetries(0)(cfg)
			assert.Equal(t, 0, cfg.maxRetries, "zero maxRetries should be set")
			assert.NoError(t, cfg.validate(), "validate should not check maxRetries")
			assert.Error(t, cfg.validateRetryParams(), "validateRetryParams should catch zero maxRetries")
		}},
		{"WithRetryDelay sets zero and validateRetryParams catches it", func(t *testing.T) {
			cfg := defaultAcquireOptions()
			WithRetryDelay(0)(cfg)
			assert.Equal(t, time.Duration(0), cfg.retryDelay, "zero retryDelay should be set")
			assert.NoError(t, cfg.validate(), "validate should not check retryDelay")
			assert.Error(t, cfg.validateRetryParams(), "validateRetryParams should catch zero retryDelay")
		}},
		{"WithMetadata ignores empty", func(t *testing.T) {
			cfg := defaultAcquireOptions()
			WithMetadata(map[string]string{})(cfg)
			assert.Nil(t, cfg.metadata, "empty metadata should not be set")
		}},
		{"WithKeyPrefix sets invalid and validate catches it", func(t *testing.T) {
			opts := defaultOptions()
			WithKeyPrefix("{invalid}")(opts)
			assert.Equal(t, "{invalid}", opts.keyPrefix, "invalid prefix should be set by setter")
			assert.Error(t, opts.validate(), "validate should catch invalid prefix")
		}},
		{"WithKeyPrefix ignores empty", func(t *testing.T) {
			opts := defaultOptions()
			original := opts.keyPrefix
			WithKeyPrefix("")(opts)
			assert.Equal(t, original, opts.keyPrefix, "empty prefix should be ignored")
		}},
		{"WithFallback sets invalid and validate catches it", func(t *testing.T) {
			opts := defaultOptions()
			WithFallback(FallbackStrategy("invalid"))(opts)
			assert.Equal(t, FallbackStrategy("invalid"), opts.fallback, "invalid fallback should be set by setter")
			assert.Error(t, opts.validate(), "validate should catch invalid fallback")
		}},
		{"WithPodCount sets zero and validate catches it", func(t *testing.T) {
			opts := defaultOptions()
			WithPodCount(0)(opts)
			assert.Equal(t, 0, opts.podCount, "zero podCount should be set by setter")
			assert.Error(t, opts.validate(), "validate should catch zero podCount")
		}},
		{"QueryWithCapacity rejects zero", func(t *testing.T) {
			cfg := defaultQueryOptions()
			QueryWithCapacity(0)(cfg)
			assert.Equal(t, 0, cfg.capacity, "zero should set capacity to 0")
			assert.Error(t, cfg.validate(), "validate should reject zero capacity for query")
		}},
		{"QueryWithTenantQuota allows zero", func(t *testing.T) {
			cfg := defaultQueryOptions()
			cfg.tenantQuota = 10
			QueryWithTenantQuota(0)(cfg)
			assert.Equal(t, 0, cfg.tenantQuota, "zero should set tenantQuota to 0")
			assert.NoError(t, cfg.validate(), "validate should accept zero tenantQuota for query")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}

// =============================================================================
// localSemaphore extendPermit 更多测试
// =============================================================================

func TestLocalSemaphore_ExtendPermit_ExpiredPermit(t *testing.T) {
	opts := defaultOptions()
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)

	// First acquire a permit
	permit, err := sem.TryAcquire(context.Background(), "test-resource", WithCapacity(10), WithTTL(1*time.Millisecond))
	if err != nil {
		t.Fatalf("failed to acquire permit: %v", err)
	}
	lp, ok := permit.(*localPermit)
	if !ok {
		t.Fatal("expected *localPermit type")
	}

	// Wait for permit to expire
	time.Sleep(5 * time.Millisecond)

	// Try to extend expired permit
	err = sem.extendPermit(context.Background(), lp, time.Now().Add(5*time.Minute))
	if err != ErrPermitNotHeld {
		t.Errorf("expected ErrPermitNotHeld for expired permit, got %v", err)
	}
}

// =============================================================================
// localSemaphore releasePermit 更多测试
// =============================================================================

func TestLocalSemaphore_ReleasePermit_NonExistent(t *testing.T) {
	opts := defaultOptions()
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)

	// Create a permit that doesn't exist in the semaphore
	fakePermit := &localPermit{
		permitBase: permitBase{
			id:       "non-existent",
			resource: "test-resource",
		},
	}

	err := sem.releasePermit(context.Background(), fakePermit)
	if err != ErrPermitNotHeld {
		t.Errorf("expected ErrPermitNotHeld, got %v", err)
	}
}

// =============================================================================
// IsCapacityFull 和 IsTenantQuotaExceeded 测试
// =============================================================================

func TestIsCapacityFull_Coverage(t *testing.T) {
	if !IsCapacityFull(ErrCapacityFull) {
		t.Error("expected true for ErrCapacityFull")
	}
	if IsCapacityFull(errors.New("other")) {
		t.Error("expected false for other error")
	}
}

func TestIsTenantQuotaExceeded_Coverage(t *testing.T) {
	if !IsTenantQuotaExceeded(ErrTenantQuotaExceeded) {
		t.Error("expected true for ErrTenantQuotaExceeded")
	}
	if IsTenantQuotaExceeded(errors.New("other")) {
		t.Error("expected false for other error")
	}
}

// =============================================================================
// initPermitBase 测试
// =============================================================================

func TestInitPermitBase(t *testing.T) {
	base := &permitBase{}
	now := time.Now()
	meta := map[string]string{"key": "value"}

	initPermitBase(base, "id", "resource", "tenant", now, 5*time.Minute, true, meta)

	if base.id != "id" {
		t.Errorf("expected id 'id', got %s", base.id)
	}
	if base.resource != "resource" {
		t.Errorf("expected resource 'resource', got %s", base.resource)
	}
	if base.tenantID != "tenant" {
		t.Errorf("expected tenantID 'tenant', got %s", base.tenantID)
	}
	if base.ttl != 5*time.Minute {
		t.Errorf("expected ttl 5m, got %v", base.ttl)
	}
	if !base.hasTenantQuota {
		t.Error("expected hasTenantQuota true")
	}
	if base.metadata["key"] != "value" {
		t.Error("expected metadata to be copied")
	}

	// Test metadata isolation
	meta["key"] = "changed"
	if base.metadata["key"] != "value" {
		t.Error("metadata should be copied, not referenced")
	}
}

// =============================================================================
// runAutoExtendLoop 错误处理测试
// =============================================================================

func TestRunAutoExtendLoop_ExtendError(t *testing.T) {
	base := &permitBase{
		id:       "test-permit",
		resource: "test-resource",
	}

	var extendCallCount atomic.Int32
	var logCalled atomic.Bool
	extendFunc := func(ctx context.Context) error {
		extendCallCount.Add(1)
		return errors.New("extend error")
	}

	stop := base.startAutoExtendLoop(10*time.Millisecond, extendFunc, &testLoggerForExtend{logCalled: &logCalled})
	defer stop()

	// Wait for a few extend attempts
	time.Sleep(50 * time.Millisecond)

	if extendCallCount.Load() == 0 {
		t.Error("expected extend to be called")
	}
	if !logCalled.Load() {
		t.Error("expected warning to be logged on extend error")
	}
}

type testLoggerForExtend struct {
	logCalled *atomic.Bool
}

func (m *testLoggerForExtend) logExtendFailed(ctx context.Context, permitID, resource string, err error) {
	m.logCalled.Store(true)
}

// =============================================================================
// fallbackSemaphore doFallback 完整测试
// =============================================================================

func TestDoFallback_AllStrategies(t *testing.T) {
	t.Run("FallbackOpen returns noop permit", func(t *testing.T) {
		opts := &options{
			fallback: FallbackOpen,
		}
		f := &fallbackSemaphore{
			strategy: FallbackOpen,
			opts:     opts,
		}

		permit, err := f.doFallback(context.Background(), "resource", []AcquireOption{
			WithCapacity(10),
			WithTTL(5 * time.Minute),
		}, true)

		require.NoError(t, err)
		require.NotNil(t, permit, "expected permit to be returned")
		// Verify it's a noop permit by checking ID prefix
		assert.True(t, len(permit.ID()) >= 5 && permit.ID()[:5] == "noop-", "expected noop permit, got ID: %s", permit.ID())
	})

	t.Run("FallbackClose returns error", func(t *testing.T) {
		opts := &options{
			fallback: FallbackClose,
		}
		f := &fallbackSemaphore{
			strategy: FallbackClose,
			opts:     opts,
		}

		permit, err := f.doFallback(context.Background(), "resource", nil, true)

		assert.ErrorIs(t, err, ErrRedisUnavailable)
		assert.Nil(t, permit, "expected nil permit")
	})

	t.Run("FallbackLocal uses local semaphore for TryAcquire", func(t *testing.T) {
		opts := &options{
			fallback: FallbackLocal,
			podCount: 1,
		}
		f := &fallbackSemaphore{
			strategy: FallbackLocal,
			opts:     opts,
		}

		permit, err := f.doFallback(context.Background(), "resource", []AcquireOption{
			WithCapacity(10),
			WithTTL(5 * time.Minute),
		}, true)

		assert.NoError(t, err)
		assert.NotNil(t, permit, "expected permit to be returned")

		// Clean up
		if permit != nil {
			_ = permit.Release(context.Background())
		}
		if f.local != nil {
			_ = f.local.Close(context.Background())
		}
	})

	t.Run("FallbackLocal uses local semaphore for Acquire", func(t *testing.T) {
		opts := &options{
			fallback: FallbackLocal,
			podCount: 1,
		}
		f := &fallbackSemaphore{
			strategy: FallbackLocal,
			opts:     opts,
		}

		permit, err := f.doFallback(context.Background(), "resource", []AcquireOption{
			WithCapacity(10),
			WithTTL(5 * time.Minute),
		}, false) // Acquire, not TryAcquire

		assert.NoError(t, err)
		assert.NotNil(t, permit, "expected permit to be returned")

		// Clean up
		if permit != nil {
			_ = permit.Release(context.Background())
		}
		if f.local != nil {
			_ = f.local.Close(context.Background())
		}
	})

	t.Run("default strategy with nil local returns error", func(t *testing.T) {
		opts := &options{
			fallback: FallbackStrategy("unknown"),
			podCount: 1,
		}
		f := &fallbackSemaphore{
			strategy: FallbackStrategy("unknown"),
			opts:     opts,
		}

		permit, err := f.doFallback(context.Background(), "resource", nil, true)

		assert.ErrorIs(t, err, ErrRedisUnavailable)
		assert.Nil(t, permit, "expected nil permit")
	})
}

// =============================================================================
// queryFallback 完整测试
// =============================================================================

func TestQueryFallback_AllStrategies(t *testing.T) {
	t.Run("FallbackOpen returns open info", func(t *testing.T) {
		opts := &options{
			fallback: FallbackOpen,
		}
		f := &fallbackSemaphore{
			strategy: FallbackOpen,
			opts:     opts,
		}

		info, err := f.queryFallback(context.Background(), "resource", []QueryOption{
			QueryWithCapacity(100),
		}, errors.New("test error"))

		require.NoError(t, err)
		require.NotNil(t, info, "expected info to be returned")
		assert.Equal(t, 0, info.GlobalUsed)
	})

	t.Run("FallbackClose returns error", func(t *testing.T) {
		opts := &options{
			fallback: FallbackClose,
		}
		f := &fallbackSemaphore{
			strategy: FallbackClose,
			opts:     opts,
		}

		info, err := f.queryFallback(context.Background(), "resource", nil, errors.New("test error"))

		assert.ErrorIs(t, err, ErrRedisUnavailable)
		assert.Nil(t, info, "expected nil info")
	})

	t.Run("FallbackLocal uses local semaphore", func(t *testing.T) {
		opts := &options{
			fallback: FallbackLocal,
			podCount: 1,
		}
		f := &fallbackSemaphore{
			strategy: FallbackLocal,
			opts:     opts,
		}

		info, err := f.queryFallback(context.Background(), "resource", []QueryOption{
			QueryWithCapacity(100),
		}, errors.New("test error"))

		require.NoError(t, err)
		require.NotNil(t, info, "expected info to be returned")

		// Clean up
		if f.local != nil {
			_ = f.local.Close(context.Background())
		}
	})

	t.Run("default strategy with nil local returns error", func(t *testing.T) {
		opts := &options{
			fallback: FallbackStrategy("unknown"),
			podCount: 1,
		}
		f := &fallbackSemaphore{
			strategy: FallbackStrategy("unknown"),
			opts:     opts,
		}

		info, err := f.queryFallback(context.Background(), "resource", nil, errors.New("test error"))

		assert.ErrorIs(t, err, ErrRedisUnavailable)
		assert.Nil(t, info, "expected nil info")
	})
}

// =============================================================================
// fallbackSemaphore Close 测试
// =============================================================================

func TestFallbackSemaphore_Close_Coverage(t *testing.T) {
	t.Run("closes both distributed and local", func(t *testing.T) {
		opts := &options{
			fallback: FallbackLocal,
			podCount: 1,
		}

		// Create a mock distributed semaphore
		mockDistributed := &closableTestSemaphore{}

		f := &fallbackSemaphore{
			distributed: mockDistributed,
			strategy:    FallbackLocal,
			opts:        opts,
		}

		// Initialize local semaphore by calling ensureLocalSemaphore
		f.ensureLocalSemaphore()

		err := f.Close(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !mockDistributed.closed {
			t.Error("distributed semaphore should be closed")
		}
	})
}

type closableTestSemaphore struct {
	closed bool
}

func (s *closableTestSemaphore) TryAcquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error) {
	return nil, nil
}
func (s *closableTestSemaphore) Acquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error) {
	return nil, nil
}
func (s *closableTestSemaphore) Query(ctx context.Context, resource string, opts ...QueryOption) (*ResourceInfo, error) {
	return nil, nil
}
func (s *closableTestSemaphore) Close(_ context.Context) error {
	s.closed = true
	return nil
}
func (s *closableTestSemaphore) Health(ctx context.Context) error {
	return nil
}
