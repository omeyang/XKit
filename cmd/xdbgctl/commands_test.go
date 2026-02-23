//go:build !windows

package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/debug/xdbg"
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
	want := "exit status 2"
	if err.Error() != want {
		t.Errorf("exitError.Error() = %q, want %q", err.Error(), want)
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

	var usageErr *usageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *usageError, got %T: %v", err, err)
	}
	if usageErr.Error() != "exec 命令需要指定要执行的调试命令" {
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

// mockExecutor 模拟 executor 接口用于测试。
type mockExecutor struct {
	executeFunc func(ctx context.Context, command string, args []string) (*xdbg.Response, error)
	pingFunc    func(ctx context.Context) error
}

func (m *mockExecutor) Execute(ctx context.Context, command string, args []string) (*xdbg.Response, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, command, args)
	}
	return &xdbg.Response{Success: true}, nil
}

func (m *mockExecutor) Ping(ctx context.Context) error {
	if m.pingFunc != nil {
		return m.pingFunc(ctx)
	}
	return nil
}

func TestProcessLine(t *testing.T) {
	mock := &mockExecutor{
		executeFunc: func(_ context.Context, command string, _ []string) (*xdbg.Response, error) {
			return &xdbg.Response{Success: true, Output: "mock: " + command}, nil
		},
	}

	tests := []struct {
		name     string
		line     string
		wantExit bool
	}{
		{"empty", "", false},
		{"quit", "quit", true},
		{"exit", "exit", true},
		{"normal_command", "help", false},
		{"command_with_args", "setlog debug", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := processLine(context.Background(), mock, tt.line)
			if got != tt.wantExit {
				t.Errorf("processLine(%q) = %v, want %v", tt.line, got, tt.wantExit)
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

func TestUsageError(t *testing.T) {
	err := &usageError{msg: "test error"}
	if err.Error() != "test error" {
		t.Errorf("usageError.Error() = %q, want %q", err.Error(), "test error")
	}

	var target *usageError
	if !errors.As(err, &target) {
		t.Error("errors.As failed for *usageError")
	}
}

func TestValidateProcessName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid_short", "myapp", false},
		{"valid_max_len", "123456789012345", false}, // 15 chars = maxCommLen
		{"too_long", "1234567890123456", true},      // 16 chars > maxCommLen
		{"control_char", "app\x01", true},
		{"null_byte", "app\x00", true},
		{"del_char", "app\x7f", true},
		{"slash", "app/name", true},
		{"backslash", "app\\name", true},
		{"valid_with_hyphen", "my-app", false},
		{"valid_with_underscore", "my_app", false},
		{"valid_with_dot", "my.app", false},
		{"empty", "", false}, // 空名称由 cmdToggle 的 nameFlag != "" 分支保护
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProcessName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateProcessName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err != nil {
				var usageErr *usageError
				if !errors.As(err, &usageErr) {
					t.Errorf("expected *usageError, got %T", err)
				}
			}
		})
	}
}

func TestValidateToggleArgs(t *testing.T) {
	tests := []struct {
		name    string
		pid     int
		procNm  string
		wantErr bool
	}{
		{"negative_pid", -1, "", true},
		{"zero_pid_no_name", 0, "", false},
		{"positive_pid_no_name", 123, "", false},
		{"valid_name", 0, "myapp", false},
		{"invalid_name_slash", 0, "app/bad", true},
		{"invalid_name_too_long", 0, "1234567890123456", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateToggleArgs(tt.pid, tt.procNm)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateToggleArgs(%d, %q) error = %v, wantErr %v",
					tt.pid, tt.procNm, err, tt.wantErr)
			}
		})
	}
}

func TestCmdToggleNegativePID(t *testing.T) {
	err := cmdToggle(context.Background(), "/nonexistent.sock", -1, "")
	if err == nil {
		t.Fatal("cmdToggle with negative PID should return error")
	}

	var usageErr *usageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *usageError, got %T: %v", err, err)
	}
}

func TestCmdToggleInvalidProcessName(t *testing.T) {
	// 控制字符
	err := cmdToggle(context.Background(), "/nonexistent.sock", 0, "app\x01")
	if err == nil {
		t.Fatal("cmdToggle with control char in name should return error")
	}

	var usageErr *usageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *usageError, got %T: %v", err, err)
	}

	// 名称过长
	err = cmdToggle(context.Background(), "/nonexistent.sock", 0, "1234567890123456")
	if err == nil {
		t.Fatal("cmdToggle with too long name should return error")
	}
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *usageError, got %T: %v", err, err)
	}
}

func TestProcessLineSpacesOnly(t *testing.T) {
	// 输入仅空格时，parseCommandLine 返回空切片，processLine 应返回 false
	mock := &mockExecutor{}
	got := processLine(context.Background(), mock, "   ")
	if got {
		t.Error("processLine with spaces-only input should return false")
	}
}

func TestCmdToggleProcessNameNotFound(t *testing.T) {
	// 使用合法但不存在的进程名，应通过 validateToggleArgs 但在 findProcessByName 失败
	err := cmdToggle(context.Background(), "/nonexistent.sock", 0, "nonexist_proc_x")
	if err == nil {
		t.Fatal("cmdToggle with nonexistent process name should return error")
	}
	// 不应是 usageError（参数校验通过了）
	var usageErr *usageError
	if errors.As(err, &usageErr) {
		t.Error("should not be usageError for valid but nonexistent process name")
	}
}

func TestCmdToggleNonexistentPID(t *testing.T) {
	// 使用合法但不存在的 PID
	err := cmdToggle(context.Background(), "/nonexistent.sock", 999999999, "")
	if err == nil {
		t.Fatal("cmdToggle with nonexistent PID should return error")
	}
}

func TestIsContainerEnvironment(t *testing.T) {
	// 不设置环境变量时不应 panic（结果取决于宿主机环境）
	_ = isContainerEnvironment()

	// 设置 KUBERNETES_SERVICE_HOST 应检测为容器环境
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	if !isContainerEnvironment() {
		t.Error("expected container detection with KUBERNETES_SERVICE_HOST set")
	}
}

func TestExecuteAndPrint(t *testing.T) {
	tests := []struct {
		name    string
		resp    *xdbg.Response
		err     error
		wantErr bool
	}{
		{
			name: "success",
			resp: &xdbg.Response{Success: true, Output: "ok"},
		},
		{
			name: "success_truncated",
			resp: &xdbg.Response{Success: true, Output: "ok", Truncated: true, OriginalSize: 1024},
		},
		{
			name:    "execute_error",
			err:     errors.New("connection failed"),
			wantErr: true,
		},
		{
			name: "response_error",
			resp: &xdbg.Response{Success: false, Error: "unknown command"},
		},
		{
			name: "empty_output",
			resp: &xdbg.Response{Success: true, Output: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockExecutor{
				executeFunc: func(_ context.Context, _ string, _ []string) (*xdbg.Response, error) {
					return tt.resp, tt.err
				},
			}
			// executeAndPrint 不返回值，只验证不 panic
			executeAndPrint(context.Background(), mock, "test", nil)
		})
	}
}
