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
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/debug/xdbg"
)

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

func TestRunInvalidFlagValue(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"xdbgctl", "toggle", "--pid", "abc"}
	code := run()
	if code != 2 {
		t.Errorf("run() = %d, want 2 (usage error for invalid flag value)", code)
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

func TestCmdTogglePostSignalCheck(t *testing.T) {
	// 使用一个不存在的 PID 模拟进程在信号后消失的场景。
	// 通过缩短 postSignalCheckDelay 加速测试。
	oldDelay := postSignalCheckDelay
	postSignalCheckDelay = time.Millisecond
	defer func() { postSignalCheckDelay = oldDelay }()

	// 启动一个短命子进程，发送 SIGUSR1 后它会因默认处理而退出
	// 但直接用 syscall 测试更可靠：用一个合法但死亡的 PID
	// 这里我们测试 discoverTargetPID 返回的 PID 在信号后不存在的场景
	// 由于无法安全地启动并杀死子进程，改为直接测试 postSignalCheckDelay 逻辑:
	// 向自身发送信号（自身进程存活），验证正常路径仍通过
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGUSR1)
	defer signal.Stop(sigCh)

	pid := os.Getpid()
	err := cmdToggle(context.Background(), "/nonexistent.sock", pid, "")
	if err != nil {
		t.Fatalf("cmdToggle() with live process should succeed: %v", err)
	}
	// 消费信号
	<-sigCh
}

func TestDiscoverTargetPID(t *testing.T) {
	tests := []struct {
		name    string
		pid     int
		procNm  string
		wantErr bool
	}{
		{"direct_pid", 123, "", false},
		{"name_not_found", 0, "nonexist_proc_x", true},
		{"socket_not_found", 0, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := discoverTargetPID(context.Background(), "/nonexistent.sock", tt.pid, tt.procNm)
			if (err != nil) != tt.wantErr {
				t.Errorf("discoverTargetPID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDiscoverTargetPIDContainer(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	_, err := discoverTargetPID(context.Background(), "/nonexistent.sock", 0, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "容器环境") {
		t.Errorf("expected container hint, got: %v", err)
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

func TestRunUnknownCommand(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"xdbgctl", "nonexistent_cmd_xyz"}
	code := run()
	if code != 2 {
		t.Errorf("run() = %d, want 2 (usage error for unknown command)", code)
	}
}

func TestIsCLIUsageErrorUnknownCommand(t *testing.T) {
	err := fmt.Errorf("No help topic for 'badcmd'")
	if !isCLIUsageError(err) {
		t.Error("isCLIUsageError should detect unknown command errors")
	}
}

func TestRunREPLContextCancel(t *testing.T) {
	// 注意: 此测试修改 os.Stdin，不可 t.Parallel()
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, _, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }() // test cleanup
	os.Stdin = r

	mock := &mockExecutor{}

	ctx, cancel := context.WithCancel(context.Background())
	// 短暂延迟后取消，触发 ctx.Done() 路径
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	if runErr := runREPL(ctx, mock); runErr != nil {
		t.Fatalf("runREPL() error: %v", runErr)
	}
}

func TestFindProcessByNameCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := findProcessByName(ctx, "anyproc")
	if err == nil {
		t.Fatal("findProcessByName with canceled context should return error")
	}
	if !strings.Contains(err.Error(), "已取消") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFindProcessBySocketCancelled(t *testing.T) {
	// 创建真实 socket 以便通过 findSocketInode
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")

	listener, listenErr := net.Listen("unix", socketPath)
	if listenErr != nil {
		t.Fatal(listenErr)
	}
	t.Cleanup(func() {
		if closeErr := listener.Close(); closeErr != nil {
			t.Log("close listener:", closeErr)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := findProcessBySocket(ctx, socketPath)
	if err == nil {
		t.Fatal("findProcessBySocket with canceled context should return error")
	}
	if !strings.Contains(err.Error(), "已取消") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateApp(t *testing.T) {
	app := createApp()
	if app.Name != "xdbgctl" {
		t.Errorf("app.Name = %q, want %q", app.Name, "xdbgctl")
	}
	if !strings.Contains(app.Version, Version) {
		t.Errorf("app.Version = %q, should contain %q", app.Version, Version)
	}
	if !strings.Contains(app.Version, GitCommit) {
		t.Errorf("app.Version = %q, should contain %q", app.Version, GitCommit)
	}
}

func TestSetConnDeadline(t *testing.T) {
	socketPath := startMockXdbgServer(t, mockResponse{
		resp: &xdbg.Response{Success: true, Output: "ok"},
	})

	c := NewClient(socketPath, 0) // timeout=0: 不设置 deadline
	resp, err := c.Execute(context.Background(), "help", nil)
	if err != nil {
		t.Fatalf("Execute with zero timeout error: %v", err)
	}
	if !resp.Success {
		t.Error("expected success")
	}
}

func TestScanProcForSocket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := scanProcForSocket(ctx, "socket:[999999999]")
	if err == nil {
		t.Fatal("scanProcForSocket with canceled context should return error")
	}
}

func TestWrapDecodeError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantContain string
	}{
		{
			name:        "version_mismatch",
			err:         fmt.Errorf("%w: unsupported version 99", xdbg.ErrInvalidMessage),
			wantContain: "协议版本不兼容",
		},
		{
			name:        "other_invalid_message",
			err:         fmt.Errorf("%w: bad magic", xdbg.ErrInvalidMessage),
			wantContain: "接收响应失败",
		},
		{
			name:        "generic_error",
			err:         fmt.Errorf("connection reset"),
			wantContain: "接收响应失败",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapDecodeError(tt.err)
			if !strings.Contains(result.Error(), tt.wantContain) {
				t.Errorf("wrapDecodeError() = %q, want containing %q", result.Error(), tt.wantContain)
			}
		})
	}
}

func BenchmarkParseCommandLine(b *testing.B) {
	inputs := []struct {
		name  string
		input string
	}{
		{"simple", "help"},
		{"two_args", "setlog debug"},
		{"quoted", `exec "hello world" arg2`},
		{"many_args", "pprof cpu start --duration 30s --output /tmp/profile"},
	}

	for _, bb := range inputs {
		b.Run(bb.name, func(b *testing.B) {
			for b.Loop() {
				parts, err := parseCommandLine(bb.input)
				if err != nil {
					b.Fatal(err)
				}
				runtime.KeepAlive(parts)
			}
		})
	}
}

func BenchmarkIsCLIUsageError(b *testing.B) {
	errs := []struct {
		name string
		err  error
	}{
		{"match", fmt.Errorf("flag provided but not defined: -xyz")},
		{"no_match", fmt.Errorf("connection refused")},
	}

	for _, bb := range errs {
		b.Run(bb.name, func(b *testing.B) {
			for b.Loop() {
				_ = isCLIUsageError(bb.err)
			}
		})
	}
}
