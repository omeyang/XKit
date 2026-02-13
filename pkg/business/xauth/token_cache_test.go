package xauth

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenCache_Get(t *testing.T) {
	ctx := context.Background()

	t.Run("cache miss", func(t *testing.T) {
		cache := NewTokenCache(TokenCacheConfig{
			EnableLocal: true,
		})

		_, _, err := cache.Get(ctx, "tenant-1")
		assert.Equal(t, ErrCacheMiss, err)
	})

	t.Run("local cache hit", func(t *testing.T) {
		cache := NewTokenCache(TokenCacheConfig{
			EnableLocal:      true,
			RefreshThreshold: 1 * time.Minute,
		})

		// Set token
		token := testToken("test-token", 3600)
		err := cache.Set(ctx, "tenant-1", token, time.Hour)
		require.NoError(t, err, "Set failed")

		// Get token
		got, needsRefresh, err := cache.Get(ctx, "tenant-1")
		require.NoError(t, err, "Get failed")
		assert.Equal(t, token.AccessToken, got.AccessToken)
		assert.False(t, needsRefresh, "needsRefresh should be false for fresh token")
	})

	t.Run("remote cache hit", func(t *testing.T) {
		remote := newMockCacheStore()
		token := testToken("remote-token", 3600)
		_ = remote.SetToken(ctx, "tenant-1", token, time.Hour)

		cache := NewTokenCache(TokenCacheConfig{
			Remote:      remote,
			EnableLocal: true,
		})

		got, _, err := cache.Get(ctx, "tenant-1")
		require.NoError(t, err, "Get failed")
		assert.Equal(t, token.AccessToken, got.AccessToken)
	})

	t.Run("expired local cache", func(t *testing.T) {
		cache := NewTokenCache(TokenCacheConfig{
			EnableLocal: true,
		})

		// Set expired token
		token := &TokenInfo{
			AccessToken: "expired-token",
			ExpiresAt:   time.Now().Add(-1 * time.Hour),
		}
		cache.setLocal("tenant-1", token)

		_, _, err := cache.Get(ctx, "tenant-1")
		assert.Equal(t, ErrCacheMiss, err, "expected ErrCacheMiss for expired token")
	})

	t.Run("needs refresh", func(t *testing.T) {
		cache := NewTokenCache(TokenCacheConfig{
			EnableLocal:      true,
			RefreshThreshold: 1 * time.Hour, // Token will expire within this threshold
		})

		// Set token that expires in 30 minutes
		token := testToken("expiring-token", 1800) // 30 minutes
		err := cache.Set(ctx, "tenant-1", token, time.Hour)
		require.NoError(t, err, "Set failed")

		_, needsRefresh, err := cache.Get(ctx, "tenant-1")
		require.NoError(t, err, "Get failed")
		assert.True(t, needsRefresh, "needsRefresh should be true for expiring token")
	})

	t.Run("remote returns nil nil treated as cache miss", func(t *testing.T) {
		// Create a mock cache store that returns (nil, nil)
		remote := &nilNilCacheStore{}

		cache := NewTokenCache(TokenCacheConfig{
			Remote:      remote,
			EnableLocal: false, // Disable local cache to test remote behavior
		})

		_, _, err := cache.Get(ctx, "tenant-1")
		assert.Equal(t, ErrCacheMiss, err, "expected ErrCacheMiss when remote returns (nil, nil)")
	})
}

// nilNilCacheStore is a mock that returns (nil, nil) for GetToken.
// This tests defensive handling of improper CacheStore implementations.
type nilNilCacheStore struct{}

func (s *nilNilCacheStore) GetToken(ctx context.Context, tenantID string) (*TokenInfo, error) {
	return nil, nil // Improper behavior: should return (nil, ErrCacheMiss)
}

func (s *nilNilCacheStore) SetToken(ctx context.Context, tenantID string, token *TokenInfo, ttl time.Duration) error {
	return nil
}

func (s *nilNilCacheStore) GetPlatformData(ctx context.Context, tenantID string, field string) (string, error) {
	return "", ErrCacheMiss
}

func (s *nilNilCacheStore) SetPlatformData(ctx context.Context, tenantID string, field, value string, ttl time.Duration) error {
	return nil
}

func (s *nilNilCacheStore) Delete(ctx context.Context, tenantID string) error {
	return nil
}

func TestTokenCache_Set(t *testing.T) {
	ctx := context.Background()

	t.Run("set nil token", func(t *testing.T) {
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})
		err := cache.Set(ctx, "tenant-1", nil, time.Hour)
		assert.NoError(t, err, "Set(nil) should not error")
	})

	t.Run("set token", func(t *testing.T) {
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})
		token := testToken("test-token", 3600)

		err := cache.Set(ctx, "tenant-1", token, time.Hour)
		require.NoError(t, err, "Set failed")

		// Verify local size
		assert.Equal(t, 1, cache.LocalSize())
	})

	t.Run("set with remote cache", func(t *testing.T) {
		remote := newMockCacheStore()
		cache := NewTokenCache(TokenCacheConfig{
			Remote:      remote,
			EnableLocal: true,
		})

		token := testToken("test-token", 3600)
		err := cache.Set(ctx, "tenant-1", token, time.Hour)
		require.NoError(t, err, "Set failed")

		// Verify remote cache
		assert.Equal(t, 1, remote.setTokenCalls)
	})

	t.Run("compute expires at", func(t *testing.T) {
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})
		token := &TokenInfo{
			AccessToken: "test-token",
			ExpiresIn:   3600,
		}

		err := cache.Set(ctx, "tenant-1", token, time.Hour)
		require.NoError(t, err, "Set failed")

		// Verify ExpiresAt was computed
		got, _, _ := cache.Get(ctx, "tenant-1")
		assert.False(t, got.ExpiresAt.IsZero(), "ExpiresAt should be computed")
	})

	t.Run("TTL calculated from token ExpiresIn", func(t *testing.T) {
		// Create a mock store that captures the TTL
		remote := newMockCacheStore()
		refreshThreshold := 5 * time.Minute

		cache := NewTokenCache(TokenCacheConfig{
			Remote:           remote,
			EnableLocal:      true,
			RefreshThreshold: refreshThreshold,
		})

		// Token with 30 minutes expiry
		token := &TokenInfo{
			AccessToken: "test-token",
			ExpiresIn:   1800, // 30 minutes
		}

		// Default TTL is 6 hours (much longer than token's actual expiry)
		defaultTTL := 6 * time.Hour
		err := cache.Set(ctx, "tenant-1", token, defaultTTL)
		require.NoError(t, err, "Set failed")

		// The actual TTL used should be based on token's ExpiresIn minus refreshThreshold
		// Expected: 30 minutes - 5 minutes = 25 minutes
		expectedTTL := time.Duration(token.ExpiresIn)*time.Second - refreshThreshold
		actualTTL := remote.lastSetTokenTTL

		// Allow some tolerance for timing
		diff := actualTTL - expectedTTL
		assert.InDelta(t, 0, diff.Seconds(), 1,
			"TTL should be approximately %v (based on token ExpiresIn, not default %v), got %v",
			expectedTTL, defaultTTL, actualTTL)
	})
}

func TestTokenCache_Set_ShortTermToken(t *testing.T) {
	ctx := context.Background()

	t.Run("short term token uses reduced TTL", func(t *testing.T) {
		remote := newMockCacheStore()
		refreshThreshold := 5 * time.Minute

		cache := NewTokenCache(TokenCacheConfig{
			Remote:           remote,
			EnableLocal:      true,
			RefreshThreshold: refreshThreshold,
		})

		// Token with 2 minute expiry (less than 5 minute threshold)
		token := &TokenInfo{
			AccessToken: "short-token",
			ExpiresIn:   120, // 2 minutes
		}

		err := cache.Set(ctx, "tenant-1", token, 6*time.Hour)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// Expected TTL: 2 minutes - 10 seconds = 110 seconds
		expectedTTL := 110 * time.Second
		actualTTL := remote.lastSetTokenTTL

		diff := actualTTL - expectedTTL
		if diff < -time.Second || diff > time.Second {
			t.Errorf("TTL = %v, expected approximately %v for short-term token", actualTTL, expectedTTL)
		}
	})

	t.Run("very short term token not cached to remote", func(t *testing.T) {
		remote := newMockCacheStore()
		refreshThreshold := 5 * time.Minute

		cache := NewTokenCache(TokenCacheConfig{
			Remote:           remote,
			EnableLocal:      true,
			RefreshThreshold: refreshThreshold,
		})

		// Token with 5 second expiry (too short to cache)
		token := &TokenInfo{
			AccessToken: "very-short-token",
			ExpiresIn:   5, // 5 seconds
		}

		err := cache.Set(ctx, "tenant-1", token, 6*time.Hour)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// Very short-term token should not be cached to remote
		if remote.setTokenCalls != 0 {
			t.Errorf("setTokenCalls = %d, expected 0 (very short-term token should not be cached)", remote.setTokenCalls)
		}

		// But should still be in local cache
		if cache.LocalSize() != 1 {
			t.Errorf("LocalSize = %d, expected 1 (should still be in local cache)", cache.LocalSize())
		}
	})
}

func TestTokenCache_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("delete existing", func(t *testing.T) {
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})
		token := testToken("test-token", 3600)

		_ = cache.Set(ctx, "tenant-1", token, time.Hour)
		if cache.LocalSize() != 1 {
			t.Fatal("token should be set")
		}

		err := cache.Delete(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		if cache.LocalSize() != 0 {
			t.Errorf("LocalSize = %d, expected 0 after delete", cache.LocalSize())
		}
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})
		err := cache.Delete(ctx, "nonexistent")
		if err != nil {
			t.Errorf("Delete nonexistent should not error: %v", err)
		}
	})
}

func TestTokenCache_GetOrLoad(t *testing.T) {
	ctx := context.Background()

	t.Run("load on cache miss", func(t *testing.T) {
		cache := NewTokenCache(TokenCacheConfig{
			EnableLocal:        true,
			EnableSingleflight: true,
		})

		loadCalls := 0
		token, err := cache.GetOrLoad(ctx, "tenant-1", func(ctx context.Context) (*TokenInfo, error) {
			loadCalls++
			return testToken("loaded-token", 3600), nil
		}, time.Hour)

		require.NoError(t, err, "GetOrLoad failed")
		assert.Equal(t, "loaded-token", token.AccessToken)
		assert.Equal(t, 1, loadCalls)
	})

	t.Run("use cache on hit", func(t *testing.T) {
		cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

		// Pre-populate cache
		_ = cache.Set(ctx, "tenant-1", testToken("cached-token", 3600), time.Hour)

		loadCalls := 0
		token, err := cache.GetOrLoad(ctx, "tenant-1", func(ctx context.Context) (*TokenInfo, error) {
			loadCalls++
			return testToken("loaded-token", 3600), nil
		}, time.Hour)

		require.NoError(t, err, "GetOrLoad failed")
		assert.Equal(t, "cached-token", token.AccessToken)
		assert.Equal(t, 0, loadCalls, "should use cache")
	})

	t.Run("singleflight prevents concurrent loads", func(t *testing.T) {
		cache := NewTokenCache(TokenCacheConfig{
			EnableLocal:        true,
			EnableSingleflight: true,
		})

		var loadCalls atomic.Int32
		var wg sync.WaitGroup

		// Launch multiple concurrent requests
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = cache.GetOrLoad(ctx, "tenant-1", func(ctx context.Context) (*TokenInfo, error) {
					loadCalls.Add(1)
					time.Sleep(50 * time.Millisecond)
					return testToken("loaded-token", 3600), nil
				}, time.Hour)
			}()
		}

		wg.Wait()

		// With singleflight, only one load should happen
		assert.Equal(t, int32(1), loadCalls.Load(), "singleflight should deduplicate")
	})

	t.Run("without singleflight allows concurrent loads", func(t *testing.T) {
		cache := NewTokenCache(TokenCacheConfig{
			EnableLocal:        true,
			EnableSingleflight: false,
		})

		var loadCalls atomic.Int32
		var wg sync.WaitGroup

		// Launch multiple concurrent requests
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = cache.GetOrLoad(ctx, "tenant-1", func(ctx context.Context) (*TokenInfo, error) {
					loadCalls.Add(1)
					time.Sleep(50 * time.Millisecond)
					return testToken("loaded-token", 3600), nil
				}, time.Hour)
			}()
		}

		wg.Wait()

		// Without singleflight, multiple loads may happen
		if loadCalls.Load() < 2 {
			t.Logf("loadCalls = %d (without singleflight, concurrent loads are allowed)", loadCalls.Load())
		}
	})

	t.Run("loader error with singleflight", func(t *testing.T) {
		cache := NewTokenCache(TokenCacheConfig{
			EnableLocal:        true,
			EnableSingleflight: true,
		})

		_, err := cache.GetOrLoad(ctx, "tenant-1", func(ctx context.Context) (*TokenInfo, error) {
			return nil, ErrTokenNotFound
		}, time.Hour)

		assert.Equal(t, ErrTokenNotFound, err)
	})

	t.Run("token needs refresh but not expired", func(t *testing.T) {
		cache := NewTokenCache(TokenCacheConfig{
			EnableLocal:      true,
			RefreshThreshold: 30 * time.Minute,
		})

		// Pre-populate with token that's expiring soon (within refresh threshold)
		expiringToken := &TokenInfo{
			AccessToken:  "expiring-token",
			RefreshToken: "refresh-token",
			ExpiresIn:    1200, // 20 minutes, less than 30min threshold
			ExpiresAt:    time.Now().Add(20 * time.Minute),
			ObtainedAt:   time.Now(),
		}
		_ = cache.Set(ctx, "tenant-1", expiringToken, time.Hour)

		loadCalls := 0
		token, err := cache.GetOrLoad(ctx, "tenant-1", func(ctx context.Context) (*TokenInfo, error) {
			loadCalls++
			return testToken("loaded-token", 3600), nil
		}, time.Hour)

		require.NoError(t, err, "GetOrLoad failed")
		// Should return cached token (not expired yet)
		assert.Equal(t, "expiring-token", token.AccessToken)
		// Should not call loader since token is not expired
		assert.Equal(t, 0, loadCalls)
	})
}

func TestTokenCache_Clear(t *testing.T) {
	ctx := context.Background()

	cache := NewTokenCache(TokenCacheConfig{EnableLocal: true})

	// Add multiple tokens
	for i := 0; i < 5; i++ {
		tenantID := "tenant-" + string(rune('0'+i))
		_ = cache.Set(ctx, tenantID, testToken("token-"+tenantID, 3600), time.Hour)
	}

	if cache.LocalSize() != 5 {
		t.Fatalf("LocalSize = %d, expected 5", cache.LocalSize())
	}

	cache.Clear()

	if cache.LocalSize() != 0 {
		t.Errorf("LocalSize = %d after Clear, expected 0", cache.LocalSize())
	}
}

func TestTokenCache_Eviction(t *testing.T) {
	ctx := context.Background()

	cache := NewTokenCache(TokenCacheConfig{
		EnableLocal:  true,
		MaxLocalSize: 10,
	})

	// Fill beyond capacity
	for i := 0; i < 20; i++ {
		tenantID := "tenant-" + string(rune('a'+i))
		_ = cache.Set(ctx, tenantID, testToken("token-"+tenantID, 3600), time.Hour)
	}

	// Size should be controlled by eviction
	if cache.LocalSize() > 15 { // Some slack for timing
		t.Errorf("LocalSize = %d, expected <= 15 after eviction", cache.LocalSize())
	}
}

func TestTokenCache_LocalSize_WithLocal(t *testing.T) {
	cache := NewTokenCache(TokenCacheConfig{
		EnableLocal: true,
	})

	// 空缓存
	if cache.LocalSize() != 0 {
		t.Errorf("LocalSize = %d, expected 0", cache.LocalSize())
	}

	// 添加一个
	ctx := context.Background()
	_ = cache.Set(ctx, "t1", testToken("tok1", 3600), time.Hour)
	if cache.LocalSize() != 1 {
		t.Errorf("LocalSize = %d, expected 1", cache.LocalSize())
	}
}

func TestTokenCache_LocalSize_WithoutLocal(t *testing.T) {
	cache := NewTokenCache(TokenCacheConfig{
		EnableLocal: false,
	})

	if cache.LocalSize() != 0 {
		t.Errorf("LocalSize = %d, expected 0", cache.LocalSize())
	}
}
