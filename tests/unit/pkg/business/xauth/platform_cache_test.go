package xauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPlatformManager_HasParentPlatform_GetFieldError(t *testing.T) {
	ctx := context.Background()

	// Server that fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	tokenCache := NewTokenCache(TokenCacheConfig{EnableLocal: true})
	tokenMgr := mustNewTokenManager(t, TokenManagerConfig{
		Config: cfg,
		HTTP:   httpClient,
		Cache:  tokenCache,
	})

	mgr := mustNewPlatformManager(t, PlatformManagerConfig{
		HTTP:     httpClient,
		TokenMgr: tokenMgr,
	})

	_, err := mgr.HasParentPlatform(ctx, "tenant-1")
	if err == nil {
		t.Error("expected error when getField fails")
	}
}

func TestPlatformManager_FetchHasParent_API(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathHasParent {
			resp := HasParentResponse{
				Data: true,
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
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	tokenCache := NewTokenCache(TokenCacheConfig{EnableLocal: true})
	tokenMgr := mustNewTokenManager(t, TokenManagerConfig{
		Config: cfg,
		HTTP:   httpClient,
		Cache:  tokenCache,
	})

	mgr := mustNewPlatformManager(t, PlatformManagerConfig{
		HTTP:     httpClient,
		TokenMgr: tokenMgr,
	})

	hasParent, err := mgr.HasParentPlatform(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("HasParentPlatform failed: %v", err)
	}
	if !hasParent {
		t.Error("expected hasParent to be true")
	}
}

func TestPlatformManager_FetchPlatformID_TokenError(t *testing.T) {
	ctx := context.Background()

	// Server that fails token request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	tokenCache := NewTokenCache(TokenCacheConfig{EnableLocal: true})
	tokenMgr := mustNewTokenManager(t, TokenManagerConfig{
		Config: cfg,
		HTTP:   httpClient,
		Cache:  tokenCache,
	})

	mgr := mustNewPlatformManager(t, PlatformManagerConfig{
		HTTP:     httpClient,
		TokenMgr: tokenMgr,
	})

	_, err := mgr.GetPlatformID(ctx, "tenant-1")
	if err == nil {
		t.Error("expected error when token fetch fails")
	}
}

func TestPlatformManager_FetchPlatformID_HTTPError(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, testHandlerMaxBodyBytes)
		// Token endpoint succeeds
		if r.FormValue("client_id") != "" {
			resp := map[string]any{
				"access_token": "test-token",
				"expires_in":   3600,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Platform endpoint fails
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	tokenCache := NewTokenCache(TokenCacheConfig{EnableLocal: true})
	tokenMgr := mustNewTokenManager(t, TokenManagerConfig{
		Config: cfg,
		HTTP:   httpClient,
		Cache:  tokenCache,
	})

	mgr := mustNewPlatformManager(t, PlatformManagerConfig{
		HTTP:     httpClient,
		TokenMgr: tokenMgr,
	})

	_, err := mgr.GetPlatformID(ctx, "tenant-1")
	if err == nil {
		t.Error("expected error when HTTP request fails")
	}
}

func TestPlatformManager_FetchUnclassRegionID_TokenError(t *testing.T) {
	ctx := context.Background()

	// Server that fails token request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	tokenCache := NewTokenCache(TokenCacheConfig{EnableLocal: true})
	tokenMgr := mustNewTokenManager(t, TokenManagerConfig{
		Config: cfg,
		HTTP:   httpClient,
		Cache:  tokenCache,
	})

	mgr := mustNewPlatformManager(t, PlatformManagerConfig{
		HTTP:     httpClient,
		TokenMgr: tokenMgr,
	})

	_, err := mgr.GetUnclassRegionID(ctx, "tenant-1")
	if err == nil {
		t.Error("expected error when token fetch fails")
	}
}

func TestPlatformManager_FetchUnclassRegionID_HTTPError(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, testHandlerMaxBodyBytes)
		// Token endpoint succeeds
		if r.FormValue("client_id") != "" {
			resp := map[string]any{
				"access_token": "test-token",
				"expires_in":   3600,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Unclass region endpoint fails
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	tokenCache := NewTokenCache(TokenCacheConfig{EnableLocal: true})
	tokenMgr := mustNewTokenManager(t, TokenManagerConfig{
		Config: cfg,
		HTTP:   httpClient,
		Cache:  tokenCache,
	})

	mgr := mustNewPlatformManager(t, PlatformManagerConfig{
		HTTP:     httpClient,
		TokenMgr: tokenMgr,
	})

	_, err := mgr.GetUnclassRegionID(ctx, "tenant-1")
	if err == nil {
		t.Error("expected error when HTTP request fails")
	}
}

func TestPlatformManager_CacheWriteError(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathPlatformSelf {
			resp := PlatformSelfResponse{
				Data: struct {
					ID string `json:"id"`
				}{ID: "platform-123"},
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
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	tokenCache := NewTokenCache(TokenCacheConfig{EnableLocal: true})
	tokenMgr := mustNewTokenManager(t, TokenManagerConfig{
		Config: cfg,
		HTTP:   httpClient,
		Cache:  tokenCache,
	})

	// Mock cache that returns errors on write
	mockCache := newMockCacheStore()
	mockCache.setPlatformErr = ErrServerError

	mgr := mustNewPlatformManager(t, PlatformManagerConfig{
		HTTP:     httpClient,
		TokenMgr: tokenMgr,
		Cache:    mockCache,
	})

	// Should still work despite cache write error
	id, err := mgr.GetPlatformID(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("GetPlatformID failed: %v", err)
	}
	if id != "platform-123" {
		t.Errorf("id = %q, expected 'platform-123'", id)
	}
}

func TestPlatformManager_URLEncoding(t *testing.T) {
	ctx := context.Background()

	t.Run("tenantID with special characters is URL encoded", func(t *testing.T) {
		var receivedPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == PathPlatformSelf {
				receivedPath = r.URL.RawQuery
				resp := PlatformSelfResponse{
					Data: struct {
						ID string `json:"id"`
					}{ID: "platform-123"},
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
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
		tokenCache := NewTokenCache(TokenCacheConfig{EnableLocal: true})
		tokenMgr := mustNewTokenManager(t, TokenManagerConfig{
			Config: cfg,
			HTTP:   httpClient,
			Cache:  tokenCache,
		})

		mgr := mustNewPlatformManager(t, PlatformManagerConfig{
			HTTP:     httpClient,
			TokenMgr: tokenMgr,
		})

		// Use tenantID with special characters that need encoding
		tenantID := "tenant/with&special=chars"
		_, err := mgr.GetPlatformID(ctx, tenantID)
		if err != nil {
			t.Fatalf("GetPlatformID failed: %v", err)
		}

		// Verify the tenantID was properly URL encoded
		expectedEncoded := "projectId=tenant%2Fwith%26special%3Dchars"
		if receivedPath != expectedEncoded {
			t.Errorf("received query = %q, expected %q (URL encoded)", receivedPath, expectedEncoded)
		}
	})
}

func TestPlatformManager_CacheError(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathPlatformSelf {
			resp := PlatformSelfResponse{
				Data: struct {
					ID string `json:"id"`
				}{ID: "platform-123"},
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
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	tokenCache := NewTokenCache(TokenCacheConfig{EnableLocal: true})
	tokenMgr := mustNewTokenManager(t, TokenManagerConfig{
		Config: cfg,
		HTTP:   httpClient,
		Cache:  tokenCache,
	})

	// Mock cache that returns errors
	mockCache := newMockCacheStore()
	mockCache.getPlatformErr = ErrServerError

	mgr := mustNewPlatformManager(t, PlatformManagerConfig{
		HTTP:     httpClient,
		TokenMgr: tokenMgr,
		Cache:    mockCache,
	})

	// Should still work by fetching from API
	id, err := mgr.GetPlatformID(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("GetPlatformID failed: %v", err)
	}
	if id != "platform-123" {
		t.Errorf("id = %q, expected 'platform-123'", id)
	}
}

func TestPlatformManager_LocalCacheTTL(t *testing.T) {
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: "https://test.com"})

	// Create manager with short TTL for testing
	mgr := mustNewPlatformManager(t, PlatformManagerConfig{
		HTTP:          httpClient,
		LocalCacheTTL: 50 * time.Millisecond,
	})

	// Set value in local cache
	mgr.setLocalCache("tenant-1", CacheFieldPlatformID, "platform-123")

	// Value should be accessible immediately
	if v := mgr.getLocalCache("tenant-1", CacheFieldPlatformID); v != "platform-123" {
		t.Errorf("got %q, expected 'platform-123'", v)
	}

	// Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)

	// Value should be expired
	if v := mgr.getLocalCache("tenant-1", CacheFieldPlatformID); v != "" {
		t.Errorf("got %q, expected empty string (expired)", v)
	}
}

func TestPlatformManager_LocalCacheSize(t *testing.T) {
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: "https://test.com"})

	// Create manager with small cache size
	mgr := mustNewPlatformManager(t, PlatformManagerConfig{
		HTTP:           httpClient,
		LocalCacheSize: 2,
		LocalCacheTTL:  time.Minute,
	})

	// Fill cache beyond capacity
	mgr.setLocalCache("tenant-1", CacheFieldPlatformID, "value-1")
	mgr.setLocalCache("tenant-2", CacheFieldPlatformID, "value-2")
	mgr.setLocalCache("tenant-3", CacheFieldPlatformID, "value-3")

	// Oldest entry should be evicted
	if v := mgr.getLocalCache("tenant-1", CacheFieldPlatformID); v != "" {
		t.Error("tenant-1 should be evicted due to LRU")
	}

	// Newer entries should still exist
	if v := mgr.getLocalCache("tenant-2", CacheFieldPlatformID); v != "value-2" {
		t.Errorf("tenant-2 = %q, expected 'value-2'", v)
	}
	if v := mgr.getLocalCache("tenant-3", CacheFieldPlatformID); v != "value-3" {
		t.Errorf("tenant-3 = %q, expected 'value-3'", v)
	}
}

func TestPlatformManager_DisableLocalCache(t *testing.T) {
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: "https://test.com"})
	enableLocal := false
	mgr := mustNewPlatformManager(t, PlatformManagerConfig{
		HTTP:        httpClient,
		EnableLocal: &enableLocal,
	})

	// localCache should be nil when disabled
	if mgr.localCache != nil {
		t.Error("localCache should be nil when EnableLocal is false")
	}

	// set/get should be no-ops without panic
	mgr.setLocalCache("tenant-1", CacheFieldPlatformID, "value-1")
	if v := mgr.getLocalCache("tenant-1", CacheFieldPlatformID); v != "" {
		t.Errorf("got %q, expected empty (local cache disabled)", v)
	}

	// ClearLocalCache and InvalidateCache should not panic
	mgr.ClearLocalCache()
	ctx := context.Background()
	_ = mgr.InvalidateCache(ctx, "tenant-1")
}
