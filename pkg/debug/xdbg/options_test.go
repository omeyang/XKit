package xdbg

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()

	if opts.SocketPath != DefaultSocketPath {
		t.Errorf("SocketPath = %q, want %q", opts.SocketPath, DefaultSocketPath)
	}

	if opts.AutoShutdown != DefaultAutoShutdown {
		t.Errorf("AutoShutdown = %v, want %v", opts.AutoShutdown, DefaultAutoShutdown)
	}

	if opts.MaxSessions != DefaultMaxSessions {
		t.Errorf("MaxSessions = %d, want %d", opts.MaxSessions, DefaultMaxSessions)
	}

	if opts.MaxConcurrentCommands != DefaultMaxConcurrentCommands {
		t.Errorf("MaxConcurrentCommands = %d, want %d", opts.MaxConcurrentCommands, DefaultMaxConcurrentCommands)
	}

	if opts.CommandTimeout != DefaultCommandTimeout {
		t.Errorf("CommandTimeout = %v, want %v", opts.CommandTimeout, DefaultCommandTimeout)
	}

	if opts.ShutdownTimeout != DefaultShutdownTimeout {
		t.Errorf("ShutdownTimeout = %v, want %v", opts.ShutdownTimeout, DefaultShutdownTimeout)
	}

	if opts.MaxOutputSize != DefaultMaxOutputSize {
		t.Errorf("MaxOutputSize = %d, want %d", opts.MaxOutputSize, DefaultMaxOutputSize)
	}

	if opts.SessionWriteTimeout != DefaultSessionWriteTimeout {
		t.Errorf("SessionWriteTimeout = %v, want %v", opts.SessionWriteTimeout, DefaultSessionWriteTimeout)
	}

	if opts.AuditLogger == nil {
		t.Error("AuditLogger should not be nil")
	}
}

func TestWithOptions(t *testing.T) {
	tests := []struct {
		name   string
		opt    Option
		check  func(*options) bool
		errMsg string
	}{
		{
			name: "WithSocketPath",
			opt:  WithSocketPath("/tmp/test.sock"),
			check: func(o *options) bool {
				return o.SocketPath == "/tmp/test.sock"
			},
			errMsg: "SocketPath not set correctly",
		},
		{
			name: "WithSocketPerm",
			opt:  WithSocketPerm(0700),
			check: func(o *options) bool {
				return o.SocketPerm == 0700
			},
			errMsg: "SocketPerm not set correctly",
		},
		{
			name: "WithAutoShutdown",
			opt:  WithAutoShutdown(10 * time.Minute),
			check: func(o *options) bool {
				return o.AutoShutdown == 10*time.Minute
			},
			errMsg: "AutoShutdown not set correctly",
		},
		{
			name: "WithMaxSessions",
			opt:  WithMaxSessions(5),
			check: func(o *options) bool {
				return o.MaxSessions == 5
			},
			errMsg: "MaxSessions not set correctly",
		},
		{
			name: "WithMaxConcurrentCommands",
			opt:  WithMaxConcurrentCommands(10),
			check: func(o *options) bool {
				return o.MaxConcurrentCommands == 10
			},
			errMsg: "MaxConcurrentCommands not set correctly",
		},
		{
			name: "WithCommandTimeout",
			opt:  WithCommandTimeout(1 * time.Minute),
			check: func(o *options) bool {
				return o.CommandTimeout == 1*time.Minute
			},
			errMsg: "CommandTimeout not set correctly",
		},
		{
			name: "WithShutdownTimeout",
			opt:  WithShutdownTimeout(30 * time.Second),
			check: func(o *options) bool {
				return o.ShutdownTimeout == 30*time.Second
			},
			errMsg: "ShutdownTimeout not set correctly",
		},
		{
			name: "WithMaxOutputSize",
			opt:  WithMaxOutputSize(2 * 1024 * 1024),
			check: func(o *options) bool {
				return o.MaxOutputSize == 2*1024*1024
			},
			errMsg: "MaxOutputSize not set correctly",
		},
		{
			name: "WithCommandWhitelist",
			opt:  WithCommandWhitelist([]string{"help", "exit"}),
			check: func(o *options) bool {
				return len(o.CommandWhitelist) == 2
			},
			errMsg: "CommandWhitelist not set correctly",
		},
		{
			name: "WithBackgroundMode",
			opt:  WithBackgroundMode(true),
			check: func(o *options) bool {
				return o.BackgroundMode == true
			},
			errMsg: "BackgroundMode not set correctly",
		},
		{
			name: "WithSessionReadTimeout",
			opt:  WithSessionReadTimeout(90 * time.Second),
			check: func(o *options) bool {
				return o.SessionReadTimeout == 90*time.Second
			},
			errMsg: "SessionReadTimeout not set correctly",
		},
		{
			name: "WithSessionWriteTimeout",
			opt:  WithSessionWriteTimeout(45 * time.Second),
			check: func(o *options) bool {
				return o.SessionWriteTimeout == 45*time.Second
			},
			errMsg: "SessionWriteTimeout not set correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultOptions()
			tt.opt(opts)
			if !tt.check(opts) {
				t.Error(tt.errMsg)
			}
		})
	}
}

func TestWithAuditLogger(t *testing.T) {
	logger := NewNoopAuditLogger()
	opts := defaultOptions()

	WithAuditLogger(logger)(opts)

	if opts.AuditLogger != logger {
		t.Error("AuditLogger not set correctly")
	}
}

func TestWithAuditSanitizer(t *testing.T) {
	sanitizer := func(command string, args []string) []string {
		if command == "config" {
			return SanitizeArgs(args)
		}
		return args
	}
	opts := defaultOptions()

	WithAuditSanitizer(sanitizer)(opts)

	if opts.AuditSanitizer == nil {
		t.Error("AuditSanitizer should not be nil")
	}

	// 验证脱敏函数工作正常
	result := opts.AuditSanitizer("config", []string{"password"})
	if len(result) != 1 || result[0] != "***" {
		t.Errorf("AuditSanitizer should sanitize config args, got %v", result)
	}

	// 验证非敏感命令不脱敏
	result = opts.AuditSanitizer("help", []string{"arg1"})
	if len(result) != 1 || result[0] != "arg1" {
		t.Errorf("AuditSanitizer should not sanitize help args, got %v", result)
	}
}

func TestWithTransport(t *testing.T) {
	mt := &testMockTransport{}
	opts := defaultOptions()

	WithTransport(mt)(opts)

	if opts.Transport == nil {
		t.Error("Transport should not be nil")
	}
	// 验证类型正确设置
	if _, ok := opts.Transport.(*testMockTransport); !ok {
		t.Error("Transport not set correctly")
	}
}

func TestWithProfileDir(t *testing.T) {
	opts := defaultOptions()

	WithProfileDir("/data/profiles")(opts)

	if opts.ProfileDir != "/data/profiles" {
		t.Errorf("ProfileDir = %q, want %q", opts.ProfileDir, "/data/profiles")
	}
}

// testMockTransport 用于测试的 mock Transport 实现。
type testMockTransport struct{}

func (m *testMockTransport) Listen(_ context.Context) error { return nil }
func (m *testMockTransport) Accept() (net.Conn, *PeerIdentity, error) {
	return nil, nil, nil
}
func (m *testMockTransport) Close() error { return nil }
func (m *testMockTransport) Addr() string { return "/tmp/mock.sock" }

func TestValidateOptions(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*options)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid default options",
			modify:  func(_ *options) {},
			wantErr: false,
		},
		{
			name: "zero MaxSessions",
			modify: func(o *options) {
				o.MaxSessions = 0
			},
			wantErr: true,
			errMsg:  "MaxSessions must be positive",
		},
		{
			name: "negative MaxSessions",
			modify: func(o *options) {
				o.MaxSessions = -1
			},
			wantErr: true,
			errMsg:  "MaxSessions must be positive",
		},
		{
			name: "zero MaxConcurrentCommands",
			modify: func(o *options) {
				o.MaxConcurrentCommands = 0
			},
			wantErr: true,
			errMsg:  "MaxConcurrentCommands must be positive",
		},
		{
			name: "zero MaxOutputSize",
			modify: func(o *options) {
				o.MaxOutputSize = 0
			},
			wantErr: true,
			errMsg:  "MaxOutputSize must be positive",
		},
		{
			name: "zero CommandTimeout",
			modify: func(o *options) {
				o.CommandTimeout = 0
			},
			wantErr: true,
			errMsg:  "CommandTimeout must be positive",
		},
		{
			name: "zero ShutdownTimeout",
			modify: func(o *options) {
				o.ShutdownTimeout = 0
			},
			wantErr: true,
			errMsg:  "ShutdownTimeout must be positive",
		},
		{
			name: "negative SessionReadTimeout",
			modify: func(o *options) {
				o.SessionReadTimeout = -1 * time.Second
			},
			wantErr: true,
			errMsg:  "SessionReadTimeout must be non-negative",
		},
		{
			name: "negative SessionWriteTimeout",
			modify: func(o *options) {
				o.SessionWriteTimeout = -1 * time.Second
			},
			wantErr: true,
			errMsg:  "SessionWriteTimeout must be non-negative",
		},
		{
			name: "MaxOutputSize exceeds MaxPayloadSize",
			modify: func(o *options) {
				o.MaxOutputSize = MaxPayloadSize + 1
			},
			wantErr: true,
			errMsg:  "MaxOutputSize",
		},
		{
			name: "empty socket path",
			modify: func(o *options) {
				o.SocketPath = ""
			},
			wantErr: true,
			errMsg:  "invalid socket path",
		},
		{
			name: "relative socket path",
			modify: func(o *options) {
				o.SocketPath = "relative/path.sock"
			},
			wantErr: true,
			errMsg:  "invalid socket path",
		},
		{
			name: "socket path in sensitive directory",
			modify: func(o *options) {
				o.SocketPath = "/etc/xdbg.sock"
			},
			wantErr: true,
			errMsg:  "invalid socket path",
		},
		{
			name: "socket path with traversal",
			modify: func(o *options) {
				o.SocketPath = "/tmp/../etc/xdbg.sock"
			},
			wantErr: true,
			errMsg:  "invalid socket path",
		},
		{
			name: "MaxSessions exceeds upper bound",
			modify: func(o *options) {
				o.MaxSessions = maxSessions + 1
			},
			wantErr: true,
			errMsg:  "MaxSessions exceeds upper bound",
		},
		{
			name: "MaxConcurrentCommands exceeds upper bound",
			modify: func(o *options) {
				o.MaxConcurrentCommands = maxConcurrentCommands + 1
			},
			wantErr: true,
			errMsg:  "MaxConcurrentCommands exceeds upper bound",
		},
		{
			name: "socket perm zero",
			modify: func(o *options) {
				o.SocketPerm = 0
			},
			wantErr: true,
			errMsg:  "SocketPerm must be non-zero",
		},
		{
			name: "socket perm other readable",
			modify: func(o *options) {
				o.SocketPerm = 0o604
			},
			wantErr: true,
			errMsg:  "SocketPerm must not grant",
		},
		{
			name: "socket perm other writable",
			modify: func(o *options) {
				o.SocketPerm = 0o602
			},
			wantErr: true,
			errMsg:  "SocketPerm must not grant",
		},
		{
			name: "socket perm world accessible",
			modify: func(o *options) {
				o.SocketPerm = 0o666
			},
			wantErr: true,
			errMsg:  "SocketPerm must not grant",
		},
		{
			name: "socket perm 0777",
			modify: func(o *options) {
				o.SocketPerm = 0o777
			},
			wantErr: true,
			errMsg:  "SocketPerm must not grant",
		},
		{
			name: "socket perm 0600 valid",
			modify: func(o *options) {
				o.SocketPerm = 0o600
			},
			wantErr: false,
		},
		{
			name: "socket perm 0660 valid (group access allowed)",
			modify: func(o *options) {
				o.SocketPerm = 0o660
			},
			wantErr: false,
		},
		{
			name: "socket perm 0700 valid",
			modify: func(o *options) {
				o.SocketPerm = 0o700
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultOptions()
			tt.modify(opts)
			err := validateOptions(opts)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errMsg != "" && !strings.HasPrefix(err.Error(), tt.errMsg) {
					t.Errorf("error message = %q, want prefix %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
