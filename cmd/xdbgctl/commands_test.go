//go:build !windows

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
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
			got, err := parseCommandLine(tt.input)
			if err != nil {
				t.Fatalf("parseCommandLine(%q) unexpected error: %v", tt.input, err)
			}
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

// mockResponse 定义 mock 服务端的响应行为。
type mockResponse struct {
	resp    *xdbg.Response
	delay   time.Duration // 响应前延迟（0 = 立即响应）
	noReply bool          // 不发送响应（模拟超时）
}

// startMockXdbgServer 启动一个模拟 xdbg 服务端，返回 socket 路径。
func startMockXdbgServer(t *testing.T, mr mockResponse) string {
	t.Helper()
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if closeErr := listener.Close(); closeErr != nil {
			t.Log("close listener:", closeErr)
		}
	})

	codec := xdbg.NewCodec()
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go handleMockConn(codec, conn, mr)
		}
	}()

	return socketPath
}

// handleMockConn 处理 mock 连接。
func handleMockConn(codec *xdbg.Codec, conn net.Conn, mr mockResponse) {
	defer func() {
		if err := conn.Close(); err != nil {
			return // 连接已关闭，忽略
		}
	}()

	if _, err := codec.DecodeRequest(conn); err != nil {
		return
	}

	if mr.noReply {
		time.Sleep(mr.delay)
		return
	}

	if mr.delay > 0 {
		time.Sleep(mr.delay)
	}

	data, err := codec.EncodeResponse(mr.resp)
	if err != nil {
		return
	}
	if _, err := conn.Write(data); err != nil {
		return
	}
}

func TestExecuteSuccess(t *testing.T) {
	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{Success: true, Output: "ok"},
	})

	client := NewClient(socketPath, 5*time.Second)
	resp, err := client.Execute(context.Background(), "help", nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !resp.Success {
		t.Error("expected success")
	}
}

func TestExecuteWithArgs(t *testing.T) {
	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{Success: true, Output: "level set to debug"},
	})

	client := NewClient(socketPath, 5*time.Second)
	resp, err := client.Execute(context.Background(), "setlog", []string{"debug"})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !resp.Success {
		t.Error("expected success")
	}
}

func TestExecuteContextTimeout(t *testing.T) {
	socketPath := startMockXdbgServer(t, mockResponse{
		noReply: true,
		delay:   5 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	client := NewClient(socketPath, 5*time.Second)
	_, err := client.Execute(ctx, "help", nil)
	if err == nil {
		t.Fatal("Execute with timeout should return error")
	}
}

func TestExecuteConnectionRefused(t *testing.T) {
	client := NewClient("/nonexistent-xdbg-test.sock", time.Second)
	_, err := client.Execute(context.Background(), "help", nil)
	if err == nil {
		t.Fatal("Execute with nonexistent socket should return error")
	}
}

func TestPingSuccess(t *testing.T) {
	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{Success: true, Output: "available commands: ..."},
	})

	client := NewClient(socketPath, 5*time.Second)
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error: %v", err)
	}
}

func TestPingFailure(t *testing.T) {
	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{Success: false, Error: "service error"},
	})

	client := NewClient(socketPath, 5*time.Second)
	err := client.Ping(context.Background())
	if err == nil {
		t.Fatal("Ping with failure response should return error")
	}
	if !strings.Contains(err.Error(), "ping 失败") {
		t.Errorf("error should contain 'ping 失败', got: %v", err)
	}
}

func TestCmdDisableSuccess(t *testing.T) {
	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{Success: true, Output: "调试服务已关闭"},
	})

	err := cmdDisable(context.Background(), socketPath, 5*time.Second)
	if err != nil {
		t.Fatalf("cmdDisable() error: %v", err)
	}
}

func TestCmdDisableFailure(t *testing.T) {
	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{Success: false, Error: "not running"},
	})

	err := cmdDisable(context.Background(), socketPath, 5*time.Second)
	if err == nil {
		t.Fatal("cmdDisable with failure response should return error")
	}
	if !strings.Contains(err.Error(), "禁用失败") {
		t.Errorf("error should contain '禁用失败', got: %v", err)
	}
}

func TestCmdExecSuccess(t *testing.T) {
	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{Success: true, Output: "executed: help"},
	})

	err := cmdExec(context.Background(), socketPath, 5*time.Second, []string{"help"})
	if err != nil {
		t.Fatalf("cmdExec() error: %v", err)
	}
}

func TestCmdExecFailure(t *testing.T) {
	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{Success: false, Error: "unknown command"},
	})

	err := cmdExec(context.Background(), socketPath, 5*time.Second, []string{"badcmd"})
	if err == nil {
		t.Fatal("cmdExec with failure response should return error")
	}
	if !strings.Contains(err.Error(), "命令执行失败") {
		t.Errorf("error should contain '命令执行失败', got: %v", err)
	}
}

func TestCmdExecTruncated(t *testing.T) {
	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{
			Success:      true,
			Output:       "truncated output",
			Truncated:    true,
			OriginalSize: 65536,
		},
	})

	err := cmdExec(context.Background(), socketPath, 5*time.Second, []string{"stack"})
	if err != nil {
		t.Fatalf("cmdExec() error: %v", err)
	}
}

func TestCmdStatusOnline(t *testing.T) {
	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{Success: true, Output: "ok"},
	})

	err := cmdStatus(context.Background(), socketPath, 5*time.Second)
	if err != nil {
		t.Fatalf("cmdStatus() error: %v", err)
	}
}

func TestValidateTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		wantErr bool
	}{
		{"positive", 5 * time.Second, false},
		{"zero", 0, true},
		{"negative", -1 * time.Second, true},
		{"small_positive", time.Millisecond, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTimeout(tt.timeout)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTimeout(%v) error = %v, wantErr %v", tt.timeout, err, tt.wantErr)
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

func TestCmdExecInvalidTimeout(t *testing.T) {
	err := cmdExec(context.Background(), "/any.sock", 0, []string{"help"})
	if err == nil {
		t.Fatal("cmdExec with zero timeout should return error")
	}
	var usageErr *usageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *usageError, got %T: %v", err, err)
	}
}

func TestCmdDisableInvalidTimeout(t *testing.T) {
	err := cmdDisable(context.Background(), "/any.sock", -time.Second)
	if err == nil {
		t.Fatal("cmdDisable with negative timeout should return error")
	}
	var usageErr *usageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *usageError, got %T: %v", err, err)
	}
}

func TestCmdStatusInvalidTimeout(t *testing.T) {
	err := cmdStatus(context.Background(), "/any.sock", 0)
	if err == nil {
		t.Fatal("cmdStatus with zero timeout should return error")
	}
	var usageErr *usageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *usageError, got %T: %v", err, err)
	}
}

func TestIsCLIUsageError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"unknown_flag", fmt.Errorf("flag provided but not defined: -xyz"), true},
		{"missing_arg", fmt.Errorf("flag needs an argument: --timeout"), true},
		{"runtime_error", fmt.Errorf("connection refused"), false},
		{"empty", fmt.Errorf(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCLIUsageError(tt.err); got != tt.want {
				t.Errorf("isCLIUsageError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestParseCommandLineErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"unclosed_double_quote", `exec "hello world`, "引号未闭合"},
		{"unclosed_single_quote", "exec 'hello world", "引号未闭合"},
		{"trailing_backslash", `exec hello\`, "尾部转义符"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCommandLine(tt.input)
			if err == nil {
				t.Fatalf("parseCommandLine(%q) expected error", tt.input)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateParseState(t *testing.T) {
	tests := []struct {
		name      string
		escaped   bool
		inQuote   bool
		quoteChar rune
		wantErr   bool
	}{
		{"normal", false, false, 0, false},
		{"escaped", true, false, 0, true},
		{"in_double_quote", false, true, '"', true},
		{"in_single_quote", false, true, '\'', true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateParseState(tt.escaped, tt.inQuote, tt.quoteChar)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateParseState() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProcessLineUnclosedQuote(t *testing.T) {
	mock := &mockExecutor{}
	// 未闭合引号不应退出，不应 panic
	got := processLine(context.Background(), mock, `exec "unclosed`)
	if got {
		t.Error("processLine with unclosed quote should not exit")
	}
}

func TestFindSocketInodeMockFile(t *testing.T) {
	// findSocketInode 直接读 /proc/net/unix；在测试环境中此文件可读
	// 用一个不存在的 socket 路径测试"未找到"场景
	_, err := findSocketInode("/nonexistent/test-xdbg.sock")
	if err == nil {
		t.Fatal("findSocketInode with nonexistent socket should return error")
	}
	if !strings.Contains(err.Error(), "未在 /proc/net/unix 中找到") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProcessHasSocket(t *testing.T) {
	// 当前进程不应持有虚构的 socket link
	if processHasSocket(os.Getpid(), "socket:[999999999]") {
		t.Error("current process should not have fake socket")
	}

	// 不存在的 PID
	if processHasSocket(999999999, "socket:[1]") {
		t.Error("nonexistent PID should return false")
	}
}

func TestFindProcessBySocket(t *testing.T) {
	// 不存在的 socket 路径
	_, err := findProcessBySocket("/nonexistent/test-xdbg.sock")
	if err == nil {
		t.Fatal("findProcessBySocket with nonexistent socket should return error")
	}
}

// writePipeLines 向 pipe 写入行并关闭。
func writePipeLines(w *os.File, lines ...string) {
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return
		}
	}
	if err := w.Close(); err != nil {
		return
	}
}

func TestRunREPLWithInput(t *testing.T) {
	// 替换 os.Stdin 为 pipe 以模拟用户输入
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r

	var commands []string
	mock := &mockExecutor{
		executeFunc: func(_ context.Context, command string, _ []string) (*xdbg.Response, error) {
			commands = append(commands, command)
			return &xdbg.Response{Success: true, Output: "ok"}, nil
		},
	}

	// 写入输入并关闭（模拟 EOF 退出）
	go writePipeLines(w, "help", "setlog debug", "", "stack")

	if runErr := runREPL(context.Background(), mock); runErr != nil {
		t.Fatalf("runREPL() error: %v", runErr)
	}

	if len(commands) != 3 {
		t.Fatalf("expected 3 commands, got %d: %v", len(commands), commands)
	}
	if commands[0] != "help" {
		t.Errorf("commands[0] = %q, want %q", commands[0], "help")
	}
}

func TestRunREPLQuit(t *testing.T) {
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r

	mock := &mockExecutor{
		executeFunc: func(_ context.Context, command string, _ []string) (*xdbg.Response, error) {
			return &xdbg.Response{Success: true, Output: "ok"}, nil
		},
	}

	go writePipeLines(w, "help", "quit", "should_not_execute")

	if runErr := runREPL(context.Background(), mock); runErr != nil {
		t.Fatalf("runREPL() error: %v", runErr)
	}
}

func TestCmdInteractiveSuccess(t *testing.T) {
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r

	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{Success: true, Output: "ok"},
	})

	go writePipeLines(w, "quit")

	if cmdErr := cmdInteractive(context.Background(), socketPath, 5*time.Second); cmdErr != nil {
		t.Fatalf("cmdInteractive() error: %v", cmdErr)
	}
}

func TestCmdInteractiveConnectionFailed(t *testing.T) {
	err := cmdInteractive(context.Background(), "/nonexistent-xdbg-test.sock", time.Second)
	if err == nil {
		t.Fatal("cmdInteractive with nonexistent socket should return error")
	}
	if !strings.Contains(err.Error(), "无法连接到调试服务") {
		t.Errorf("error should contain '无法连接到调试服务', got: %v", err)
	}
}

func TestFindProcessBySocketRealSocket(t *testing.T) {
	// 创建真实 socket 并测试发现逻辑
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := listener.Close(); closeErr != nil {
			t.Log("close listener:", closeErr)
		}
	})

	// findProcessBySocket 应该能找到当前进程
	pid, findErr := findProcessBySocket(socketPath)
	if findErr != nil {
		// 在某些 CI 环境中 /proc 可能不完整，跳过
		t.Skipf("findProcessBySocket() error (可能 /proc 不完整): %v", findErr)
	}
	if pid != os.Getpid() {
		t.Errorf("findProcessBySocket() = %d, want %d (current PID)", pid, os.Getpid())
	}
}

func TestFindSocketInodeSuccess(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := listener.Close(); closeErr != nil {
			t.Log("close listener:", closeErr)
		}
	})

	ino, findErr := findSocketInode(socketPath)
	if findErr != nil {
		t.Skipf("findSocketInode() error: %v", findErr)
	}
	if ino == 0 {
		t.Error("inode should be nonzero")
	}
}

func TestCmdInteractiveInvalidTimeout(t *testing.T) {
	err := cmdInteractive(context.Background(), "/any.sock", 0)
	if err == nil {
		t.Fatal("cmdInteractive with zero timeout should return error")
	}
	var usageErr *usageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected *usageError, got %T: %v", err, err)
	}
}

func TestRunStatusOffline(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"xdbgctl", "status", "-s", "/nonexistent-xdbg-test.sock", "-t", "100ms"}
	code := run()
	if code != 1 {
		t.Errorf("run() = %d, want 1 (offline status)", code)
	}
}

func TestRunStatusOnline(t *testing.T) {
	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{Success: true, Output: "ok"},
	})

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"xdbgctl", "status", "-s", socketPath, "-t", "5s"}
	code := run()
	if code != 0 {
		t.Errorf("run() = %d, want 0 (online status)", code)
	}
}

func TestRunExecNoArgs(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"xdbgctl", "exec", "-t", "100ms"}
	code := run()
	if code != 2 {
		t.Errorf("run() = %d, want 2 (usage error)", code)
	}
}

func TestRunDisableSuccess(t *testing.T) {
	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{Success: true, Output: "调试服务已关闭"},
	})

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"xdbgctl", "disable", "-s", socketPath, "-t", "5s"}
	code := run()
	if code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
}

func TestRunExecSuccess(t *testing.T) {
	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{Success: true, Output: "ok"},
	})

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"xdbgctl", "exec", "-s", socketPath, "-t", "5s", "help"}
	code := run()
	if code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
}

func TestRunShortcutCommand(t *testing.T) {
	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{Success: true, Output: "ok"},
	})

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"xdbgctl", "stack", "-s", socketPath, "-t", "5s"}
	code := run()
	if code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
}

func TestRunToggleNegativePID(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"xdbgctl", "toggle", "--pid", "-1"}
	code := run()
	if code != 2 {
		t.Errorf("run() = %d, want 2 (usage error for negative PID)", code)
	}
}

func TestRunToggleZeroPID(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"xdbgctl", "toggle", "--pid", "0"}
	code := run()
	if code != 2 {
		t.Errorf("run() = %d, want 2 (usage error for PID 0)", code)
	}
}

func TestRunToggleInvalidName(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"xdbgctl", "toggle", "--name", "app/bad"}
	code := run()
	if code != 2 {
		t.Errorf("run() = %d, want 2 (usage error for invalid name)", code)
	}
}

func TestRunInvalidTimeout(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"xdbgctl", "status", "-t", "-1s"}
	code := run()
	if code != 2 {
		t.Errorf("run() = %d, want 2 (usage error for invalid timeout)", code)
	}
}

func TestRunInteractive(t *testing.T) {
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r

	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{Success: true, Output: "ok"},
	})

	go writePipeLines(w, "quit")

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"xdbgctl", "interactive", "-s", socketPath, "-t", "5s"}
	code := run()
	if code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
}

func TestRunToggleNonexistentProcess(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"xdbgctl", "toggle", "--name", "nonexist_proc_x"}
	code := run()
	if code != 1 {
		t.Errorf("run() = %d, want 1 (runtime error)", code)
	}
}

func TestCmdToggleSocketDiscoveryNonContainer(t *testing.T) {
	// 无 --pid 无 --name，走 socket 发现路径，失败后给出普通提示
	err := cmdToggle(context.Background(), "/nonexistent-xdbg-test.sock", 0, "")
	if err == nil {
		t.Fatal("expected error from socket discovery")
	}
	if !strings.Contains(err.Error(), "无法自动发现进程") {
		t.Errorf("expected discovery error, got: %v", err)
	}
}

func TestCmdToggleSocketDiscoveryContainer(t *testing.T) {
	// 在容器环境中，失败后给出容器专用提示
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	err := cmdToggle(context.Background(), "/nonexistent-xdbg-test.sock", 0, "")
	if err == nil {
		t.Fatal("expected error from socket discovery")
	}
	if !strings.Contains(err.Error(), "容器环境") {
		t.Errorf("expected container hint, got: %v", err)
	}
}

func TestCmdToggleSuccessSendSignal(t *testing.T) {
	// 向自身进程发送 SIGUSR1 测试成功路径
	// 先设置信号忽略，避免 SIGUSR1 意外影响测试进程
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGUSR1)
	defer signal.Stop(sigCh)

	pid := os.Getpid()
	err := cmdToggle(context.Background(), "/nonexistent.sock", pid, "")
	if err != nil {
		t.Fatalf("cmdToggle() with current PID error: %v", err)
	}

	// 验证信号已被接收
	select {
	case <-sigCh:
		// 成功接收信号
	case <-time.After(time.Second):
		t.Error("SIGUSR1 not received within 1s")
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
