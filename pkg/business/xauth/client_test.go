package xauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestClient_Request(t *testing.T) {
	ctx := context.Background()

	t.Run("nil request", func(t *testing.T) {
		cfg := testConfig()

		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()

		err = c.Request(ctx, nil)
		if err == nil {
			t.Error("expected error for nil request")
		}
	})

	t.Run("client closed", func(t *testing.T) {
		cfg := testConfig()

		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		c.Close()

		err = c.Request(ctx, &AuthRequest{URL: "/test"})
		if err != ErrClientClosed {
			t.Errorf("expected ErrClientClosed, got %v", err)
		}
	})

	t.Run("successful request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Token endpoint (client_id is in query params for token requests)
			if r.URL.Query().Get("client_id") != "" {
				resp := map[string]any{
					"access_token": "test-token",
					"expires_in":   3600,
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}

			// Custom endpoint - should have Authorization header
			if r.URL.Path == "/test" {
				auth := r.Header.Get("Authorization")
				if auth == "" || auth != "Bearer test-token" {
					t.Errorf("Authorization header = %q, expected 'Bearer test-token'", auth)
				}
				resp := map[string]string{"status": "ok"}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}

			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL

		c, err := NewClient(cfg, WithLocalCache(true))
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()

		var result map[string]string
		err = c.Request(ctx, &AuthRequest{
			TenantID: "tenant-1",
			URL:      "/test",
			Method:   "GET",
			Response: &result,
		})
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
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
		if r.URL.Query().Get("client_id") != "" {
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
		if r.URL.Query().Get("client_id") != "" {
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

func TestDefaultTLSConfig(t *testing.T) {
	cfg := defaultTLSConfig()

	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true by default")
	}
	if cfg.MinVersion != 0x0303 { // tls.VersionTLS12
		t.Errorf("MinVersion = %x, expected TLS 1.2", cfg.MinVersion)
	}
}

func TestClient_Request_AutoRetryOn401(t *testing.T) {
	ctx := context.Background()

	t.Run("retry on 401 when enabled", func(t *testing.T) {
		requestCount := 0
		tokenRequestCount := 0

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Token endpoint
			if r.URL.Query().Get("client_id") != "" {
				tokenRequestCount++
				resp := map[string]any{
					"access_token": "token-" + string(rune('0'+tokenRequestCount)),
					"expires_in":   3600,
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}

			// Custom endpoint
			requestCount++
			if requestCount == 1 {
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
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL

		c, err := NewClient(cfg,
			WithLocalCache(true),
			WithAutoRetryOn401(true), // Enable auto-retry
		)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close()

		var result map[string]string
		err = c.Request(ctx, &AuthRequest{
			TenantID: "tenant-1",
			URL:      "/test",
			Method:   "GET",
			Response: &result,
		})
		if err != nil {
			t.Fatalf("Request should succeed on retry: %v", err)
		}

		// Should have made 2 requests to the endpoint
		if requestCount != 2 {
			t.Errorf("requestCount = %d, expected 2 (initial + retry)", requestCount)
		}

		// Should have fetched token twice (initial + after cache clear)
		if tokenRequestCount != 2 {
			t.Errorf("tokenRequestCount = %d, expected 2", tokenRequestCount)
		}
	})

	t.Run("no retry on 401 when disabled", func(t *testing.T) {
		requestCount := 0

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Token endpoint
			if r.URL.Query().Get("client_id") != "" {
				resp := map[string]any{
					"access_token": "test-token",
					"expires_in":   3600,
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}

			// Custom endpoint - always return 401
			requestCount++
			w.WriteHeader(http.StatusUnauthorized)
			resp := map[string]any{"code": 401, "message": "unauthorized"}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL

		c, err := NewClient(cfg,
			WithLocalCache(true),
			WithAutoRetryOn401(false), // Disable auto-retry (default)
		)
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
			t.Error("expected error on 401")
		}

		// Should have made only 1 request
		if requestCount != 1 {
			t.Errorf("requestCount = %d, expected 1 (no retry)", requestCount)
		}
	})

	t.Run("no infinite retry on persistent 401", func(t *testing.T) {
		requestCount := 0

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Token endpoint
			if r.URL.Query().Get("client_id") != "" {
				resp := map[string]any{
					"access_token": "test-token",
					"expires_in":   3600,
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}

			// Custom endpoint - always return 401
			requestCount++
			w.WriteHeader(http.StatusUnauthorized)
			resp := map[string]any{"code": 401, "message": "unauthorized"}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL

		c, err := NewClient(cfg,
			WithLocalCache(true),
			WithAutoRetryOn401(true), // Enable auto-retry
		)
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
			t.Error("expected error on persistent 401")
		}

		// Should have made exactly 2 requests (initial + one retry)
		if requestCount != 2 {
			t.Errorf("requestCount = %d, expected 2 (initial + one retry only)", requestCount)
		}
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
