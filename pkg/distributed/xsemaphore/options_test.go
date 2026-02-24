package xsemaphore

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/omeyang/xkit/pkg/observability/xlog"
)

// =============================================================================
// 工厂选项测试
// =============================================================================

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()

	assert.Equal(t, DefaultKeyPrefix, opts.keyPrefix)
	assert.Equal(t, DefaultPodCount, opts.podCount)
	assert.Nil(t, opts.logger)
	assert.Nil(t, opts.metrics)
	assert.Equal(t, FallbackNone, opts.fallback)
}

func TestWithKeyPrefix(t *testing.T) {
	t.Run("valid prefix", func(t *testing.T) {
		opts := defaultOptions()
		WithKeyPrefix("my:prefix:")(opts)
		assert.Equal(t, "my:prefix:", opts.keyPrefix)
	})

	t.Run("empty prefix keeps default", func(t *testing.T) {
		opts := defaultOptions()
		WithKeyPrefix("")(opts)
		assert.Equal(t, DefaultKeyPrefix, opts.keyPrefix)
	})

	t.Run("invalid prefix detected by validate", func(t *testing.T) {
		opts := defaultOptions()
		WithKeyPrefix("bad{prefix}")(opts)
		assert.Equal(t, "bad{prefix}", opts.keyPrefix) // setter 不拦截
		assert.Error(t, opts.validate())               // validate 检测到错误
	})
}

func TestWithLogger(t *testing.T) {
	opts := defaultOptions()

	logger, cleanup, err := xlog.New().SetOutput(io.Discard).Build()
	assert.NoError(t, err)
	defer cleanup()

	WithLogger(logger)(opts)

	assert.NotNil(t, opts.logger)
}

func TestWithFallback(t *testing.T) {
	tests := []struct {
		name     string
		strategy FallbackStrategy
		expected FallbackStrategy
	}{
		{"local", FallbackLocal, FallbackLocal},
		{"open", FallbackOpen, FallbackOpen},
		{"close", FallbackClose, FallbackClose},
		{"none", FallbackNone, FallbackNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultOptions()
			WithFallback(tt.strategy)(opts)
			assert.Equal(t, tt.expected, opts.fallback)
		})
	}

	t.Run("invalid detected by validate", func(t *testing.T) {
		opts := defaultOptions()
		WithFallback(FallbackStrategy("invalid"))(opts)
		assert.Equal(t, FallbackStrategy("invalid"), opts.fallback)
		assert.Error(t, opts.validate())
	})
}

func TestWithPodCount(t *testing.T) {
	t.Run("valid count", func(t *testing.T) {
		opts := defaultOptions()
		WithPodCount(5)(opts)
		assert.Equal(t, 5, opts.podCount)
		assert.NoError(t, opts.validate())
	})

	t.Run("large count", func(t *testing.T) {
		opts := defaultOptions()
		WithPodCount(100)(opts)
		assert.Equal(t, 100, opts.podCount)
		assert.NoError(t, opts.validate())
	})

	t.Run("zero detected by validate", func(t *testing.T) {
		opts := defaultOptions()
		WithPodCount(0)(opts)
		assert.Equal(t, 0, opts.podCount)
		assert.Error(t, opts.validate())
	})

	t.Run("negative detected by validate", func(t *testing.T) {
		opts := defaultOptions()
		WithPodCount(-1)(opts)
		assert.Equal(t, -1, opts.podCount)
		assert.Error(t, opts.validate())
	})
}

func TestWithOnFallback(t *testing.T) {
	opts := defaultOptions()

	called := false
	WithOnFallback(func(resource string, strategy FallbackStrategy, err error) {
		called = true
	})(opts)

	assert.NotNil(t, opts.onFallback)

	// 调用回调
	opts.onFallback("test", FallbackLocal, nil)
	assert.True(t, called)
}

func TestWithIDGenerator(t *testing.T) {
	t.Run("custom generator", func(t *testing.T) {
		called := false
		gen := IDGeneratorFunc(func(_ context.Context) (string, error) {
			called = true
			return "custom-id", nil
		})

		opts := defaultOptions()
		WithIDGenerator(gen)(opts)
		assert.NotNil(t, opts.idGenerator)

		id, err := opts.effectiveIDGenerator()(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "custom-id", id)
		assert.True(t, called)
	})

	t.Run("nil generator keeps default", func(t *testing.T) {
		opts := defaultOptions()
		WithIDGenerator(nil)(opts)
		assert.Nil(t, opts.idGenerator)

		// effectiveIDGenerator 应返回 xid.NewStringWithRetry
		id, err := opts.effectiveIDGenerator()(context.Background())
		assert.NoError(t, err)
		assert.NotEmpty(t, id)
	})

	t.Run("error propagation", func(t *testing.T) {
		gen := IDGeneratorFunc(func(_ context.Context) (string, error) {
			return "", io.ErrUnexpectedEOF
		})

		opts := defaultOptions()
		WithIDGenerator(gen)(opts)

		_, err := opts.effectiveIDGenerator()(context.Background())
		assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
	})
}

func TestEffectivePodCount(t *testing.T) {
	t.Run("valid count", func(t *testing.T) {
		opts := &options{podCount: 5}
		assert.Equal(t, 5, opts.effectivePodCount())
	})

	t.Run("zero returns default", func(t *testing.T) {
		opts := &options{podCount: 0}
		assert.Equal(t, DefaultPodCount, opts.effectivePodCount())
	})

	t.Run("negative returns default", func(t *testing.T) {
		opts := &options{podCount: -1}
		assert.Equal(t, DefaultPodCount, opts.effectivePodCount())
	})
}

func TestLogExtendFailed(t *testing.T) {
	t.Run("localSemaphore with logger", func(t *testing.T) {
		logger, cleanup, err := xlog.New().SetOutput(io.Discard).Build()
		assert.NoError(t, err)
		defer cleanup()

		sem := &localSemaphore{opts: &options{logger: logger}}

		// 不应 panic
		sem.logExtendFailed(context.Background(), "permit-1", "resource-1", ErrPermitNotHeld)
	})

	t.Run("localSemaphore without logger", func(t *testing.T) {
		sem := &localSemaphore{opts: &options{}}

		// 不应 panic
		sem.logExtendFailed(context.Background(), "permit-1", "resource-1", ErrPermitNotHeld)
	})
}

// =============================================================================
// 获取选项测试
// =============================================================================

func TestAcquireOptions_Validate(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(*acquireOptions)
		wantError bool
	}{
		{
			name:      "valid defaults",
			modify:    func(o *acquireOptions) {},
			wantError: false,
		},
		{
			name:      "zero capacity",
			modify:    func(o *acquireOptions) { o.capacity = 0 },
			wantError: true,
		},
		{
			name:      "negative capacity",
			modify:    func(o *acquireOptions) { o.capacity = -1 },
			wantError: true,
		},
		{
			name:      "zero ttl",
			modify:    func(o *acquireOptions) { o.ttl = 0 },
			wantError: true,
		},
		{
			name:      "negative ttl",
			modify:    func(o *acquireOptions) { o.ttl = -1 },
			wantError: true,
		},
		{
			name:      "negative tenant quota",
			modify:    func(o *acquireOptions) { o.tenantQuota = -1 },
			wantError: true,
		},
		{
			name:      "tenant quota exceeds capacity",
			modify:    func(o *acquireOptions) { o.capacity = 10; o.tenantQuota = 20 },
			wantError: true,
		},
		{
			name:      "valid with tenant quota",
			modify:    func(o *acquireOptions) { o.capacity = 100; o.tenantQuota = 10 },
			wantError: false,
		},
		{
			name:      "zero tenant quota is valid",
			modify:    func(o *acquireOptions) { o.tenantQuota = 0 },
			wantError: false,
		},
		{
			name:      "zero max retries is valid for validate (checked by validateRetryParams)",
			modify:    func(o *acquireOptions) { o.maxRetries = 0 },
			wantError: false,
		},
		{
			name:      "zero retry delay is valid for validate (checked by validateRetryParams)",
			modify:    func(o *acquireOptions) { o.retryDelay = 0 },
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultAcquireOptions()
			tt.modify(opts)

			err := opts.validate()
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAcquireOptionFunctions(t *testing.T) {
	t.Run("WithCapacity", func(t *testing.T) {
		opts := defaultAcquireOptions()
		WithCapacity(50)(opts)
		assert.Equal(t, 50, opts.capacity)

		// 无效值直接设置，由 validate 检测
		WithCapacity(0)(opts)
		assert.Equal(t, 0, opts.capacity)
		assert.ErrorIs(t, opts.validate(), ErrInvalidCapacity)
	})

	t.Run("WithTenantID", func(t *testing.T) {
		opts := defaultAcquireOptions()
		WithTenantID("my-tenant")(opts)
		assert.Equal(t, "my-tenant", opts.tenantID)

		// 空值可以设置
		WithTenantID("")(opts)
		assert.Equal(t, "", opts.tenantID)
	})

	t.Run("WithTenantQuota", func(t *testing.T) {
		opts := defaultAcquireOptions()
		WithTenantQuota(10)(opts)
		assert.Equal(t, 10, opts.tenantQuota)

		// 负值直接设置，由 validate 检测
		WithTenantQuota(-1)(opts)
		assert.Equal(t, -1, opts.tenantQuota)
		assert.ErrorIs(t, opts.validate(), ErrInvalidTenantQuota)

		// 零值可以设置
		WithTenantQuota(0)(opts)
		assert.Equal(t, 0, opts.tenantQuota)
	})

	t.Run("WithTTL", func(t *testing.T) {
		opts := defaultAcquireOptions()
		WithTTL(10 * time.Second)(opts)
		assert.Equal(t, 10*time.Second, opts.ttl)

		// 无效值直接设置，由 validate 检测
		WithTTL(0)(opts)
		assert.Equal(t, time.Duration(0), opts.ttl)
		assert.ErrorIs(t, opts.validate(), ErrInvalidTTL)

		WithTTL(-1)(opts)
		assert.Equal(t, time.Duration(-1), opts.ttl)
		assert.ErrorIs(t, opts.validate(), ErrInvalidTTL)
	})

	t.Run("WithMaxRetries", func(t *testing.T) {
		opts := defaultAcquireOptions()
		WithMaxRetries(5)(opts)
		assert.Equal(t, 5, opts.maxRetries)

		// 无效值直接设置，由 validateRetryParams 检测（非 validate）
		WithMaxRetries(0)(opts)
		assert.Equal(t, 0, opts.maxRetries)
		assert.ErrorIs(t, opts.validateRetryParams(), ErrInvalidMaxRetries)

		WithMaxRetries(-1)(opts)
		assert.Equal(t, -1, opts.maxRetries)
		assert.ErrorIs(t, opts.validateRetryParams(), ErrInvalidMaxRetries)
	})

	t.Run("WithRetryDelay", func(t *testing.T) {
		opts := defaultAcquireOptions()
		WithRetryDelay(500 * time.Millisecond)(opts)
		assert.Equal(t, 500*time.Millisecond, opts.retryDelay)

		// 无效值直接设置，由 validateRetryParams 检测（非 validate）
		WithRetryDelay(0)(opts)
		assert.Equal(t, time.Duration(0), opts.retryDelay)
		assert.ErrorIs(t, opts.validateRetryParams(), ErrInvalidRetryDelay)

		WithRetryDelay(-1)(opts)
		assert.Equal(t, time.Duration(-1), opts.retryDelay)
		assert.ErrorIs(t, opts.validateRetryParams(), ErrInvalidRetryDelay)
	})
}

func TestAcquireOptions_ValidateRetryParams(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(*acquireOptions)
		wantError bool
		wantErr   error
	}{
		{
			name:      "valid defaults",
			modify:    func(o *acquireOptions) {},
			wantError: false,
		},
		{
			name:      "zero max retries",
			modify:    func(o *acquireOptions) { o.maxRetries = 0 },
			wantError: true,
			wantErr:   ErrInvalidMaxRetries,
		},
		{
			name:      "negative max retries",
			modify:    func(o *acquireOptions) { o.maxRetries = -1 },
			wantError: true,
			wantErr:   ErrInvalidMaxRetries,
		},
		{
			name:      "zero retry delay",
			modify:    func(o *acquireOptions) { o.retryDelay = 0 },
			wantError: true,
			wantErr:   ErrInvalidRetryDelay,
		},
		{
			name:      "negative retry delay",
			modify:    func(o *acquireOptions) { o.retryDelay = -1 },
			wantError: true,
			wantErr:   ErrInvalidRetryDelay,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultAcquireOptions()
			tt.modify(opts)
			err := opts.validateRetryParams()
			if tt.wantError {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// 查询选项测试
// =============================================================================

func TestDefaultQueryOptions(t *testing.T) {
	opts := defaultQueryOptions()

	assert.Equal(t, DefaultCapacity, opts.capacity)
	assert.Equal(t, "", opts.tenantID)
	assert.Equal(t, 0, opts.tenantQuota)
}

func TestQueryOptionFunctions(t *testing.T) {
	t.Run("QueryWithCapacity", func(t *testing.T) {
		opts := defaultQueryOptions()
		QueryWithCapacity(100)(opts)
		assert.Equal(t, 100, opts.capacity)

		// 零值设置后 validate 应失败（与 acquireOptions 一致）
		QueryWithCapacity(0)(opts)
		assert.Equal(t, 0, opts.capacity)
		assert.ErrorIs(t, opts.validate(), ErrInvalidCapacity)

		// 负值直接设置，由 validate 检测
		QueryWithCapacity(-1)(opts)
		assert.Equal(t, -1, opts.capacity)
		assert.ErrorIs(t, opts.validate(), ErrInvalidCapacity)
	})

	t.Run("QueryWithTenantID", func(t *testing.T) {
		opts := defaultQueryOptions()
		QueryWithTenantID("query-tenant")(opts)
		assert.Equal(t, "query-tenant", opts.tenantID)

		// 空值可以设置
		QueryWithTenantID("")(opts)
		assert.Equal(t, "", opts.tenantID)
	})

	t.Run("QueryWithTenantQuota", func(t *testing.T) {
		opts := defaultQueryOptions()
		QueryWithTenantQuota(10)(opts)
		assert.Equal(t, 10, opts.tenantQuota)

		// 零值可以设置
		QueryWithTenantQuota(0)(opts)
		assert.Equal(t, 0, opts.tenantQuota)
		assert.NoError(t, opts.validate())

		// 负值直接设置，由 validate 检测
		QueryWithTenantQuota(-1)(opts)
		assert.Equal(t, -1, opts.tenantQuota)
		assert.ErrorIs(t, opts.validate(), ErrInvalidTenantQuota)
	})
}

// =============================================================================
// 工厂选项验证测试
// =============================================================================

func TestOptionsValidate(t *testing.T) {
	t.Run("valid defaults", func(t *testing.T) {
		opts := defaultOptions()
		assert.NoError(t, opts.validate())
	})

	t.Run("invalid key prefix", func(t *testing.T) {
		opts := defaultOptions()
		opts.keyPrefix = "bad{prefix}"
		assert.ErrorIs(t, opts.validate(), ErrInvalidKeyPrefix)
	})

	t.Run("zero pod count", func(t *testing.T) {
		opts := defaultOptions()
		opts.podCount = 0
		assert.Error(t, opts.validate())
	})

	t.Run("negative pod count", func(t *testing.T) {
		opts := defaultOptions()
		opts.podCount = -1
		assert.Error(t, opts.validate())
	})

	t.Run("invalid fallback strategy", func(t *testing.T) {
		opts := defaultOptions()
		opts.fallback = FallbackStrategy("invalid")
		assert.Error(t, opts.validate())
	})

	t.Run("FallbackNone skips strategy validation", func(t *testing.T) {
		opts := defaultOptions()
		opts.fallback = FallbackNone
		assert.NoError(t, opts.validate())
	})

	t.Run("valid fallback strategy", func(t *testing.T) {
		opts := defaultOptions()
		opts.fallback = FallbackLocal
		assert.NoError(t, opts.validate())
	})
}

// =============================================================================
// 降级策略测试
// =============================================================================

func TestFallbackStrategy_IsValid_Comprehensive(t *testing.T) {
	validStrategies := []FallbackStrategy{
		FallbackNone,
		FallbackLocal,
		FallbackOpen,
		FallbackClose,
	}

	for _, s := range validStrategies {
		assert.True(t, s.IsValid(), "strategy %q should be valid", s)
	}

	invalidStrategies := []FallbackStrategy{
		"invalid",
		"LOCAL",
		"OPEN",
		"CLOSE",
		"random",
	}

	for _, s := range invalidStrategies {
		assert.False(t, s.IsValid(), "strategy %q should be invalid", s)
	}
}
