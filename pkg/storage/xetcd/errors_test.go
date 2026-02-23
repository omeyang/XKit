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

func TestErrWatchDisconnected(t *testing.T) {
	// 验证 ErrWatchDisconnected 是可检查的哨兵错误
	err := ErrWatchDisconnected
	if !errors.Is(err, ErrWatchDisconnected) {
		t.Error("errors.Is(ErrWatchDisconnected, ErrWatchDisconnected) should be true")
	}

	// 验证包装后仍可检查
	wrapped := fmt.Errorf("watch failed: %w", ErrWatchDisconnected)
	if !errors.Is(wrapped, ErrWatchDisconnected) {
		t.Error("wrapped ErrWatchDisconnected should be detectable via errors.Is")
	}
}

func TestErrMaxRetriesExceeded(t *testing.T) {
	err := ErrMaxRetriesExceeded
	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Error("errors.Is(ErrMaxRetriesExceeded, ErrMaxRetriesExceeded) should be true")
	}

	wrapped := fmt.Errorf("watch stopped: %w", ErrMaxRetriesExceeded)
	if !errors.Is(wrapped, ErrMaxRetriesExceeded) {
		t.Error("wrapped ErrMaxRetriesExceeded should be detectable via errors.Is")
	}
}
