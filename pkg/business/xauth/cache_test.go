package xauth

import (
	"context"
	"encoding/json"
	"testing"
	"time"
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
		if err != nil {
			t.Fatalf("SetToken failed: %v", err)
		}

		// Get token
		got, err := store.GetToken(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("GetToken failed: %v", err)
		}
		if got.AccessToken != token.AccessToken {
			t.Errorf("AccessToken = %q, expected %q", got.AccessToken, token.AccessToken)
		}

		// Get nonexistent
		_, err = store.GetToken(ctx, "nonexistent")
		if err != ErrCacheMiss {
			t.Errorf("expected ErrCacheMiss, got %v", err)
		}

		// Delete
		err = store.Delete(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Verify deleted
		_, err = store.GetToken(ctx, "tenant-1")
		if err != ErrCacheMiss {
			t.Errorf("expected ErrCacheMiss after delete, got %v", err)
		}
	})

	t.Run("platform data operations", func(t *testing.T) {
		// Set platform data
		err := store.SetPlatformData(ctx, "tenant-1", "platform_id", "platform-123", time.Hour)
		if err != nil {
			t.Fatalf("SetPlatformData failed: %v", err)
		}

		// Get platform data
		value, err := store.GetPlatformData(ctx, "tenant-1", "platform_id")
		if err != nil {
			t.Fatalf("GetPlatformData failed: %v", err)
		}
		if value != "platform-123" {
			t.Errorf("value = %q, expected 'platform-123'", value)
		}

		// Get nonexistent field
		_, err = store.GetPlatformData(ctx, "tenant-1", "nonexistent")
		if err != ErrCacheMiss {
			t.Errorf("expected ErrCacheMiss, got %v", err)
		}

		// Get nonexistent tenant
		_, err = store.GetPlatformData(ctx, "nonexistent", "platform_id")
		if err != ErrCacheMiss {
			t.Errorf("expected ErrCacheMiss, got %v", err)
		}
	})

	t.Run("error injection", func(t *testing.T) {
		store := newMockCacheStore()

		// Inject errors
		store.getTokenErr = ErrServerError
		_, err := store.GetToken(ctx, "tenant-1")
		if err != ErrServerError {
			t.Errorf("expected ErrServerError, got %v", err)
		}

		store.setTokenErr = ErrServerError
		err = store.SetToken(ctx, "tenant-1", testToken("token", 3600), time.Hour)
		if err != ErrServerError {
			t.Errorf("expected ErrServerError, got %v", err)
		}

		store.getPlatformErr = ErrServerError
		_, err = store.GetPlatformData(ctx, "tenant-1", "field")
		if err != ErrServerError {
			t.Errorf("expected ErrServerError, got %v", err)
		}

		store.setPlatformErr = ErrServerError
		err = store.SetPlatformData(ctx, "tenant-1", "field", "value", time.Hour)
		if err != ErrServerError {
			t.Errorf("expected ErrServerError, got %v", err)
		}

		store.deleteErr = ErrServerError
		err = store.Delete(ctx, "tenant-1")
		if err != ErrServerError {
			t.Errorf("expected ErrServerError, got %v", err)
		}
	})

	t.Run("call counting", func(t *testing.T) {
		store := newMockCacheStore()

		_ = store.SetToken(ctx, "tenant-1", testToken("token", 3600), time.Hour)
		_ = store.SetToken(ctx, "tenant-2", testToken("token", 3600), time.Hour)
		_, _ = store.GetToken(ctx, "tenant-1")
		_, _ = store.GetToken(ctx, "tenant-1")
		_, _ = store.GetToken(ctx, "tenant-2")

		if store.setTokenCalls != 2 {
			t.Errorf("setTokenCalls = %d, expected 2", store.setTokenCalls)
		}
		if store.getTokenCalls != 3 {
			t.Errorf("getTokenCalls = %d, expected 3", store.getTokenCalls)
		}
	})
}

func TestMockClient(t *testing.T) {
	ctx := context.Background()
	mc := newMockClient()

	t.Run("GetToken", func(t *testing.T) {
		// Default behavior
		token, err := mc.GetToken(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("GetToken failed: %v", err)
		}
		if token != "mock-token-tenant-1" {
			t.Errorf("token = %q, expected 'mock-token-tenant-1'", token)
		}

		// With preset value
		mc.tokens["tenant-2"] = "custom-token"
		token, err = mc.GetToken(ctx, "tenant-2")
		if err != nil {
			t.Fatalf("GetToken failed: %v", err)
		}
		if token != "custom-token" {
			t.Errorf("token = %q, expected 'custom-token'", token)
		}

		// With error
		mc.getTokenErr = ErrTokenNotFound
		_, err = mc.GetToken(ctx, "tenant-1")
		if err != ErrTokenNotFound {
			t.Errorf("expected ErrTokenNotFound, got %v", err)
		}
	})

	t.Run("VerifyToken", func(t *testing.T) {
		mc := newMockClient()

		// Default behavior
		info, err := mc.VerifyToken(ctx, "test-token")
		if err != nil {
			t.Fatalf("VerifyToken failed: %v", err)
		}
		if info.AccessToken != "test-token" {
			t.Errorf("AccessToken = %q, expected 'test-token'", info.AccessToken)
		}

		// With preset value
		mc.verifyData["custom-token"] = testToken("custom-verified", 3600)
		info, err = mc.VerifyToken(ctx, "custom-token")
		if err != nil {
			t.Fatalf("VerifyToken failed: %v", err)
		}
		if info.AccessToken != "custom-verified" {
			t.Errorf("AccessToken = %q, expected 'custom-verified'", info.AccessToken)
		}

		// With error
		mc.verifyTokenErr = ErrTokenInvalid
		_, err = mc.VerifyToken(ctx, "test-token")
		if err != ErrTokenInvalid {
			t.Errorf("expected ErrTokenInvalid, got %v", err)
		}
	})

	t.Run("GetPlatformID", func(t *testing.T) {
		mc := newMockClient()

		// Default behavior
		id, err := mc.GetPlatformID(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("GetPlatformID failed: %v", err)
		}
		if id != "platform-tenant-1" {
			t.Errorf("id = %q, expected 'platform-tenant-1'", id)
		}

		// With error
		mc.getPlatformIDErr = ErrPlatformIDNotFound
		_, err = mc.GetPlatformID(ctx, "tenant-1")
		if err != ErrPlatformIDNotFound {
			t.Errorf("expected ErrPlatformIDNotFound, got %v", err)
		}
	})

	t.Run("HasParentPlatform", func(t *testing.T) {
		mc := newMockClient()

		// Default behavior (false)
		hasParent, err := mc.HasParentPlatform(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("HasParentPlatform failed: %v", err)
		}
		if hasParent {
			t.Error("expected hasParent to be false by default")
		}

		// With preset value
		mc.hasParent["tenant-2"] = true
		hasParent, err = mc.HasParentPlatform(ctx, "tenant-2")
		if err != nil {
			t.Fatalf("HasParentPlatform failed: %v", err)
		}
		if !hasParent {
			t.Error("expected hasParent to be true")
		}
	})

	t.Run("GetUnclassRegionID", func(t *testing.T) {
		mc := newMockClient()

		// Default behavior
		id, err := mc.GetUnclassRegionID(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("GetUnclassRegionID failed: %v", err)
		}
		if id != "region-tenant-1" {
			t.Errorf("id = %q, expected 'region-tenant-1'", id)
		}
	})

	t.Run("Request", func(t *testing.T) {
		mc := newMockClient()

		// Default behavior
		err := mc.Request(ctx, &AuthRequest{})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// With error
		mc.requestErr = ErrRequestFailed
		err = mc.Request(ctx, &AuthRequest{})
		if err != ErrRequestFailed {
			t.Errorf("expected ErrRequestFailed, got %v", err)
		}
	})

	t.Run("Close", func(t *testing.T) {
		mc := newMockClient()

		if mc.closed {
			t.Error("should not be closed initially")
		}

		err := mc.Close()
		if err != nil {
			t.Errorf("Close failed: %v", err)
		}

		if !mc.closed {
			t.Error("should be closed after Close()")
		}
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
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		// Unmarshal back
		var restored TokenInfo
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		// ObtainedAtUnix should be preserved
		if restored.ObtainedAtUnix != original.ObtainedAtUnix {
			t.Errorf("ObtainedAtUnix = %d, expected %d", restored.ObtainedAtUnix, original.ObtainedAtUnix)
		}

		// Simulate RedisCacheStore.GetToken logic
		if restored.ObtainedAtUnix > 0 {
			restored.ObtainedAt = time.Unix(restored.ObtainedAtUnix, 0)
		}
		if restored.ExpiresAt.IsZero() && restored.ExpiresIn > 0 {
			restored.ExpiresAt = restored.ObtainedAt.Add(time.Duration(restored.ExpiresIn) * time.Second)
		}

		// ObtainedAt should be restored to the original time
		if restored.ObtainedAt.Unix() != obtainedAt.Unix() {
			t.Errorf("ObtainedAt = %v, expected %v", restored.ObtainedAt, obtainedAt)
		}

		// ExpiresAt should be 30 minutes from now (1 hour from obtainedAt)
		expectedExpiresAt := obtainedAt.Add(time.Hour)
		if restored.ExpiresAt.Unix() != expectedExpiresAt.Unix() {
			t.Errorf("ExpiresAt = %v, expected %v", restored.ExpiresAt, expectedExpiresAt)
		}
	})

	t.Run("missing ObtainedAtUnix fallback", func(t *testing.T) {
		// 缺少 ObtainedAtUnix 字段的数据
		jsonData := `{"access_token":"old-token","expires_in":3600}`

		var token TokenInfo
		if err := json.Unmarshal([]byte(jsonData), &token); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		// ObtainedAtUnix should be 0
		if token.ObtainedAtUnix != 0 {
			t.Errorf("ObtainedAtUnix = %d, expected 0", token.ObtainedAtUnix)
		}

		// 模拟 RedisCacheStore.GetToken 的容错逻辑
		if token.ObtainedAtUnix > 0 {
			token.ObtainedAt = time.Unix(token.ObtainedAtUnix, 0)
		} else if token.ObtainedAt.IsZero() {
			token.ObtainedAt = time.Now()
		}

		// ObtainedAt should be set to current time (fallback)
		if token.ObtainedAt.IsZero() {
			t.Error("ObtainedAt should be set via fallback")
		}
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
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		// Unmarshal back
		var restored TokenInfo
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		// ExpiresAt and ObtainedAt should be zero after JSON round-trip
		if !restored.ExpiresAt.IsZero() {
			t.Error("ExpiresAt should be zero after JSON round-trip")
		}
		if !restored.ObtainedAt.IsZero() {
			t.Error("ObtainedAt should be zero after JSON round-trip")
		}

		// Other fields should be preserved
		if restored.AccessToken != original.AccessToken {
			t.Errorf("AccessToken = %q, expected %q", restored.AccessToken, original.AccessToken)
		}
		if restored.ExpiresIn != original.ExpiresIn {
			t.Errorf("ExpiresIn = %d, expected %d", restored.ExpiresIn, original.ExpiresIn)
		}
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
		if !token.IsExpired() {
			t.Error("Token with zero ExpiresAt should be considered expired")
		}

		// Simulate the reconstruction logic from RedisCacheStore.GetToken
		if token.ObtainedAt.IsZero() {
			token.ObtainedAt = time.Now()
		}
		if token.ExpiresAt.IsZero() && token.ExpiresIn > 0 {
			token.ExpiresAt = token.ObtainedAt.Add(time.Duration(token.ExpiresIn) * time.Second)
		}

		// After reconstruction, token should not be expired
		if token.IsExpired() {
			t.Error("Token should not be expired after ExpiresAt reconstruction")
		}

		// ExpiresAt should be approximately 1 hour from now
		expectedExpiry := time.Now().Add(time.Hour)
		diff := token.ExpiresAt.Sub(expectedExpiry)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("ExpiresAt = %v, expected approximately %v", token.ExpiresAt, expectedExpiry)
		}
	})
}

func TestTestHelpers(t *testing.T) {
	t.Run("testToken", func(t *testing.T) {
		token := testToken("my-token", 3600)

		if token.AccessToken != "my-token" {
			t.Errorf("AccessToken = %q, expected 'my-token'", token.AccessToken)
		}
		if token.RefreshToken != "refresh-my-token" {
			t.Errorf("RefreshToken = %q, expected 'refresh-my-token'", token.RefreshToken)
		}
		if token.ExpiresIn != 3600 {
			t.Errorf("ExpiresIn = %d, expected 3600", token.ExpiresIn)
		}
		if token.TokenType != "bearer" {
			t.Errorf("TokenType = %q, expected 'bearer'", token.TokenType)
		}
		if token.ExpiresAt.IsZero() {
			t.Error("ExpiresAt should be set")
		}
		if token.ObtainedAt.IsZero() {
			t.Error("ObtainedAt should be set")
		}
	})

	t.Run("testConfig", func(t *testing.T) {
		cfg := testConfig()

		if cfg.Host == "" {
			t.Error("Host should not be empty")
		}
		if cfg.ClientID == "" {
			t.Error("ClientID should not be empty")
		}
		if cfg.ClientSecret == "" {
			t.Error("ClientSecret should not be empty")
		}
		if cfg.Timeout == 0 {
			t.Error("Timeout should not be zero")
		}
	})
}
