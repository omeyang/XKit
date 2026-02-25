package xauth

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisCacheStore_TokenKey(t *testing.T) {
	store := &RedisCacheStore{
		keyPrefix: "xauth:",
	}

	key := store.tokenKey("tenant-123")
	expected := "xauth:token:tenant-123"
	if key != expected {
		t.Errorf("tokenKey = %q, expected %q", key, expected)
	}
}

func TestRedisCacheStore_PlatformKey(t *testing.T) {
	store := &RedisCacheStore{
		keyPrefix: "xauth:",
	}

	key := store.platformKey("tenant-123")
	expected := "xauth:platform:tenant-123"
	if key != expected {
		t.Errorf("platformKey = %q, expected %q", key, expected)
	}
}

func TestRedisCacheStore_DefaultKeyPrefix(t *testing.T) {
	// Without prefix (empty)
	store := &RedisCacheStore{keyPrefix: ""}
	key := store.tokenKey("tenant-123")
	expected := "token:tenant-123"
	if key != expected {
		t.Errorf("tokenKey without prefix = %q, expected %q", key, expected)
	}
}

func TestNewRedisCacheStore(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		_, err := NewRedisCacheStore(nil)
		assert.ErrorIs(t, err, ErrNilRedisClient)
	})

	t.Run("default key prefix", func(t *testing.T) {
		// We can't test with real Redis client, but we can test construction
		store := &RedisCacheStore{
			keyPrefix: "xauth:",
		}
		if store.keyPrefix != "xauth:" {
			t.Errorf("keyPrefix = %q, expected 'xauth:'", store.keyPrefix)
		}
	})

	t.Run("with key prefix option", func(t *testing.T) {
		store := &RedisCacheStore{
			keyPrefix: "custom:",
		}
		opt := WithKeyPrefix("myapp:")
		opt(store)
		if store.keyPrefix != "myapp:" {
			t.Errorf("keyPrefix = %q, expected 'myapp:'", store.keyPrefix)
		}
	})
}

func TestMockCacheStore_Integration(t *testing.T) {
	ctx := context.Background()
	store := newMockCacheStore()

	t.Run("token operations", func(t *testing.T) {
		// Set token
		token := testToken("test-token", 3600)
		err := store.SetToken(ctx, "tenant-1", token, time.Hour)
		require.NoError(t, err, "SetToken failed")

		// Get token
		got, err := store.GetToken(ctx, "tenant-1")
		require.NoError(t, err, "GetToken failed")
		assert.Equal(t, token.AccessToken, got.AccessToken)

		// Get nonexistent
		_, err = store.GetToken(ctx, "nonexistent")
		assert.Equal(t, ErrCacheMiss, err)

		// Delete
		err = store.Delete(ctx, "tenant-1")
		require.NoError(t, err, "Delete failed")

		// Verify deleted
		_, err = store.GetToken(ctx, "tenant-1")
		assert.Equal(t, ErrCacheMiss, err, "expected ErrCacheMiss after delete")
	})

	t.Run("platform data operations", func(t *testing.T) {
		// Set platform data
		err := store.SetPlatformData(ctx, "tenant-1", "platform_id", "platform-123", time.Hour)
		require.NoError(t, err, "SetPlatformData failed")

		// Get platform data
		value, err := store.GetPlatformData(ctx, "tenant-1", "platform_id")
		require.NoError(t, err, "GetPlatformData failed")
		assert.Equal(t, "platform-123", value)

		// Get nonexistent field
		_, err = store.GetPlatformData(ctx, "tenant-1", "nonexistent")
		assert.Equal(t, ErrCacheMiss, err)

		// Get nonexistent tenant
		_, err = store.GetPlatformData(ctx, "nonexistent", "platform_id")
		assert.Equal(t, ErrCacheMiss, err)
	})

	t.Run("error injection", func(t *testing.T) {
		store := newMockCacheStore()

		// Inject errors
		store.getTokenErr = ErrServerError
		_, err := store.GetToken(ctx, "tenant-1")
		assert.Equal(t, ErrServerError, err)

		store.setTokenErr = ErrServerError
		err = store.SetToken(ctx, "tenant-1", testToken("token", 3600), time.Hour)
		assert.Equal(t, ErrServerError, err)

		store.getPlatformErr = ErrServerError
		_, err = store.GetPlatformData(ctx, "tenant-1", "field")
		assert.Equal(t, ErrServerError, err)

		store.setPlatformErr = ErrServerError
		err = store.SetPlatformData(ctx, "tenant-1", "field", "value", time.Hour)
		assert.Equal(t, ErrServerError, err)

		store.deleteErr = ErrServerError
		err = store.Delete(ctx, "tenant-1")
		assert.Equal(t, ErrServerError, err)
	})

	t.Run("call counting", func(t *testing.T) {
		store := newMockCacheStore()

		_ = store.SetToken(ctx, "tenant-1", testToken("token", 3600), time.Hour)
		_ = store.SetToken(ctx, "tenant-2", testToken("token", 3600), time.Hour)
		_, _ = store.GetToken(ctx, "tenant-1")
		_, _ = store.GetToken(ctx, "tenant-1")
		_, _ = store.GetToken(ctx, "tenant-2")

		assert.Equal(t, 2, store.setTokenCalls)
		assert.Equal(t, 3, store.getTokenCalls)
	})
}

func TestMockClient(t *testing.T) {
	ctx := context.Background()
	mc := newMockClient()

	t.Run("GetToken", func(t *testing.T) {
		// Default behavior
		token, err := mc.GetToken(ctx, "tenant-1")
		require.NoError(t, err, "GetToken failed")
		assert.Equal(t, "mock-token-tenant-1", token)

		// With preset value
		mc.tokens["tenant-2"] = "custom-token"
		token, err = mc.GetToken(ctx, "tenant-2")
		require.NoError(t, err, "GetToken failed")
		assert.Equal(t, "custom-token", token)

		// With error
		mc.getTokenErr = ErrTokenNotFound
		_, err = mc.GetToken(ctx, "tenant-1")
		assert.Equal(t, ErrTokenNotFound, err)
	})

	t.Run("VerifyToken", func(t *testing.T) {
		mc := newMockClient()

		// Default behavior
		info, err := mc.VerifyToken(ctx, "test-token")
		require.NoError(t, err, "VerifyToken failed")
		assert.Equal(t, "test-token", info.AccessToken)

		// With preset value
		mc.verifyData["custom-token"] = testToken("custom-verified", 3600)
		info, err = mc.VerifyToken(ctx, "custom-token")
		require.NoError(t, err, "VerifyToken failed")
		assert.Equal(t, "custom-verified", info.AccessToken)

		// With error
		mc.verifyTokenErr = ErrTokenInvalid
		_, err = mc.VerifyToken(ctx, "test-token")
		assert.Equal(t, ErrTokenInvalid, err)
	})

	t.Run("GetPlatformID", func(t *testing.T) {
		mc := newMockClient()

		// Default behavior
		id, err := mc.GetPlatformID(ctx, "tenant-1")
		require.NoError(t, err, "GetPlatformID failed")
		assert.Equal(t, "platform-tenant-1", id)

		// With error
		mc.getPlatformIDErr = ErrPlatformIDNotFound
		_, err = mc.GetPlatformID(ctx, "tenant-1")
		assert.Equal(t, ErrPlatformIDNotFound, err)
	})

	t.Run("HasParentPlatform", func(t *testing.T) {
		mc := newMockClient()

		// Default behavior (false)
		hasParent, err := mc.HasParentPlatform(ctx, "tenant-1")
		require.NoError(t, err, "HasParentPlatform failed")
		assert.False(t, hasParent, "expected hasParent to be false by default")

		// With preset value
		mc.hasParent["tenant-2"] = true
		hasParent, err = mc.HasParentPlatform(ctx, "tenant-2")
		require.NoError(t, err, "HasParentPlatform failed")
		assert.True(t, hasParent, "expected hasParent to be true")
	})

	t.Run("GetUnclassRegionID", func(t *testing.T) {
		mc := newMockClient()

		// Default behavior
		id, err := mc.GetUnclassRegionID(ctx, "tenant-1")
		require.NoError(t, err, "GetUnclassRegionID failed")
		assert.Equal(t, "region-tenant-1", id)
	})

	t.Run("Request", func(t *testing.T) {
		mc := newMockClient()

		// Default behavior
		err := mc.Request(ctx, &AuthRequest{})
		assert.NoError(t, err)

		// With error
		mc.requestErr = ErrRequestFailed
		err = mc.Request(ctx, &AuthRequest{})
		assert.Equal(t, ErrRequestFailed, err)
	})

	t.Run("Close", func(t *testing.T) {
		mc := newMockClient()

		assert.False(t, mc.closed, "should not be closed initially")

		err := mc.Close(context.Background())
		assert.NoError(t, err, "Close failed")
		assert.True(t, mc.closed, "should be closed after Close()")
	})
}

func TestTokenInfo_ObtainedAtUnix_Serialization(t *testing.T) {
	t.Run("ObtainedAtUnix preserves real time", func(t *testing.T) {
		// Create token with a specific ObtainedAt time
		obtainedAt := time.Now().Add(-30 * time.Minute) // 30 minutes ago
		original := &TokenInfo{
			AccessToken:    "test-token",
			ExpiresIn:      3600, // 1 hour
			ObtainedAt:     obtainedAt,
			ObtainedAtUnix: obtainedAt.Unix(),
		}

		// Marshal to JSON
		data, err := json.Marshal(original)
		require.NoError(t, err, "Marshal failed")

		// Unmarshal back
		var restored TokenInfo
		require.NoError(t, json.Unmarshal(data, &restored), "Unmarshal failed")

		// ObtainedAtUnix should be preserved
		assert.Equal(t, original.ObtainedAtUnix, restored.ObtainedAtUnix)

		// Simulate RedisCacheStore.GetToken logic
		if restored.ObtainedAtUnix > 0 {
			restored.ObtainedAt = time.Unix(restored.ObtainedAtUnix, 0)
		}
		if restored.ExpiresAt.IsZero() && restored.ExpiresIn > 0 {
			restored.ExpiresAt = restored.ObtainedAt.Add(time.Duration(restored.ExpiresIn) * time.Second)
		}

		// ObtainedAt should be restored to the original time
		assert.Equal(t, obtainedAt.Unix(), restored.ObtainedAt.Unix())

		// ExpiresAt should be 30 minutes from now (1 hour from obtainedAt)
		expectedExpiresAt := obtainedAt.Add(time.Hour)
		assert.Equal(t, expectedExpiresAt.Unix(), restored.ExpiresAt.Unix())
	})

	t.Run("missing ObtainedAtUnix fallback", func(t *testing.T) {
		// 缺少 ObtainedAtUnix 字段的数据
		jsonData := `{"access_token":"old-token","expires_in":3600}`

		var token TokenInfo
		require.NoError(t, json.Unmarshal([]byte(jsonData), &token), "Unmarshal failed")

		// ObtainedAtUnix should be 0
		assert.Equal(t, int64(0), token.ObtainedAtUnix)

		// 模拟 RedisCacheStore.GetToken 的容错逻辑
		if token.ObtainedAtUnix > 0 {
			token.ObtainedAt = time.Unix(token.ObtainedAtUnix, 0)
		} else if token.ObtainedAt.IsZero() {
			token.ObtainedAt = time.Now()
		}

		// ObtainedAt should be set via fallback
		assert.False(t, token.ObtainedAt.IsZero(), "ObtainedAt should be set via fallback")
	})
}

func TestTokenInfo_JSONSerialization(t *testing.T) {
	// This test verifies that ExpiresAt and ObtainedAt are properly handled
	// after JSON serialization/deserialization (they have json:"-" tags)

	t.Run("ExpiresAt and ObtainedAt excluded from JSON", func(t *testing.T) {
		original := &TokenInfo{
			AccessToken:  "test-token",
			ExpiresIn:    3600,
			ExpiresAt:    time.Now().Add(time.Hour),
			ObtainedAt:   time.Now(),
			TokenType:    "bearer",
			RefreshToken: "refresh-token",
		}

		// Marshal to JSON
		data, err := json.Marshal(original)
		require.NoError(t, err, "Marshal failed")

		// Unmarshal back
		var restored TokenInfo
		require.NoError(t, json.Unmarshal(data, &restored), "Unmarshal failed")

		// ExpiresAt and ObtainedAt should be zero after JSON round-trip
		assert.True(t, restored.ExpiresAt.IsZero(), "ExpiresAt should be zero after JSON round-trip")
		assert.True(t, restored.ObtainedAt.IsZero(), "ObtainedAt should be zero after JSON round-trip")

		// Other fields should be preserved
		assert.Equal(t, original.AccessToken, restored.AccessToken)
		assert.Equal(t, original.ExpiresIn, restored.ExpiresIn)
	})

	t.Run("reconstruction of ExpiresAt from ExpiresIn", func(t *testing.T) {
		// Simulate a token retrieved from JSON storage (like Redis)
		// where ExpiresAt/ObtainedAt are zero
		token := &TokenInfo{
			AccessToken: "test-token",
			ExpiresIn:   3600, // 1 hour
			// ExpiresAt and ObtainedAt are zero (as they would be after JSON unmarshal)
		}

		// Verify the token appears expired before reconstruction
		assert.True(t, token.IsExpired(), "Token with zero ExpiresAt should be considered expired")

		// Simulate the reconstruction logic from RedisCacheStore.GetToken
		if token.ObtainedAt.IsZero() {
			token.ObtainedAt = time.Now()
		}
		if token.ExpiresAt.IsZero() && token.ExpiresIn > 0 {
			token.ExpiresAt = token.ObtainedAt.Add(time.Duration(token.ExpiresIn) * time.Second)
		}

		// After reconstruction, token should not be expired
		assert.False(t, token.IsExpired(), "Token should not be expired after ExpiresAt reconstruction")

		// ExpiresAt should be approximately 1 hour from now
		expectedExpiry := time.Now().Add(time.Hour)
		diff := token.ExpiresAt.Sub(expectedExpiry)
		assert.InDelta(t, 0, diff.Seconds(), 1, "ExpiresAt should be approximately 1 hour from now")
	})
}

// newMiniredisStore 创建基于 miniredis 的 RedisCacheStore。
func newMiniredisStore(t *testing.T) (*RedisCacheStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store, err := NewRedisCacheStore(client)
	require.NoError(t, err)
	return store, mr
}

func TestRedisCacheStore_Integration(t *testing.T) {
	ctx := context.Background()

	t.Run("SetToken and GetToken round-trip", func(t *testing.T) {
		store, _ := newMiniredisStore(t)
		token := testToken("redis-token", 3600)

		err := store.SetToken(ctx, "tenant-1", token, time.Hour)
		require.NoError(t, err)

		got, err := store.GetToken(ctx, "tenant-1")
		require.NoError(t, err)
		assert.Equal(t, "redis-token", got.AccessToken)
		assert.Equal(t, int64(3600), got.ExpiresIn)
		assert.False(t, got.ObtainedAt.IsZero(), "ObtainedAt should be reconstructed")
		assert.False(t, got.ExpiresAt.IsZero(), "ExpiresAt should be reconstructed")
	})

	t.Run("GetToken cache miss", func(t *testing.T) {
		store, _ := newMiniredisStore(t)
		_, err := store.GetToken(ctx, "nonexistent")
		assert.ErrorIs(t, err, ErrCacheMiss)
	})

	t.Run("SetToken nil token", func(t *testing.T) {
		store, _ := newMiniredisStore(t)
		err := store.SetToken(ctx, "tenant-1", nil, time.Hour)
		assert.NoError(t, err)
	})

	t.Run("SetToken does not mutate caller", func(t *testing.T) {
		store, _ := newMiniredisStore(t)
		token := testToken("immutable-token", 3600)
		originalUnix := token.ObtainedAtUnix

		err := store.SetToken(ctx, "tenant-1", token, time.Hour)
		require.NoError(t, err)
		assert.Equal(t, originalUnix, token.ObtainedAtUnix, "caller's TokenInfo should not be mutated")
	})

	t.Run("GetPlatformData and SetPlatformData", func(t *testing.T) {
		store, _ := newMiniredisStore(t)

		err := store.SetPlatformData(ctx, "tenant-1", "platform_id", "plat-123", time.Hour)
		require.NoError(t, err)

		value, err := store.GetPlatformData(ctx, "tenant-1", "platform_id")
		require.NoError(t, err)
		assert.Equal(t, "plat-123", value)
	})

	t.Run("GetPlatformData cache miss", func(t *testing.T) {
		store, _ := newMiniredisStore(t)
		_, err := store.GetPlatformData(ctx, "tenant-1", "nonexistent")
		assert.ErrorIs(t, err, ErrCacheMiss)
	})

	t.Run("SetPlatformData without TTL", func(t *testing.T) {
		store, _ := newMiniredisStore(t)
		err := store.SetPlatformData(ctx, "tenant-1", "field", "value", 0)
		require.NoError(t, err)

		value, err := store.GetPlatformData(ctx, "tenant-1", "field")
		require.NoError(t, err)
		assert.Equal(t, "value", value)
	})

	t.Run("Delete removes token and platform data", func(t *testing.T) {
		store, _ := newMiniredisStore(t)

		// Set both token and platform data
		err := store.SetToken(ctx, "tenant-1", testToken("del-token", 3600), time.Hour)
		require.NoError(t, err)
		err = store.SetPlatformData(ctx, "tenant-1", "platform_id", "plat-1", time.Hour)
		require.NoError(t, err)

		// Delete
		err = store.Delete(ctx, "tenant-1")
		require.NoError(t, err)

		// Both should be gone
		_, err = store.GetToken(ctx, "tenant-1")
		assert.ErrorIs(t, err, ErrCacheMiss)
		_, err = store.GetPlatformData(ctx, "tenant-1", "platform_id")
		assert.ErrorIs(t, err, ErrCacheMiss)
	})

	t.Run("DeleteToken removes only token", func(t *testing.T) {
		store, _ := newMiniredisStore(t)

		// Set both token and platform data
		err := store.SetToken(ctx, "tenant-1", testToken("del-token", 3600), time.Hour)
		require.NoError(t, err)
		err = store.SetPlatformData(ctx, "tenant-1", "platform_id", "plat-1", time.Hour)
		require.NoError(t, err)

		// DeleteToken
		err = store.DeleteToken(ctx, "tenant-1")
		require.NoError(t, err)

		// Token should be gone
		_, err = store.GetToken(ctx, "tenant-1")
		assert.ErrorIs(t, err, ErrCacheMiss)

		// Platform data should still exist
		value, err := store.GetPlatformData(ctx, "tenant-1", "platform_id")
		require.NoError(t, err)
		assert.Equal(t, "plat-1", value)
	})

	t.Run("DeletePlatformData removes only platform data", func(t *testing.T) {
		store, _ := newMiniredisStore(t)

		// Set both token and platform data
		err := store.SetToken(ctx, "tenant-1", testToken("keep-token", 3600), time.Hour)
		require.NoError(t, err)
		err = store.SetPlatformData(ctx, "tenant-1", "platform_id", "plat-1", time.Hour)
		require.NoError(t, err)

		// DeletePlatformData
		err = store.DeletePlatformData(ctx, "tenant-1")
		require.NoError(t, err)

		// Platform data should be gone
		_, err = store.GetPlatformData(ctx, "tenant-1", "platform_id")
		assert.ErrorIs(t, err, ErrCacheMiss)

		// Token should still exist
		got, err := store.GetToken(ctx, "tenant-1")
		require.NoError(t, err)
		assert.Equal(t, "keep-token", got.AccessToken)
	})

	t.Run("DeleteToken and DeletePlatformData on Redis error", func(t *testing.T) {
		store, mr := newMiniredisStore(t)
		mr.Close()

		err := store.DeleteToken(ctx, "tenant-1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "redis del token failed")

		err = store.DeletePlatformData(ctx, "tenant-1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "redis del platform data failed")
	})

	t.Run("WithKeyPrefix option", func(t *testing.T) {
		mr := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		store, err := NewRedisCacheStore(client, WithKeyPrefix("myapp:"))
		require.NoError(t, err)

		err = store.SetToken(ctx, "tenant-1", testToken("prefixed", 3600), time.Hour)
		require.NoError(t, err)

		got, err := store.GetToken(ctx, "tenant-1")
		require.NoError(t, err)
		assert.Equal(t, "prefixed", got.AccessToken)

		// Verify the key uses the custom prefix
		assert.Equal(t, "myapp:", store.keyPrefix)
	})

	t.Run("GetToken ObtainedAtUnix reconstruction", func(t *testing.T) {
		store, _ := newMiniredisStore(t)
		token := testToken("unix-token", 3600)
		// ObtainedAt is already set by testToken

		err := store.SetToken(ctx, "tenant-1", token, time.Hour)
		require.NoError(t, err)

		got, err := store.GetToken(ctx, "tenant-1")
		require.NoError(t, err)

		// ObtainedAt should be reconstructed from ObtainedAtUnix
		assert.Equal(t, token.ObtainedAt.Unix(), got.ObtainedAt.Unix())
	})

	t.Run("GetToken corrupted data", func(t *testing.T) {
		store, mr := newMiniredisStore(t)
		// Write invalid JSON directly to Redis
		mr.Set("xauth:token:bad", "not-json")

		_, err := store.GetToken(ctx, "bad")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshal token failed")
	})

	t.Run("Redis error propagation", func(t *testing.T) {
		store, mr := newMiniredisStore(t)
		mr.Close()

		_, err := store.GetToken(ctx, "tenant-1")
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrCacheMiss)

		err = store.SetToken(ctx, "tenant-1", testToken("t", 3600), time.Hour)
		assert.Error(t, err)

		_, err = store.GetPlatformData(ctx, "tenant-1", "field")
		assert.Error(t, err)

		err = store.SetPlatformData(ctx, "tenant-1", "f", "v", time.Hour)
		assert.Error(t, err)

		err = store.Delete(ctx, "tenant-1")
		assert.Error(t, err)
	})
}

func TestTestHelpers(t *testing.T) {
	t.Run("testToken", func(t *testing.T) {
		token := testToken("my-token", 3600)

		assert.Equal(t, "my-token", token.AccessToken)
		assert.Equal(t, "refresh-my-token", token.RefreshToken)
		assert.Equal(t, int64(3600), token.ExpiresIn)
		assert.Equal(t, "bearer", token.TokenType)
		assert.False(t, token.ExpiresAt.IsZero(), "ExpiresAt should be set")
		assert.False(t, token.ObtainedAt.IsZero(), "ObtainedAt should be set")
	})

	t.Run("testConfig", func(t *testing.T) {
		cfg := testConfig()

		assert.NotEmpty(t, cfg.Host, "Host should not be empty")
		assert.NotEmpty(t, cfg.ClientID, "ClientID should not be empty")
		assert.NotEmpty(t, cfg.ClientSecret, "ClientSecret should not be empty")
		assert.NotZero(t, cfg.Timeout, "Timeout should not be zero")
	})
}
