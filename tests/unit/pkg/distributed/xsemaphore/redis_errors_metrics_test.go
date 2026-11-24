package xsemaphore

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xlog"
)

// =============================================================================
// effectivePodCount 测试
// =============================================================================

func TestOptions_EffectivePodCount(t *testing.T) {
	tests := []struct {
		name     string
		podCount int
		expected int
	}{
		{"zero returns default", 0, DefaultPodCount},
		{"negative returns default", -1, DefaultPodCount},
		{"positive returns value", 5, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &options{podCount: tt.podCount}
			if result := opts.effectivePodCount(); result != tt.expected {
				t.Errorf("effectivePodCount() = %d, want %d", result, tt.expected)
			}
		})
	}
}

// =============================================================================
// WarmupScripts 测试
// =============================================================================

func TestWarmupScripts_NilClient(t *testing.T) {
	err := WarmupScripts(context.Background(), nil)
	if err != ErrNilClient {
		t.Errorf("expected ErrNilClient, got %v", err)
	}
}

// =============================================================================
// isNetworkError 测试
// =============================================================================

func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"regular error", errors.New("error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNetworkError(tt.err)
			if result != tt.expected {
				t.Errorf("isNetworkError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// Metrics nil safety 测试
// =============================================================================

func TestMetrics_NilSafety(t *testing.T) {
	var m *Metrics = nil

	// All these should not panic
	m.RecordAcquire(context.Background(), "distributed", "resource", true, ReasonUnknown, time.Millisecond)
	m.RecordRelease(context.Background(), "distributed", "resource")
	m.RecordExtend(context.Background(), "distributed", "resource", true)
	m.RecordFallback(context.Background(), FallbackLocal, "resource", "reason")
}

// =============================================================================
// setSpanError/setSpanOK nil safety 测试
// =============================================================================

func TestSpanHelpers_NilSafety(t *testing.T) {
	// Should not panic with nil span
	setSpanError(nil, errors.New("error"))
	setSpanOK(nil)
}

// =============================================================================
// logAcquireExhausted 和 logExtendFailed 测试
// =============================================================================

// testLogger 实现 xlog.Logger 接口用于测试
type testLogger struct {
	warnCalled  bool
	errorCalled bool
	infoCalled  bool
	debugCalled bool
}

func (m *testLogger) Debug(ctx context.Context, msg string, attrs ...slog.Attr) { m.debugCalled = true }
func (m *testLogger) Info(ctx context.Context, msg string, attrs ...slog.Attr)  { m.infoCalled = true }
func (m *testLogger) Warn(ctx context.Context, msg string, attrs ...slog.Attr)  { m.warnCalled = true }
func (m *testLogger) Error(ctx context.Context, msg string, attrs ...slog.Attr) { m.errorCalled = true }
func (m *testLogger) Stack(ctx context.Context, msg string, attrs ...slog.Attr) {}
func (m *testLogger) With(attrs ...slog.Attr) xlog.Logger                       { return m }
func (m *testLogger) WithGroup(name string) xlog.Logger                         { return m }

// 编译时接口检查
var _ xlog.Logger = (*testLogger)(nil)

func TestLogAcquireExhausted_Coverage(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		logger := &testLogger{}
		opts := &options{logger: logger}
		sem := &redisSemaphore{opts: opts}

		sem.logAcquireExhausted(context.Background(), "resource", 10, ReasonCapacityFull)

		if !logger.warnCalled {
			t.Error("expected Warn to be called")
		}
	})

	t.Run("without logger", func(t *testing.T) {
		opts := &options{logger: nil}
		sem := &redisSemaphore{opts: opts}

		// Should not panic
		sem.logAcquireExhausted(context.Background(), "resource", 10, ReasonCapacityFull)
	})
}

func TestLogExtendFailed_Coverage(t *testing.T) {
	t.Run("via redisSemaphore with logger", func(t *testing.T) {
		logger := &testLogger{}
		opts := &options{logger: logger}
		sem := &redisSemaphore{opts: opts}

		sem.logExtendFailed(context.Background(), "permit-id", "resource", errors.New("extend failed"))

		if !logger.warnCalled {
			t.Error("expected Warn to be called")
		}
	})

	t.Run("via redisSemaphore without logger", func(t *testing.T) {
		opts := &options{logger: nil}
		sem := &redisSemaphore{opts: opts}

		// Should not panic
		sem.logExtendFailed(context.Background(), "permit-id", "resource", errors.New("extend failed"))
	})

	t.Run("via localSemaphore with logger", func(t *testing.T) {
		logger := &testLogger{}
		opts := &options{logger: logger}
		sem := &localSemaphore{opts: opts}

		sem.logExtendFailed(context.Background(), "permit-id", "resource", errors.New("extend failed"))

		if !logger.warnCalled {
			t.Error("expected Warn to be called")
		}
	})

	t.Run("via localSemaphore without logger", func(t *testing.T) {
		opts := &options{logger: nil}
		sem := &localSemaphore{opts: opts}

		// Should not panic
		sem.logExtendFailed(context.Background(), "permit-id", "resource", errors.New("extend failed"))
	})
}

// =============================================================================
// WithTracerProvider 和 WithDisableResourceLabel 测试
// =============================================================================

func TestWithTracerProvider(t *testing.T) {
	t.Run("sets tracer provider", func(t *testing.T) {
		opts := defaultOptions()
		// Just verify the option function doesn't panic
		WithTracerProvider(nil)(opts)
		// TracerProvider being nil is valid
	})
}

func TestWithDisableResourceLabel(t *testing.T) {
	t.Run("sets disable resource label", func(t *testing.T) {
		opts := defaultOptions()
		if opts.disableResourceLabel {
			t.Error("expected disableResourceLabel to be false initially")
		}

		WithDisableResourceLabel()(opts)

		if !opts.disableResourceLabel {
			t.Error("expected disableResourceLabel to be true after applying option")
		}
	})
}

// =============================================================================
// MetricsWithDisableResourceLabel 测试
// =============================================================================

func TestMetricsWithDisableResourceLabel(t *testing.T) {
	t.Run("sets disable resource label on metrics", func(t *testing.T) {
		m := &Metrics{}
		if m.disableResourceLabel {
			t.Error("expected disableResourceLabel to be false initially")
		}

		MetricsWithDisableResourceLabel()(m)

		if !m.disableResourceLabel {
			t.Error("expected disableResourceLabel to be true after applying option")
		}
	})
}

// =============================================================================
// handleAcquireResult 扩展测试
// =============================================================================

func TestHandleAcquireResult_UnknownStatus(t *testing.T) {
	logger := &testLogger{}
	opts := &options{logger: logger}
	sem := &redisSemaphore{opts: opts}

	// Test unknown status code
	result := []int64{999, 0, 0} // Unknown status code
	permit, reason, err := sem.handleAcquireResult(
		context.Background(),
		result,
		"permit-id",
		"resource",
		"tenant",
		time.Now().Add(5*time.Minute),
		&acquireOptions{},
		false,
	)

	if permit != nil {
		t.Error("expected nil permit for unknown status")
	}
	if reason != ReasonUnknown {
		t.Errorf("expected ReasonUnknown, got %v", reason)
	}
	if err == nil {
		t.Error("expected error for unknown status")
	}
	if !logger.warnCalled {
		t.Error("expected warning to be logged for unknown status")
	}
}

// =============================================================================
// isRedisClusterError 扩展测试（更多覆盖）
// =============================================================================

func TestIsRedisClusterError_AllBranches(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "regular error",
			err:      errors.New("some error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRedisClusterError(tt.err)
			if result != tt.expected {
				t.Errorf("isRedisClusterError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// isRedisProtocolError 测试
// =============================================================================

func TestIsRedisProtocolError_Coverage(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"regular error", errors.New("some error"), false},
		{"unknown command error", errors.New("ERR unknown command 'eval'"), true},
		{"NOSCRIPT error", errors.New("NOSCRIPT No matching script"), true},
		{"cluster support disabled", errors.New("cluster support disabled"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRedisProtocolError(tt.err)
			if result != tt.expected {
				t.Errorf("isRedisProtocolError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// IsRedisError 完整测试
// =============================================================================

func TestIsRedisError_Complete(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"context.Canceled", context.Canceled, false},
		{"context.DeadlineExceeded", context.DeadlineExceeded, false},
		{"unknown command", errors.New("ERR unknown command 'eval'"), true},
		{"NOSCRIPT", errors.New("NOSCRIPT No matching script"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRedisError(tt.err)
			if result != tt.expected {
				t.Errorf("IsRedisError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// IsRetryable 测试
// =============================================================================

func TestIsRetryable_Coverage(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"capacity full", ErrCapacityFull, false},
		{"tenant quota exceeded", ErrTenantQuotaExceeded, false},
		{"redis unavailable", ErrRedisUnavailable, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// recordAcquireMetrics 测试
// =============================================================================

func TestRecordAcquireMetrics(t *testing.T) {
	t.Run("with nil metrics", func(t *testing.T) {
		opts := &options{metrics: nil}
		sem := &redisSemaphore{opts: opts}

		// Should not panic
		sem.recordAcquireMetrics(context.Background(), "resource", true, ReasonUnknown, time.Millisecond)
	})
}

// =============================================================================
// waitIfNotLastRetry 测试
// =============================================================================

func TestWaitIfNotLastRetry(t *testing.T) {
	sem := &redisSemaphore{opts: defaultOptions()}

	t.Run("not last retry waits", func(t *testing.T) {
		ctx := context.Background()
		cfg := &acquireOptions{maxRetries: 3, retryDelay: 10 * time.Millisecond}

		start := time.Now()
		err := sem.waitIfNotLastRetry(ctx, 0, cfg) // i=0, maxRetries=3, so should wait
		duration := time.Since(start)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if duration < 5*time.Millisecond {
			t.Error("expected to wait")
		}
	})

	t.Run("last retry does not wait", func(t *testing.T) {
		ctx := context.Background()
		cfg := &acquireOptions{maxRetries: 3, retryDelay: 100 * time.Millisecond}

		start := time.Now()
		err := sem.waitIfNotLastRetry(ctx, 2, cfg) // i=2, maxRetries=3, so last retry
		duration := time.Since(start)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if duration > 50*time.Millisecond {
			t.Error("should not wait on last retry")
		}
	})

	t.Run("context canceled during wait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cfg := &acquireOptions{maxRetries: 3, retryDelay: 1 * time.Second}

		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		err := sem.waitIfNotLastRetry(ctx, 0, cfg)
		if err == nil {
			t.Error("expected error due to context cancellation")
		}
	})
}

// =============================================================================
// evalScriptInt64Slice 错误分支测试
// =============================================================================

func TestEvalScriptInt64Slice_ErrorBranches(t *testing.T) {
	// Test the type conversion error paths (we can't easily test with real Redis)
	t.Run("unexpected type in array", func(t *testing.T) {
		// This tests the validation of result types
		// We can only test validateScriptResult directly
		err := validateScriptResult([]int64{1}, 2)
		if err == nil {
			t.Error("expected error for insufficient length")
		}
	})
}

// =============================================================================
// localSemaphore cleanupExpiredLocked 测试
// =============================================================================

func TestCleanupExpiredLocked(t *testing.T) {
	opts := defaultOptions()
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)

	t.Run("cleans up expired global permits", func(t *testing.T) {
		rp := newResourcePermits()

		// Add expired permit
		expiredEntry := &permitEntry{
			id:        "expired",
			resource:  "test",
			tenantID:  "",
			expiresAt: time.Now().Add(-1 * time.Minute),
		}
		rp.global["expired"] = expiredEntry

		// Add valid permit
		validEntry := &permitEntry{
			id:        "valid",
			resource:  "test",
			tenantID:  "",
			expiresAt: time.Now().Add(5 * time.Minute),
		}
		rp.global["valid"] = validEntry

		sem.cleanupExpiredLocked(rp, time.Now())

		if _, exists := rp.global["expired"]; exists {
			t.Error("expired permit should be removed")
		}
		if _, exists := rp.global["valid"]; !exists {
			t.Error("valid permit should remain")
		}
	})

	t.Run("cleans up expired tenant permits", func(t *testing.T) {
		rp := newResourcePermits()

		// Add expired permit with tenant
		expiredEntry := &permitEntry{
			id:        "expired-tenant",
			resource:  "test",
			tenantID:  "tenant1",
			expiresAt: time.Now().Add(-1 * time.Minute),
		}
		rp.global["expired-tenant"] = expiredEntry
		rp.tenants["tenant1"] = map[string]*permitEntry{
			"expired-tenant": expiredEntry,
		}

		sem.cleanupExpiredLocked(rp, time.Now())

		if _, exists := rp.global["expired-tenant"]; exists {
			t.Error("expired permit should be removed from global")
		}
		if _, exists := rp.tenants["tenant1"]; exists {
			t.Error("empty tenant map should be removed")
		}
	})
}

// =============================================================================
// removeExpiredPermitLocked 测试
// =============================================================================

func TestRemoveExpiredPermitLocked(t *testing.T) {
	opts := defaultOptions()
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)

	t.Run("removes permit from global and tenant", func(t *testing.T) {
		rp := newResourcePermits()
		p := &localPermit{
			permitBase: permitBase{
				id:             "test-permit",
				resource:       "test",
				tenantID:       "tenant1",
				hasTenantQuota: true,
			},
		}

		// Add to global
		rp.global["test-permit"] = &permitEntry{
			id:        "test-permit",
			resource:  "test",
			tenantID:  "tenant1",
			expiresAt: time.Now().Add(5 * time.Minute),
		}

		// Add to tenant
		rp.tenants["tenant1"] = map[string]*permitEntry{
			"test-permit": rp.global["test-permit"],
		}

		sem.removeExpiredPermitLocked(rp, p)

		if _, exists := rp.global["test-permit"]; exists {
			t.Error("permit should be removed from global")
		}
		if _, exists := rp.tenants["tenant1"]; exists {
			t.Error("empty tenant map should be removed")
		}
	})

	t.Run("handles permit without tenant quota", func(t *testing.T) {
		rp := newResourcePermits()
		p := &localPermit{
			permitBase: permitBase{
				id:             "test-permit",
				resource:       "test",
				tenantID:       "tenant1",
				hasTenantQuota: false,
			},
		}

		rp.global["test-permit"] = &permitEntry{}

		// Should not panic
		sem.removeExpiredPermitLocked(rp, p)

		if _, exists := rp.global["test-permit"]; exists {
			t.Error("permit should be removed from global")
		}
	})
}

// =============================================================================
// recordExtendMetrics 测试
// =============================================================================

func TestRecordExtendMetrics(t *testing.T) {
	t.Run("with nil metrics", func(t *testing.T) {
		opts := &options{metrics: nil}
		sem := &localSemaphore{opts: opts}

		// Should not panic
		sem.recordExtendMetrics(context.Background(), "resource", true)
	})
}

// =============================================================================
// noopPermit StartAutoExtend 完整测试
// =============================================================================

func TestNoopPermit_StartAutoExtend_WithLogger(t *testing.T) {
	opts := defaultOptions()
	permit, err := newNoopPermit(context.Background(), "resource", "tenant", 100*time.Millisecond, nil, opts)
	if err != nil {
		t.Fatalf("failed to create noop permit: %v", err)
	}

	// 记录初始过期时间
	initialExpiry := permit.ExpiresAt()

	// 启动自动续租（间隔 50ms，TTL 100ms）
	stop := permit.StartAutoExtend(50 * time.Millisecond)
	defer stop()

	// 等待至少一次续租
	time.Sleep(120 * time.Millisecond)

	// 验证 expiresAt 已被更新（续租生效）
	updatedExpiry := permit.ExpiresAt()
	if !updatedExpiry.After(initialExpiry) {
		t.Errorf("expected expiresAt to be updated after auto-extend, initial=%v, updated=%v", initialExpiry, updatedExpiry)
	}

	// 停止自动续租
	stop()
}

// =============================================================================
// fallbackSemaphore 测试
// =============================================================================

func TestFallbackSemaphore_LogFallback(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		logger := &testLogger{}
		opts := &options{
			logger:   logger,
			fallback: FallbackLocal,
		}

		// Create a mock distributed semaphore for testing
		f := &fallbackSemaphore{
			strategy: FallbackLocal,
			opts:     opts,
		}

		f.logFallback(context.Background(), "resource", errors.New("test error"))

		if !logger.warnCalled {
			t.Error("expected Warn to be called")
		}
	})

	t.Run("without logger", func(t *testing.T) {
		opts := &options{
			logger:   nil,
			fallback: FallbackLocal,
		}

		f := &fallbackSemaphore{
			strategy: FallbackLocal,
			opts:     opts,
		}

		// Should not panic
		f.logFallback(context.Background(), "resource", errors.New("test error"))
	})
}

func TestFallbackSemaphore_SafeOnFallback(t *testing.T) {
	t.Run("callback is called", func(t *testing.T) {
		callbackCalled := false
		opts := &options{
			onFallback: func(resource string, strategy FallbackStrategy, err error) {
				callbackCalled = true
			},
		}

		f := &fallbackSemaphore{
			strategy: FallbackLocal,
			opts:     opts,
		}

		f.safeOnFallback(context.Background(), "resource", errors.New("test error"))

		if !callbackCalled {
			t.Error("expected callback to be called")
		}
	})

	t.Run("callback panic is recovered", func(t *testing.T) {
		logger := &testLogger{}
		opts := &options{
			logger: logger,
			onFallback: func(resource string, strategy FallbackStrategy, err error) {
				panic("test panic")
			},
		}

		f := &fallbackSemaphore{
			strategy: FallbackLocal,
			opts:     opts,
		}

		// Should not panic
		f.safeOnFallback(context.Background(), "resource", errors.New("test error"))

		if !logger.errorCalled {
			t.Error("expected Error to be called when callback panics")
		}
	})

	t.Run("nil callback", func(t *testing.T) {
		opts := &options{
			onFallback: nil,
		}

		f := &fallbackSemaphore{
			strategy: FallbackLocal,
			opts:     opts,
		}

		// Should not panic
		f.safeOnFallback(context.Background(), "resource", errors.New("test error"))
	})
}
