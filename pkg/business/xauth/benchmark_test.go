package xauth

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func BenchmarkTokenCache_Get(b *testing.B) {
	ctx := context.Background()
	cache := NewTokenCache(TokenCacheConfig{
		EnableLocal:        true,
		MaxLocalSize:       1000,
		EnableSingleflight: true,
	})

	// Pre-populate cache
	for i := range 100 {
		tenantID := "tenant-" + strconv.Itoa(i)
		_ = cache.Set(ctx, tenantID, testToken("token-"+tenantID, 3600), time.Hour)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			tenantID := "tenant-" + strconv.Itoa(i%100)
			_, _, _ = cache.Get(ctx, tenantID)
			i++
		}
	})
}

func BenchmarkTokenCache_Set(b *testing.B) {
	ctx := context.Background()
	cache := NewTokenCache(TokenCacheConfig{
		EnableLocal:  true,
		MaxLocalSize: 10000,
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			tenantID := "tenant-" + strconv.Itoa(i%1000)
			_ = cache.Set(ctx, tenantID, testToken("token-"+tenantID, 3600), time.Hour)
			i++
		}
	})
}

func BenchmarkTokenCache_GetOrLoad_CacheHit(b *testing.B) {
	ctx := context.Background()
	cache := NewTokenCache(TokenCacheConfig{
		EnableLocal:        true,
		EnableSingleflight: true,
	})

	// Pre-populate cache
	_ = cache.Set(ctx, "tenant-1", testToken("cached-token", 3600), time.Hour)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = cache.GetOrLoad(ctx, "tenant-1", func(ctx context.Context) (*TokenInfo, error) {
				return testToken("loaded-token", 3600), nil
			}, time.Hour)
		}
	})
}

func BenchmarkTokenInfo_IsExpired(b *testing.B) {
	token := testToken("test-token", 3600)

	b.ResetTimer()
	for b.Loop() {
		_ = token.IsExpired()
	}
}

func BenchmarkTokenInfo_IsExpiringSoon(b *testing.B) {
	token := testToken("test-token", 3600)
	threshold := 5 * time.Minute

	b.ResetTimer()
	for b.Loop() {
		_ = token.IsExpiringSoon(threshold)
	}
}

func BenchmarkConfig_Validate(b *testing.B) {
	cfg := &Config{
		Host:                  "https://auth.test.com",
		ClientID:              "test-client",
		ClientSecret:          "test-secret",
		Timeout:               15 * time.Second,
		TokenRefreshThreshold: 5 * time.Minute,
	}

	b.ResetTimer()
	for b.Loop() {
		_ = cfg.Validate()
	}
}

func BenchmarkConfig_Clone(b *testing.B) {
	cfg := &Config{
		Host:                  "https://auth.test.com",
		ClientID:              "test-client",
		ClientSecret:          "test-secret",
		Timeout:               15 * time.Second,
		TokenRefreshThreshold: 5 * time.Minute,
		TLS: &TLSConfig{
			InsecureSkipVerify: true,
		},
	}

	b.ResetTimer()
	for b.Loop() {
		_ = cfg.Clone()
	}
}

func BenchmarkIsRetryable(b *testing.B) {
	errs := []error{
		nil,
		ErrServerError,
		ErrUnauthorized,
		NewTemporaryError(ErrRequestFailed),
		NewPermanentError(ErrTokenInvalid),
	}

	b.ResetTimer()
	for b.Loop() {
		for _, err := range errs {
			_ = IsRetryable(err)
		}
	}
}

func BenchmarkMockCacheStore_GetToken(b *testing.B) {
	ctx := context.Background()
	store := newMockCacheStore()

	// Pre-populate
	for i := range 100 {
		tenantID := "tenant-" + strconv.Itoa(i)
		_ = store.SetToken(ctx, tenantID, testToken("token-"+tenantID, 3600), time.Hour)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			tenantID := "tenant-" + strconv.Itoa(i%100)
			_, _ = store.GetToken(ctx, tenantID)
			i++
		}
	})
}
