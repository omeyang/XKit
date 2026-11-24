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
		defer c.Close(context.Background())

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
		defer c.Close(context.Background())
	})

	t.Run("with custom HTTP client", func(t *testing.T) {
		cfg := testConfig()
		customHTTP := &http.Client{Timeout: 30 * time.Second}

		c, err := NewClient(cfg, WithHTTPClient(customHTTP))
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer c.Close(context.Background())
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
		defer c.Close(context.Background())
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
		defer c.Close(context.Background())

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

		c.Close(context.Background())

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
		defer c.Close(context.Background())

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
		defer c.Close(context.Background())

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
		c.Close(context.Background())

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
		defer c.Close(context.Background())

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
		c.Close(context.Background())

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
		defer c.Close(context.Background())

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
		defer c.Close(context.Background())

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
		c.Close(context.Background())

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
		defer c.Close(context.Background())

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
		defer c.Close(context.Background())

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
		c.Close(context.Background())

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
		defer c.Close(context.Background())

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
		r.Body = http.MaxBytesReader(w, r.Body, testHandlerMaxBodyBytes)
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
		defer c.Close(context.Background())

		err = c.Request(ctx, nil)
		assert.Error(t, err, "expected error for nil request")
	})

	t.Run("client closed", func(t *testing.T) {
		cfg := testConfig()

		c, err := NewClient(cfg)
		require.NoError(t, err, "NewClient failed")
		c.Close(context.Background())

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
		defer c.Close(context.Background())

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

		err = c.Close(context.Background())
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

		_ = c.Close(context.Background())
		err = c.Close(context.Background()) // Should not error on second close
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
		defer c.Close(context.Background())

		err = c.InvalidateToken(ctx, "tenant-1")
		assert.NoError(t, err)
	})

	t.Run("client closed", func(t *testing.T) {
		cfg := testConfig()
		c, err := NewClient(cfg)
		require.NoError(t, err)
		c.Close(context.Background())

		err = c.InvalidateToken(ctx, "tenant-1")
		assert.Equal(t, ErrClientClosed, err)
	})

	t.Run("missing tenant ID", func(t *testing.T) {
		cfg := testConfig()
		c, err := NewClient(cfg)
		require.NoError(t, err)
		defer c.Close(context.Background())

		err = c.InvalidateToken(ctx, "")
		assert.Equal(t, ErrMissingTenantID, err)
	})
}
