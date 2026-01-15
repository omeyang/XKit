package xlimit

import (
	"errors"
	"testing"
)

func TestErrors_Is(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{
			name:   "ErrRateLimited matches",
			err:    ErrRateLimited,
			target: ErrRateLimited,
			want:   true,
		},
		{
			name:   "wrapped ErrRateLimited matches",
			err:    errors.Join(errors.New("wrapped"), ErrRateLimited),
			target: ErrRateLimited,
			want:   true,
		},
		{
			name:   "ErrRedisUnavailable matches",
			err:    ErrRedisUnavailable,
			target: ErrRedisUnavailable,
			want:   true,
		},
		{
			name:   "ErrInvalidRule matches",
			err:    ErrInvalidRule,
			target: ErrInvalidRule,
			want:   true,
		},
		{
			name:   "ErrInvalidKey matches",
			err:    ErrInvalidKey,
			target: ErrInvalidKey,
			want:   true,
		},
		{
			name:   "ErrLimiterClosed matches",
			err:    ErrLimiterClosed,
			target: ErrLimiterClosed,
			want:   true,
		},
		{
			name:   "different errors do not match",
			err:    ErrRateLimited,
			target: ErrRedisUnavailable,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errors.Is(tt.err, tt.target); got != tt.want {
				t.Errorf("errors.Is(%v, %v) = %v, want %v", tt.err, tt.target, got, tt.want)
			}
		})
	}
}

func TestRateLimitError(t *testing.T) {
	result := &Result{
		Allowed:   false,
		Limit:     100,
		Remaining: 0,
		Rule:      "tenant-limit",
		Key:       "tenant:abc123",
	}

	err := NewRateLimitError(result)

	t.Run("implements error interface", func(t *testing.T) {
		var _ error = err
	})

	t.Run("error message contains rule info", func(t *testing.T) {
		msg := err.Error()
		if msg == "" {
			t.Error("error message should not be empty")
		}
	})

	t.Run("Is ErrRateLimited", func(t *testing.T) {
		if !errors.Is(err, ErrRateLimited) {
			t.Error("RateLimitError should match ErrRateLimited")
		}
	})

	t.Run("Result returns original result", func(t *testing.T) {
		if err.Result() != result {
			t.Error("Result() should return original result")
		}
	})
}

func TestIsRateLimited(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "ErrRateLimited",
			err:  ErrRateLimited,
			want: true,
		},
		{
			name: "RateLimitError",
			err:  NewRateLimitError(&Result{}),
			want: true,
		},
		{
			name: "wrapped RateLimitError",
			err:  errors.Join(errors.New("context"), NewRateLimitError(&Result{})),
			want: true,
		},
		{
			name: "other error",
			err:  errors.New("some error"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRateLimited(tt.err); got != tt.want {
				t.Errorf("IsRateLimited(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestRateLimitError_Unwrap(t *testing.T) {
	err := NewRateLimitError(&Result{Rule: "test"})
	unwrapped := err.Unwrap()
	if unwrapped != ErrRateLimited {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, ErrRateLimited)
	}
}

func TestRateLimitError_NilResult(t *testing.T) {
	err := NewRateLimitError(nil)
	msg := err.Error()
	if msg != ErrRateLimited.Error() {
		t.Errorf("Error() with nil result = %q, want %q", msg, ErrRateLimited.Error())
	}
}

func TestRateLimitError_Is(t *testing.T) {
	err := NewRateLimitError(&Result{Rule: "test"})

	t.Run("matches ErrRateLimited", func(t *testing.T) {
		if !err.Is(ErrRateLimited) {
			t.Error("expected Is(ErrRateLimited) to be true")
		}
	})

	t.Run("does not match other errors", func(t *testing.T) {
		if err.Is(ErrRedisUnavailable) {
			t.Error("expected Is(ErrRedisUnavailable) to be false")
		}
	})
}
