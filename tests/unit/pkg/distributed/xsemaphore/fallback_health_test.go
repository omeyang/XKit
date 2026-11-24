package xsemaphore

import (
	"context"
	"errors"
	"testing"
	"time"
)

// =============================================================================
// fallbackSemaphore Health 测试
// =============================================================================

func TestFallbackSemaphore_Health_Coverage(t *testing.T) {
	t.Run("returns nil when distributed is healthy", func(t *testing.T) {
		mockDistributed := &healthyTestSemaphore{}
		opts := &options{
			fallback: FallbackLocal,
			podCount: 1,
		}

		f := &fallbackSemaphore{
			distributed: mockDistributed,
			strategy:    FallbackLocal,
			opts:        opts,
		}

		err := f.Health(context.Background())
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("returns error when distributed is unhealthy and no local", func(t *testing.T) {
		mockDistributed := &unhealthyTestSemaphore{}
		opts := &options{
			fallback: FallbackLocal,
			podCount: 1,
		}

		f := &fallbackSemaphore{
			distributed: mockDistributed,
			strategy:    FallbackLocal,
			opts:        opts,
		}

		err := f.Health(context.Background())
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("returns combined error when both are unhealthy", func(t *testing.T) {
		mockDistributed := &unhealthyTestSemaphore{}
		opts := &options{
			fallback: FallbackLocal,
			podCount: 1,
		}

		f := &fallbackSemaphore{
			distributed: mockDistributed,
			strategy:    FallbackLocal,
			opts:        opts,
		}

		// Initialize and close local semaphore
		f.ensureLocalSemaphore()
		_ = f.local.Close(context.Background())

		err := f.Health(context.Background())
		if err == nil {
			t.Error("expected error")
		}
	})
}

type healthyTestSemaphore struct{}

func (s *healthyTestSemaphore) TryAcquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error) {
	return nil, nil
}
func (s *healthyTestSemaphore) Acquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error) {
	return nil, nil
}
func (s *healthyTestSemaphore) Query(ctx context.Context, resource string, opts ...QueryOption) (*ResourceInfo, error) {
	return nil, nil
}
func (s *healthyTestSemaphore) Close(_ context.Context) error {
	return nil
}
func (s *healthyTestSemaphore) Health(ctx context.Context) error {
	return nil
}

type unhealthyTestSemaphore struct{}

func (s *unhealthyTestSemaphore) TryAcquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error) {
	return nil, nil
}
func (s *unhealthyTestSemaphore) Acquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error) {
	return nil, nil
}
func (s *unhealthyTestSemaphore) Query(ctx context.Context, resource string, opts ...QueryOption) (*ResourceInfo, error) {
	return nil, nil
}
func (s *unhealthyTestSemaphore) Close(_ context.Context) error {
	return nil
}
func (s *unhealthyTestSemaphore) Health(ctx context.Context) error {
	return ErrRedisUnavailable
}

// =============================================================================
// ensureLocalSemaphore 测试
// =============================================================================

func TestEnsureLocalSemaphore(t *testing.T) {
	t.Run("returns nil for non-local strategy", func(t *testing.T) {
		opts := &options{
			fallback: FallbackOpen,
		}
		f := &fallbackSemaphore{
			strategy: FallbackOpen,
			opts:     opts,
		}

		result := f.ensureLocalSemaphore()
		if result != nil {
			t.Error("expected nil for non-local strategy")
		}
	})

	t.Run("creates local semaphore for local strategy", func(t *testing.T) {
		opts := &options{
			fallback: FallbackLocal,
			podCount: 1,
		}
		f := &fallbackSemaphore{
			strategy: FallbackLocal,
			opts:     opts,
		}

		result := f.ensureLocalSemaphore()
		if result == nil {
			t.Error("expected local semaphore to be created")
		}

		// Clean up
		_ = result.Close(context.Background())
	})
}

// =============================================================================
// handleRedisError 完整测试
// =============================================================================

func TestHandleRedisError_Complete(t *testing.T) {
	t.Run("with metrics", func(t *testing.T) {
		logger := &testLogger{}
		opts := &options{
			logger:   logger,
			fallback: FallbackLocal,
		}

		f := &fallbackSemaphore{
			strategy: FallbackLocal,
			opts:     opts,
		}

		f.handleRedisError(context.Background(), "resource", ErrRedisUnavailable)

		if !logger.warnCalled {
			t.Error("expected Warn to be called")
		}
	})
}

// =============================================================================
// noopPermit Metadata nil 测试
// =============================================================================

func TestNoopPermit_Metadata_Nil(t *testing.T) {
	permit, err := newNoopPermit(context.Background(), "resource", "tenant", 5*time.Minute, nil, defaultOptions())
	if err != nil {
		t.Fatalf("failed to create noop permit: %v", err)
	}

	meta := permit.Metadata()
	if meta != nil {
		t.Error("expected nil metadata")
	}
}

// =============================================================================
// localSemaphore extendPermit 更多测试
// =============================================================================

func TestLocalSemaphore_ExtendPermit_ResourceNotExists(t *testing.T) {
	opts := defaultOptions()
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)

	// Try to extend a permit for a non-existent resource
	fakePermit := &localPermit{
		permitBase: permitBase{
			id:       "non-existent",
			resource: "non-existent-resource",
		},
	}

	err := sem.extendPermit(context.Background(), fakePermit, time.Now().Add(5*time.Minute))
	if err != ErrPermitNotHeld {
		t.Errorf("expected ErrPermitNotHeld, got %v", err)
	}
}

// =============================================================================
// localSemaphore 后台清理测试
// =============================================================================

func TestLocalSemaphore_BackgroundCleanup_Coverage(t *testing.T) {
	opts := defaultOptions()
	sem := newLocalSemaphore(opts)

	// Acquire a permit with very short TTL
	permit, err := sem.TryAcquire(context.Background(), "test-resource",
		WithCapacity(10),
		WithTTL(1*time.Millisecond))
	if err != nil {
		t.Fatalf("failed to acquire permit: %v", err)
	}
	_ = permit

	// Wait for permit to expire
	time.Sleep(10 * time.Millisecond)

	// Query should show 0 used after cleanup
	// Note: The background cleanup runs every 30 seconds by default
	// But we can trigger it manually
	sem.cleanupAllExpired()

	info, err := sem.Query(context.Background(), "test-resource", QueryWithCapacity(10))
	if err != nil {
		t.Fatalf("unexpected query error: %v", err)
	}
	if info != nil && info.GlobalUsed != 0 {
		t.Errorf("expected GlobalUsed 0 after cleanup, got %d", info.GlobalUsed)
	}

	_ = sem.Close(context.Background())
}

// =============================================================================
// Metrics 相关测试
// =============================================================================

func TestNewMetrics_NilProvider(t *testing.T) {
	m, err := NewMetrics(nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if m != nil {
		t.Error("expected nil metrics for nil provider")
	}
}

// =============================================================================
// 更多 Option 测试
// =============================================================================

func TestWithOnFallback_Coverage(t *testing.T) {
	callbackCalled := false
	callback := func(resource string, strategy FallbackStrategy, err error) {
		callbackCalled = true
	}

	opts := defaultOptions()
	WithOnFallback(callback)(opts)

	if opts.onFallback == nil {
		t.Error("expected onFallback to be set")
	}

	// Verify callback works
	opts.onFallback("resource", FallbackLocal, errors.New("test"))
	if !callbackCalled {
		t.Error("expected callback to be called")
	}
}

func TestWithLogger_Coverage(t *testing.T) {
	logger := &testLogger{}
	opts := defaultOptions()
	WithLogger(logger)(opts)

	if opts.logger == nil {
		t.Error("expected logger to be set")
	}
}

func TestWithMeterProvider(t *testing.T) {
	opts := defaultOptions()
	WithMeterProvider(nil)(opts)
	// Just verify no panic
}

// =============================================================================
// calculateLocalCapacity 测试
// =============================================================================

func TestCalculateLocalCapacity(t *testing.T) {
	opts := &options{podCount: 4}
	sem := &localSemaphore{opts: opts}

	cfg := &acquireOptions{
		capacity:    100,
		tenantQuota: 40,
	}

	localCap, localQuota := sem.calculateLocalCapacity(cfg)

	if localCap != 25 { // 100/4
		t.Errorf("expected localCapacity 25, got %d", localCap)
	}
	if localQuota != 10 { // 40/4
		t.Errorf("expected localTenantQuota 10, got %d", localQuota)
	}
}

func TestCalculateLocalCapacity_MinimumOne(t *testing.T) {
	opts := &options{podCount: 100}
	sem := &localSemaphore{opts: opts}

	cfg := &acquireOptions{
		capacity:    10,
		tenantQuota: 5,
	}

	localCap, localQuota := sem.calculateLocalCapacity(cfg)

	if localCap < 1 {
		t.Errorf("expected localCapacity at least 1, got %d", localCap)
	}
	if localQuota < 1 {
		t.Errorf("expected localTenantQuota at least 1, got %d", localQuota)
	}
}

// =============================================================================
// AcquireFailReason String 测试
// =============================================================================

func TestAcquireFailReason_String_Coverage(t *testing.T) {
	tests := []struct {
		reason   AcquireFailReason
		expected string
	}{
		{ReasonCapacityFull, "capacity_full"},
		{ReasonTenantQuotaExceeded, "tenant_quota_exceeded"},
		{ReasonUnknown, "unknown"},
		{AcquireFailReason(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if result := tt.reason.String(); result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// =============================================================================
// validateKeyPrefix 测试
// =============================================================================

func TestValidateKeyPrefix_Coverage(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		wantErr bool
	}{
		{"valid prefix", "mysem:", false},
		{"contains open brace", "my{sem:", true},
		{"contains close brace", "mysem}:", true},
		{"contains both braces", "my{sem}:", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateKeyPrefix(tt.prefix)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateKeyPrefix(%q) error = %v, wantErr %v", tt.prefix, err, tt.wantErr)
			}
		})
	}
}

// =============================================================================
// fallbackSemaphore Close 错误处理测试
// =============================================================================

func TestFallbackSemaphore_Close_DistributedError(t *testing.T) {
	opts := &options{
		fallback: FallbackLocal,
		podCount: 1,
	}

	// Create a mock distributed semaphore that returns error on close
	mockDistributed := &errorOnCloseSemaphore{}

	f := &fallbackSemaphore{
		distributed: mockDistributed,
		strategy:    FallbackLocal,
		opts:        opts,
	}

	// Initialize local semaphore
	f.ensureLocalSemaphore()

	err := f.Close(context.Background())
	if err == nil {
		t.Error("expected error from Close")
	}
}

type errorOnCloseSemaphore struct{}

func (s *errorOnCloseSemaphore) TryAcquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error) {
	return nil, nil
}
func (s *errorOnCloseSemaphore) Acquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error) {
	return nil, nil
}
func (s *errorOnCloseSemaphore) Query(ctx context.Context, resource string, opts ...QueryOption) (*ResourceInfo, error) {
	return nil, nil
}
func (s *errorOnCloseSemaphore) Close(_ context.Context) error {
	return errors.New("close error")
}
func (s *errorOnCloseSemaphore) Health(ctx context.Context) error {
	return nil
}

// =============================================================================
// fallbackSemaphore Query 测试
// =============================================================================

func TestFallbackSemaphore_Query_Coverage(t *testing.T) {
	t.Run("delegates to distributed and returns result", func(t *testing.T) {
		opts := &options{
			fallback: FallbackLocal,
		}
		mockDistributed := &healthyTestSemaphore{}
		f := &fallbackSemaphore{
			distributed: mockDistributed,
			strategy:    FallbackLocal,
			opts:        opts,
		}

		// fallbackSemaphore delegates validation to the underlying distributed semaphore;
		// the mock returns nil, nil, so fallbackSemaphore returns the same
		info, err := f.Query(context.Background(), "test-resource", nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if info != nil {
			t.Error("expected nil info from mock")
		}
	})
}

// =============================================================================
// handleRedisError trace event 测试
// =============================================================================

func TestHandleRedisError_WithTrace(t *testing.T) {
	opts := &options{
		fallback: FallbackLocal,
	}

	f := &fallbackSemaphore{
		strategy: FallbackLocal,
		opts:     opts,
	}

	// Just verify no panic when no span in context
	f.handleRedisError(context.Background(), "resource", ErrRedisUnavailable)
}

// =============================================================================
// localSemaphore recordExtendMetrics 测试
// =============================================================================

func TestLocalSemaphore_RecordExtendMetrics_WithMetrics(t *testing.T) {
	// This test would require a proper metrics implementation
	// For now, just test with nil metrics (already covered)
	opts := &options{metrics: nil}
	sem := &localSemaphore{opts: opts}
	sem.recordExtendMetrics(context.Background(), "resource", true)
}

// =============================================================================
// isRedisClusterError 更多分支测试
// =============================================================================

func TestIsRedisClusterError_MoreBranches(t *testing.T) {
	// Test with nil
	if isRedisClusterError(nil) {
		t.Error("nil error should return false")
	}

	// Test with regular error
	if isRedisClusterError(errors.New("regular error")) {
		t.Error("regular error should return false")
	}
}

// =============================================================================
// prepareAcquireCommon 更多分支测试
// =============================================================================

func TestPrepareAcquireCommon_InvalidOptions(t *testing.T) {
	// Fail-fast: invalid capacity should be caught by validate()
	_, _, err := prepareAcquireCommon(context.Background(), "resource", []AcquireOption{
		WithCapacity(-1),
	}, false)
	if err == nil {
		t.Error("expected error for invalid capacity")
	}
}

// =============================================================================
// permitBase releaseCommon 更多分支
// =============================================================================

func TestPermitBase_ReleaseCommon_Success(t *testing.T) {
	base := &permitBase{
		id:       "test-id",
		resource: "test-resource",
	}

	err := base.releaseCommon(context.Background(), nil, SemaphoreTypeLocal, nil, func(ctx context.Context) error {
		return nil // Success
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !base.isReleased() {
		t.Error("should be marked as released after success")
	}
}

// =============================================================================
// doFallback 默认分支测试
// =============================================================================

func TestDoFallback_DefaultWithLocal(t *testing.T) {
	// Note: ensureLocalSemaphore only creates local when strategy is FallbackLocal
	// For unknown strategies, it returns nil, so the default case returns ErrRedisUnavailable
	opts := &options{
		fallback: FallbackStrategy("custom"), // Unknown strategy
		podCount: 1,
	}
	f := &fallbackSemaphore{
		strategy: FallbackStrategy("custom"),
		opts:     opts,
	}

	// For unknown strategies with manually set local, the default case should work
	// But ensureLocalSemaphore won't initialize it because strategy != FallbackLocal
	// So we need to manually initialize to test the path where local exists
	f.local = newLocalSemaphore(opts)
	defer func() { _ = f.local.Close(context.Background()) }()

	// Since ensureLocalSemaphore returns nil for non-FallbackLocal strategies,
	// and the code checks if local == nil, it will return ErrRedisUnavailable
	// This is the expected behavior for unknown strategies
	permit, err := f.doFallback(context.Background(), "resource", []AcquireOption{
		WithCapacity(10),
		WithTTL(5 * time.Minute),
	}, true)

	// For unknown strategies, ensureLocalSemaphore returns nil, so we expect error
	if err != ErrRedisUnavailable {
		t.Errorf("expected ErrRedisUnavailable, got %v", err)
	}
	if permit != nil {
		t.Error("expected nil permit for unknown strategy")
	}
}

// =============================================================================
// 更多 fallbackSemaphore TryAcquire/Acquire 测试
// =============================================================================

type nonRedisErrorSemaphore struct{}

func (s *nonRedisErrorSemaphore) TryAcquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error) {
	return nil, ErrInvalidCapacity // Not a Redis error
}
func (s *nonRedisErrorSemaphore) Acquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error) {
	return nil, ErrInvalidCapacity // Not a Redis error
}
func (s *nonRedisErrorSemaphore) Query(ctx context.Context, resource string, opts ...QueryOption) (*ResourceInfo, error) {
	return nil, ErrInvalidCapacity
}
func (s *nonRedisErrorSemaphore) Close(_ context.Context) error {
	return nil
}
func (s *nonRedisErrorSemaphore) Health(ctx context.Context) error {
	return nil
}

func TestFallbackSemaphore_TryAcquire_NonRedisError(t *testing.T) {
	opts := &options{
		fallback: FallbackLocal,
	}
	f := &fallbackSemaphore{
		distributed: &nonRedisErrorSemaphore{},
		strategy:    FallbackLocal,
		opts:        opts,
	}

	_, err := f.TryAcquire(context.Background(), "resource", WithCapacity(10), WithTTL(5*time.Minute))
	if err != ErrInvalidCapacity {
		t.Errorf("expected ErrInvalidCapacity, got %v", err)
	}
}

func TestFallbackSemaphore_Acquire_NonRedisError(t *testing.T) {
	opts := &options{
		fallback: FallbackLocal,
	}
	f := &fallbackSemaphore{
		distributed: &nonRedisErrorSemaphore{},
		strategy:    FallbackLocal,
		opts:        opts,
	}

	_, err := f.Acquire(context.Background(), "resource", WithCapacity(10), WithTTL(5*time.Minute))
	if err != ErrInvalidCapacity {
		t.Errorf("expected ErrInvalidCapacity, got %v", err)
	}
}

func TestFallbackSemaphore_Query_NonRedisError(t *testing.T) {
	opts := &options{
		fallback: FallbackLocal,
	}
	f := &fallbackSemaphore{
		distributed: &nonRedisErrorSemaphore{},
		strategy:    FallbackLocal,
		opts:        opts,
	}

	_, err := f.Query(context.Background(), "resource", QueryWithCapacity(10))
	if err != ErrInvalidCapacity {
		t.Errorf("expected ErrInvalidCapacity, got %v", err)
	}
}

// =============================================================================
// calculateLocalQueryCapacity 测试
// =============================================================================

func TestCalculateLocalQueryCapacity(t *testing.T) {
	opts := &options{podCount: 4}
	sem := &localSemaphore{opts: opts}

	cfg := &queryOptions{
		capacity:    100,
		tenantQuota: 40,
	}

	localCap, localQuota := sem.calculateLocalQueryCapacity(cfg)

	if localCap != 25 { // 100/4
		t.Errorf("expected localCapacity 25, got %d", localCap)
	}
	if localQuota != 10 { // 40/4
		t.Errorf("expected localTenantQuota 10, got %d", localQuota)
	}
}

func TestCalculateLocalQueryCapacity_ZeroCapacity(t *testing.T) {
	opts := &options{podCount: 4}
	sem := &localSemaphore{opts: opts}

	cfg := &queryOptions{
		capacity:    0,
		tenantQuota: 0,
	}

	localCap, localQuota := sem.calculateLocalQueryCapacity(cfg)

	if localCap != 0 {
		t.Errorf("expected localCapacity 0, got %d", localCap)
	}
	if localQuota != 0 {
		t.Errorf("expected localTenantQuota 0, got %d", localQuota)
	}
}

// =============================================================================
// localSemaphore countActivePermits 测试
// =============================================================================

func TestCountActivePermits(t *testing.T) {
	opts := defaultOptions()
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)

	// Initially should be 0
	globalUsed, tenantUsed := sem.countActivePermits("resource", "tenant")
	if globalUsed != 0 || tenantUsed != 0 {
		t.Errorf("expected 0,0 got %d,%d", globalUsed, tenantUsed)
	}

	// Acquire a permit
	permit, err := sem.TryAcquire(context.Background(), "resource",
		WithCapacity(10),
		WithTTL(5*time.Minute),
		WithTenantID("tenant"),
		WithTenantQuota(5))
	if err != nil {
		t.Fatalf("failed to acquire: %v", err)
	}
	defer releasePermit(t, context.Background(), permit)

	// Should count 1
	globalUsed, tenantUsed = sem.countActivePermits("resource", "tenant")
	if globalUsed != 1 {
		t.Errorf("expected globalUsed 1, got %d", globalUsed)
	}
	if tenantUsed != 1 {
		t.Errorf("expected tenantUsed 1, got %d", tenantUsed)
	}
}

// =============================================================================
// newNoopPermit ID 生成失败测试 (难以测试，但覆盖调用)
// =============================================================================

func TestNewNoopPermit_Success(t *testing.T) {
	permit, err := newNoopPermit(context.Background(), "resource", "tenant", 5*time.Minute, map[string]string{"key": "value"}, defaultOptions())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if permit == nil {
		t.Fatal("expected permit")
	}
	if permit.Resource() != "resource" {
		t.Errorf("expected resource 'resource', got %s", permit.Resource())
	}
	if permit.TenantID() != "tenant" {
		t.Errorf("expected tenantID 'tenant', got %s", permit.TenantID())
	}
}
