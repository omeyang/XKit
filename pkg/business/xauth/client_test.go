package xauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		_, err := NewClient(nil)
		if err != ErrNilConfig {
			t.Errorf("expected ErrNilConfig, got %v", err)
		}
	})

	t.Run("invalid config", func(t *testing.T) {
		_, err := NewClient(&Config{})
		if err == nil {
			t.Error("expected error for invalid config")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		cfg := testConfig()
		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()

		if c == nil {
			t.Error("client should not be nil")
		}
	})

	t.Run("with options", func(t *testing.T) {
		cfg := testConfig()
		mockCache := newMockCacheStore()

		c, err := NewClient(cfg,
			WithCache(mockCache),
			WithLocalCache(true),
			WithSingleflight(true),
			WithBackgroundRefresh(true),
			WithTokenRefreshThreshold(10*time.Minute),
			WithPlatformDataCacheTTL(30*time.Minute),
		)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()
	})

	t.Run("with custom HTTP client", func(t *testing.T) {
		cfg := testConfig()
		customHTTP := &http.Client{Timeout: 30 * time.Second}

		c, err := NewClient(cfg, WithHTTPClient(customHTTP))
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()
	})

	t.Run("with TLS config", func(t *testing.T) {
		cfg := testConfig()
		cfg.TLS = &TLSConfig{
			InsecureSkipVerify: true,
		}

		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()
	})
}

func TestClient_GetToken(t *testing.T) {
	ctx := context.Background()

	t.Run("successful get token", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]any{
				"access_token": "test-token-123",
				"expires_in":   3600,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL

		c, err := NewClient(cfg, WithLocalCache(true))
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()

		token, err := c.GetToken(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("GetToken failed: %v", err)
		}
		if token != "test-token-123" {
			t.Errorf("token = %q, expected 'test-token-123'", token)
		}
	})

	t.Run("client closed", func(t *testing.T) {
		cfg := testConfig()

		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		c.Close()

		_, err = c.GetToken(ctx, "tenant-1")
		if err != ErrClientClosed {
			t.Errorf("expected ErrClientClosed, got %v", err)
		}
	})

	t.Run("missing tenant ID", func(t *testing.T) {
		cfg := testConfig()

		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()

		_, err = c.GetToken(ctx, "")
		if err != ErrMissingTenantID {
			t.Errorf("expected ErrMissingTenantID, got %v", err)
		}
	})
}

func TestClient_VerifyToken(t *testing.T) {
	ctx := context.Background()

	t.Run("valid token", func(t *testing.T) {
		expTime := time.Now().Add(time.Hour).Unix()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := VerifyResponse{
				Data: VerifyData{
					Active: true,
					Exp:    expTime,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL

		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()

		info, err := c.VerifyToken(ctx, "test-token")
		if err != nil {
			t.Fatalf("VerifyToken failed: %v", err)
		}
		if info.AccessToken != "test-token" {
			t.Errorf("AccessToken = %q, expected 'test-token'", info.AccessToken)
		}
	})

	t.Run("client closed", func(t *testing.T) {
		cfg := testConfig()

		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		c.Close()

		_, err = c.VerifyToken(ctx, "test-token")
		if err != ErrClientClosed {
			t.Errorf("expected ErrClientClosed, got %v", err)
		}
	})
}

func TestClient_GetPlatformID(t *testing.T) {
	ctx := context.Background()

	t.Run("successful", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == PathPlatformSelf {
				resp := PlatformSelfResponse{
					Data: struct {
						ID string `json:"id"`
					}{ID: "platform-456"},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			// Token endpoint
			resp := map[string]any{
				"access_token": "test-token",
				"expires_in":   3600,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL

		c, err := NewClient(cfg, WithLocalCache(true))
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()

		id, err := c.GetPlatformID(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("GetPlatformID failed: %v", err)
		}
		if id != "platform-456" {
			t.Errorf("id = %q, expected 'platform-456'", id)
		}
	})

	t.Run("client closed", func(t *testing.T) {
		cfg := testConfig()

		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		c.Close()

		_, err = c.GetPlatformID(ctx, "tenant-1")
		if err != ErrClientClosed {
			t.Errorf("expected ErrClientClosed, got %v", err)
		}
	})

	t.Run("missing tenant ID", func(t *testing.T) {
		cfg := testConfig()

		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()

		_, err = c.GetPlatformID(ctx, "")
		if err != ErrMissingTenantID {
			t.Errorf("expected ErrMissingTenantID, got %v", err)
		}
	})
}

func TestClient_HasParentPlatform(t *testing.T) {
	ctx := context.Background()

	t.Run("has parent", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == PathHasParent {
				resp := HasParentResponse{Data: true}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			// Token endpoint
			resp := map[string]any{
				"access_token": "test-token",
				"expires_in":   3600,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL

		c, err := NewClient(cfg, WithLocalCache(true))
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()

		hasParent, err := c.HasParentPlatform(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("HasParentPlatform failed: %v", err)
		}
		if !hasParent {
			t.Error("expected hasParent to be true")
		}
	})

	t.Run("client closed", func(t *testing.T) {
		cfg := testConfig()

		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		c.Close()

		_, err = c.HasParentPlatform(ctx, "tenant-1")
		if err != ErrClientClosed {
			t.Errorf("expected ErrClientClosed, got %v", err)
		}
	})

	t.Run("missing tenant ID", func(t *testing.T) {
		cfg := testConfig()

		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()

		_, err = c.HasParentPlatform(ctx, "")
		if err != ErrMissingTenantID {
			t.Errorf("expected ErrMissingTenantID, got %v", err)
		}
	})
}

func TestClient_GetUnclassRegionID(t *testing.T) {
	ctx := context.Background()

	t.Run("successful", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == PathUnclassRegion {
				resp := UnclassRegionResponse{
					Data: struct {
						ID string `json:"id"`
					}{ID: "region-789"},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			// Token endpoint
			resp := map[string]any{
				"access_token": "test-token",
				"expires_in":   3600,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL

		c, err := NewClient(cfg, WithLocalCache(true))
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()

		id, err := c.GetUnclassRegionID(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("GetUnclassRegionID failed: %v", err)
		}
		if id != "region-789" {
			t.Errorf("id = %q, expected 'region-789'", id)
		}
	})

	t.Run("client closed", func(t *testing.T) {
		cfg := testConfig()

		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		c.Close()

		_, err = c.GetUnclassRegionID(ctx, "tenant-1")
		if err != ErrClientClosed {
			t.Errorf("expected ErrClientClosed, got %v", err)
		}
	})

	t.Run("missing tenant ID", func(t *testing.T) {
		cfg := testConfig()

		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()

		_, err = c.GetUnclassRegionID(ctx, "")
		if err != ErrMissingTenantID {
			t.Errorf("expected ErrMissingTenantID, got %v", err)
		}
	})
}

// handleTokenAndCustomEndpoint is a named HTTP handler that serves token requests
// and a custom /test endpoint with auth verification.
// Extracting this from the test function closure reduces TestClient_Request's CC.
func handleTokenAndCustomEndpoint(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		// Token endpoint (client_id is in query params for token requests)
		if r.FormValue("client_id") != "" {
			writeJSONToken(w, "test-token")
			return
		}

		// Custom endpoint - should have Authorization header
		if r.URL.Path == "/test" {
			auth := r.Header.Get("Authorization")
			assert.Equal(t, "Bearer test-token", auth, "Authorization header mismatch")
			resp := map[string]string{"status": "ok"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

// writeJSONToken writes a standard token JSON response with a 1-hour expiry.
func writeJSONToken(w http.ResponseWriter, accessToken string) {
	resp := map[string]any{
		"access_token": accessToken,
		"expires_in":   3600,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func TestClient_Request(t *testing.T) {
	ctx := context.Background()

	t.Run("nil request", func(t *testing.T) {
		cfg := testConfig()

		c, err := NewClient(cfg)
		require.NoError(t, err, "NewClient failed")
		defer c.Close()

		err = c.Request(ctx, nil)
		assert.Error(t, err, "expected error for nil request")
	})

	t.Run("client closed", func(t *testing.T) {
		cfg := testConfig()

		c, err := NewClient(cfg)
		require.NoError(t, err, "NewClient failed")
		c.Close()

		err = c.Request(ctx, &AuthRequest{URL: "/test"})
		assert.Equal(t, ErrClientClosed, err)
	})

	t.Run("successful request", func(t *testing.T) {
		server := httptest.NewServer(handleTokenAndCustomEndpoint(t))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL

		c, err := NewClient(cfg, WithLocalCache(true))
		require.NoError(t, err, "NewClient failed")
		defer c.Close()

		var result map[string]string
		err = c.Request(ctx, &AuthRequest{
			TenantID: "tenant-1",
			URL:      "/test",
			Method:   "GET",
			Response: &result,
		})
		require.NoError(t, err, "Request failed")
	})
}

func TestClient_Close(t *testing.T) {
	t.Run("close once", func(t *testing.T) {
		cfg := testConfig()

		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		err = c.Close()
		if err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	t.Run("close twice", func(t *testing.T) {
		cfg := testConfig()

		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		_ = c.Close()
		err = c.Close() // Should not error on second close
		if err != nil {
			t.Errorf("Second Close should not error: %v", err)
		}
	})
}

func TestClient_InvalidateToken(t *testing.T) {
	ctx := context.Background()

	t.Run("successful invalidation", func(t *testing.T) {
		cfg := testConfig()
		c, err := NewClient(cfg, WithLocalCache(true))
		require.NoError(t, err)
		defer c.Close()

		err = c.InvalidateToken(ctx, "tenant-1")
		assert.NoError(t, err)
	})

	t.Run("client closed", func(t *testing.T) {
		cfg := testConfig()
		c, err := NewClient(cfg)
		require.NoError(t, err)
		c.Close()

		err = c.InvalidateToken(ctx, "tenant-1")
		assert.Equal(t, ErrClientClosed, err)
	})

	t.Run("missing tenant ID", func(t *testing.T) {
		cfg := testConfig()
		c, err := NewClient(cfg)
		require.NoError(t, err)
		defer c.Close()

		err = c.InvalidateToken(ctx, "")
		assert.Equal(t, ErrMissingTenantID, err)
	})
}

func TestMustNewClient(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cfg := testConfig()

		// Should not panic
		c := MustNewClient(cfg)
		defer c.Close()

		if c == nil {
			t.Error("client should not be nil")
		}
	})

	t.Run("panic on error", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for nil config")
			}
		}()

		MustNewClient(nil)
	})
}

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
	defer c.Close()

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
	defer c.Close()

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
	defer c.Close()

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
	defer c.Close()

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
		// Token endpoint
		if r.FormValue("client_id") != "" {
			*tokenRequestCount++
			writeJSONToken(w, "token-"+string(rune('0'+*tokenRequestCount)))
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
		defer c.Close()

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
		defer c.Close()

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
		defer c.Close()

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
		defer c.Close()

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
		defer c.Close()

		// The request will fail because the URL points to a non-existent host,
		// but it should NOT be blocked by the insecure URL check.
		err = c.Request(ctx, &AuthRequest{
			TenantID: "tenant-1",
			URL:      "http://localhost:1/test",
			Method:   "GET",
		})
		assert.NotErrorIs(t, err, ErrInsecureHost)
	})

	t.Run("relative URL always allowed", func(t *testing.T) {
		cfg := testConfig()
		cfg.Host = server.URL
		cfg.AllowInsecure = true
		c, err := NewClient(cfg)
		require.NoError(t, err)
		defer c.Close()

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
		defer c.Close()

		err = c.InvalidatePlatformCache(ctx, "tenant-1")
		assert.NoError(t, err)
	})

	t.Run("client closed", func(t *testing.T) {
		cfg := testConfig()
		c, err := NewClient(cfg)
		require.NoError(t, err)
		c.Close()

		err = c.InvalidatePlatformCache(ctx, "tenant-1")
		assert.Equal(t, ErrClientClosed, err)
	})

	t.Run("missing tenant ID", func(t *testing.T) {
		cfg := testConfig()
		c, err := NewClient(cfg)
		require.NoError(t, err)
		defer c.Close()

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
