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

func TestIsDenied(t *testing.T) {
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
			name: "LimitError",
			err:  &LimitError{Key: Key{Tenant: "test"}, Rule: "rule1"},
			want: true,
		},
		{
			name: "wrapped LimitError",
			err:  errors.Join(errors.New("context"), &LimitError{Key: Key{Tenant: "test"}, Rule: "rule1"}),
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
			if got := IsDenied(tt.err); got != tt.want {
				t.Errorf("IsDenied(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
