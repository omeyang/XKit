package xauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Request_GetTokenError(t *testing.T) {
	ctx := context.Background()

	// Server that fails token request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		resp := map[string]any{"error": "unauthorized"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL

	c, err := NewClient(cfg, WithLocalCache(true))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Close(context.Background())

	err = c.Request(ctx, &AuthRequest{
		TenantID: "tenant-1",
		URL:      "/test",
		Method:   "GET",
	})
	if err == nil {
		t.Error("expected error when token fetch fails")
	}
}

func TestClient_Request_HTTPError(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, testHandlerMaxBodyBytes)
		// Token endpoint
		if r.FormValue("client_id") != "" {
			resp := map[string]any{
				"access_token": "test-token",
				"expires_in":   3600,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Custom endpoint - return error
		w.WriteHeader(http.StatusInternalServerError)
		resp := map[string]any{"error": "server error"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL

	c, err := NewClient(cfg, WithLocalCache(true))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Close(context.Background())

	err = c.Request(ctx, &AuthRequest{
		TenantID: "tenant-1",
		URL:      "/test",
		Method:   "GET",
	})
	if err == nil {
		t.Error("expected error when HTTP request fails")
	}
}

func TestNewClient_TLSConfigError(t *testing.T) {
	cfg := testConfig()
	cfg.TLS = &TLSConfig{
		RootCAFile: "/nonexistent/ca.crt", // This will cause an error
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Error("expected error for invalid TLS config")
	}
}

func TestClient_Request_NilHeaders(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, testHandlerMaxBodyBytes)
		// Token endpoint
		if r.FormValue("client_id") != "" {
			resp := map[string]any{
				"access_token": "test-token",
				"expires_in":   3600,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Custom endpoint - verify headers were created
		if r.Header.Get("Authorization") == "" {
			t.Error("Authorization header should be set")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL

	c, err := NewClient(cfg, WithLocalCache(true))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Close(context.Background())

	// Request with nil Headers - should create headers map
	err = c.Request(ctx, &AuthRequest{
		TenantID: "tenant-1",
		URL:      "/test",
		Method:   "GET",
		Headers:  nil, // Explicitly nil
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
}

func TestClient_Request_HeadersNotMutated(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, testHandlerMaxBodyBytes)
		if r.FormValue("client_id") != "" {
			resp := map[string]any{
				"access_token": "test-token",
				"expires_in":   3600,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL

	c, err := NewClient(cfg, WithLocalCache(true))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Close(context.Background())

	// 创建带自定义 Headers 的请求
	headers := map[string]string{"X-Custom": "value"}
	err = c.Request(ctx, &AuthRequest{
		TenantID: "tenant-1",
		URL:      "/test",
		Method:   "GET",
		Headers:  headers,
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// 验证调用方的 Headers 未被修改
	if _, exists := headers["Authorization"]; exists {
		t.Error("caller's Headers map should not be mutated with Authorization")
	}
	if len(headers) != 1 {
		t.Errorf("caller's Headers should still have 1 entry, got %d", len(headers))
	}
}

func TestDefaultTLSConfig(t *testing.T) {
	cfg := defaultTLSConfig()

	if cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be false by default (secure)")
	}
	if cfg.MinVersion != 0x0303 { // tls.VersionTLS12
		t.Errorf("MinVersion = %x, expected TLS 1.2", cfg.MinVersion)
	}
}

// handleRetryOn401 creates an HTTP handler that returns 401 on the first request
// and 200 on subsequent requests. It tracks request and token request counts.
func handleRetryOn401(requestCount, tokenRequestCount *int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, testHandlerMaxBodyBytes)
		// Token endpoint
		if r.FormValue("client_id") != "" {
			*tokenRequestCount++
			writeJSONToken(w, fmt.Sprintf("token-%d", *tokenRequestCount))
			return
		}

		// Custom endpoint
		*requestCount++
		if *requestCount == 1 {
			// First request: return 401
			w.WriteHeader(http.StatusUnauthorized)
			resp := map[string]any{"code": 401, "message": "unauthorized"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Second request: success
		w.WriteHeader(http.StatusOK)
		resp := map[string]string{"status": "ok"}
		json.NewEncoder(w).Encode(resp)
	}
}

// handleAlways401 creates an HTTP handler that always returns 401 for custom endpoints
// while serving tokens normally. It tracks request count.
func handleAlways401(requestCount *int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, testHandlerMaxBodyBytes)
		// Token endpoint
		if r.FormValue("client_id") != "" {
			writeJSONToken(w, "test-token")
			return
		}

		// Custom endpoint - always return 401
		*requestCount++
		w.WriteHeader(http.StatusUnauthorized)
		resp := map[string]any{"code": 401, "message": "unauthorized"}
		json.NewEncoder(w).Encode(resp)
	}
}

func TestClient_Request_AutoRetryOn401(t *testing.T) {
	ctx := context.Background()

	t.Run("retry on 401 when enabled", func(t *testing.T) {
		requestCount := 0
		tokenRequestCount := 0

		server := httptest.NewServer(handleRetryOn401(&requestCount, &tokenRequestCount))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL

		c, err := NewClient(cfg,
			WithLocalCache(true),
			WithAutoRetryOn401(true), // Enable auto-retry
		)
		require.NoError(t, err, "NewClient failed")
		defer c.Close(context.Background())

		var result map[string]string
		err = c.Request(ctx, &AuthRequest{
			TenantID: "tenant-1",
			URL:      "/test",
			Method:   "GET",
			Response: &result,
		})
		require.NoError(t, err, "Request should succeed on retry")

		// Should have made 2 requests to the endpoint
		assert.Equal(t, 2, requestCount, "expected initial + retry")

		// Should have fetched token twice (initial + after cache clear)
		assert.Equal(t, 2, tokenRequestCount)
	})

	t.Run("no retry on 401 when disabled", func(t *testing.T) {
		requestCount := 0

		server := httptest.NewServer(handleAlways401(&requestCount))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL

		c, err := NewClient(cfg,
			WithLocalCache(true),
			WithAutoRetryOn401(false), // Disable auto-retry (default)
		)
		require.NoError(t, err, "NewClient failed")
		defer c.Close(context.Background())

		err = c.Request(ctx, &AuthRequest{
			TenantID: "tenant-1",
			URL:      "/test",
			Method:   "GET",
		})
		assert.Error(t, err, "expected error on 401")

		// Should have made only 1 request
		assert.Equal(t, 1, requestCount, "no retry expected")
	})

	t.Run("no infinite retry on persistent 401", func(t *testing.T) {
		requestCount := 0

		server := httptest.NewServer(handleAlways401(&requestCount))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL

		c, err := NewClient(cfg,
			WithLocalCache(true),
			WithAutoRetryOn401(true), // Enable auto-retry
		)
		require.NoError(t, err, "NewClient failed")
		defer c.Close(context.Background())

		err = c.Request(ctx, &AuthRequest{
			TenantID: "tenant-1",
			URL:      "/test",
			Method:   "GET",
		})
		assert.Error(t, err, "expected error on persistent 401")

		// Should have made exactly 2 requests (initial + one retry)
		assert.Equal(t, 2, requestCount, "expected initial + one retry only")
	})
}

func TestClient_Request_AutoRetryOn401_IOReaderBody(t *testing.T) {
	ctx := context.Background()
	requestCount := 0
	var retryBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, testHandlerMaxBodyBytes)
		if r.FormValue("client_id") != "" {
			writeJSONToken(w, fmt.Sprintf("token-%d", requestCount))
			return
		}
		requestCount++
		body, _ := io.ReadAll(r.Body)
		if requestCount == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{"code": 401})
			return
		}
		retryBody = string(body)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"ok": "1"})
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	c, err := NewClient(cfg, WithAutoRetryOn401(true))
	require.NoError(t, err)
	defer c.Close(context.Background())

	var result map[string]string
	err = c.Request(ctx, &AuthRequest{
		TenantID: "t1",
		URL:      "/test",
		Method:   "POST",
		Body:     strings.NewReader(`{"key":"value"}`),
		Response: &result,
	})
	require.NoError(t, err)
	assert.Equal(t, `{"key":"value"}`, retryBody, "retry should replay buffered body")
}

func TestClient_Request_AutoRetryOn401_DoesNotMutateCallerBody(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, testHandlerMaxBodyBytes)
		if r.FormValue("client_id") != "" {
			writeJSONToken(w, "token-1")
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"ok": "1"})
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	c, err := NewClient(cfg, WithAutoRetryOn401(true))
	require.NoError(t, err)
	defer c.Close(context.Background())

	reader := strings.NewReader(`{"key":"value"}`)
	req := &AuthRequest{
		TenantID: "t1",
		URL:      "/test",
		Method:   "POST",
		Body:     reader,
	}
	originalBody := req.Body
	err = c.Request(ctx, req)
	require.NoError(t, err)
	assert.Same(t, originalBody, req.Body, "caller's AuthRequest.Body must not be mutated")
}

func TestClient_Request_RejectsInsecureAbsoluteURL(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSONToken(w, "test-token")
	}))
	defer server.Close()

	t.Run("http absolute URL rejected when AllowInsecure=false", func(t *testing.T) {
		cfg := testConfig()
		cfg.Host = server.URL
		// testConfig sets AllowInsecure=true for httptest. Create client, then
		// flip AllowInsecure to false to test request-level URL validation.
		c, err := NewClient(cfg)
		require.NoError(t, err)
		defer c.Close(context.Background())

		// Override AllowInsecure on the internal client to false after creation
		internal := c.(*client)
		internal.config.AllowInsecure = false

		err = c.Request(ctx, &AuthRequest{
			TenantID: "tenant-1",
			URL:      "http://evil.com/steal",
			Method:   "GET",
		})
		assert.ErrorIs(t, err, ErrInsecureHost)
	})

	t.Run("http absolute URL allowed when AllowInsecure=true", func(t *testing.T) {
		cfg := testConfig()
		cfg.Host = server.URL

		c, err := NewClient(cfg)
		require.NoError(t, err)
		defer c.Close(context.Background())

		// The request will fail because the URL points to a non-existent host,
		// but it should NOT be blocked by the insecure URL check.
		err = c.Request(ctx, &AuthRequest{
			TenantID: "tenant-1",
			URL:      "http://localhost:1/test",
			Method:   "GET",
		})
		assert.NotErrorIs(t, err, ErrInsecureHost)
	})

	t.Run("mixed case http scheme rejected", func(t *testing.T) {
		cfg := testConfig()
		cfg.Host = server.URL
		c, err := NewClient(cfg)
		require.NoError(t, err)
		defer c.Close(context.Background())

		internal := c.(*client)
		internal.config.AllowInsecure = false

		for _, scheme := range []string{"HTTP://", "Http://", "HtTp://"} {
			err = c.Request(ctx, &AuthRequest{
				TenantID: "tenant-1",
				URL:      scheme + "evil.com/steal",
				Method:   "GET",
			})
			assert.ErrorIs(t, err, ErrInsecureHost, "scheme %q should be rejected", scheme)
		}
	})

	t.Run("relative URL always allowed", func(t *testing.T) {
		cfg := testConfig()
		cfg.Host = server.URL
		cfg.AllowInsecure = true
		c, err := NewClient(cfg)
		require.NoError(t, err)
		defer c.Close(context.Background())

		internal := c.(*client)
		internal.config.AllowInsecure = false

		// Relative URL should pass the insecure check (even though the request may fail)
		err = c.Request(ctx, &AuthRequest{
			TenantID: "tenant-1",
			URL:      "/api/test",
			Method:   "GET",
		})
		assert.NotErrorIs(t, err, ErrInsecureHost)
	})
}

func TestClient_InvalidatePlatformCache(t *testing.T) {
	ctx := context.Background()

	t.Run("successful invalidation", func(t *testing.T) {
		cfg := testConfig()
		c, err := NewClient(cfg, WithLocalCache(true))
		require.NoError(t, err)
		defer c.Close(context.Background())

		err = c.InvalidatePlatformCache(ctx, "tenant-1")
		assert.NoError(t, err)
	})

	t.Run("client closed", func(t *testing.T) {
		cfg := testConfig()
		c, err := NewClient(cfg)
		require.NoError(t, err)
		c.Close(context.Background())

		err = c.InvalidatePlatformCache(ctx, "tenant-1")
		assert.Equal(t, ErrClientClosed, err)
	})

	t.Run("missing tenant ID", func(t *testing.T) {
		cfg := testConfig()
		c, err := NewClient(cfg)
		require.NoError(t, err)
		defer c.Close(context.Background())

		err = c.InvalidatePlatformCache(ctx, "")
		assert.Equal(t, ErrMissingTenantID, err)
	})
}

func TestIsUnauthorizedError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"ErrUnauthorized", ErrUnauthorized, true},
		{"APIError 401", NewAPIError(401, 0, "unauthorized"), true},
		{"APIError 403", NewAPIError(403, 0, "forbidden"), false},
		{"APIError 500", NewAPIError(500, 0, "server error"), false},
		{"other error", ErrTokenNotFound, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isUnauthorizedError(tt.err)
			if result != tt.expected {
				t.Errorf("isUnauthorizedError(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}
