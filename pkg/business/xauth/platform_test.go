package xauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewPlatformManager(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: "https://test.com"})

		mgr := NewPlatformManager(PlatformManagerConfig{
			HTTP: httpClient,
		})

		if mgr.logger == nil {
			t.Error("logger should have default value")
		}
		if mgr.observer == nil {
			t.Error("observer should have default value")
		}
		if mgr.cacheTTL != DefaultPlatformDataCacheTTL {
			t.Errorf("cacheTTL = %v, expected %v", mgr.cacheTTL, DefaultPlatformDataCacheTTL)
		}
		if mgr.cache == nil {
			t.Error("cache should have default value")
		}
	})

	t.Run("custom values", func(t *testing.T) {
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: "https://test.com"})
		customCache := newMockCacheStore()
		customTTL := 10 * time.Minute

		mgr := NewPlatformManager(PlatformManagerConfig{
			HTTP:     httpClient,
			Cache:    customCache,
			CacheTTL: customTTL,
		})

		if mgr.cacheTTL != customTTL {
			t.Errorf("cacheTTL = %v, expected %v", mgr.cacheTTL, customTTL)
		}
	})
}

func TestPlatformManager_GetPlatformID(t *testing.T) {
	ctx := context.Background()

	t.Run("from local cache", func(t *testing.T) {
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: "https://test.com"})
		mgr := NewPlatformManager(PlatformManagerConfig{HTTP: httpClient})

		// Pre-populate local cache
		mgr.setLocalCache("tenant-1", CacheFieldPlatformID, "platform-123")

		id, err := mgr.GetPlatformID(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("GetPlatformID failed: %v", err)
		}
		if id != "platform-123" {
			t.Errorf("id = %q, expected 'platform-123'", id)
		}
	})

	t.Run("from remote cache", func(t *testing.T) {
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: "https://test.com"})
		mockCache := newMockCacheStore()
		_ = mockCache.SetPlatformData(ctx, "tenant-1", CacheFieldPlatformID, "remote-platform-123", time.Hour)

		mgr := NewPlatformManager(PlatformManagerConfig{
			HTTP:  httpClient,
			Cache: mockCache,
		})

		id, err := mgr.GetPlatformID(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("GetPlatformID failed: %v", err)
		}
		if id != "remote-platform-123" {
			t.Errorf("id = %q, expected 'remote-platform-123'", id)
		}

		// Verify local cache was populated
		if mgr.getLocalCache("tenant-1", CacheFieldPlatformID) != "remote-platform-123" {
			t.Error("local cache should be populated from remote")
		}
	})

	t.Run("from API", func(t *testing.T) {
		// Create mock token server
		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]any{
				"access_token": "test-token",
				"expires_in":   3600,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer tokenServer.Close()

		// Create mock platform server
		platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == PathPlatformSelf {
				resp := PlatformSelfResponse{
					Data: struct {
						ID string `json:"id"`
					}{ID: "api-platform-123"},
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
		defer platformServer.Close()

		cfg := testConfig()
		cfg.Host = platformServer.URL
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: platformServer.URL})
		tokenCache := NewTokenCache(TokenCacheConfig{EnableLocal: true})
		tokenMgr := mustNewTokenManager(t, TokenManagerConfig{
			Config: cfg,
			HTTP:   httpClient,
			Cache:  tokenCache,
		})

		mgr := NewPlatformManager(PlatformManagerConfig{
			HTTP:     httpClient,
			TokenMgr: tokenMgr,
		})

		id, err := mgr.GetPlatformID(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("GetPlatformID failed: %v", err)
		}
		if id != "api-platform-123" {
			t.Errorf("id = %q, expected 'api-platform-123'", id)
		}
	})
}

func TestPlatformManager_HasParentPlatform(t *testing.T) {
	ctx := context.Background()

	t.Run("from local cache - true", func(t *testing.T) {
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: "https://test.com"})
		mgr := NewPlatformManager(PlatformManagerConfig{HTTP: httpClient})

		mgr.setLocalCache("tenant-1", CacheFieldHasParent, "true")

		hasParent, err := mgr.HasParentPlatform(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("HasParentPlatform failed: %v", err)
		}
		if !hasParent {
			t.Error("expected hasParent to be true")
		}
	})

	t.Run("from local cache - false", func(t *testing.T) {
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: "https://test.com"})
		mgr := NewPlatformManager(PlatformManagerConfig{HTTP: httpClient})

		mgr.setLocalCache("tenant-1", CacheFieldHasParent, "false")

		hasParent, err := mgr.HasParentPlatform(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("HasParentPlatform failed: %v", err)
		}
		if hasParent {
			t.Error("expected hasParent to be false")
		}
	})
}

func TestPlatformManager_GetUnclassRegionID(t *testing.T) {
	ctx := context.Background()

	t.Run("from local cache", func(t *testing.T) {
		httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: "https://test.com"})
		mgr := NewPlatformManager(PlatformManagerConfig{HTTP: httpClient})

		mgr.setLocalCache("tenant-1", CacheFieldUnclassRegionID, "region-456")

		id, err := mgr.GetUnclassRegionID(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("GetUnclassRegionID failed: %v", err)
		}
		if id != "region-456" {
			t.Errorf("id = %q, expected 'region-456'", id)
		}
	})
}

func TestPlatformManager_LocalCache(t *testing.T) {
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: "https://test.com"})
	mgr := NewPlatformManager(PlatformManagerConfig{HTTP: httpClient})

	t.Run("set and get", func(t *testing.T) {
		mgr.setLocalCache("tenant-1", "field-1", "value-1")
		mgr.setLocalCache("tenant-1", "field-2", "value-2")
		mgr.setLocalCache("tenant-2", "field-1", "value-3")

		if v := mgr.getLocalCache("tenant-1", "field-1"); v != "value-1" {
			t.Errorf("got %q, expected 'value-1'", v)
		}
		if v := mgr.getLocalCache("tenant-1", "field-2"); v != "value-2" {
			t.Errorf("got %q, expected 'value-2'", v)
		}
		if v := mgr.getLocalCache("tenant-2", "field-1"); v != "value-3" {
			t.Errorf("got %q, expected 'value-3'", v)
		}
	})

	t.Run("get nonexistent", func(t *testing.T) {
		if v := mgr.getLocalCache("nonexistent", "field"); v != "" {
			t.Errorf("got %q, expected empty string", v)
		}
	})
}

func TestPlatformManager_ClearLocalCache(t *testing.T) {
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: "https://test.com"})
	mgr := NewPlatformManager(PlatformManagerConfig{HTTP: httpClient})

	// Populate cache
	mgr.setLocalCache("tenant-1", "field-1", "value-1")
	mgr.setLocalCache("tenant-2", "field-2", "value-2")

	// Clear cache
	mgr.ClearLocalCache()

	// Verify cleared
	if v := mgr.getLocalCache("tenant-1", "field-1"); v != "" {
		t.Error("cache should be cleared")
	}
	if v := mgr.getLocalCache("tenant-2", "field-2"); v != "" {
		t.Error("cache should be cleared")
	}
}

func TestPlatformManager_InvalidateCache(t *testing.T) {
	ctx := context.Background()
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: "https://test.com"})
	mockCache := newMockCacheStore()

	mgr := NewPlatformManager(PlatformManagerConfig{
		HTTP:  httpClient,
		Cache: mockCache,
	})

	// Populate caches with known field names
	mgr.setLocalCache("tenant-1", CacheFieldPlatformID, "platform-123")
	mgr.setLocalCache("tenant-1", CacheFieldHasParent, "true")
	_ = mockCache.SetPlatformData(ctx, "tenant-1", CacheFieldPlatformID, "platform-123", time.Hour)

	// Invalidate
	err := mgr.InvalidateCache(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("InvalidateCache failed: %v", err)
	}

	// Verify local cache cleared
	if v := mgr.getLocalCache("tenant-1", CacheFieldPlatformID); v != "" {
		t.Error("local cache for platform_id should be cleared")
	}
	if v := mgr.getLocalCache("tenant-1", CacheFieldHasParent); v != "" {
		t.Error("local cache for has_parent should be cleared")
	}
}

func TestPlatformManager_Singleflight(t *testing.T) {
	ctx := context.Background()

	// Create mock server that counts calls
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathPlatformSelf {
			callCount.Add(1)
			time.Sleep(50 * time.Millisecond) // Simulate slow API
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

	mgr := NewPlatformManager(PlatformManagerConfig{
		HTTP:     httpClient,
		TokenMgr: tokenMgr,
	})

	// Launch concurrent requests
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			_, _ = mgr.GetPlatformID(ctx, "tenant-1")
		})
	}
	wg.Wait()

	// With singleflight, only one API call should be made
	if callCount.Load() != 1 {
		t.Errorf("API calls = %d, expected 1 (singleflight should deduplicate)", callCount.Load())
	}
}

func TestPlatformManager_FetchPlatformID_Empty(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathPlatformSelf {
			// Return empty platform ID
			resp := PlatformSelfResponse{
				Data: struct {
					ID string `json:"id"`
				}{ID: ""},
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

	mgr := NewPlatformManager(PlatformManagerConfig{
		HTTP:     httpClient,
		TokenMgr: tokenMgr,
	})

	_, err := mgr.GetPlatformID(ctx, "tenant-1")
	if err != ErrPlatformIDNotFound {
		t.Errorf("expected ErrPlatformIDNotFound, got %v", err)
	}
}

func TestPlatformManager_FetchUnclassRegionID_Empty(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathUnclassRegion {
			// Return empty region ID
			resp := UnclassRegionResponse{
				Data: struct {
					ID string `json:"id"`
				}{ID: ""},
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

	mgr := NewPlatformManager(PlatformManagerConfig{
		HTTP:     httpClient,
		TokenMgr: tokenMgr,
	})

	_, err := mgr.GetUnclassRegionID(ctx, "tenant-1")
	if err != ErrUnclassRegionIDNotFound {
		t.Errorf("expected ErrUnclassRegionIDNotFound, got %v", err)
	}
}

func TestPlatformManager_HasParentPlatform_ParseError(t *testing.T) {
	ctx := context.Background()

	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: "https://test.com"})
	mgr := NewPlatformManager(PlatformManagerConfig{HTTP: httpClient})

	// Set invalid bool value in cache
	mgr.setLocalCache("tenant-1", CacheFieldHasParent, "invalid-not-a-bool")

	_, err := mgr.HasParentPlatform(ctx, "tenant-1")
	if err == nil {
		t.Error("expected parse error for invalid bool")
	}
}

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

	mgr := NewPlatformManager(PlatformManagerConfig{
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

	mgr := NewPlatformManager(PlatformManagerConfig{
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

	mgr := NewPlatformManager(PlatformManagerConfig{
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

	mgr := NewPlatformManager(PlatformManagerConfig{
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

	mgr := NewPlatformManager(PlatformManagerConfig{
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

	mgr := NewPlatformManager(PlatformManagerConfig{
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

	mgr := NewPlatformManager(PlatformManagerConfig{
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

		mgr := NewPlatformManager(PlatformManagerConfig{
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

	mgr := NewPlatformManager(PlatformManagerConfig{
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
	mgr := NewPlatformManager(PlatformManagerConfig{
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
	mgr := NewPlatformManager(PlatformManagerConfig{
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
	mgr := NewPlatformManager(PlatformManagerConfig{
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
