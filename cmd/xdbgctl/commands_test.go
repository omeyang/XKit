//go:build !windows

package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestParseCommandLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"single_word", "help", []string{"help"}},
		{"two_words", "setlog debug", []string{"setlog", "debug"}},
		{"double_quoted", `exec "hello world"`, []string{"exec", "hello world"}},
		{"single_quoted", "exec 'hello world'", []string{"exec", "hello world"}},
		{"escaped_quote_in_double", `exec "hello\"world"`, []string{"exec", `hello"world`}},
		{"escaped_backslash", `exec "hello\\world"`, []string{"exec", `hello\world`}},
		{"escape_outside_quotes", `exec hello\ world`, []string{"exec", "hello world"}},
		{"multiple_spaces", "  setlog   debug  ", []string{"setlog", "debug"}},
		{"mixed_quotes", `exec "arg1" 'arg2'`, []string{"exec", "arg1", "arg2"}},
		{"empty_quotes", `exec ""`, []string{"exec"}},
		{"three_args", "pprof cpu start", []string{"pprof", "cpu", "start"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommandLine(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseCommandLine(%q) = %v (len %d), want %v (len %d)",
					tt.input, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseCommandLine(%q)[%d] = %q, want %q",
						tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsQuoteStart(t *testing.T) {
	tests := []struct {
		r       rune
		inQuote bool
		want    bool
	}{
		{'"', false, true},
		{'\'', false, true},
		{'"', true, false},
		{'\'', true, false},
		{'a', false, false},
		{' ', false, false},
	}

	for _, tt := range tests {
		if got := isQuoteStart(tt.r, tt.inQuote); got != tt.want {
			t.Errorf("isQuoteStart(%q, %v) = %v, want %v",
				tt.r, tt.inQuote, got, tt.want)
		}
	}
}

func TestIsQuoteEnd(t *testing.T) {
	tests := []struct {
		r         rune
		quoteChar rune
		inQuote   bool
		want      bool
	}{
		{'"', '"', true, true},
		{'\'', '\'', true, true},
		{'"', '\'', true, false},
		{'\'', '"', true, false},
		{'"', '"', false, false},
	}

	for _, tt := range tests {
		if got := isQuoteEnd(tt.r, tt.quoteChar, tt.inQuote); got != tt.want {
			t.Errorf("isQuoteEnd(%q, %q, %v) = %v, want %v",
				tt.r, tt.quoteChar, tt.inQuote, got, tt.want)
		}
	}
}

func TestIsWordSeparator(t *testing.T) {
	tests := []struct {
		r       rune
		inQuote bool
		want    bool
	}{
		{' ', false, true},
		{' ', true, false},
		{'a', false, false},
		{'\t', false, false},
	}

	for _, tt := range tests {
		if got := isWordSeparator(tt.r, tt.inQuote); got != tt.want {
			t.Errorf("isWordSeparator(%q, %v) = %v, want %v",
				tt.r, tt.inQuote, got, tt.want)
		}
	}
}

func TestAbsPath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty", "", true},
		{"relative", "test.sock", false},
		{"absolute", "/var/run/xdbg.sock", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := absPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("absPath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && result == "" {
				t.Errorf("absPath(%q) returned empty string", tt.input)
			}
		})
	}
}

func TestExitError(t *testing.T) {
	err := &exitError{code: 2}
	if err.Error() != "" {
		t.Errorf("exitError.Error() = %q, want empty string", err.Error())
	}

	// exitError 应可通过 errors.As 检测
	var target *exitError
	if !errors.As(err, &target) {
		t.Error("errors.As failed for *exitError")
	}
	if target.code != 2 {
		t.Errorf("exitError.code = %d, want 2", target.code)
	}
}

func TestCmdExecNoArgs(t *testing.T) {
	err := cmdExec(context.Background(), "/nonexistent.sock", time.Second, nil)
	if err == nil {
		t.Fatal("cmdExec with no args should return error")
	}
	if err.Error() != "exec 命令需要指定要执行的调试命令" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCmdToggleCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	err := cmdToggle(ctx, "/nonexistent.sock", 0, "")
	if err == nil {
		t.Fatal("cmdToggle with canceled context should return error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestCmdStatusOffline(t *testing.T) {
	// 使用不存在的 socket，应返回 exitError
	err := cmdStatus(context.Background(), "/nonexistent-xdbg-test.sock", 100*time.Millisecond)
	if err == nil {
		t.Fatal("cmdStatus with nonexistent socket should return error")
	}

	var exitErr *exitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exitError, got %T: %v", err, err)
	}
	if exitErr.code != 1 {
		t.Errorf("exitError.code = %d, want 1", exitErr.code)
	}
}

func TestProcessLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantExit bool
	}{
		{"empty", "", false},
		{"quit", "quit", true},
		{"exit", "exit", true},
		{"normal_command", "help", false},
	}

	// 使用 nil client — processLine 仅对 quit/exit/空行返回 true，
	// 其余情况执行 client.Execute 会 panic，但我们不测试那些路径
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantExit || tt.line == "" {
				got := processLine(context.Background(), nil, tt.line)
				if got != tt.wantExit {
					t.Errorf("processLine(%q) = %v, want %v", tt.line, got, tt.wantExit)
				}
			}
		})
	}
}

func TestCreateShortcutCommand(t *testing.T) {
	cmd := createShortcutCommand("testcmd", "test usage", "[args]")
	if cmd.Name != "testcmd" {
		t.Errorf("Name = %q, want %q", cmd.Name, "testcmd")
	}
	if cmd.Usage != "test usage" {
		t.Errorf("Usage = %q, want %q", cmd.Usage, "test usage")
	}
	if cmd.ArgsUsage != "[args]" {
		t.Errorf("ArgsUsage = %q, want %q", cmd.ArgsUsage, "[args]")
	}

	// 不指定 argsUsage
	cmd2 := createShortcutCommand("testcmd2", "test usage 2", "")
	if cmd2.ArgsUsage != "" {
		t.Errorf("ArgsUsage = %q, want empty", cmd2.ArgsUsage)
	}
}

func TestNewClient(t *testing.T) {
	c := NewClient("/tmp/test.sock", 5*time.Second)
	if c.socketPath != "/tmp/test.sock" {
		t.Errorf("socketPath = %q, want %q", c.socketPath, "/tmp/test.sock")
	}
	if c.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want %v", c.timeout, 5*time.Second)
	}
	if c.codec == nil {
		t.Error("codec is nil")
	}
}

func TestValidateSocket(t *testing.T) {
	// 不存在的路径
	c := NewClient("/nonexistent-xdbg-test.sock", time.Second)
	err := c.validateSocket()
	if err == nil {
		t.Fatal("validateSocket with nonexistent path should return error")
	}

	// 普通文件（非 socket）
	f, fErr := os.CreateTemp("", "xdbgctl-test-*")
	if fErr != nil {
		t.Fatal(fErr)
	}
	defer func() { _ = os.Remove(f.Name()) }() //nolint:errcheck // test cleanup
	_ = f.Close()                              //nolint:errcheck // test cleanup

	c2 := NewClient(f.Name(), time.Second)
	err = c2.validateSocket()
	if err == nil {
		t.Fatal("validateSocket with regular file should return error")
	}
}

func TestCreateCommands(t *testing.T) {
	cmds := createCommands()
	if len(cmds) == 0 {
		t.Fatal("createCommands returned empty slice")
	}

	// 验证基础命令存在
	names := make(map[string]bool)
	for _, cmd := range cmds {
		names[cmd.Name] = true
	}

	expected := []string{"toggle", "disable", "exec", "status", "interactive",
		"setlog", "stack", "freemem", "pprof", "breaker", "limit", "cache", "config"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing command %q", name)
		}
	}
}
