package xauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewTokenManager(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		_, err := NewTokenManager(TokenManagerConfig{
			HTTP:  NewHTTPClient(HTTPClientConfig{BaseURL: "https://test.com"}),
			Cache: NewTokenCache(TokenCacheConfig{}),
		})
		if err != ErrNilConfig {
			t.Errorf("expected ErrNilConfig, got %v", err)
		}
	})

	t.Run("nil http", func(t *testing.T) {
		_, err := NewTokenManager(TokenManagerConfig{
			Config: testConfig(),
			Cache:  NewTokenCache(TokenCacheConfig{}),
		})
		if err != ErrNilHTTPClient {
			t.Errorf("expected ErrNilHTTPClient, got %v", err)
		}
	})

	t.Run("nil cache", func(t *testing.T) {
		_, err := NewTokenManager(TokenManagerConfig{
			Config: testConfig(),
			HTTP:   NewHTTPClient(HTTPClientConfig{BaseURL: "https://test.com"}),
		})
		if err != ErrNilCache {
			t.Errorf("expected ErrNilCache, got %v", err)
		}
	})

	t.Run("default values", func(t *testing.T) {
		cfg := testConfig()
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: cfg.Host})
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

		mgr := mustNewTokenManager(t, TokenManagerConfig{
			Config: cfg,
			HTTP:   httpClient,
			Cache:  cache,
		})

		if mgr.logger == nil {
			t.Error("logger should have default value")
		}
		if mgr.observer == nil {
			t.Error("observer should have default value")
		}
		if mgr.refreshThreshold != cfg.TokenRefreshThreshold {
			t.Errorf("refreshThreshold = %v, expected %v", mgr.refreshThreshold, cfg.TokenRefreshThreshold)
		}
	})

	t.Run("custom refresh threshold", func(t *testing.T) {
		cfg := testConfig()
		cfg.TokenRefreshThreshold = 0 // ensure default is applied
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: cfg.Host})
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

		customThreshold := 10 * time.Minute
		mgr := mustNewTokenManager(t, TokenManagerConfig{
			Config:           cfg,
			HTTP:             httpClient,
			Cache:            cache,
			RefreshThreshold: customThreshold,
		})

		if mgr.refreshThreshold != customThreshold {
			t.Errorf("refreshThreshold = %v, expected %v", mgr.refreshThreshold, customThreshold)
		}
	})

	t.Run("default refresh threshold when both config and option are zero", func(t *testing.T) {
		cfg := testConfig()
		cfg.TokenRefreshThreshold = 0 // Zero in config
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: cfg.Host})
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

		mgr := mustNewTokenManager(t, TokenManagerConfig{
			Config:           cfg,
			HTTP:             httpClient,
			Cache:            cache,
			RefreshThreshold: 0, // Also zero
		})

		if mgr.refreshThreshold != DefaultTokenRefreshThreshold {
			t.Errorf("refreshThreshold = %v, expected %v", mgr.refreshThreshold, DefaultTokenRefreshThreshold)
		}
	})
}

func TestTokenManager_GetToken(t *testing.T) {
	ctx := context.Background()

	t.Run("cache hit", func(t *testing.T) {
		cfg := testConfig()
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: cfg.Host})

		// Pre-populate cache
		expectedToken := testToken("cached-token", 3600)
		_ = cache.Set(ctx, "tenant-1", expectedToken, time.Hour)

		mgr := mustNewTokenManager(t, TokenManagerConfig{
			Config: cfg,
			HTTP:   httpClient,
			Cache:  cache,
		})

		token, err := mgr.GetToken(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("GetToken failed: %v", err)
		}
		if token != expectedToken.AccessToken {
			t.Errorf("token = %q, expected %q", token, expectedToken.AccessToken)
		}
	})

	t.Run("cache miss loads token", func(t *testing.T) {
		// Create mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]any{
				"access_token": "new-token",
				"token_type":   "bearer",
				"expires_in":   3600,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true, EnableSingleflight: true})
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		mgr := mustNewTokenManager(t, TokenManagerConfig{
			Config: cfg,
			HTTP:   httpClient,
			Cache:  cache,
		})

		token, err := mgr.GetToken(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("GetToken failed: %v", err)
		}
		if token != "new-token" {
			t.Errorf("token = %q, expected 'new-token'", token)
		}
	})
}

func TestTokenManager_ObtainClientToken(t *testing.T) {
	ctx := context.Background()

	t.Run("successful obtain", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request parameters
			if r.FormValue("client_id") == "" {
				t.Error("missing client_id")
			}
			if r.FormValue("grant_type") != "client_credentials" {
				t.Error("wrong grant_type")
			}

			resp := map[string]any{
				"access_token": "obtained-token",
				"token_type":   "bearer",
				"expires_in":   3600,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

		mgr := mustNewTokenManager(t, TokenManagerConfig{
			Config: cfg,
			HTTP:   httpClient,
			Cache:  cache,
		})

		token, err := mgr.obtainClientToken(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("obtainClientToken failed: %v", err)
		}
		if token.AccessToken != "obtained-token" {
			t.Errorf("AccessToken = %q, expected 'obtained-token'", token.AccessToken)
		}
		if token.ObtainedAt.IsZero() {
			t.Error("ObtainedAt should be set")
		}
		if token.ExpiresAt.IsZero() {
			t.Error("ExpiresAt should be set")
		}
	})

	t.Run("empty token response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]any{
				"access_token": "",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

		mgr := mustNewTokenManager(t, TokenManagerConfig{
			Config: cfg,
			HTTP:   httpClient,
			Cache:  cache,
		})

		_, err := mgr.obtainClientToken(ctx, "tenant-1")
		if err != ErrTokenNotFound {
			t.Errorf("expected ErrTokenNotFound, got %v", err)
		}
	})
}

func TestTokenManager_ObtainAPIKeyToken(t *testing.T) {
	ctx := context.Background()

	t.Run("missing api key", func(t *testing.T) {
		cfg := testConfig()
		cfg.APIKey = ""
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: cfg.Host})
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

		mgr := mustNewTokenManager(t, TokenManagerConfig{
			Config: cfg,
			HTTP:   httpClient,
			Cache:  cache,
		})

		_, err := mgr.obtainAPIKeyToken(ctx, "tenant-1")
		if err != ErrMissingAPIKey {
			t.Errorf("expected ErrMissingAPIKey, got %v", err)
		}
	})

	t.Run("successful api key token", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]any{
				"code":    0,
				"message": "success",
				"data": map[string]string{
					"access_token": "api-key-token",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL
		cfg.APIKey = "test-api-key"
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

		mgr := mustNewTokenManager(t, TokenManagerConfig{
			Config: cfg,
			HTTP:   httpClient,
			Cache:  cache,
		})

		token, err := mgr.obtainAPIKeyToken(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("obtainAPIKeyToken failed: %v", err)
		}
		if token.AccessToken != "api-key-token" {
			t.Errorf("AccessToken = %q, expected 'api-key-token'", token.AccessToken)
		}
	})
}

func TestTokenManager_VerifyToken(t *testing.T) {
	ctx := context.Background()

	t.Run("empty token", func(t *testing.T) {
		cfg := testConfig()
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: cfg.Host})
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

		mgr := mustNewTokenManager(t, TokenManagerConfig{
			Config: cfg,
			HTTP:   httpClient,
			Cache:  cache,
		})

		_, err := mgr.VerifyToken(ctx, "")
		if err != ErrMissingToken {
			t.Errorf("expected ErrMissingToken, got %v", err)
		}
	})

	t.Run("valid token", func(t *testing.T) {
		expTime := time.Now().Add(time.Hour).Unix()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := VerifyResponse{
				Data: VerifyData{
					Active:       true,
					Exp:          expTime,
					TenantID:     "tenant-abc",
					UserID:       "user-123",
					Authorities:  []string{"ROLE_ADMIN"},
					Scope:        []string{"read", "write"},
					IdentityType: "user",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

		mgr := mustNewTokenManager(t, TokenManagerConfig{
			Config: cfg,
			HTTP:   httpClient,
			Cache:  cache,
		})

		token, err := mgr.VerifyToken(ctx, "test-token")
		if err != nil {
			t.Fatalf("VerifyToken failed: %v", err)
		}
		if token.AccessToken != "test-token" {
			t.Errorf("AccessToken = %q, expected 'test-token'", token.AccessToken)
		}
		// 验证 Claims 已填充
		if token.Claims == nil {
			t.Fatal("Claims should not be nil after VerifyToken")
		}
		if token.Claims.TenantID != "tenant-abc" {
			t.Errorf("Claims.TenantID = %q, expected 'tenant-abc'", token.Claims.TenantID)
		}
		if token.Claims.UserID != "user-123" {
			t.Errorf("Claims.UserID = %q, expected 'user-123'", token.Claims.UserID)
		}
		if len(token.Claims.Authorities) != 1 || token.Claims.Authorities[0] != "ROLE_ADMIN" {
			t.Errorf("Claims.Authorities = %v, expected [ROLE_ADMIN]", token.Claims.Authorities)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := VerifyResponse{
				Data: VerifyData{
					Active: false,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

		mgr := mustNewTokenManager(t, TokenManagerConfig{
			Config: cfg,
			HTTP:   httpClient,
			Cache:  cache,
		})

		_, err := mgr.VerifyToken(ctx, "invalid-token")
		if err != ErrTokenInvalid {
			t.Errorf("expected ErrTokenInvalid, got %v", err)
		}
	})
}

func TestTokenManager_VerifyToken_HTTPError(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config: cfg,
		HTTP:   httpClient,
		Cache:  cache,
	})

	_, err := mgr.VerifyToken(ctx, "test-token")
	if err == nil {
		t.Error("expected error when HTTP request fails")
	}
}

func TestTokenManager_RefreshToken(t *testing.T) {
	ctx := context.Background()

	t.Run("refresh with refresh_token", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.FormValue("grant_type") == "refresh_token" {
				resp := map[string]any{
					"access_token":  "refreshed-token",
					"refresh_token": "new-refresh-token",
					"expires_in":    3600,
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

		mgr := mustNewTokenManager(t, TokenManagerConfig{
			Config: cfg,
			HTTP:   httpClient,
			Cache:  cache,
		})

		currentToken := testToken("current-token", 3600)
		newToken, err := mgr.RefreshToken(ctx, "tenant-1", currentToken)
		if err != nil {
			t.Fatalf("RefreshToken failed: %v", err)
		}
		if newToken.AccessToken != "refreshed-token" {
			t.Errorf("AccessToken = %q, expected 'refreshed-token'", newToken.AccessToken)
		}
	})

	t.Run("refresh without refresh_token obtains new", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]any{
				"access_token": "new-token",
				"expires_in":   3600,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.Host = server.URL
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

		mgr := mustNewTokenManager(t, TokenManagerConfig{
			Config: cfg,
			HTTP:   httpClient,
			Cache:  cache,
		})

		currentToken := &TokenInfo{
			AccessToken:  "current-token",
			RefreshToken: "", // no refresh token
		}
		newToken, err := mgr.RefreshToken(ctx, "tenant-1", currentToken)
		if err != nil {
			t.Fatalf("RefreshToken failed: %v", err)
		}
		if newToken.AccessToken != "new-token" {
			t.Errorf("AccessToken = %q, expected 'new-token'", newToken.AccessToken)
		}
	})
}

func TestTokenManager_InvalidateToken(t *testing.T) {
	ctx := context.Background()

	cfg := testConfig()
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: cfg.Host})
	cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

	// Pre-populate cache
	_ = cache.Set(ctx, "tenant-1", testToken("test-token", 3600), time.Hour)

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config: cfg,
		HTTP:   httpClient,
		Cache:  cache,
	})

	err := mgr.InvalidateToken(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("InvalidateToken failed: %v", err)
	}

	// Verify cache is cleared
	_, _, err = cache.Get(ctx, "tenant-1")
	if err != ErrCacheMiss {
		t.Errorf("expected ErrCacheMiss after invalidate, got %v", err)
	}
}

func TestTokenManager_CalculateTokenTTL(t *testing.T) {
	cfg := testConfig()
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: cfg.Host})
	cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config:           cfg,
		HTTP:             httpClient,
		Cache:            cache,
		RefreshThreshold: 5 * time.Minute,
	})

	t.Run("nil token", func(t *testing.T) {
		ttl := mgr.calculateTokenTTL(nil)
		if ttl != DefaultTokenCacheTTL {
			t.Errorf("TTL = %v, expected %v", ttl, DefaultTokenCacheTTL)
		}
	})

	t.Run("zero expires_in", func(t *testing.T) {
		token := &TokenInfo{ExpiresIn: 0}
		ttl := mgr.calculateTokenTTL(token)
		if ttl != DefaultTokenCacheTTL {
			t.Errorf("TTL = %v, expected %v", ttl, DefaultTokenCacheTTL)
		}
	})

	t.Run("normal expires_in", func(t *testing.T) {
		token := &TokenInfo{ExpiresIn: 3600} // 1 hour
		ttl := mgr.calculateTokenTTL(token)
		expected := time.Hour - 5*time.Minute
		if ttl != expected {
			t.Errorf("TTL = %v, expected %v", ttl, expected)
		}
	})

	t.Run("short expires_in", func(t *testing.T) {
		token := &TokenInfo{ExpiresIn: 60} // 1 minute, less than threshold
		ttl := mgr.calculateTokenTTL(token)
		expected := 30 * time.Second // half of expires_in
		if ttl != expected {
			t.Errorf("TTL = %v, expected %v", ttl, expected)
		}
	})
}

func TestTokenManager_ObtainToken_APIKeyFallback(t *testing.T) {
	ctx := context.Background()

	// Server that fails API key but succeeds client credentials
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Path == PathAPIAccessToken {
			// API key endpoint fails
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Client credentials endpoint succeeds
		resp := map[string]any{
			"access_token": "fallback-token",
			"expires_in":   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	cfg.APIKey = "test-api-key" // Will fail
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config: cfg,
		HTTP:   httpClient,
		Cache:  cache,
	})

	token, err := mgr.obtainToken(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("obtainToken failed: %v", err)
	}
	if token.AccessToken != "fallback-token" {
		t.Errorf("AccessToken = %q, expected 'fallback-token'", token.AccessToken)
	}
}

func TestTokenManager_RefreshWithRefreshToken_NilToken(t *testing.T) {
	ctx := context.Background()

	cfg := testConfig()
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: cfg.Host})
	cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config: cfg,
		HTTP:   httpClient,
		Cache:  cache,
	})

	_, err := mgr.refreshWithRefreshToken(ctx, "tenant-1", nil)
	if err != ErrRefreshTokenNotFound {
		t.Errorf("expected ErrRefreshTokenNotFound, got %v", err)
	}
}

func TestTokenManager_RefreshWithRefreshToken_EmptyRefreshToken(t *testing.T) {
	ctx := context.Background()

	cfg := testConfig()
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: cfg.Host})
	cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config: cfg,
		HTTP:   httpClient,
		Cache:  cache,
	})

	currentToken := &TokenInfo{
		AccessToken:  "current-token",
		RefreshToken: "", // Empty refresh token
	}
	_, err := mgr.refreshWithRefreshToken(ctx, "tenant-1", currentToken)
	if err != ErrRefreshTokenNotFound {
		t.Errorf("expected ErrRefreshTokenNotFound, got %v", err)
	}
}

func TestTokenManager_RefreshWithRefreshToken_HTTPError(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config: cfg,
		HTTP:   httpClient,
		Cache:  cache,
	})

	currentToken := &TokenInfo{
		AccessToken:  "current-token",
		RefreshToken: "refresh-token",
	}
	_, err := mgr.refreshWithRefreshToken(ctx, "tenant-1", currentToken)
	if err == nil {
		t.Error("expected error when HTTP request fails")
	}
}

func TestTokenManager_RefreshWithRefreshToken_EmptyTokenResponse(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"access_token": "", // Empty token
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config: cfg,
		HTTP:   httpClient,
		Cache:  cache,
	})

	currentToken := &TokenInfo{
		AccessToken:  "current-token",
		RefreshToken: "refresh-token",
	}
	_, err := mgr.refreshWithRefreshToken(ctx, "tenant-1", currentToken)
	if err != ErrTokenNotFound {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}
}

func TestTokenManager_RefreshToken_RefreshFails_FallsBackToObtain(t *testing.T) {
	ctx := context.Background()

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// First call is refresh, make it fail
		if r.FormValue("grant_type") == "refresh_token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Second call is obtain, make it succeed
		resp := map[string]any{
			"access_token": "new-token",
			"expires_in":   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config: cfg,
		HTTP:   httpClient,
		Cache:  cache,
	})

	currentToken := &TokenInfo{
		AccessToken:  "current-token",
		RefreshToken: "refresh-token",
	}
	newToken, err := mgr.RefreshToken(ctx, "tenant-1", currentToken)
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if newToken.AccessToken != "new-token" {
		t.Errorf("AccessToken = %q, expected 'new-token'", newToken.AccessToken)
	}
}

func TestTokenManager_GetToken_WithBackgroundRefresh(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"access_token": "refreshed-token",
			"expires_in":   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	cfg.Timeout = 5 * time.Second
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

	// Pre-populate with token that's expiring soon
	expiringToken := &TokenInfo{
		AccessToken:  "expiring-token",
		RefreshToken: "refresh-token",
		ExpiresIn:    120, // 2 minutes
		ExpiresAt:    time.Now().Add(2 * time.Minute),
		ObtainedAt:   time.Now(),
	}
	_ = cache.Set(ctx, "tenant-1", expiringToken, time.Hour)

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config:                  cfg,
		HTTP:                    httpClient,
		Cache:                   cache,
		RefreshThreshold:        5 * time.Minute, // Token is within threshold
		EnableBackgroundRefresh: true,
	})

	// Get token - should trigger background refresh
	token, err := mgr.GetToken(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}
	// Should return the current token immediately
	if token != "expiring-token" {
		t.Errorf("token = %q, expected 'expiring-token'", token)
	}

	// Wait a bit for background refresh to complete
	time.Sleep(100 * time.Millisecond)
}

func TestTokenManager_BackgroundRefresh_Canceled(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// 慢响应，模拟刷新中被取消
		time.Sleep(500 * time.Millisecond)
		resp := map[string]any{
			"access_token": "new-token",
			"expires_in":   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	cfg.Timeout = 5 * time.Second
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

	// 预填充即将过期的 Token
	expiringToken := &TokenInfo{
		AccessToken: "expiring",
		ExpiresIn:   30,
		ExpiresAt:   time.Now().Add(30 * time.Second),
		ObtainedAt:  time.Now(),
	}
	_ = cache.Set(ctx, "tenant-1", expiringToken, time.Hour)

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config:                  cfg,
		HTTP:                    httpClient,
		Cache:                   cache,
		RefreshThreshold:        5 * time.Minute,
		EnableBackgroundRefresh: true,
	})

	// 触发后台刷新
	_, err := mgr.GetToken(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}

	// 立即 Stop —— 取消后台刷新
	mgr.Stop()
	time.Sleep(50 * time.Millisecond)
}

func TestTokenManager_BackgroundRefresh_CacheGetFails(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"access_token": "token",
			"expires_in":   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	cfg.Timeout = 5 * time.Second
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

	// 使用 mockCacheStore 注入 Get 错误
	mockCache := newMockCacheStore()
	cache := NewTokenCache(TokenCacheConfig{
		Remote:             mockCache,
		EnableLocal:        true,
		EnableSingleflight: true,
	})

	// 预填充即将过期的 Token（在本地缓存中）
	expiringToken := &TokenInfo{
		AccessToken: "expiring",
		ExpiresIn:   30,
		ExpiresAt:   time.Now().Add(30 * time.Second),
		ObtainedAt:  time.Now(),
	}
	_ = cache.Set(ctx, "tenant-1", expiringToken, time.Hour)

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config:                  cfg,
		HTTP:                    httpClient,
		Cache:                   cache,
		RefreshThreshold:        5 * time.Minute,
		EnableBackgroundRefresh: true,
	})

	// 设置 mockCache 在后台刷新中 Get 出错
	mockCache.getTokenErr = errors.New("cache error")
	// 清除本地缓存中的 token，只保留过期的以触发刷新
	cache.local.Delete("tenant-1")
	// 重新填充本地缓存让 GetToken 能成功
	cache.local.Set("tenant-1", expiringToken)

	_, err := mgr.GetToken(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}

	// 等待后台刷新完成（会失败，因为 Get 出错后走到 Refresh → obtainToken）
	time.Sleep(200 * time.Millisecond)
	mgr.Stop()
}

func TestTokenManager_BackgroundRefresh_CancelBeforeStart(t *testing.T) {
	cfg := testConfig()
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: cfg.Host})
	cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config:                  cfg,
		HTTP:                    httpClient,
		Cache:                   cache,
		RefreshThreshold:        5 * time.Minute,
		EnableBackgroundRefresh: true,
	})

	// 先停止（取消 context），再调用 backgroundRefresh
	mgr.Stop()

	// 此时 ctx 已取消，backgroundRefresh 应立即返回
	mgr.wg.Add(1)
	go func() {
		defer mgr.wg.Done()
		mgr.backgroundRefresh("tenant-cancel")
	}()
	mgr.wg.Wait()
}

func TestTokenManager_BackgroundRefresh_RefreshError(t *testing.T) {
	// 服务器对刷新请求返回错误
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

	// 预填充即将过期的 Token（使 cache.Get 成功但后续 Refresh 失败）
	ctx := context.Background()
	expiringToken := &TokenInfo{
		AccessToken: "expiring",
		ExpiresIn:   30,
		ExpiresAt:   time.Now().Add(30 * time.Second),
		ObtainedAt:  time.Now(),
	}
	_ = cache.Set(ctx, "tenant-err", expiringToken, time.Hour)

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config:                  cfg,
		HTTP:                    httpClient,
		Cache:                   cache,
		RefreshThreshold:        5 * time.Minute,
		EnableBackgroundRefresh: true,
	})
	defer mgr.Stop()

	// 直接调用 backgroundRefresh（Get 成功，RefreshToken 失败）
	mgr.wg.Add(1)
	go func() {
		defer mgr.wg.Done()
		mgr.backgroundRefresh("tenant-err")
	}()
	mgr.wg.Wait()
}

func TestTokenManager_BackgroundRefresh_CacheSetError(t *testing.T) {
	// 服务器正常返回新 Token
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"access_token": "refreshed-token",
			"expires_in":   7200,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

	// 使用有错误的缓存：Set 会返回错误
	mockRemote := newMockCacheStore()
	mockRemote.setTokenErr = fmt.Errorf("cache write error")

	cache := NewTokenCache(TokenCacheConfig{
		Remote:      mockRemote,
		EnableLocal: true,
	})

	// 预填充即将过期的 Token
	ctx := context.Background()
	expiringToken := &TokenInfo{
		AccessToken: "expiring",
		ExpiresIn:   30,
		ExpiresAt:   time.Now().Add(30 * time.Second),
		ObtainedAt:  time.Now(),
	}
	cache.setLocal("tenant-cache-err", expiringToken)

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config:                  cfg,
		HTTP:                    httpClient,
		Cache:                   cache,
		RefreshThreshold:        5 * time.Minute,
		EnableBackgroundRefresh: true,
	})
	defer mgr.Stop()

	// 直接调用 backgroundRefresh（Get 成功，Refresh 成功，Set 失败 → 只是 warn log）
	mgr.wg.Add(1)
	go func() {
		defer mgr.wg.Done()
		mgr.backgroundRefresh("tenant-cache-err")
	}()
	mgr.wg.Wait()

	// 验证后台 goroutine 正常完成（无 panic）
	_ = ctx
}

func TestTokenManager_Stop_WaitsForGoroutines(t *testing.T) {
	ctx := context.Background()
	refreshStarted := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Token 获取：立即返回即将过期的 token
		if r.FormValue("client_id") != "" {
			resp := map[string]any{
				"access_token": "new-token",
				"expires_in":   3600,
			}
			// 在后台刷新中的第二次调用中延迟以验证 Stop 等待
			select {
			case <-refreshStarted:
				time.Sleep(200 * time.Millisecond)
			default:
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	cfg.Timeout = 5 * time.Second
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

	// 预填充即将过期的 Token
	expiringToken := &TokenInfo{
		AccessToken: "expiring",
		ExpiresIn:   30,
		ExpiresAt:   time.Now().Add(30 * time.Second),
		ObtainedAt:  time.Now(),
	}
	_ = cache.Set(ctx, "tenant-1", expiringToken, time.Hour)

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config:                  cfg,
		HTTP:                    httpClient,
		Cache:                   cache,
		RefreshThreshold:        5 * time.Minute,
		EnableBackgroundRefresh: true,
	})

	close(refreshStarted)

	// 触发后台刷新
	_, err := mgr.GetToken(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}

	// Stop 应等待后台 goroutine 完成而不 panic
	mgr.Stop()
}

func TestVerifyTokenForTenant(t *testing.T) {
	ctx := context.Background()

	t.Run("matching tenant", func(t *testing.T) {
		mc := newMockClient()
		mc.verifyData["good-token"] = &TokenInfo{
			AccessToken: "good-token",
			ExpiresAt:   time.Now().Add(time.Hour),
			Claims: &VerifyData{
				Active:   true,
				TenantID: "tenant-abc",
			},
		}

		info, err := VerifyTokenForTenant(ctx, mc, "good-token", "tenant-abc")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if info.AccessToken != "good-token" {
			t.Errorf("AccessToken = %q, expected 'good-token'", info.AccessToken)
		}
	})

	t.Run("mismatched tenant", func(t *testing.T) {
		mc := newMockClient()
		mc.verifyData["bad-token"] = &TokenInfo{
			AccessToken: "bad-token",
			ExpiresAt:   time.Now().Add(time.Hour),
			Claims: &VerifyData{
				Active:   true,
				TenantID: "tenant-other",
			},
		}

		_, err := VerifyTokenForTenant(ctx, mc, "bad-token", "tenant-abc")
		if err == nil {
			t.Fatal("expected error for mismatched tenant")
		}
		if !errors.Is(err, ErrTokenInvalid) {
			t.Errorf("expected ErrTokenInvalid, got %v", err)
		}
	})

	t.Run("empty claims tenant allows any", func(t *testing.T) {
		mc := newMockClient()
		mc.verifyData["no-tenant-token"] = &TokenInfo{
			AccessToken: "no-tenant-token",
			ExpiresAt:   time.Now().Add(time.Hour),
			Claims: &VerifyData{
				Active:   true,
				TenantID: "", // 服务端未返回 tenant_id
			},
		}

		info, err := VerifyTokenForTenant(ctx, mc, "no-tenant-token", "tenant-abc")
		if err != nil {
			t.Fatalf("expected nil error for empty claims tenant, got %v", err)
		}
		if info.AccessToken != "no-tenant-token" {
			t.Errorf("AccessToken = %q", info.AccessToken)
		}
	})

	t.Run("nil claims allows any", func(t *testing.T) {
		mc := newMockClient()
		mc.verifyData["nil-claims-token"] = &TokenInfo{
			AccessToken: "nil-claims-token",
			ExpiresAt:   time.Now().Add(time.Hour),
			// Claims is nil
		}

		info, err := VerifyTokenForTenant(ctx, mc, "nil-claims-token", "tenant-abc")
		if err != nil {
			t.Fatalf("expected nil error for nil claims, got %v", err)
		}
		if info.AccessToken != "nil-claims-token" {
			t.Errorf("AccessToken = %q", info.AccessToken)
		}
	})

	t.Run("verify error propagated", func(t *testing.T) {
		mc := newMockClient()
		mc.verifyTokenErr = ErrTokenInvalid

		_, err := VerifyTokenForTenant(ctx, mc, "any-token", "tenant-abc")
		if !errors.Is(err, ErrTokenInvalid) {
			t.Errorf("expected ErrTokenInvalid, got %v", err)
		}
	})

	t.Run("nil client returns ErrNilClient", func(t *testing.T) {
		_, err := VerifyTokenForTenant(ctx, nil, "any-token", "tenant-abc")
		if err != ErrNilClient {
			t.Errorf("expected ErrNilClient, got %v", err)
		}
	})
}
