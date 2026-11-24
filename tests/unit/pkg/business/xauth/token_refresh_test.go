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
		r.Body = http.MaxBytesReader(w, r.Body, testHandlerMaxBodyBytes)
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
		r.Body = http.MaxBytesReader(w, r.Body, testHandlerMaxBodyBytes)
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

func TestTokenManager_StopPreventsNewRefresh(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, testHandlerMaxBodyBytes)
		resp := map[string]any{"access_token": "tok", "expires_in": 3600}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Host = server.URL
	httpClient := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})
	cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

	mgr := mustNewTokenManager(t, TokenManagerConfig{
		Config:                  cfg,
		HTTP:                    httpClient,
		Cache:                   cache,
		RefreshThreshold:        5 * time.Minute,
		EnableBackgroundRefresh: true,
	})

	// 先 Stop
	mgr.Stop()

	// 预填充即将过期的 Token，触发后台刷新条件
	expiringToken := &TokenInfo{
		AccessToken: "expiring",
		ExpiresIn:   30,
		ExpiresAt:   time.Now().Add(30 * time.Second),
		ObtainedAt:  time.Now(),
	}
	_ = cache.Set(ctx, "tenant-stop", expiringToken, time.Hour)

	// GetToken 不应 panic（tryStartRefresh 检查 stopped）
	_, err := mgr.GetToken(ctx, "tenant-stop")
	if err != nil {
		t.Fatalf("GetToken after Stop should still work: %v", err)
	}
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
		const token = "test"
		mc := newMockClient()
		mc.verifyData[token] = &TokenInfo{
			AccessToken: token,
			ExpiresAt:   time.Now().Add(time.Hour),
			// Claims is nil
		}

		info, err := VerifyTokenForTenant(ctx, mc, token, "tenant-abc")
		if err != nil {
			t.Fatalf("expected nil error for nil claims, got %v", err)
		}
		if info.AccessToken != token {
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
