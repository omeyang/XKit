package xsemaphore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
