package xetcd

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsKeyNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "ErrKeyNotFound",
			err:  ErrKeyNotFound,
			want: true,
		},
		{
			name: "wrapped ErrKeyNotFound",
			err:  fmt.Errorf("get failed: %w", ErrKeyNotFound),
			want: true,
		},
		{
			name: "other error",
			err:  errors.New("other error"),
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
			if got := IsKeyNotFound(tt.err); got != tt.want {
				t.Errorf("IsKeyNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsClientClosed(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "ErrClientClosed",
			err:  ErrClientClosed,
			want: true,
		},
		{
			name: "wrapped ErrClientClosed",
			err:  fmt.Errorf("operation failed: %w", ErrClientClosed),
			want: true,
		},
		{
			name: "other error",
			err:  errors.New("other error"),
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
			if got := IsClientClosed(tt.err); got != tt.want {
				t.Errorf("IsClientClosed() = %v, want %v", got, tt.want)
			}
		})
	}
}
