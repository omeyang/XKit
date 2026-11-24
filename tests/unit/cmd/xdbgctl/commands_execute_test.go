//go:build !windows

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/debug/xdbg"
)

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
	if !errors.Is(err, errServerRejected) {
		t.Errorf("error should wrap errServerRejected, got: %v", err)
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
	if !errors.Is(err, errServerRejected) {
		t.Errorf("error should wrap errServerRejected, got: %v", err)
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
		{"max_boundary", maxTimeout, false},
		{"over_max", maxTimeout + time.Second, true},
		{"extreme", 999 * time.Hour, true},
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
		{"invalid_value", fmt.Errorf(`invalid value "abc" for flag -pid: strconv.ParseInt`), true},
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
	_, err := findProcessBySocket(context.Background(), "/nonexistent/test-xdbg.sock")
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
	pid, findErr := findProcessBySocket(context.Background(), socketPath)
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

func TestFindSocketInodePathWithSpaces(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test socket.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Skipf("cannot create socket with spaces in path: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := listener.Close(); closeErr != nil {
			t.Log("close listener:", closeErr)
		}
	})

	ino, findErr := findSocketInode(socketPath)
	if findErr != nil {
		t.Fatalf("findSocketInode() with space in path should succeed: %v", findErr)
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
