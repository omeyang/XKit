package xlog_test

import (
	"log/slog"
	"runtime"
	"testing"

	"github.com/omeyang/xkit/pkg/observability/xlog"
)

func TestLevelConstants(t *testing.T) {
	// 验证与 slog 级别对应
	tests := []struct {
		level    xlog.Level
		slogLvl  slog.Level
		name     string
		wantName string
	}{
		{xlog.LevelDebug, slog.LevelDebug, "LevelDebug", "DEBUG"},
		{xlog.LevelInfo, slog.LevelInfo, "LevelInfo", "INFO"},
		{xlog.LevelWarn, slog.LevelWarn, "LevelWarn", "WARN"},
		{xlog.LevelError, slog.LevelError, "LevelError", "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if slog.Level(tt.level) != tt.slogLvl {
				t.Errorf("%s = %d, want slog equivalent %d", tt.name, tt.level, tt.slogLvl)
			}
		})
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  xlog.Level
		err   bool
	}{
		// 小写
		{"debug", xlog.LevelDebug, false},
		{"info", xlog.LevelInfo, false},
		{"warn", xlog.LevelWarn, false},
		{"error", xlog.LevelError, false},

		// 大写
		{"DEBUG", xlog.LevelDebug, false},
		{"INFO", xlog.LevelInfo, false},
		{"WARN", xlog.LevelWarn, false},
		{"ERROR", xlog.LevelError, false},

		// 混合大小写
		{"Debug", xlog.LevelDebug, false},
		{"Info", xlog.LevelInfo, false},

		// warning 别名
		{"warning", xlog.LevelWarn, false},
		{"WARNING", xlog.LevelWarn, false},

		// TrimSpace
		{" info ", xlog.LevelInfo, false},
		{"\tdebug\n", xlog.LevelDebug, false},

		// 无效输入
		{"", xlog.LevelInfo, true},
		{"invalid", xlog.LevelInfo, true},
		{"trace", xlog.LevelInfo, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := xlog.ParseLevel(tt.input)
			if tt.err {
				if err == nil {
					t.Errorf("ParseLevel(%q) should return error", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseLevel(%q) error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestLevel_String(t *testing.T) {
	tests := []struct {
		level xlog.Level
		want  string
	}{
		{xlog.LevelDebug, "DEBUG"},
		{xlog.LevelInfo, "INFO"},
		{xlog.LevelWarn, "WARN"},
		{xlog.LevelError, "ERROR"},
		{xlog.Level(-100), "DEBUG-96"}, // 非标准级别委托 slog.Level.String()
		{xlog.Level(2), "INFO+2"},      // 非标准级别委托 slog.Level.String()
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.level.String(); got != tt.want {
				t.Errorf("Level.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLevel_MarshalText(t *testing.T) {
	tests := []struct {
		level xlog.Level
		want  string
	}{
		{xlog.LevelDebug, "DEBUG"},
		{xlog.LevelInfo, "INFO"},
		{xlog.LevelWarn, "WARN"},
		{xlog.LevelError, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got, err := tt.level.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText() error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("MarshalText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLevel_UnmarshalText(t *testing.T) {
	tests := []struct {
		input string
		want  xlog.Level
		err   bool
	}{
		{"debug", xlog.LevelDebug, false},
		{"INFO", xlog.LevelInfo, false},
		{"warn", xlog.LevelWarn, false},
		{"ERROR", xlog.LevelError, false},
		{"invalid", xlog.LevelInfo, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var level xlog.Level
			err := level.UnmarshalText([]byte(tt.input))
			if tt.err {
				if err == nil {
					t.Errorf("UnmarshalText(%q) should return error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalText(%q) error: %v", tt.input, err)
			}
			if level != tt.want {
				t.Errorf("UnmarshalText(%q) = %v, want %v", tt.input, level, tt.want)
			}
		})
	}
}

// TestLevel_RoundTrip 验证 MarshalText/UnmarshalText 往返一致性
func TestLevel_RoundTrip(t *testing.T) {
	for _, level := range []xlog.Level{xlog.LevelDebug, xlog.LevelInfo, xlog.LevelWarn, xlog.LevelError} {
		data, err := level.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText(%v) error: %v", level, err)
		}
		var got xlog.Level
		if err := got.UnmarshalText(data); err != nil {
			t.Fatalf("UnmarshalText(%q) error: %v", data, err)
		}
		if got != level {
			t.Errorf("round trip: %v -> %q -> %v", level, data, got)
		}
	}
}

func BenchmarkParseLevel(b *testing.B) {
	for i := 0; i < b.N; i++ {
		level, err := xlog.ParseLevel("info")
		runtime.KeepAlive(level)
		runtime.KeepAlive(err)
	}
}
