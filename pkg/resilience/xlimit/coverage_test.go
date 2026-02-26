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
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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

// =============================================================================
// New/NewLocal with metrics 覆盖
// =============================================================================

func TestNew_WithMetrics(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }() //nolint:errcheck // defer cleanup

	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }() //nolint:errcheck // defer cleanup

	limiter, err := New(client,
		WithRules(TenantRule("tenant", 100, time.Minute)),
		WithMetrics(true),
		WithMeterProvider(provider),
		WithFallback(""),
	)
	require.NoError(t, err)
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup

	_, err = limiter.Allow(context.Background(), Key{Tenant: "t1"})
	require.NoError(t, err)
}

func TestNewLocal_WithMetrics(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }() //nolint:errcheck // defer cleanup

	limiter, err := NewLocal(
		WithRules(TenantRule("tenant", 100, time.Minute)),
		WithMetrics(true),
		WithMeterProvider(provider),
	)
	require.NoError(t, err)
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup

	_, err = limiter.Allow(context.Background(), Key{Tenant: "t1"})
	require.NoError(t, err)
}

// =============================================================================
// WithConfigProvider 覆盖
// =============================================================================

func TestWithConfigProvider_NilProvider(t *testing.T) {
	opts := defaultOptions()
	WithConfigProvider(nil)(opts)
	assert.Equal(t, "ratelimit:", opts.config.KeyPrefix)
}

// =============================================================================
// backend_local CheckRule 上下文取消分支
// =============================================================================

func TestLocalBackend_CheckRule_CanceledContext(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 10, time.Minute)),
	)
	require.NoError(t, err)
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = limiter.Allow(ctx, Key{Tenant: "t"})
	assert.Error(t, err)
}

// =============================================================================
// backend_local getPodCount 动态获取路径
// =============================================================================

func TestLocalBackend_DynamicPodCount(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 100, time.Minute)),
		WithPodCountProvider(StaticPodCount(5)),
	)
	require.NoError(t, err)
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	result, err := limiter.Allow(ctx, Key{Tenant: "t"})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, 20, result.Limit) // 100/5 = 20
}

// =============================================================================
// gRPC 拦截器选项覆盖
// =============================================================================

func TestWithGRPCCallerHeader(t *testing.T) {
	ext := NewGRPCKeyExtractor(WithGRPCCallerHeader("X-Caller"))
	assert.Equal(t, "X-Caller", ext.callerHeader)
}

func TestWithGRPCResourceExtractor(t *testing.T) {
	called := false
	ext := NewGRPCKeyExtractor(WithGRPCResourceExtractor(
		func(_ context.Context, _ *grpc.UnaryServerInfo) string {
			called = true
			return "resource"
		},
	))
	require.NotNil(t, ext.resourceExtractor)
	result := ext.resourceExtractor(context.Background(), nil)
	assert.True(t, called)
	assert.Equal(t, "resource", result)
}

func TestWithGRPCStreamSkipFunc(t *testing.T) {
	opts := defaultGRPCInterceptorOptions()
	WithGRPCStreamSkipFunc(func(_ context.Context, _ *grpc.StreamServerInfo) bool {
		return true
	})(opts)
	assert.NotNil(t, opts.StreamSkipFunc)
}

// =============================================================================
// WithConfigProvider 有效 provider 路径覆盖
// =============================================================================

// mockConfigProvider 模拟配置提供器
type mockConfigProvider struct {
	config Config
	err    error
}

func (m *mockConfigProvider) Load() (Config, error) {
	return m.config, m.err
}

func (m *mockConfigProvider) Watch(_ context.Context) (<-chan ConfigChange, error) {
	return nil, nil
}

func TestWithConfigProvider_ValidProvider(t *testing.T) {
	provider := &mockConfigProvider{
		config: Config{
			KeyPrefix: "custom:",
			Rules:     []Rule{TenantRule("test", 100, time.Minute)},
		},
	}
	opts := defaultOptions()
	WithConfigProvider(provider)(opts)
	assert.Equal(t, "custom:", opts.config.KeyPrefix)
}

func TestWithConfigProvider_LoadError(t *testing.T) {
	provider := &mockConfigProvider{
		err: errors.New("load failed"),
	}
	opts := defaultOptions()
	WithConfigProvider(provider)(opts)
	// 加载失败应该保持默认配置
	assert.Equal(t, "ratelimit:", opts.config.KeyPrefix)
	// 但 initErr 应该被设置，阻止 New/NewLocal 创建限流器
	assert.Error(t, opts.initErr)
}

func TestWithConfigProvider_LoadError_PropagatedToNew(t *testing.T) {
	provider := &mockConfigProvider{
		err: errors.New("config center unavailable"),
	}
	_, err := NewLocal(WithConfigProvider(provider))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config provider load failed")
	assert.Contains(t, err.Error(), "config center unavailable")
}

// =============================================================================
// FindRule 更多分支覆盖
// =============================================================================

func TestRuleMatcher_FindRule_Rendered(t *testing.T) {
	rules := []Rule{
		TenantRule("tenant-limit", 100, time.Minute),
	}
	matcher := newRuleMatcher(rules)

	// 带有 tenant 的 key 应该匹配并渲染模板
	rule, found := matcher.FindRule(Key{Tenant: "test-tenant"})
	assert.True(t, found)
	assert.Equal(t, "tenant-limit", rule.Name)
}

// =============================================================================
// StreamServerInterceptor 更多路径覆盖
// =============================================================================

// =============================================================================
// Render 更多分支覆盖（unclosed variable）
// =============================================================================

func TestKey_Render_UnclosedVariable(t *testing.T) {
	key := Key{Tenant: "t1"}
	// 未关闭的变量应该原样保留
	result := key.Render("prefix:${tenant_id")
	assert.Equal(t, "prefix:${tenant_id", result)
}

// =============================================================================
// Config.Validate 更多分支（Rule.Validate）
// =============================================================================

func TestConfig_Validate_InvalidFallbackOnly(t *testing.T) {
	config := Config{
		Fallback: FallbackStrategy("bad"),
	}
	err := config.Validate()
	assert.Error(t, err)
}

// =============================================================================
// writeResponse 错误路径覆盖
// =============================================================================

func TestHTTPMiddleware_DenyResponse(t *testing.T) {
	// 创建一个 limit=0 的限流器，使所有请求被拒绝
	limiter, err := NewLocal(
		WithRules(Rule{
			Name:        "strict",
			KeyTemplate: "global",
			Limit:       1,
			Window:      time.Hour,
		}),
	)
	require.NoError(t, err)
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup

	middleware := HTTPMiddleware(limiter, WithMiddlewareHeaders(true))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 第一次请求通过
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	// 第二次请求被拒绝，触发 defaultDenyHandler → writeResponse
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
	assert.Equal(t, "Too Many Requests", rec2.Body.String())
}

// =============================================================================
// FG-S1: AllowN n<=0 参数校验
// =============================================================================

func TestAllowN_InvalidN(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 100, time.Minute)),
	)
	require.NoError(t, err)
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup

	ctx := context.Background()
	key := Key{Tenant: "test"}

	tests := []struct {
		name string
		n    int
	}{
		{"zero", 0},
		{"negative", -1},
		{"large negative", -100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := limiter.AllowN(ctx, key, tc.n)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidN)
		})
	}
}

// =============================================================================
// FG-M2: WithMiddlewareHeaders(false) 在拒绝路径不设置头
// =============================================================================

func TestHTTPMiddleware_DenyWithHeadersDisabled(t *testing.T) {
	limiter, err := NewLocal(
		WithRules(Rule{
			Name:        "strict",
			KeyTemplate: "global",
			Limit:       1,
			Window:      time.Hour,
		}),
	)
	require.NoError(t, err)
	defer func() { _ = limiter.Close(context.Background()) }() //nolint:errcheck // defer cleanup

	middleware := HTTPMiddleware(limiter, WithMiddlewareHeaders(false))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 第一次请求通过 — 不应有限流头
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)
	assert.Empty(t, rec1.Header().Get("X-RateLimit-Limit"))

	// 第二次请求被拒绝 — 也不应有限流头
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
	assert.Empty(t, rec2.Header().Get("X-RateLimit-Limit"),
		"deny path should not set rate limit headers when EnableHeaders=false")
}

// =============================================================================
// FG-M4: Retry-After 向上取整
// =============================================================================

func TestResult_Headers_RetryAfterCeiling(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter time.Duration
		want       string
	}{
		{"sub-second rounds up to 1", 500 * time.Millisecond, "1"},
		{"100ms rounds up to 1", 100 * time.Millisecond, "1"},
		{"exact second stays 1", time.Second, "1"},
		{"1.1 seconds rounds up to 2", 1100 * time.Millisecond, "2"},
		{"2 seconds stays 2", 2 * time.Second, "2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &Result{RetryAfter: tc.retryAfter}
			headers := r.Headers()
			assert.Equal(t, tc.want, headers["Retry-After"])
		})
	}
}

// =============================================================================
// FG-M5: classifyError 错误分类
// =============================================================================

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"redis unavailable", ErrRedisUnavailable, "redis_unavailable"},
		{"connection refused", syscall.ECONNREFUSED, "network_error"},
		{"connection reset", syscall.ECONNRESET, "network_error"},
		{"unknown error", errors.New("something else"), "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, classifyError(tc.err))
		})
	}
}

// =============================================================================
// FG-M2: 中间件 nil result 防御性检查
// =============================================================================

// mockNilResultLimiter 模拟违反契约的 Limiter：err==nil 且 result==nil
type mockNilResultLimiter struct{}

func (m *mockNilResultLimiter) Allow(_ context.Context, _ Key) (*Result, error) {
	return nil, nil //nolint:nilnil // 故意违反契约用于测试
}

func (m *mockNilResultLimiter) AllowN(_ context.Context, _ Key, _ int) (*Result, error) {
	return nil, nil //nolint:nilnil // 故意违反契约用于测试
}

func (m *mockNilResultLimiter) Close(_ context.Context) error { return nil }

func TestHTTPMiddleware_NilResultFailsOpen(t *testing.T) {
	// FG-M2: 第三方 Limiter 返回 nil result + nil error 时应 fail-open（不 panic）
	middleware := HTTPMiddleware(&mockNilResultLimiter{})

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "nil-result-tenant")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code,
		"nil result + nil error should fail-open with 200")
}

func TestGRPCUnaryInterceptor_NilResultFailsOpen(t *testing.T) {
	// FG-M2: gRPC 一元拦截器对 nil result + nil error 的防御
	interceptor := UnaryServerInterceptor(&mockNilResultLimiter{})

	md := metadata.Pairs("x-tenant-id", "nil-result-tenant")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(_ context.Context, _ any) (any, error) {
		return "response", nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	resp, err := interceptor(ctx, "request", info, handler)
	require.NoError(t, err)
	assert.Equal(t, "response", resp,
		"nil result + nil error should fail-open and call handler")
}

func TestGRPCStreamInterceptor_NilResultFailsOpen(t *testing.T) {
	// FG-M2: gRPC 流式拦截器对 nil result + nil error 的防御
	interceptor := StreamServerInterceptor(&mockNilResultLimiter{})

	md := metadata.Pairs("x-tenant-id", "nil-result-tenant")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handlerCalled := false
	handler := func(_ any, _ grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/StreamMethod"}
	mockStream := &mockServerStream{ctx: ctx}

	err := interceptor(nil, mockStream, info, handler)
	require.NoError(t, err)
	assert.True(t, handlerCalled,
		"nil result + nil error should fail-open and call handler")
}

// =============================================================================
// FG-L3: gRPC 拦截器覆盖率提升
// =============================================================================

func TestGRPCUnaryInterceptor_FallbackCloseDeny(t *testing.T) {
	// FallbackClose 返回 Allowed=false + ErrRedisUnavailable → ResourceExhausted
	interceptor := UnaryServerInterceptor(&mockFallbackCloseLimiter{})

	md := metadata.Pairs("x-tenant-id", "close-tenant")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(_ context.Context, _ any) (any, error) {
		t.Fatal("handler should not be called when denied")
		return nil, nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	_, err := interceptor(ctx, "request", info, handler)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestGRPCStreamInterceptor_FallbackCloseDeny(t *testing.T) {
	// FallbackClose 返回 Allowed=false + ErrRedisUnavailable → ResourceExhausted
	interceptor := StreamServerInterceptor(&mockFallbackCloseLimiter{})

	md := metadata.Pairs("x-tenant-id", "close-tenant")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(_ any, _ grpc.ServerStream) error {
		t.Fatal("handler should not be called when denied")
		return nil
	}
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/StreamMethod"}
	mockStream := &mockServerStream{ctx: ctx}

	err := interceptor(nil, mockStream, info, handler)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestGRPCStreamInterceptor_SkipFunc(t *testing.T) {
	limiter := setupGRPCTestLimiter(t, 1)

	skipFunc := func(_ context.Context, info *grpc.StreamServerInfo) bool {
		return info.FullMethod == "/grpc.health.v1.Health/Watch"
	}
	interceptor := StreamServerInterceptor(limiter,
		WithGRPCStreamSkipFunc(skipFunc),
	)

	md := metadata.Pairs("x-tenant-id", "skip-tenant")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(_ any, _ grpc.ServerStream) error { return nil }
	healthInfo := &grpc.StreamServerInfo{FullMethod: "/grpc.health.v1.Health/Watch"}
	mockStream := &mockServerStream{ctx: ctx}

	// 跳过函数匹配时不受限流影响
	for i := 0; i < 5; i++ {
		err := interceptor(nil, mockStream, healthInfo, handler)
		require.NoError(t, err, "health watch stream %d should pass", i+1)
	}
}

func TestGRPCStreamInterceptor_ErrorNilResultFailsOpen(t *testing.T) {
	// err != nil + result == nil → fail-open
	interceptor := StreamServerInterceptor(&mockNilResultErrorLimiter{})

	md := metadata.Pairs("x-tenant-id", "error-tenant")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handlerCalled := false
	handler := func(_ any, _ grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/StreamMethod"}
	mockStream := &mockServerStream{ctx: ctx}

	err := interceptor(nil, mockStream, info, handler)
	require.NoError(t, err)
	assert.True(t, handlerCalled, "error + nil result should fail-open")
}

// =============================================================================
// FG-S1: warnDefaultPodCount 路径覆盖
// =============================================================================

func TestWarnDefaultPodCount_Skips(t *testing.T) {
	// 非 FallbackLocal → 不告警
	warnDefaultPodCount(&options{
		config: Config{Fallback: FallbackOpen},
	})

	// 有 PodCountProvider → 不告警
	warnDefaultPodCount(&options{
		config:           Config{Fallback: FallbackLocal, LocalPodCount: 1},
		podCountProvider: StaticPodCount(3),
	})

	// LocalPodCount > 1 → 不告警
	warnDefaultPodCount(&options{
		config: Config{Fallback: FallbackLocal, LocalPodCount: 5},
	})
}

func TestWarnDefaultPodCount_Warns(t *testing.T) {
	// FallbackLocal + 默认 PodCount=1 + 无 Provider → 触发告警（不 panic 即可）
	warnDefaultPodCount(&options{
		config: Config{Fallback: FallbackLocal, LocalPodCount: 1},
	})
}
