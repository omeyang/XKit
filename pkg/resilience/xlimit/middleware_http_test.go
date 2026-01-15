//nolint:errcheck // 测试文件中的错误处理简化
package xlimit

import (
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
		limiter.Close()
		client.Close()
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
		_, _ = w.Write([]byte("OK"))
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
		_, _ = w.Write([]byte("Custom: " + result.Rule))
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
	defer limiter.Close()

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
	defer limiter.Close()

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
