package xlimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupTestLimiter(t *testing.T, limit int) Limiter {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	limiter, err := New(client,
		WithRules(TenantRule("tenant-limit", limit, time.Minute)),
		WithFallback(""),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	t.Cleanup(func() {
		_ = limiter.Close() //nolint:errcheck // cleanup
		_ = client.Close()  //nolint:errcheck // cleanup
		mr.Close()
	})

	return limiter
}

func TestHTTPMiddleware_Basic(t *testing.T) {
	limiter := setupTestLimiter(t, 10)
	middleware := HTTPMiddleware(limiter)

	// 创建测试处理器
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK")) //nolint:errcheck // test handler
	}))

	// 第一个请求应该通过
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "test-tenant")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	// 验证限流头
	if rr.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("expected X-RateLimit-Limit header")
	}
}

func TestHTTPMiddleware_RateLimited(t *testing.T) {
	limiter := setupTestLimiter(t, 2)
	middleware := HTTPMiddleware(limiter)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 消耗配额
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("X-Tenant-ID", "limited-tenant")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("request %d should pass, got %d", i+1, rr.Code)
		}
	}

	// 第三个请求应该被限流
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "limited-tenant")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", rr.Code)
	}

	// 验证 RetryAfter 头
	if rr.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header when rate limited")
	}
}

func TestHTTPMiddleware_CustomDenyHandler(t *testing.T) {
	limiter := setupTestLimiter(t, 1)

	customDenyHandler := func(w http.ResponseWriter, _ *http.Request, result *Result) {
		w.Header().Set("X-Custom-Header", "rate-limited")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("Custom: " + result.Rule)) //nolint:errcheck // test handler
	}

	middleware := HTTPMiddleware(limiter, WithDenyHandler(customDenyHandler))

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 消耗配额
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "custom-tenant")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// 第二个请求应该触发自定义拒绝处理器
	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "custom-tenant")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rr.Code)
	}
	if rr.Header().Get("X-Custom-Header") != "rate-limited" {
		t.Error("expected custom header")
	}
}

func TestHTTPMiddleware_SkipFunc(t *testing.T) {
	limiter := setupTestLimiter(t, 1)

	// 跳过健康检查端点
	skipFunc := func(r *http.Request) bool {
		return r.URL.Path == "/health"
	}

	middleware := HTTPMiddleware(limiter, WithSkipFunc(skipFunc))

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 健康检查应该不受限流影响
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("X-Tenant-ID", "skip-tenant")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("health check %d should pass, got %d", i+1, rr.Code)
		}
	}
}

func TestHTTPMiddleware_DisableHeaders(t *testing.T) {
	limiter := setupTestLimiter(t, 10)
	middleware := HTTPMiddleware(limiter, WithMiddlewareHeaders(false))

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "no-headers-tenant")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	// 不应该有限流头
	if rr.Header().Get("X-RateLimit-Limit") != "" {
		t.Error("should not have rate limit headers when disabled")
	}
}

func TestHTTPMiddleware_CustomKeyExtractor(t *testing.T) {
	limiter := setupTestLimiter(t, 10)

	customExtractor := NewHTTPKeyExtractor(
		WithTenantHeader("X-Custom-Tenant"),
	)

	middleware := HTTPMiddleware(limiter, WithMiddlewareKeyExtractor(customExtractor))

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Custom-Tenant", "custom-extracted-tenant")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestHTTPMiddleware_LocalLimiter(t *testing.T) {
	// 测试本地限流器与中间件配合
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 3, time.Minute)),
	)
	if err != nil {
		t.Fatalf("failed to create local limiter: %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	middleware := HTTPMiddleware(limiter)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 消耗配额
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("X-Tenant-ID", "local-tenant")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("request %d should pass, got %d", i+1, rr.Code)
		}
	}

	// 第四个请求应该被限流
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "local-tenant")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", rr.Code)
	}
}

func TestHTTPMiddleware_DifferentTenants(t *testing.T) {
	limiter := setupTestLimiter(t, 2)
	middleware := HTTPMiddleware(limiter)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 租户 A 消耗配额
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("X-Tenant-ID", "tenant-a")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("tenant-a request %d should pass, got %d", i+1, rr.Code)
		}
	}

	// 租户 A 被限流
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "tenant-a")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("tenant-a should be limited, got %d", rr.Code)
	}

	// 租户 B 应该不受影响
	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "tenant-b")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("tenant-b should pass, got %d", rr.Code)
	}
}

func TestHTTPMiddleware_FallbackCloseRejectsDeny(t *testing.T) {
	// FG-S1: FallbackClose 返回 Allowed=false + error 时，中间件应拒绝而非放行
	mockLimiter := &mockFallbackCloseLimiter{}
	middleware := HTTPMiddleware(mockLimiter)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "close-tenant")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("FallbackClose should result in 429, got %d", rr.Code)
	}
}

func TestHTTPMiddleware_ErrorWithNilResultFailsOpen(t *testing.T) {
	// 当 result 为 nil 且有 error 时，应该 fail-open
	mockLimiter := &mockNilResultErrorLimiter{}
	middleware := HTTPMiddleware(mockLimiter)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "error-tenant")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("nil result + error should fail-open with 200, got %d", rr.Code)
	}
}

func TestHTTPMiddleware_NilKeyExtractorFallback(t *testing.T) {
	// FG-M4: 传入 nil KeyExtractor 应回退到默认值而非 panic
	limiter := setupTestLimiter(t, 10)
	middleware := HTTPMiddleware(limiter, WithMiddlewareKeyExtractor(nil))

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "nil-extractor-tenant")
	rr := httptest.NewRecorder()

	// Should not panic
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestHTTPMiddleware_NilDenyHandlerFallback(t *testing.T) {
	limiter := setupTestLimiter(t, 1)
	middleware := HTTPMiddleware(limiter, WithDenyHandler(nil))

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request passes
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "nil-deny-tenant")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Second request should be denied with default handler (not panic)
	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "nil-deny-tenant")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rr.Code)
	}
}

// mockFallbackCloseLimiter 模拟 FallbackClose 行为：返回 Allowed=false + error
type mockFallbackCloseLimiter struct{}

func (m *mockFallbackCloseLimiter) Allow(_ context.Context, _ Key) (*Result, error) {
	return &Result{Allowed: false, Rule: "fallback-close"}, ErrRedisUnavailable
}

func (m *mockFallbackCloseLimiter) AllowN(_ context.Context, _ Key, _ int) (*Result, error) {
	return &Result{Allowed: false, Rule: "fallback-close"}, ErrRedisUnavailable
}

func (m *mockFallbackCloseLimiter) Close() error { return nil }

// mockNilResultErrorLimiter 模拟普通错误：返回 nil result + error
type mockNilResultErrorLimiter struct{}

func (m *mockNilResultErrorLimiter) Allow(_ context.Context, _ Key) (*Result, error) {
	return nil, ErrRedisUnavailable
}

func (m *mockNilResultErrorLimiter) AllowN(_ context.Context, _ Key, _ int) (*Result, error) {
	return nil, ErrRedisUnavailable
}

func (m *mockNilResultErrorLimiter) Close() error { return nil }

func TestHTTPMiddleware_NilLimiterPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil limiter")
		}
		msg, ok := r.(string)
		if !ok || msg != "xlimit: HTTPMiddleware requires a non-nil Limiter" {
			t.Errorf("unexpected panic message: %v", r)
		}
	}()
	HTTPMiddleware(nil)
}

func BenchmarkHTTPMiddleware(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	limiter, err := New(client,
		WithRules(TenantRule("tenant-limit", 1000000, time.Minute)),
		WithFallback(""),
	)
	if err != nil {
		b.Fatalf("failed to create limiter: %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	middleware := HTTPMiddleware(limiter)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "bench-tenant")

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
		}
	})
}
