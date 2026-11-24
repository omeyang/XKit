package xlimit

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/omeyang/xkit/pkg/observability/xlog"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric"
)

// =============================================================================
// LimitError 方法覆盖
// =============================================================================

func TestLimitError_Error(t *testing.T) {
	t.Run("with reason", func(t *testing.T) {
		e := &LimitError{
			Key:       Key{Tenant: "t1"},
			Rule:      "rule1",
			Limit:     100,
			Remaining: 0,
			Reason:    "quota exhausted",
		}
		msg := e.Error()
		assert.Contains(t, msg, "rule1")
		assert.Contains(t, msg, "quota exhausted")
		assert.Contains(t, msg, "t1")
	})

	t.Run("without reason", func(t *testing.T) {
		e := &LimitError{
			Key:       Key{Tenant: "t2"},
			Rule:      "rule2",
			Limit:     50,
			Remaining: 10,
		}
		msg := e.Error()
		assert.Contains(t, msg, "rule2")
		assert.NotContains(t, msg, "reason=")
	})
}

func TestLimitError_Unwrap(t *testing.T) {
	e := &LimitError{Rule: "test"}
	assert.Equal(t, ErrRateLimited, e.Unwrap())
}

func TestLimitError_Retryable(t *testing.T) {
	e := &LimitError{Rule: "test"}
	assert.False(t, e.Retryable())
}

func TestLimitError_Is(t *testing.T) {
	e := &LimitError{Rule: "test"}
	assert.True(t, e.Is(ErrRateLimited))
	assert.False(t, e.Is(ErrRedisUnavailable))
}

// =============================================================================
// IsRetryable 覆盖
// =============================================================================

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"rate limited error", ErrRateLimited, false},
		{"LimitError", &LimitError{Rule: "test"}, false},
		{"Redis unavailable", ErrRedisUnavailable, true},
		{"connection refused", syscall.ECONNREFUSED, true},
		{"io.EOF", io.EOF, true},
		{"other error", errors.New("other"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsRetryable(tc.err))
		})
	}
}

// =============================================================================
// Query 覆盖（core.go + backend_local.go + fallback.go）
// =============================================================================

func TestLocalLimiter_Query(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 10, time.Minute)),
	)
	require.NoError(t, err)
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	key := Key{Tenant: "query-tenant"}

	// 消耗一些配额
	_, err = limiter.AllowN(ctx, key, 3)
	require.NoError(t, err)

	// 查询配额状态
	querier, ok := limiter.(Querier)
	require.True(t, ok, "limiter should implement Querier")

	info, err := querier.Query(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, 10, info.Limit)
	assert.GreaterOrEqual(t, info.Remaining, 0)
	assert.NotEmpty(t, info.Rule)
	assert.NotEmpty(t, info.Key)
}

func TestLocalLimiter_QueryClosed(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 10, time.Minute)),
	)
	require.NoError(t, err)
	require.NoError(t, limiter.Close(context.Background()))

	querier, ok := limiter.(Querier)
	require.True(t, ok)
	_, err = querier.Query(context.Background(), Key{Tenant: "t"})
	assert.ErrorIs(t, err, ErrLimiterClosed)
}

func TestLocalLimiter_QueryNoRuleMatched(t *testing.T) {
	limiter, err := NewLocal()
	require.NoError(t, err)
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup

	querier, ok := limiter.(Querier)
	require.True(t, ok)
	_, err = querier.Query(context.Background(), Key{Tenant: "t"})
	assert.ErrorIs(t, err, ErrNoRuleMatched)
}

func TestFallbackLimiter_Query(t *testing.T) {
	t.Run("query from distributed", func(t *testing.T) {
		distributed := &mockFailingLimiter{failOnAllow: false}
		local, err := NewLocal(WithRules(TenantRule("test", 10, time.Minute)))
		require.NoError(t, err)
		defer func() { _ = local.Close(context.Background()) }() //nolint:errcheck // defer cleanup

		fb := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackLocal}})
		info, err := fb.Query(context.Background(), Key{Tenant: "t"})
		require.NoError(t, err)
		assert.Equal(t, 100, info.Limit)
	})

	t.Run("fallback to local on Redis error", func(t *testing.T) {
		distributed := &mockFailingLimiter{
			failOnAllow: true,
			failErr:     syscall.ECONNREFUSED,
		}
		local, err := NewLocal(WithRules(TenantRule("test", 5, time.Minute)))
		require.NoError(t, err)
		defer func() { _ = local.Close(context.Background()) }() //nolint:errcheck // defer cleanup

		fb := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackLocal}})
		info, err := fb.Query(context.Background(), Key{Tenant: "t"})
		require.NoError(t, err)
		assert.Equal(t, 5, info.Limit)
	})

	t.Run("non-Redis error propagated", func(t *testing.T) {
		nonRedisErr := errors.New("non-redis error")
		distributed := &mockFailingLimiter{
			failOnAllow: true,
			failErr:     nonRedisErr,
		}
		local, err := NewLocal(WithRules(TenantRule("test", 5, time.Minute)))
		require.NoError(t, err)
		defer func() { _ = local.Close(context.Background()) }() //nolint:errcheck // defer cleanup

		fb := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackLocal}})
		_, err = fb.Query(context.Background(), Key{Tenant: "t"})
		assert.Equal(t, nonRedisErr, err)
	})

	t.Run("neither supports query", func(t *testing.T) {
		distributed := &simpleNoQueryLimiter{}
		local := &simpleNoQueryLimiter{}

		fb := newFallbackLimiter(distributed, local, &options{config: Config{Fallback: FallbackLocal}})
		_, err := fb.Query(context.Background(), Key{Tenant: "t"})
		assert.ErrorIs(t, err, ErrQueryNotSupported)
	})
}

// simpleNoQueryLimiter 没有 Query 方法的限流器
type simpleNoQueryLimiter struct{}

func (s *simpleNoQueryLimiter) Allow(ctx context.Context, key Key) (*Result, error) {
	return &Result{Allowed: true}, nil
}

func (s *simpleNoQueryLimiter) AllowN(_ context.Context, _ Key, _ int) (*Result, error) {
	return &Result{Allowed: true}, nil
}

func (s *simpleNoQueryLimiter) Close(_ context.Context) error { return nil }

// =============================================================================
// KeyFromContext 覆盖
// =============================================================================

func TestKeyFromContext(t *testing.T) {
	ctx := context.Background()
	key := KeyFromContext(ctx)
	assert.Empty(t, key.Tenant)
}

// =============================================================================
// NewWithFallback 覆盖
// =============================================================================

func TestNewWithFallback(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }() //nolint:errcheck // defer cleanup

	limiter, err := NewWithFallback(client,
		WithRules(TenantRule("tenant", 100, time.Minute)),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup

	_, isFallback := limiter.(*fallbackLimiter)
	assert.True(t, isFallback, "NewWithFallback should return a fallbackLimiter")

	ctx := context.Background()
	result, err := limiter.Allow(ctx, Key{Tenant: "t1"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

// =============================================================================
// 选项覆盖（WithLogger, WithObserver, WithOnFallback, WithMeterProvider）
// =============================================================================

func TestWithLogger_Option(t *testing.T) {
	opts := defaultOptions()
	WithLogger(nil)(opts)
	assert.Nil(t, opts.logger)
}

func TestWithObserver_Option(t *testing.T) {
	opts := defaultOptions()
	WithObserver(nil)(opts)
	assert.Nil(t, opts.observer)
}

func TestWithOnFallback_Option(t *testing.T) {
	called := false
	opts := defaultOptions()
	WithOnFallback(func(_ Key, _ FallbackStrategy, _ error) {
		called = true
	})(opts)
	require.NotNil(t, opts.onFallback)
	opts.onFallback(Key{}, FallbackLocal, nil)
	assert.True(t, called)
}

func TestWithMeterProvider_Option(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }() //nolint:errcheck // defer cleanup

	opts := defaultOptions()
	WithMeterProvider(provider)(opts)
	assert.NotNil(t, opts.meterProvider)
}

func TestWithCustomFallback_Option(t *testing.T) {
	opts := defaultOptions()
	WithCustomFallback(func(_ context.Context, _ Key, _ int, _ error) (*Result, error) {
		return &Result{Allowed: true}, nil
	})(opts)
	assert.NotNil(t, opts.customFallback)
}

func TestWithPodCountProvider_Option(t *testing.T) {
	opts := defaultOptions()
	WithPodCountProvider(StaticPodCount(3))(opts)
	assert.NotNil(t, opts.podCountProvider)
}

// =============================================================================
// RuleBuilder.Enabled 覆盖
// =============================================================================

func TestRuleBuilder_Enabled(t *testing.T) {
	rule := NewRuleBuilder("disabled-rule").
		KeyTemplate("tenant:${tenant_id}").
		Limit(100).
		Window(time.Minute).
		Enabled(false).
		Build()

	assert.False(t, rule.IsEnabled())
}

// =============================================================================
// Rule.Clone with Enabled 和 Overrides 覆盖
// =============================================================================

func TestRule_Clone_WithEnabledAndOverrides(t *testing.T) {
	enabled := true
	original := Rule{
		Name:        "test",
		KeyTemplate: "tenant:${tenant_id}",
		Limit:       100,
		Window:      time.Minute,
		Enabled:     &enabled,
		Overrides: []Override{
			{Match: "tenant:vip", Limit: 1000},
		},
	}

	clone := original.Clone()

	// 修改克隆不影响原始
	clone.Overrides[0].Limit = 9999
	assert.Equal(t, 1000, original.Overrides[0].Limit)

	// Enabled 也是深拷贝
	newVal := false
	clone.Enabled = &newVal
	assert.True(t, *original.Enabled)
}

// =============================================================================
// Rule.Validate 更多分支
// =============================================================================

func TestRule_Validate_MoreBranches(t *testing.T) {
	tests := []struct {
		name    string
		rule    Rule
		wantErr bool
	}{
		{
			name:    "missing key template",
			rule:    Rule{Name: "test", Limit: 100, Window: time.Second},
			wantErr: true,
		},
		{
			name:    "zero window",
			rule:    Rule{Name: "test", KeyTemplate: "key", Limit: 100},
			wantErr: true,
		},
		{
			name:    "negative limit",
			rule:    Rule{Name: "test", KeyTemplate: "key", Limit: -1, Window: time.Second},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rule.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// FindRule (public) 覆盖 — 仅覆盖 empty matcher 路径
// =============================================================================

func TestRuleMatcher_FindRule_EmptyMatcher(t *testing.T) {
	emptyMatcher := newRuleMatcher(nil)
	_, found := emptyMatcher.FindRule(Key{Tenant: "test"})
	assert.False(t, found)
}

func TestRuleMatcher_FindRule_NoMatchNoFallback(t *testing.T) {
	// 设计决策: resolveVar 对缺失的 Extra 变量返回空字符串（与内置字段一致），
	// 因此模板会被渲染为 "custom:"（变量替换为空），FindRule 判定为匹配。
	// 这与核心执行路径（findRule 按名称查找，不检查渲染）的行为一致。
	rules := []Rule{
		NewRule("custom-limit", "custom:${custom_var}", 100, time.Minute),
	}
	matcher := newRuleMatcher(rules)
	_, found := matcher.FindRule(Key{Tenant: "test"}) // 无 custom_var Extra → 渲染为 "custom:" → 匹配
	assert.True(t, found, "FindRule should match when Extra variable is missing (renders to empty string)")
}

// =============================================================================
// HTTPMiddlewareFunc 覆盖
// =============================================================================

func TestHTTPMiddlewareFunc(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 100, time.Minute)),
	)
	require.NoError(t, err)
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup

	handlerCalled := false
	handler := HTTPMiddlewareFunc(limiter)(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Tenant-ID", "test-tenant")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.True(t, handlerCalled)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// =============================================================================
// fallback AllowN 回调和指标路径覆盖
// =============================================================================

func TestFallbackLimiter_WithCallbacksAndMetrics(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }() //nolint:errcheck // defer cleanup

	m, err := NewMetrics(provider)
	require.NoError(t, err)

	var fallbackCalled bool

	distributed := &mockFailingLimiter{
		failOnAllow: true,
		failErr:     syscall.ECONNREFUSED,
	}
	local, err := NewLocal(WithRules(TenantRule("test", 10, time.Minute)))
	require.NoError(t, err)
	defer func() { _ = local.Close(context.Background()) }() //nolint:errcheck // defer cleanup

	opts := &options{
		config:  Config{Fallback: FallbackLocal},
		metrics: m,
		onFallback: func(_ Key, _ FallbackStrategy, _ error) {
			fallbackCalled = true
		},
	}
	fb := newFallbackLimiter(distributed, local, opts)

	_, err = fb.Allow(context.Background(), Key{Tenant: "t"})
	require.NoError(t, err)
	assert.True(t, fallbackCalled)
}

func TestFallbackLimiter_CustomFallback(t *testing.T) {
	distributed := &mockFailingLimiter{
		failOnAllow: true,
		failErr:     syscall.ECONNREFUSED,
	}
	local, err := NewLocal(WithRules(TenantRule("test", 10, time.Minute)))
	require.NoError(t, err)
	defer func() { _ = local.Close(context.Background()) }() //nolint:errcheck // defer cleanup

	opts := &options{
		config: Config{Fallback: FallbackLocal},
		customFallback: func(_ context.Context, _ Key, _ int, _ error) (*Result, error) {
			return &Result{Allowed: true, Rule: "custom-fallback"}, nil
		},
	}
	fb := newFallbackLimiter(distributed, local, opts)

	result, err := fb.Allow(context.Background(), Key{Tenant: "t"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, "custom-fallback", result.Rule)
}

func TestFallbackLimiter_CustomFallback_NilResultNilError(t *testing.T) {
	distributed := &mockFailingLimiter{
		failOnAllow: true,
		failErr:     syscall.ECONNREFUSED,
	}
	local, err := NewLocal(WithRules(TenantRule("test", 10, time.Minute)))
	require.NoError(t, err)
	defer func() { _ = local.Close(context.Background()) }() //nolint:errcheck // defer cleanup

	opts := &options{
		config: Config{Fallback: FallbackLocal},
		customFallback: func(_ context.Context, _ Key, _ int, _ error) (*Result, error) {
			return nil, nil //nolint:nilnil // 故意违反契约用于测试
		},
	}
	fb := newFallbackLimiter(distributed, local, opts)

	result, err := fb.Allow(context.Background(), Key{Tenant: "t"})
	require.NoError(t, err)
	require.NotNil(t, result, "nil result + nil error should be guarded to non-nil result")
	assert.True(t, result.Allowed, "should fail-open on nil result")
	assert.Equal(t, "custom-fallback", result.Rule)
}

// noopLogger 实现 xlog.Logger 接口用于测试
type noopLogger struct{}

func (n *noopLogger) Debug(_ context.Context, _ string, _ ...slog.Attr) {}
func (n *noopLogger) Info(_ context.Context, _ string, _ ...slog.Attr)  {}
func (n *noopLogger) Warn(_ context.Context, _ string, _ ...slog.Attr)  {}
func (n *noopLogger) Error(_ context.Context, _ string, _ ...slog.Attr) {}
func (n *noopLogger) Stack(_ context.Context, _ string, _ ...slog.Attr) {}
func (n *noopLogger) With(_ ...slog.Attr) xlog.Logger                   { return n }
func (n *noopLogger) WithGroup(_ string) xlog.Logger                    { return n }

func TestFallbackLimiter_LogFallbackWithLogger(t *testing.T) {
	distributed := &mockFailingLimiter{
		failOnAllow: true,
		failErr:     syscall.ECONNREFUSED,
	}
	local, err := NewLocal(WithRules(TenantRule("test", 10, time.Minute)))
	require.NoError(t, err)
	defer func() { _ = local.Close(context.Background()) }() //nolint:errcheck // defer cleanup

	opts := &options{
		config: Config{Fallback: FallbackLocal},
		logger: &noopLogger{},
	}
	fb := newFallbackLimiter(distributed, local, opts)

	_, err = fb.Allow(context.Background(), Key{Tenant: "t"})
	require.NoError(t, err)
}

// =============================================================================
// core.go callOnAllow/callOnDeny with logger 覆盖
// =============================================================================

func TestLimiterCore_WithLogger(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 1, time.Minute)),
		WithLogger(&noopLogger{}),
	)
	require.NoError(t, err)
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	key := Key{Tenant: "logger-tenant"}

	// 触发 callOnAllow (with logger)
	result, err := limiter.Allow(ctx, key)
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// 触发 callOnDeny (with logger)
	result, err = limiter.Allow(ctx, key)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
}
