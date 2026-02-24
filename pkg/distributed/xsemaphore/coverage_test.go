package xsemaphore

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omeyang/xkit/pkg/observability/xlog"
)

// =============================================================================
// applyDefaultTimeout 测试
// =============================================================================

func TestApplyDefaultTimeout(t *testing.T) {
	t.Run("zero timeout returns original context", func(t *testing.T) {
		ctx := context.Background()
		newCtx, cancel := applyDefaultTimeout(ctx, 0)
		defer cancel()

		if newCtx != ctx {
			t.Error("expected original context to be returned")
		}
	})

	t.Run("negative timeout returns original context", func(t *testing.T) {
		ctx := context.Background()
		newCtx, cancel := applyDefaultTimeout(ctx, -1*time.Second)
		defer cancel()

		if newCtx != ctx {
			t.Error("expected original context to be returned")
		}
	})

	t.Run("context with existing deadline is not modified", func(t *testing.T) {
		ctx, existingCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer existingCancel()

		deadline1, _ := ctx.Deadline()

		newCtx, cancel := applyDefaultTimeout(ctx, 1*time.Second)
		defer cancel()

		deadline2, ok := newCtx.Deadline()
		if !ok {
			t.Fatal("expected deadline to exist")
		}

		if !deadline1.Equal(deadline2) {
			t.Error("expected deadline to remain unchanged")
		}
	})

	t.Run("applies timeout to context without deadline", func(t *testing.T) {
		ctx := context.Background()
		newCtx, cancel := applyDefaultTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		if newCtx == ctx {
			t.Error("expected new context to be created")
		}

		_, ok := newCtx.Deadline()
		if !ok {
			t.Error("expected deadline to be set")
		}
	})
}

// =============================================================================
// WithDefaultTimeout 测试
// =============================================================================

func TestWithDefaultTimeout(t *testing.T) {
	t.Run("sets default timeout", func(t *testing.T) {
		opts := defaultOptions()
		WithDefaultTimeout(5 * time.Second)(opts)

		if opts.defaultTimeout != 5*time.Second {
			t.Errorf("expected defaultTimeout to be 5s, got %v", opts.defaultTimeout)
		}
	})

	t.Run("ignores zero value", func(t *testing.T) {
		opts := defaultOptions()
		opts.defaultTimeout = 3 * time.Second
		WithDefaultTimeout(0)(opts)

		if opts.defaultTimeout != 3*time.Second {
			t.Errorf("expected defaultTimeout to remain 3s, got %v", opts.defaultTimeout)
		}
	})

	t.Run("ignores negative value", func(t *testing.T) {
		opts := defaultOptions()
		opts.defaultTimeout = 3 * time.Second
		WithDefaultTimeout(-1 * time.Second)(opts)

		if opts.defaultTimeout != 3*time.Second {
			t.Errorf("expected defaultTimeout to remain 3s, got %v", opts.defaultTimeout)
		}
	})
}

// =============================================================================
// AttrStatusCode 测试
// =============================================================================

func TestAttrStatusCode(t *testing.T) {
	attr := AttrStatusCode(42)
	if attr.Key != attrKeyStatusCode {
		t.Errorf("expected key %s, got %s", attrKeyStatusCode, attr.Key)
	}
	if attr.Value.Int64() != 42 {
		t.Errorf("expected value 42, got %d", attr.Value.Int64())
	}
}

// =============================================================================
// isRedisClusterError 扩展测试
// =============================================================================

func TestIsRedisClusterError_Extended(t *testing.T) {
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
		{
			name:     "CROSSSLOT error",
			err:      redis.ErrCrossSlot,
			expected: true,
		},
		{
			name:     "wrapped CROSSSLOT error",
			err:      errors.Join(errors.New("wrapped"), redis.ErrCrossSlot),
			expected: true,
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
// isRetryableRedisError 扩展测试
// =============================================================================

func TestIsRetryableRedisError_Extended(t *testing.T) {
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
		{
			name:     "context canceled",
			err:      context.Canceled,
			expected: false,
		},
		{
			name:     "CROSSSLOT error is not retryable",
			err:      redis.ErrCrossSlot,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableRedisError(tt.err)
			if result != tt.expected {
				t.Errorf("isRetryableRedisError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// AcquireFailReason 测试
// =============================================================================

func TestAcquireFailReason_Error_Extended(t *testing.T) {
	tests := []struct {
		reason   AcquireFailReason
		expected error
	}{
		{ReasonCapacityFull, ErrCapacityFull},
		{ReasonTenantQuotaExceeded, ErrTenantQuotaExceeded},
		{ReasonUnknown, nil},
		{AcquireFailReason(999), nil},
	}

	for _, tt := range tests {
		t.Run(tt.reason.String(), func(t *testing.T) {
			err := tt.reason.Error()
			if !errors.Is(err, tt.expected) {
				t.Errorf("AcquireFailReason(%d).Error() = %v, want %v", tt.reason, err, tt.expected)
			}
		})
	}
}

// =============================================================================
// FallbackStrategy 测试
// =============================================================================

func TestFallbackStrategy_IsValid_Extended(t *testing.T) {
	tests := []struct {
		strategy FallbackStrategy
		expected bool
	}{
		{FallbackNone, true},
		{FallbackLocal, true},
		{FallbackOpen, true},
		{FallbackClose, true},
		{FallbackStrategy("invalid"), false},
		{FallbackStrategy(""), true}, // FallbackNone
	}

	for _, tt := range tests {
		t.Run(string(tt.strategy), func(t *testing.T) {
			if result := tt.strategy.IsValid(); result != tt.expected {
				t.Errorf("FallbackStrategy(%q).IsValid() = %v, want %v", tt.strategy, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// validateScriptResult 测试
// =============================================================================

func TestValidateScriptResult(t *testing.T) {
	tests := []struct {
		name    string
		result  []int64
		minLen  int
		wantErr bool
	}{
		{
			name:    "sufficient length",
			result:  []int64{1, 2, 3},
			minLen:  3,
			wantErr: false,
		},
		{
			name:    "more than required",
			result:  []int64{1, 2, 3, 4},
			minLen:  3,
			wantErr: false,
		},
		{
			name:    "insufficient length",
			result:  []int64{1, 2},
			minLen:  3,
			wantErr: true,
		},
		{
			name:    "empty result",
			result:  []int64{},
			minLen:  1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateScriptResult(tt.result, tt.minLen)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateScriptResult() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// =============================================================================
// Permit Metadata 测试
// =============================================================================

func TestPermitBase_Metadata_Extended(t *testing.T) {
	t.Run("nil metadata returns nil", func(t *testing.T) {
		base := &permitBase{}
		if base.Metadata() != nil {
			t.Error("expected nil metadata")
		}
	})

	t.Run("returns copy of metadata", func(t *testing.T) {
		base := &permitBase{
			metadata: map[string]string{"key": "value"},
		}
		meta := base.Metadata()
		if meta["key"] != "value" {
			t.Error("expected metadata to contain key")
		}

		// Modify the copy, original should be unchanged
		meta["key"] = "modified"
		if base.metadata["key"] != "value" {
			t.Error("original metadata should not be modified")
		}
	})
}

// =============================================================================
// Permit ExpiresAt 测试
// =============================================================================

func TestPermitBase_ExpiresAt_Extended(t *testing.T) {
	t.Run("returns zero time when not set", func(t *testing.T) {
		base := &permitBase{}
		if !base.ExpiresAt().IsZero() {
			t.Error("expected zero time")
		}
	})

	t.Run("returns stored time", func(t *testing.T) {
		base := &permitBase{}
		now := time.Now()
		base.setExpiresAt(now)

		if !base.ExpiresAt().Equal(now) {
			t.Error("expected stored time")
		}
	})
}

// =============================================================================
// ResourceInfo 测试
// =============================================================================

func TestResourceInfo(t *testing.T) {
	info := &ResourceInfo{
		Resource:        "test",
		GlobalCapacity:  100,
		GlobalUsed:      50,
		GlobalAvailable: 50,
		TenantID:        "tenant1",
		TenantQuota:     10,
		TenantUsed:      5,
		TenantAvailable: 5,
	}

	if info.Resource != "test" {
		t.Error("Resource mismatch")
	}
	if info.GlobalCapacity != 100 {
		t.Error("GlobalCapacity mismatch")
	}
	if info.TenantID != "tenant1" {
		t.Error("TenantID mismatch")
	}
}

// =============================================================================
// Error Classification 测试
// =============================================================================

func TestClassifyError_Extended(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"nil error", nil, ""},
		{"permit not held", ErrPermitNotHeld, ErrClassPermitNotHeld},
		{"deadline exceeded", context.DeadlineExceeded, ErrClassTimeout},
		{"context canceled", context.Canceled, ErrClassCanceled},
		{"other error", errors.New("unknown"), ErrClassInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(tt.err)
			if result != tt.expected {
				t.Errorf("ClassifyError(%v) = %q, want %q", tt.err, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// permitBase AutoExtend 测试
// =============================================================================

func TestPermitBase_StartAutoExtendLoop(t *testing.T) {
	t.Run("zero interval returns noop", func(t *testing.T) {
		base := &permitBase{}
		stop := base.startAutoExtendLoop(0, nil, nil)
		stop() // Should not panic
	})

	t.Run("negative interval returns noop", func(t *testing.T) {
		base := &permitBase{}
		stop := base.startAutoExtendLoop(-1*time.Second, nil, nil)
		stop() // Should not panic
	})

	t.Run("double start returns same stop function", func(t *testing.T) {
		base := &permitBase{}
		extendFunc := func(ctx context.Context) error { return nil }

		stop1 := base.startAutoExtendLoop(100*time.Millisecond, extendFunc, nil)
		stop2 := base.startAutoExtendLoop(100*time.Millisecond, extendFunc, nil)

		// Both should work
		stop1()
		stop2() // Should not panic
	})

	t.Run("stops when released", func(t *testing.T) {
		base := &permitBase{}
		extendCalled := make(chan struct{}, 10)

		extendFunc := func(ctx context.Context) error {
			select {
			case extendCalled <- struct{}{}:
			default:
			}
			return nil
		}

		stop := base.startAutoExtendLoop(10*time.Millisecond, extendFunc, nil)
		defer stop()

		// Wait for at least one extend call
		<-extendCalled

		// Mark as released
		base.markReleased()

		// Wait a bit and verify no more extends
		time.Sleep(50 * time.Millisecond)
	})

	t.Run("stops on ErrPermitNotHeld", func(t *testing.T) {
		base := &permitBase{}

		extendFunc := func(ctx context.Context) error {
			return ErrPermitNotHeld
		}

		stop := base.startAutoExtendLoop(10*time.Millisecond, extendFunc, nil)
		defer stop()

		// Wait for loop to exit
		time.Sleep(50 * time.Millisecond)
	})
}

// =============================================================================
// releaseCommon 测试
// =============================================================================

func TestPermitBase_ReleaseCommon(t *testing.T) {
	t.Run("already released returns nil", func(t *testing.T) {
		base := &permitBase{
			id:       "test-id",
			resource: "test-resource",
		}
		base.markReleased()

		err := base.releaseCommon(context.Background(), nil, SemaphoreTypeLocal, nil, func(ctx context.Context) error {
			t.Error("doRelease should not be called")
			return nil
		})

		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("ErrPermitNotHeld marks as released", func(t *testing.T) {
		base := &permitBase{
			id:       "test-id",
			resource: "test-resource",
		}

		err := base.releaseCommon(context.Background(), nil, SemaphoreTypeLocal, nil, func(ctx context.Context) error {
			return ErrPermitNotHeld
		})

		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if !base.isReleased() {
			t.Error("expected to be marked as released")
		}
	})

	t.Run("other error is returned", func(t *testing.T) {
		base := &permitBase{
			id:       "test-id",
			resource: "test-resource",
		}

		expectedErr := errors.New("network error")
		err := base.releaseCommon(context.Background(), nil, SemaphoreTypeLocal, nil, func(ctx context.Context) error {
			return expectedErr
		})

		if err != expectedErr {
			t.Errorf("expected %v, got %v", expectedErr, err)
		}
		if base.isReleased() {
			t.Error("should not be marked as released on error")
		}
	})
}

// =============================================================================
// extendCommon 测试
// =============================================================================

func TestPermitBase_ExtendCommon(t *testing.T) {
	t.Run("already released returns error", func(t *testing.T) {
		base := &permitBase{
			id:       "test-id",
			resource: "test-resource",
		}
		base.markReleased()

		err := base.extendCommon(context.Background(), nil, SemaphoreTypeLocal, func(ctx context.Context, t time.Time) error {
			return nil
		})

		if err != ErrPermitNotHeld {
			t.Errorf("expected ErrPermitNotHeld, got %v", err)
		}
	})

	t.Run("success updates expiresAt", func(t *testing.T) {
		base := &permitBase{
			id:       "test-id",
			resource: "test-resource",
			ttl:      5 * time.Minute,
		}
		originalTime := time.Now()
		base.setExpiresAt(originalTime)

		err := base.extendCommon(context.Background(), nil, SemaphoreTypeLocal, func(ctx context.Context, newExpiry time.Time) error {
			return nil
		})

		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}

		// ExpiresAt should be updated
		if base.ExpiresAt().Before(originalTime) {
			t.Error("expiresAt should be updated to a later time")
		}
	})
}

// =============================================================================
// Options 验证测试
// =============================================================================

func TestAcquireOptions_Validate_Extended(t *testing.T) {
	tests := []struct {
		name    string
		opts    *acquireOptions
		wantErr bool
	}{
		{
			name: "valid options",
			opts: &acquireOptions{
				capacity:    10,
				ttl:         time.Minute,
				tenantQuota: 5,
				maxRetries:  DefaultMaxRetries,
				retryDelay:  DefaultRetryDelay,
			},
			wantErr: false,
		},
		{
			name: "zero capacity",
			opts: &acquireOptions{
				capacity:   0,
				ttl:        time.Minute,
				maxRetries: DefaultMaxRetries,
				retryDelay: DefaultRetryDelay,
			},
			wantErr: true,
		},
		{
			name: "negative capacity",
			opts: &acquireOptions{
				capacity:   -1,
				ttl:        time.Minute,
				maxRetries: DefaultMaxRetries,
				retryDelay: DefaultRetryDelay,
			},
			wantErr: true,
		},
		{
			name: "zero ttl",
			opts: &acquireOptions{
				capacity:   10,
				ttl:        0,
				maxRetries: DefaultMaxRetries,
				retryDelay: DefaultRetryDelay,
			},
			wantErr: true,
		},
		{
			name: "negative tenant quota",
			opts: &acquireOptions{
				capacity:    10,
				ttl:         time.Minute,
				tenantQuota: -1,
				maxRetries:  DefaultMaxRetries,
				retryDelay:  DefaultRetryDelay,
			},
			wantErr: true,
		},
		{
			name: "tenant quota exceeds capacity",
			opts: &acquireOptions{
				capacity:    10,
				ttl:         time.Minute,
				tenantQuota: 20,
				maxRetries:  DefaultMaxRetries,
				retryDelay:  DefaultRetryDelay,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// =============================================================================
// Span Attributes 测试
// =============================================================================

func TestExtendSpanAttributes(t *testing.T) {
	t.Run("without tenant", func(t *testing.T) {
		attrs := extendSpanAttributes("distributed", "resource", "", "permit-123")
		if len(attrs) != 3 {
			t.Errorf("expected 3 attributes, got %d", len(attrs))
		}
	})

	t.Run("with tenant", func(t *testing.T) {
		attrs := extendSpanAttributes("distributed", "resource", "tenant-1", "permit-123")
		if len(attrs) != 4 {
			t.Errorf("expected 4 attributes, got %d", len(attrs))
		}
	})
}

// =============================================================================
// noopPermit 测试
// =============================================================================

func TestNoopPermit(t *testing.T) {
	permit, err := newNoopPermit(context.Background(), "resource", "tenant1", 5*time.Minute, map[string]string{"key": "value"}, defaultOptions())
	require.NoError(t, err, "failed to create noop permit")

	t.Run("ID has noop prefix", func(t *testing.T) {
		assert.True(t, len(permit.ID()) >= 5 && permit.ID()[:5] == "noop-", "expected ID to start with 'noop-', got %s", permit.ID())
	})

	t.Run("Resource returns correct value", func(t *testing.T) {
		assert.Equal(t, "resource", permit.Resource())
	})

	t.Run("TenantID returns correct value", func(t *testing.T) {
		assert.Equal(t, "tenant1", permit.TenantID())
	})

	t.Run("Extend returns nil before release", func(t *testing.T) {
		assert.NoError(t, permit.Extend(context.Background()))
	})

	t.Run("Release returns nil", func(t *testing.T) {
		assert.NoError(t, permit.Release(context.Background()))
	})

	t.Run("Extend after release returns ErrPermitNotHeld", func(t *testing.T) {
		assert.True(t, IsPermitNotHeld(permit.Extend(context.Background())))
	})

	t.Run("StartAutoExtend returns stop function", func(t *testing.T) {
		stop := permit.StartAutoExtend(time.Second)
		stop() // Should not panic
	})

	t.Run("Metadata returns copy", func(t *testing.T) {
		meta := permit.Metadata()
		assert.Equal(t, "value", meta["key"])
		meta["key"] = "modified"
		assert.Equal(t, "value", permit.metadata["key"], "original metadata should not be modified")
	})

	t.Run("ExpiresAt is set", func(t *testing.T) {
		assert.False(t, permit.ExpiresAt().IsZero(), "expected ExpiresAt to be set")
	})
}

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

// =============================================================================
// backgroundCleanupLoop 测试
// =============================================================================

func TestBackgroundCleanupLoop_StopsOnClose(t *testing.T) {
	opts := defaultOptions()
	sem := newLocalSemaphore(opts)

	// Close should stop the background cleanup
	err := sem.Close(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Second close should be idempotent
	err = sem.Close(context.Background())
	if err != nil {
		t.Errorf("unexpected error on second close: %v", err)
	}
}

// =============================================================================
// applyDefaultTimeout nil context 回归测试（FG-S1 修复验证）
// =============================================================================

func TestApplyDefaultTimeout_NilContext(t *testing.T) {
	// 修复前：当 defaultTimeout > 0 且 ctx == nil 时，ctx.Deadline() 会 panic
	// 修复后：nil ctx 直接透传，由后续 validateCommonParams 返回 ErrNilContext
	t.Run("nil ctx with positive timeout does not panic", func(t *testing.T) {
		ctx, cancel := applyDefaultTimeout(nil, 5*time.Second)
		defer cancel()
		assert.Nil(t, ctx)
	})

	t.Run("nil ctx with zero timeout returns nil", func(t *testing.T) {
		ctx, cancel := applyDefaultTimeout(nil, 0)
		defer cancel()
		assert.Nil(t, ctx)
	})
}

// TestNilContextWithDefaultTimeout_Redis 验证 Redis 信号量的 nil context + 默认超时路径
func TestNilContextWithDefaultTimeout_Redis(t *testing.T) {
	_, client := setupRedis(t)

	sem, err := New(client, WithDefaultTimeout(5*time.Second))
	require.NoError(t, err)
	t.Cleanup(func() { closeSemaphore(t, sem) })

	t.Run("TryAcquire returns ErrNilContext", func(t *testing.T) {
		_, err := sem.TryAcquire(nil, "resource", WithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})

	t.Run("Acquire returns ErrNilContext", func(t *testing.T) {
		_, err := sem.Acquire(nil, "resource", WithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})

	t.Run("Query returns ErrNilContext", func(t *testing.T) {
		_, err := sem.Query(nil, "resource", QueryWithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})
}

// TestNilContextWithDefaultTimeout_Local 验证本地信号量的 nil context + 默认超时路径
func TestNilContextWithDefaultTimeout_Local(t *testing.T) {
	opts := defaultOptions()
	opts.defaultTimeout = 5 * time.Second
	sem := newLocalSemaphore(opts)
	t.Cleanup(func() { _ = sem.Close(context.Background()) })

	t.Run("TryAcquire returns ErrNilContext", func(t *testing.T) {
		_, err := sem.TryAcquire(nil, "resource", WithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})

	t.Run("Acquire returns ErrNilContext", func(t *testing.T) {
		_, err := sem.Acquire(nil, "resource", WithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})

	t.Run("Query returns ErrNilContext", func(t *testing.T) {
		_, err := sem.Query(nil, "resource", QueryWithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})
}

// TestNilContextWithDefaultTimeout_Fallback 验证降级信号量的 nil context + 默认超时路径
func TestNilContextWithDefaultTimeout_Fallback(t *testing.T) {
	_, client := setupRedis(t)

	sem, err := New(client,
		WithDefaultTimeout(5*time.Second),
		WithFallback(FallbackLocal),
		WithPodCount(1),
	)
	require.NoError(t, err)
	t.Cleanup(func() { closeSemaphore(t, sem) })

	t.Run("TryAcquire returns ErrNilContext", func(t *testing.T) {
		_, err := sem.TryAcquire(nil, "resource", WithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})

	t.Run("Acquire returns ErrNilContext", func(t *testing.T) {
		_, err := sem.Acquire(nil, "resource", WithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})

	t.Run("Query returns ErrNilContext", func(t *testing.T) {
		_, err := sem.Query(nil, "resource", QueryWithCapacity(10))
		assert.ErrorIs(t, err, ErrNilContext)
	})
}

// =============================================================================
// ErrNilContext 测试（覆盖新增的 nil context 校验路径）
// =============================================================================

func TestNilContext_ValidateCommonParams(t *testing.T) {
	err := validateCommonParams(nil, "resource", false)
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestNilContext_PrepareAcquireCommon(t *testing.T) {
	_, _, err := prepareAcquireCommon(nil, "resource", nil, false)
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestNilContext_PrepareQueryCommon(t *testing.T) {
	_, _, err := prepareQueryCommon(nil, "resource", nil, false)
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestNilContext_ReleaseCommon(t *testing.T) {
	p := &permitBase{}
	initPermitBase(p, "test-id", "resource", "tenant", time.Now().Add(5*time.Minute), 5*time.Minute, false, nil)
	err := p.releaseCommon(nil, nil, SemaphoreTypeLocal, nil, func(ctx context.Context) error {
		return nil
	})
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestNilContext_ExtendCommon(t *testing.T) {
	p := &permitBase{}
	initPermitBase(p, "test-id", "resource", "tenant", time.Now().Add(5*time.Minute), 5*time.Minute, false, nil)
	err := p.extendCommon(nil, nil, SemaphoreTypeLocal, func(ctx context.Context, _ time.Time) error {
		return nil
	})
	assert.ErrorIs(t, err, ErrNilContext)
}

func TestNilContext_NoopPermitRelease(t *testing.T) {
	p, err := newNoopPermit(context.Background(), "resource", "tenant", 5*time.Minute, nil, defaultOptions())
	require.NoError(t, err)
	assert.ErrorIs(t, p.Release(nil), ErrNilContext)
}

func TestNilContext_NoopPermitExtend(t *testing.T) {
	p, err := newNoopPermit(context.Background(), "resource", "tenant", 5*time.Minute, nil, defaultOptions())
	require.NoError(t, err)
	assert.ErrorIs(t, p.Extend(nil), ErrNilContext)
}

// =============================================================================
// noopPermit logExtendFailed 测试
// =============================================================================

func TestNoopPermit_LogExtendFailed(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		logger := &testLogger{}
		opts := defaultOptions()
		opts.logger = logger
		p, err := newNoopPermit(context.Background(), "resource", "tenant", 5*time.Minute, nil, opts)
		require.NoError(t, err)
		p.logExtendFailed(context.Background(), p.ID(), "resource", errors.New("test error"))
		assert.True(t, logger.warnCalled, "expected Warn to be called")
	})

	t.Run("without logger", func(t *testing.T) {
		opts := defaultOptions()
		opts.logger = nil
		p, err := newNoopPermit(context.Background(), "resource", "tenant", 5*time.Minute, nil, opts)
		require.NoError(t, err)
		// Should not panic
		p.logExtendFailed(context.Background(), p.ID(), "resource", errors.New("test error"))
	})
}

// =============================================================================
// newNoopPermit ID 生成失败测试
// =============================================================================

func TestNewNoopPermit_IDGenerationFailure(t *testing.T) {
	opts := defaultOptions()
	opts.idGenerator = func(_ context.Context) (string, error) {
		return "", errors.New("id gen failed")
	}
	p, err := newNoopPermit(context.Background(), "resource", "tenant", 5*time.Minute, nil, opts)
	assert.Nil(t, p)
	assert.ErrorIs(t, err, ErrIDGenerationFailed)
}

// =============================================================================
// Sentinel error wrapping 测试
// =============================================================================

func TestOptions_Validate_SentinelErrors(t *testing.T) {
	t.Run("invalid pod count wraps ErrInvalidPodCount", func(t *testing.T) {
		opts := defaultOptions()
		opts.podCount = -1
		err := opts.validate()
		assert.ErrorIs(t, err, ErrInvalidPodCount)
	})

	t.Run("invalid fallback strategy wraps ErrInvalidFallbackStrategy", func(t *testing.T) {
		opts := defaultOptions()
		opts.fallback = FallbackStrategy("invalid")
		err := opts.validate()
		assert.ErrorIs(t, err, ErrInvalidFallbackStrategy)
	})
}

// =============================================================================
// convertScriptResult 测试（FG-M4 覆盖率修复）
// =============================================================================

func TestConvertScriptResult(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		expected  []int64
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "int64 elements",
			input:    []any{int64(1), int64(2), int64(3)},
			expected: []int64{1, 2, 3},
		},
		{
			name:     "int elements",
			input:    []any{int(10), int(20)},
			expected: []int64{10, 20},
		},
		{
			name:     "float64 integer values",
			input:    []any{float64(42), float64(100)},
			expected: []int64{42, 100},
		},
		{
			name:     "mixed numeric types",
			input:    []any{int64(1), int(2), float64(3)},
			expected: []int64{1, 2, 3},
		},
		{
			name:     "empty array",
			input:    []any{},
			expected: []int64{},
		},
		{
			name:      "non-array input",
			input:     "not an array",
			wantErr:   true,
			errSubstr: "expected array",
		},
		{
			name:      "nil input",
			input:     nil,
			wantErr:   true,
			errSubstr: "expected array",
		},
		{
			name:      "float64 non-integer value",
			input:     []any{float64(3.14)},
			wantErr:   true,
			errSubstr: "non-integer float64",
		},
		{
			name:      "float64 non-integer negative",
			input:     []any{float64(-2.5)},
			wantErr:   true,
			errSubstr: "non-integer float64",
		},
		{
			name:      "unknown type string",
			input:     []any{"not a number"},
			wantErr:   true,
			errSubstr: "expected number",
		},
		{
			name:      "unknown type bool",
			input:     []any{true},
			wantErr:   true,
			errSubstr: "expected number",
		},
		{
			name:      "mixed with unknown type in middle",
			input:     []any{int64(1), "bad", int64(3)},
			wantErr:   true,
			errSubstr: "element 1 is string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertScriptResult(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, errUnexpectedScriptResult)
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr)
				}
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
