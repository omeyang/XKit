package xlimit

import (
	"testing"
	"time"
)

func TestWithRules(t *testing.T) {
	rules := []Rule{
		TenantRule("tenant-limit", 1000, time.Minute),
	}

	opts := defaultOptions()
	WithRules(rules...)(opts)

	if len(opts.config.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(opts.config.Rules))
	}
	if opts.config.Rules[0].Name != "tenant-limit" {
		t.Errorf("expected rule name tenant-limit, got %s", opts.config.Rules[0].Name)
	}
}

func TestWithKeyPrefix(t *testing.T) {
	opts := defaultOptions()
	WithKeyPrefix("custom:")(opts)

	if opts.config.KeyPrefix != "custom:" {
		t.Errorf("expected prefix custom:, got %s", opts.config.KeyPrefix)
	}
}

func TestWithFallback(t *testing.T) {
	opts := defaultOptions()
	WithFallback(FallbackOpen)(opts)

	if opts.config.Fallback != FallbackOpen {
		t.Errorf("expected fallback fail-open, got %s", opts.config.Fallback)
	}
}

func TestWithPodCount(t *testing.T) {
	opts := defaultOptions()
	WithPodCount(5)(opts)

	if opts.config.LocalPodCount != 5 {
		t.Errorf("expected pod count 5, got %d", opts.config.LocalPodCount)
	}
}

func TestWithMetrics(t *testing.T) {
	opts := defaultOptions()
	WithMetrics(false)(opts)

	if opts.config.EnableMetrics {
		t.Error("expected metrics disabled")
	}
}

func TestWithHeaders(t *testing.T) {
	opts := defaultOptions()
	WithHeaders(false)(opts)

	if opts.config.EnableHeaders {
		t.Error("expected headers disabled")
	}
}

func TestWithConfig(t *testing.T) {
	config := Config{
		KeyPrefix:     "myprefix:",
		Rules:         []Rule{TenantRule("test", 100, time.Second)},
		Fallback:      FallbackClose,
		LocalPodCount: 3,
	}

	opts := defaultOptions()
	WithConfig(config)(opts)

	if opts.config.KeyPrefix != "myprefix:" {
		t.Errorf("expected prefix myprefix:, got %s", opts.config.KeyPrefix)
	}
	if opts.config.Fallback != FallbackClose {
		t.Errorf("expected fallback fail-close, got %s", opts.config.Fallback)
	}
}

func TestWithOnAllow(t *testing.T) {
	called := false
	callback := func(key Key, result *Result) {
		called = true
	}

	opts := defaultOptions()
	WithOnAllow(callback)(opts)

	if opts.onAllow == nil {
		t.Error("expected onAllow callback set")
	}

	// 调用回调
	opts.onAllow(Key{}, &Result{})
	if !called {
		t.Error("expected callback to be called")
	}
}

func TestWithOnDeny(t *testing.T) {
	called := false
	callback := func(key Key, result *Result) {
		called = true
	}

	opts := defaultOptions()
	WithOnDeny(callback)(opts)

	if opts.onDeny == nil {
		t.Error("expected onDeny callback set")
	}

	// 调用回调
	opts.onDeny(Key{}, &Result{})
	if !called {
		t.Error("expected callback to be called")
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()

	if opts.config.KeyPrefix != "ratelimit:" {
		t.Errorf("expected default prefix ratelimit:, got %s", opts.config.KeyPrefix)
	}
	if opts.config.Fallback != FallbackLocal {
		t.Errorf("expected default fallback local, got %s", opts.config.Fallback)
	}
	if opts.config.LocalPodCount != 1 {
		t.Errorf("expected default pod count 1, got %d", opts.config.LocalPodCount)
	}
}

func TestOptionChaining(t *testing.T) {
	opts := defaultOptions()

	WithKeyPrefix("test:")(opts)
	WithRules(TenantRule("t1", 100, time.Second))(opts)
	WithFallback(FallbackOpen)(opts)
	WithPodCount(3)(opts)
	WithMetrics(false)(opts)
	WithHeaders(false)(opts)

	if opts.config.KeyPrefix != "test:" {
		t.Error("KeyPrefix not set correctly")
	}
	if len(opts.config.Rules) != 1 {
		t.Error("Rules not set correctly")
	}
	if opts.config.Fallback != FallbackOpen {
		t.Error("Fallback not set correctly")
	}
	if opts.config.LocalPodCount != 3 {
		t.Error("LocalPodCount not set correctly")
	}
	if opts.config.EnableMetrics {
		t.Error("EnableMetrics not set correctly")
	}
	if opts.config.EnableHeaders {
		t.Error("EnableHeaders not set correctly")
	}
}
