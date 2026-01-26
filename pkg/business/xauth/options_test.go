package xauth

import (
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"
)

func TestWithCache(t *testing.T) {
	mockCache := newMockCacheStore()
	opt := WithCache(mockCache)

	options := &Options{}
	opt(options)

	if options.Cache != mockCache {
		t.Error("Cache option not applied")
	}
}

func TestWithLogger(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	opt := WithLogger(logger)

	options := &Options{}
	opt(options)

	if options.Logger != logger {
		t.Error("Logger option not applied")
	}
}

func TestWithObserver(t *testing.T) {
	observer := xmetrics.NoopObserver{}
	opt := WithObserver(observer)

	options := &Options{}
	opt(options)

	if options.Observer != observer {
		t.Error("Observer option not applied")
	}
}

func TestWithHTTPClient(t *testing.T) {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	opt := WithHTTPClient(httpClient)

	options := &Options{}
	opt(options)

	if options.HTTPClient != httpClient {
		t.Error("HTTPClient option not applied")
	}
}

func TestWithLocalCache(t *testing.T) {
	t.Run("enable", func(t *testing.T) {
		opt := WithLocalCache(true)

		options := &Options{}
		opt(options)

		if !options.EnableLocalCache {
			t.Error("EnableLocalCache should be true")
		}
	})

	t.Run("disable", func(t *testing.T) {
		opt := WithLocalCache(false)

		options := &Options{EnableLocalCache: true}
		opt(options)

		if options.EnableLocalCache {
			t.Error("EnableLocalCache should be false")
		}
	})
}

func TestWithLocalCacheMaxSize(t *testing.T) {
	opt := WithLocalCacheMaxSize(1000)

	options := &Options{}
	opt(options)

	if options.LocalCacheMaxSize != 1000 {
		t.Errorf("LocalCacheMaxSize = %d, expected 1000", options.LocalCacheMaxSize)
	}
}

func TestWithSingleflight(t *testing.T) {
	t.Run("enable", func(t *testing.T) {
		opt := WithSingleflight(true)

		options := &Options{}
		opt(options)

		if !options.EnableSingleflight {
			t.Error("EnableSingleflight should be true")
		}
	})

	t.Run("disable", func(t *testing.T) {
		opt := WithSingleflight(false)

		options := &Options{EnableSingleflight: true}
		opt(options)

		if options.EnableSingleflight {
			t.Error("EnableSingleflight should be false")
		}
	})
}

func TestWithBackgroundRefresh(t *testing.T) {
	t.Run("enable", func(t *testing.T) {
		opt := WithBackgroundRefresh(true)

		options := &Options{}
		opt(options)

		if !options.EnableBackgroundRefresh {
			t.Error("EnableBackgroundRefresh should be true")
		}
	})

	t.Run("disable", func(t *testing.T) {
		opt := WithBackgroundRefresh(false)

		options := &Options{EnableBackgroundRefresh: true}
		opt(options)

		if options.EnableBackgroundRefresh {
			t.Error("EnableBackgroundRefresh should be false")
		}
	})
}

func TestWithTokenRefreshThreshold(t *testing.T) {
	threshold := 10 * time.Minute
	opt := WithTokenRefreshThreshold(threshold)

	options := &Options{}
	opt(options)

	if options.TokenRefreshThreshold != threshold {
		t.Errorf("TokenRefreshThreshold = %v, expected %v", options.TokenRefreshThreshold, threshold)
	}
}

func TestWithPlatformDataCacheTTL(t *testing.T) {
	ttl := 30 * time.Minute
	opt := WithPlatformDataCacheTTL(ttl)

	options := &Options{}
	opt(options)

	if options.PlatformDataCacheTTL != ttl {
		t.Errorf("PlatformDataCacheTTL = %v, expected %v", options.PlatformDataCacheTTL, ttl)
	}
}

func TestApplyOptions(t *testing.T) {
	t.Run("no options", func(t *testing.T) {
		options := applyOptions(nil)
		if options == nil {
			t.Error("options should not be nil")
		}
	})

	t.Run("multiple options", func(t *testing.T) {
		mockCache := newMockCacheStore()
		logger := slog.Default()

		options := applyOptions([]Option{
			WithCache(mockCache),
			WithLogger(logger),
			WithLocalCache(true),
			WithSingleflight(true),
		})

		if options.Cache != mockCache {
			t.Error("Cache not set")
		}
		if options.Logger != logger {
			t.Error("Logger not set")
		}
		if !options.EnableLocalCache {
			t.Error("EnableLocalCache not set")
		}
		if !options.EnableSingleflight {
			t.Error("EnableSingleflight not set")
		}
	})
}

func TestWithAutoRetryOn401(t *testing.T) {
	t.Run("enable", func(t *testing.T) {
		opt := WithAutoRetryOn401(true)

		options := &Options{}
		opt(options)

		if !options.EnableAutoRetryOn401 {
			t.Error("EnableAutoRetryOn401 should be true")
		}
	})

	t.Run("disable", func(t *testing.T) {
		opt := WithAutoRetryOn401(false)

		options := &Options{EnableAutoRetryOn401: true}
		opt(options)

		if options.EnableAutoRetryOn401 {
			t.Error("EnableAutoRetryOn401 should be false")
		}
	})
}

func TestDefaultOptions(t *testing.T) {
	options := applyOptions(nil)

	if options == nil {
		t.Fatal("applyOptions should not return nil")
	}

	// Verify default values
	if !options.EnableLocalCache {
		t.Error("EnableLocalCache should be true by default")
	}
	if !options.EnableSingleflight {
		t.Error("EnableSingleflight should be true by default")
	}
	if options.LocalCacheMaxSize != 1000 {
		t.Errorf("LocalCacheMaxSize = %d, expected 1000", options.LocalCacheMaxSize)
	}
	if !options.EnableBackgroundRefresh {
		t.Error("EnableBackgroundRefresh should be true by default")
	}
	if options.EnableAutoRetryOn401 {
		t.Error("EnableAutoRetryOn401 should be false by default")
	}
}
