package xauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewTokenManager(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		cfg := testConfig()
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: cfg.Host})
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

		mgr := NewTokenManager(TokenManagerConfig{
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
		mgr := NewTokenManager(TokenManagerConfig{
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

		mgr := NewTokenManager(TokenManagerConfig{
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

		mgr := NewTokenManager(TokenManagerConfig{
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

		mgr := NewTokenManager(TokenManagerConfig{
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
			if r.URL.Query().Get("client_id") == "" {
				t.Error("missing client_id")
			}
			if r.URL.Query().Get("grant_type") != "client_credentials" {
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

		mgr := NewTokenManager(TokenManagerConfig{
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

		mgr := NewTokenManager(TokenManagerConfig{
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

		mgr := NewTokenManager(TokenManagerConfig{
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

		mgr := NewTokenManager(TokenManagerConfig{
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

		mgr := NewTokenManager(TokenManagerConfig{
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
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

		mgr := NewTokenManager(TokenManagerConfig{
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

		mgr := NewTokenManager(TokenManagerConfig{
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

	mgr := NewTokenManager(TokenManagerConfig{
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
			if r.URL.Query().Get("grant_type") == "refresh_token" {
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

		mgr := NewTokenManager(TokenManagerConfig{
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

		mgr := NewTokenManager(TokenManagerConfig{
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

	mgr := NewTokenManager(TokenManagerConfig{
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

	mgr := NewTokenManager(TokenManagerConfig{
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

	mgr := NewTokenManager(TokenManagerConfig{
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

	mgr := NewTokenManager(TokenManagerConfig{
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

	mgr := NewTokenManager(TokenManagerConfig{
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

	mgr := NewTokenManager(TokenManagerConfig{
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

	mgr := NewTokenManager(TokenManagerConfig{
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
		if r.URL.Query().Get("grant_type") == "refresh_token" {
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

	mgr := NewTokenManager(TokenManagerConfig{
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

	mgr := NewTokenManager(TokenManagerConfig{
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
