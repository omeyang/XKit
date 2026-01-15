package xauth

import (
	"errors"
	"testing"
)

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "server error",
			err:      ErrServerError,
			expected: true,
		},
		{
			name:     "request failed",
			err:      ErrRequestFailed,
			expected: true,
		},
		{
			name:     "unauthorized",
			err:      ErrUnauthorized,
			expected: false,
		},
		{
			name:     "forbidden",
			err:      ErrForbidden,
			expected: false,
		},
		{
			name:     "token invalid",
			err:      ErrTokenInvalid,
			expected: false,
		},
		{
			name:     "temporary error",
			err:      NewTemporaryError(errors.New("temp")),
			expected: true,
		},
		{
			name:     "permanent error",
			err:      NewPermanentError(errors.New("perm")),
			expected: false,
		},
		{
			name:     "wrapped server error",
			err:      errors.Join(errors.New("context"), ErrServerError),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("IsRetryable(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsPermanent(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "server error - not permanent",
			err:      ErrServerError,
			expected: false,
		},
		{
			name:     "unauthorized - permanent",
			err:      ErrUnauthorized,
			expected: true,
		},
		{
			name:     "permanent error wrapper",
			err:      NewPermanentError(errors.New("perm")),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPermanent(tt.err)
			if result != tt.expected {
				t.Errorf("IsPermanent(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestTemporaryError(t *testing.T) {
	underlying := errors.New("network timeout")
	err := NewTemporaryError(underlying)

	t.Run("Error", func(t *testing.T) {
		if err.Error() != "network timeout" {
			t.Errorf("Error() = %q, expected %q", err.Error(), "network timeout")
		}
	})

	t.Run("Unwrap", func(t *testing.T) {
		if !errors.Is(err, underlying) {
			t.Error("Unwrap() should return underlying error")
		}
	})

	t.Run("Retryable", func(t *testing.T) {
		if !err.Retryable() {
			t.Error("Retryable() should return true")
		}
	})

	t.Run("nil underlying", func(t *testing.T) {
		nilErr := NewTemporaryError(nil)
		if nilErr.Error() != "xauth: temporary error" {
			t.Errorf("Error() with nil = %q", nilErr.Error())
		}
	})
}

func TestPermanentError(t *testing.T) {
	underlying := errors.New("invalid credentials")
	err := NewPermanentError(underlying)

	t.Run("Error", func(t *testing.T) {
		if err.Error() != "invalid credentials" {
			t.Errorf("Error() = %q, expected %q", err.Error(), "invalid credentials")
		}
	})

	t.Run("Unwrap", func(t *testing.T) {
		if !errors.Is(err, underlying) {
			t.Error("Unwrap() should return underlying error")
		}
	})

	t.Run("Retryable", func(t *testing.T) {
		if err.Retryable() {
			t.Error("Retryable() should return false")
		}
	})

	t.Run("nil underlying", func(t *testing.T) {
		nilErr := NewPermanentError(nil)
		if nilErr.Error() != "xauth: permanent error" {
			t.Errorf("Error() with nil = %q", nilErr.Error())
		}
	})
}

func TestAPIError(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		err := NewAPIError(500, 1001, "internal server error")
		expected := "xauth: api error: status=500, code=1001, message=internal server error"
		if err.Error() != expected {
			t.Errorf("Error() = %q, expected %q", err.Error(), expected)
		}
	})

	t.Run("empty message", func(t *testing.T) {
		err := NewAPIError(404, 0, "")
		expected := "xauth: api error: status=404, code=0"
		if err.Error() != expected {
			t.Errorf("Error() = %q, expected %q", err.Error(), expected)
		}
	})

	t.Run("retryable - 5xx", func(t *testing.T) {
		err := NewAPIError(500, 0, "")
		if !err.Retryable() {
			t.Error("5xx error should be retryable")
		}
	})

	t.Run("not retryable - 4xx", func(t *testing.T) {
		err := NewAPIError(400, 0, "")
		if err.Retryable() {
			t.Error("4xx error should not be retryable")
		}
	})

	t.Run("Is - unauthorized", func(t *testing.T) {
		err := NewAPIError(401, 0, "")
		if !errors.Is(err, ErrUnauthorized) {
			t.Error("401 error should match ErrUnauthorized")
		}
	})

	t.Run("Is - forbidden", func(t *testing.T) {
		err := NewAPIError(403, 0, "")
		if !errors.Is(err, ErrForbidden) {
			t.Error("403 error should match ErrForbidden")
		}
	})

	t.Run("Is - not found", func(t *testing.T) {
		err := NewAPIError(404, 0, "")
		if !errors.Is(err, ErrNotFound) {
			t.Error("404 error should match ErrNotFound")
		}
	})

	t.Run("Is - server error", func(t *testing.T) {
		err := NewAPIError(503, 0, "")
		if !errors.Is(err, ErrServerError) {
			t.Error("5xx error should match ErrServerError")
		}
	})

	t.Run("Is - no match for other status codes", func(t *testing.T) {
		err := NewAPIError(400, 0, "")
		if errors.Is(err, ErrUnauthorized) {
			t.Error("400 error should not match ErrUnauthorized")
		}
		if errors.Is(err, ErrServerError) {
			t.Error("400 error should not match ErrServerError")
		}
	})

	t.Run("Unwrap returns nil", func(t *testing.T) {
		err := NewAPIError(500, 0, "")
		if err.Unwrap() != nil {
			t.Error("Unwrap should return nil")
		}
	})
}
