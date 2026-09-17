package xauth

import (
	"context"
	"testing"
	"time"
)

func TestTokenInfo_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		token    *TokenInfo
		expected bool
	}{
		{
			name:     "nil token",
			token:    nil,
			expected: true,
		},
		{
			name:     "empty access token",
			token:    &TokenInfo{ExpiresAt: time.Now().Add(time.Hour)},
			expected: true,
		},
		{
			name: "expired token",
			token: &TokenInfo{
				AccessToken: "test-token",
				ExpiresAt:   time.Now().Add(-time.Hour),
			},
			expected: true,
		},
		{
			name: "valid token",
			token: &TokenInfo{
				AccessToken: "test-token",
				ExpiresAt:   time.Now().Add(time.Hour),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.token.IsExpired()
			if result != tt.expected {
				t.Errorf("IsExpired() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestTokenInfo_IsExpiringSoon(t *testing.T) {
	tests := []struct {
		name      string
		token     *TokenInfo
		threshold time.Duration
		expected  bool
	}{
		{
			name:      "nil token",
			token:     nil,
			threshold: 5 * time.Minute,
			expected:  true,
		},
		{
			name:      "empty access token",
			token:     &TokenInfo{ExpiresAt: time.Now().Add(time.Hour)},
			threshold: 5 * time.Minute,
			expected:  true,
		},
		{
			name: "expiring soon",
			token: &TokenInfo{
				AccessToken: "test-token",
				ExpiresAt:   time.Now().Add(3 * time.Minute),
			},
			threshold: 5 * time.Minute,
			expected:  true,
		},
		{
			name: "not expiring soon",
			token: &TokenInfo{
				AccessToken: "test-token",
				ExpiresAt:   time.Now().Add(time.Hour),
			},
			threshold: 5 * time.Minute,
			expected:  false,
		},
		{
			name: "exactly at threshold",
			token: &TokenInfo{
				AccessToken: "test-token",
				ExpiresAt:   time.Now().Add(5 * time.Minute),
			},
			threshold: 5 * time.Minute,
			expected:  true, // >= threshold means expiring soon
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.token.IsExpiringSoon(tt.threshold)
			if result != tt.expected {
				t.Errorf("IsExpiringSoon(%v) = %v, expected %v", tt.threshold, result, tt.expected)
			}
		})
	}
}

func TestTokenInfo_TTL(t *testing.T) {
	tests := []struct {
		name        string
		token       *TokenInfo
		expectedMin time.Duration
		expectedMax time.Duration
	}{
		{
			name:        "nil token",
			token:       nil,
			expectedMin: 0,
			expectedMax: 0,
		},
		{
			name: "expired token",
			token: &TokenInfo{
				AccessToken: "test-token",
				ExpiresAt:   time.Now().Add(-time.Hour),
			},
			expectedMin: 0,
			expectedMax: 0,
		},
		{
			name: "valid token",
			token: &TokenInfo{
				AccessToken: "test-token",
				ExpiresAt:   time.Now().Add(time.Hour),
			},
			expectedMin: 59 * time.Minute,
			expectedMax: 61 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.token.TTL()
			if result < tt.expectedMin || result > tt.expectedMax {
				t.Errorf("TTL() = %v, expected between %v and %v", result, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

func TestNoopCacheStore(t *testing.T) {
	store := NoopCacheStore{}
	ctx := testContext()

	t.Run("GetToken returns cache miss", func(t *testing.T) {
		_, err := store.GetToken(ctx, "tenant-1")
		if err != ErrCacheMiss {
			t.Errorf("expected ErrCacheMiss, got %v", err)
		}
	})

	t.Run("SetToken does nothing", func(t *testing.T) {
		err := store.SetToken(ctx, "tenant-1", testToken("test", 3600), time.Hour)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("GetPlatformData returns cache miss", func(t *testing.T) {
		_, err := store.GetPlatformData(ctx, "tenant-1", "platform_id")
		if err != ErrCacheMiss {
			t.Errorf("expected ErrCacheMiss, got %v", err)
		}
	})

	t.Run("SetPlatformData does nothing", func(t *testing.T) {
		err := store.SetPlatformData(ctx, "tenant-1", "platform_id", "value", time.Hour)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Delete does nothing", func(t *testing.T) {
		err := store.Delete(ctx, "tenant-1")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// testContext creates a test context
func testContext() context.Context {
	return context.Background()
}
