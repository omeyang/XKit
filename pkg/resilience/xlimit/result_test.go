package xlimit

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestResult_Basic(t *testing.T) {
	result := &Result{
		Allowed:    true,
		Limit:      100,
		Remaining:  50,
		ResetAt:    time.Now().Add(time.Minute),
		RetryAfter: 0,
		Rule:       "tenant-limit",
		Key:        "tenant:abc123",
	}

	t.Run("Allowed is true", func(t *testing.T) {
		if !result.Allowed {
			t.Error("expected Allowed to be true")
		}
	})

	t.Run("Remaining is set", func(t *testing.T) {
		if result.Remaining != 50 {
			t.Errorf("expected Remaining=50, got %d", result.Remaining)
		}
	})
}

func TestResult_Denied(t *testing.T) {
	result := &Result{
		Allowed:    false,
		Limit:      100,
		Remaining:  0,
		RetryAfter: 30 * time.Second,
		Rule:       "api-limit",
		Key:        "api:POST:/v1/users",
	}

	if result.Allowed {
		t.Error("expected Allowed to be false")
	}

	if result.RetryAfter != 30*time.Second {
		t.Errorf("expected RetryAfter=30s, got %v", result.RetryAfter)
	}
}

func TestResult_Headers(t *testing.T) {
	resetAt := time.Now().Add(time.Minute)
	result := &Result{
		Allowed:    true,
		Limit:      100,
		Remaining:  42,
		ResetAt:    resetAt,
		RetryAfter: 0,
	}

	t.Run("Headers returns correct headers", func(t *testing.T) {
		headers := result.Headers()

		if headers["X-RateLimit-Limit"] != "100" {
			t.Errorf("expected X-RateLimit-Limit=100, got %s", headers["X-RateLimit-Limit"])
		}

		if headers["X-RateLimit-Remaining"] != "42" {
			t.Errorf("expected X-RateLimit-Remaining=42, got %s", headers["X-RateLimit-Remaining"])
		}

		if _, ok := headers["X-RateLimit-Reset"]; !ok {
			t.Error("expected X-RateLimit-Reset header")
		}
	})

	t.Run("Headers with RetryAfter", func(t *testing.T) {
		deniedResult := &Result{
			Allowed:    false,
			Limit:      100,
			Remaining:  0,
			RetryAfter: 30 * time.Second,
		}

		headers := deniedResult.Headers()

		if headers["Retry-After"] != "30" {
			t.Errorf("expected Retry-After=30, got %s", headers["Retry-After"])
		}
	})
}

func TestResult_SetHeaders(t *testing.T) {
	result := &Result{
		Allowed:    false,
		Limit:      100,
		Remaining:  0,
		ResetAt:    time.Now().Add(time.Minute),
		RetryAfter: 30 * time.Second,
	}

	recorder := httptest.NewRecorder()
	result.SetHeaders(recorder)

	resp := recorder.Result()
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // defer cleanup

	if resp.Header.Get("X-RateLimit-Limit") != "100" {
		t.Errorf("expected X-RateLimit-Limit=100, got %s", resp.Header.Get("X-RateLimit-Limit"))
	}

	if resp.Header.Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("expected X-RateLimit-Remaining=0, got %s", resp.Header.Get("X-RateLimit-Remaining"))
	}

	if resp.Header.Get("Retry-After") != "30" {
		t.Errorf("expected Retry-After=30, got %s", resp.Header.Get("Retry-After"))
	}
}

func TestResult_SetHeaders_SkipsWhenNoQuota(t *testing.T) {
	// FG-M1: FallbackOpen 或无匹配规则时 Limit=0，不应写入误导性的配额头
	result := &Result{
		Allowed: true,
		Limit:   0,
		Rule:    "fallback-open",
	}

	recorder := httptest.NewRecorder()
	result.SetHeaders(recorder)

	if recorder.Header().Get("X-RateLimit-Limit") != "" {
		t.Error("should not set X-RateLimit-Limit when Limit=0")
	}
	if recorder.Header().Get("X-RateLimit-Remaining") != "" {
		t.Error("should not set X-RateLimit-Remaining when Limit=0")
	}
	if recorder.Header().Get("X-RateLimit-Reset") != "" {
		t.Error("should not set X-RateLimit-Reset when Limit=0")
	}
}

func TestAllowedResult(t *testing.T) {
	result := AllowedResult(100, 99)

	if !result.Allowed {
		t.Error("expected Allowed to be true")
	}

	if result.Limit != 100 {
		t.Errorf("expected Limit=100, got %d", result.Limit)
	}

	if result.Remaining != 99 {
		t.Errorf("expected Remaining=99, got %d", result.Remaining)
	}
}

func TestDeniedResult(t *testing.T) {
	retryAfter := 10 * time.Second
	result := DeniedResult(100, retryAfter, "test-rule", "test-key")

	if result.Allowed {
		t.Error("expected Allowed to be false")
	}

	if result.Limit != 100 {
		t.Errorf("expected Limit=100, got %d", result.Limit)
	}

	if result.Remaining != 0 {
		t.Errorf("expected Remaining=0, got %d", result.Remaining)
	}

	if result.RetryAfter != retryAfter {
		t.Errorf("expected RetryAfter=%v, got %v", retryAfter, result.RetryAfter)
	}

	if result.Rule != "test-rule" {
		t.Errorf("expected Rule=test-rule, got %s", result.Rule)
	}

	if result.Key != "test-key" {
		t.Errorf("expected Key=test-key, got %s", result.Key)
	}
}
