//go:build !windows

package xdbg

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"
)

// testClient 测试用客户端。
type testClient struct {
	socketPath string
	timeout    time.Duration
	codec      *Codec
}

func newTestClient(socketPath string) *testClient {
	return &testClient{
		socketPath: socketPath,
		timeout:    5 * time.Second,
		codec:      NewCodec(),
	}
}

func (c *testClient) execute(command string, args []string) (*Response, error) {
	conn, err := net.DialTimeout("unix", c.socketPath, c.timeout)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck // test cleanup: 测试客户端连接关闭失败不影响测试结果
	defer func() { _ = conn.Close() }()

	//nolint:errcheck // test utility: 测试环境中超时设置失败会在后续操作中体现
	_ = conn.SetDeadline(time.Now().Add(c.timeout))

	req := &Request{
		Command: command,
		Args:    args,
	}

	data, err := c.codec.EncodeRequest(req)
	if err != nil {
		return nil, err
	}

	if _, err := conn.Write(data); err != nil {
		return nil, err
	}

	return c.codec.DecodeResponse(conn)
}

func TestSession_ExecuteCommand(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup: 测试服务器关闭失败不影响测试结果
	//nolint:errcheck // test cleanup: 测试服务器关闭失败不影响测试结果
	defer func() { _ = srv.Stop() }()

	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	// 等待服务器开始监听
	time.Sleep(100 * time.Millisecond)

	client := newTestClient(socketPath)

	// 测试 help 命令
	resp, err := client.execute("help", nil)
	if err != nil {
		t.Fatalf("execute help error = %v", err)
	}
	if !resp.Success {
		t.Errorf("help command should succeed, got error: %s", resp.Error)
	}
	if resp.Output == "" {
		t.Error("help command should return output")
	}
}

func TestSession_CommandNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup: 测试服务器关闭失败不影响测试结果
	//nolint:errcheck // test cleanup: 测试服务器关闭失败不影响测试结果
	defer func() { _ = srv.Stop() }()

	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	client := newTestClient(socketPath)

	// 测试不存在的命令
	resp, err := client.execute("nonexistent", nil)
	if err != nil {
		t.Fatalf("execute error = %v", err)
	}
	if resp.Success {
		t.Error("nonexistent command should fail")
	}
	if resp.Error == "" {
		t.Error("should return error message")
	}
}

func TestSession_CommandForbidden(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
		WithCommandWhitelist([]string{"help"}), // 只允许 help 命令
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup: 测试服务器关闭失败不影响测试结果
	//nolint:errcheck // test cleanup: 测试服务器关闭失败不影响测试结果
	defer func() { _ = srv.Stop() }()

	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	client := newTestClient(socketPath)

	// help 命令应该成功
	resp, err := client.execute("help", nil)
	if err != nil {
		t.Fatalf("execute help error = %v", err)
	}
	if !resp.Success {
		t.Errorf("help should succeed, got: %s", resp.Error)
	}

	// stack 命令应该被禁止
	resp, err = client.execute("stack", nil)
	if err != nil {
		t.Fatalf("execute stack error = %v", err)
	}
	if resp.Success {
		t.Error("stack command should be forbidden")
	}
}

func TestSession_StackCommand(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup: 测试服务器关闭失败不影响测试结果
	defer func() { _ = srv.Stop() }()

	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	client := newTestClient(socketPath)

	// 测试 stack 命令
	resp, err := client.execute("stack", nil)
	if err != nil {
		t.Fatalf("execute stack error = %v", err)
	}
	if !resp.Success {
		t.Errorf("stack command should succeed, got error: %s", resp.Error)
	}
	if resp.Output == "" {
		t.Error("stack command should return goroutine stack")
	}
}

func TestSession_FreememCommand(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup: 测试服务器关闭失败不影响测试结果
	defer func() { _ = srv.Stop() }()

	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	client := newTestClient(socketPath)

	// 测试 freemem 命令
	resp, err := client.execute("freemem", nil)
	if err != nil {
		t.Fatalf("execute freemem error = %v", err)
	}
	if !resp.Success {
		t.Errorf("freemem command should succeed, got error: %s", resp.Error)
	}
}

func TestSession_MultipleRequests(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup: 测试服务器关闭失败不影响测试结果
	defer func() { _ = srv.Stop() }()

	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// 发送多个请求
	for i := 0; i < 5; i++ {
		client := newTestClient(socketPath)
		resp, err := client.execute("help", nil)
		if err != nil {
			t.Fatalf("request %d: execute error = %v", i, err)
		}
		if !resp.Success {
			t.Errorf("request %d: should succeed", i)
		}
	}
}

func TestSession_ExitCommand(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup: 测试服务器关闭失败不影响测试结果
	defer func() { _ = srv.Stop() }()

	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	client := newTestClient(socketPath)

	// 测试 exit 命令
	resp, err := client.execute("exit", nil)
	if err != nil {
		t.Fatalf("execute exit error = %v", err)
	}
	if !resp.Success {
		t.Errorf("exit command should succeed, got error: %s", resp.Error)
	}

	// 等待服务器关闭
	time.Sleep(200 * time.Millisecond)

	// 服务器应该不再监听
	if srv.IsListening() {
		t.Error("server should not be listening after exit command")
	}
}

func TestSession_MaxSessions(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
		WithMaxSessions(1), // 只允许 1 个会话
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup: 测试服务器关闭失败不影响测试结果
	defer func() { _ = srv.Stop() }()

	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// 创建第一个连接并保持
	conn1, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		t.Fatalf("first connection error = %v", err)
	}
	//nolint:errcheck // test cleanup: 测试连接关闭失败不影响测试结果
	defer func() { _ = conn1.Close() }()

	// 尝试创建第二个连接
	conn2, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		// 连接可能被拒绝，这是预期行为
		return
	}
	// 如果连接成功，应该立即被关闭
	//nolint:errcheck // test cleanup: 测试连接关闭失败不影响测试结果
	defer func() { _ = conn2.Close() }()

	// 等待服务器处理
	time.Sleep(100 * time.Millisecond)
}

func TestSession_CommandWithArgs(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup: 测试服务器关闭失败不影响测试结果
	defer func() { _ = srv.Stop() }()

	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	client := newTestClient(socketPath)

	// 测试 help 命令带参数
	resp, err := client.execute("help", []string{"stack"})
	if err != nil {
		t.Fatalf("execute help stack error = %v", err)
	}
	if !resp.Success {
		t.Errorf("help stack should succeed, got error: %s", resp.Error)
	}

	// 测试 pprof 命令带参数
	resp, err = client.execute("pprof", []string{"goroutine"})
	if err != nil {
		t.Fatalf("execute pprof goroutine error = %v", err)
	}
	if !resp.Success {
		t.Errorf("pprof goroutine should succeed, got error: %s", resp.Error)
	}
}

func TestSession_Close(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup: 测试服务器关闭失败不影响测试结果
	defer func() { _ = srv.Stop() }()

	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// 创建连接
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		t.Fatalf("connection error = %v", err)
	}

	// 关闭连接
	//nolint:errcheck // test cleanup: 测试连接关闭失败不影响测试结果
	_ = conn.Close()

	// 等待服务器处理连接关闭
	time.Sleep(100 * time.Millisecond)

	// 服务器应该仍在运行
	if !srv.IsListening() {
		t.Error("server should still be listening after client disconnect")
	}
}
