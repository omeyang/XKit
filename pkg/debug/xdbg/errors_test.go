package xdbg

import (
	"errors"
	"testing"
)

func TestErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "ErrNotRunning",
			err:  ErrNotRunning,
			want: "xdbg: debug server is not running",
		},
		{
			name: "ErrAlreadyRunning",
			err:  ErrAlreadyRunning,
			want: "xdbg: debug server is already running",
		},
		{
			name: "ErrCommandNotFound",
			err:  ErrCommandNotFound,
			want: "xdbg: command not found",
		},
		{
			name: "ErrCommandForbidden",
			err:  ErrCommandForbidden,
			want: "xdbg: command is forbidden",
		},
		{
			name: "ErrTimeout",
			err:  ErrTimeout,
			want: "xdbg: command execution timeout",
		},
		{
			name: "ErrTooManySessions",
			err:  ErrTooManySessions,
			want: "xdbg: too many concurrent sessions",
		},
		{
			name: "ErrTooManyCommands",
			err:  ErrTooManyCommands,
			want: "xdbg: too many concurrent commands",
		},
		{
			name: "ErrInvalidMessage",
			err:  ErrInvalidMessage,
			want: "xdbg: invalid message format",
		},
		{
			name: "ErrMessageTooLarge",
			err:  ErrMessageTooLarge,
			want: "xdbg: message too large",
		},
		{
			name: "ErrConnectionClosed",
			err:  ErrConnectionClosed,
			want: "xdbg: connection closed",
		},
		{
			name: "ErrOutputTruncated",
			err:  ErrOutputTruncated,
			want: "xdbg: output truncated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("error message = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorsIs(t *testing.T) {
	// 测试 errors.Is 是否正常工作
	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{
			name:   "ErrNotRunning matches itself",
			err:    ErrNotRunning,
			target: ErrNotRunning,
			want:   true,
		},
		{
			name:   "ErrCommandNotFound does not match ErrNotRunning",
			err:    ErrCommandNotFound,
			target: ErrNotRunning,
			want:   false,
		},
		{
			name:   "wrapped error matches base",
			err:    errors.Join(ErrTimeout, errors.New("additional context")),
			target: ErrTimeout,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errors.Is(tt.err, tt.target); got != tt.want {
				t.Errorf("errors.Is() = %v, want %v", got, tt.want)
			}
		})
	}
}
