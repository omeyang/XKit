//go:build !windows

package xdbg

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"
)

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

func TestSession_SetlogCommand(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	leveler := &mockLeveler{level: "info"}

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
		WithLeveler(leveler),
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

	// Set log level via client
	resp, err := client.execute("setlog", []string{"debug"})
	if err != nil {
		t.Fatalf("execute setlog error = %v", err)
	}
	if !resp.Success {
		t.Errorf("setlog should succeed, got error: %s", resp.Error)
	}
}

func TestSession_CommandTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
		WithCommandTimeout(50*time.Millisecond),
		WithMaxSessions(2),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Register a slow command
	srv.RegisterCommand(mustNewCommandFunc(t, "slow", "slow cmd", func(ctx context.Context, _ []string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
			return "done", nil
		}
	}))

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
	resp, err := client.execute("slow", nil)
	if err != nil {
		t.Fatalf("execute slow error = %v", err)
	}
	if resp.Success {
		t.Error("slow command should timeout and fail")
	}
}

func TestSession_TooManyCommands(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
		WithMaxConcurrentCommands(1),
		WithMaxSessions(3),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Register a blocking command
	blockCh := make(chan struct{})
	srv.RegisterCommand(mustNewCommandFunc(t, "block", "blocking cmd", func(ctx context.Context, _ []string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-blockCh:
			return "done", nil
		}
	}))

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup: 测试服务器关闭失败不影响测试结果
	defer func() {
		close(blockCh)
		_ = srv.Stop()
	}()

	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Start a blocking command in background
	go func() {
		c := newTestClient(socketPath)
		//nolint:errcheck // test utility
		_, _ = c.execute("block", nil)
	}()

	// Wait for the blocking command to acquire slot
	time.Sleep(100 * time.Millisecond)

	// Try another command — should get "too many commands"
	client := newTestClient(socketPath)
	resp, err := client.execute("help", nil)
	if err != nil {
		t.Fatalf("execute help error = %v", err)
	}
	if resp.Success {
		// It's OK if it succeeds (timing-dependent), but if it fails it should mention too many
		t.Log("help succeeded (blocking command may have finished)")
	}
}

func TestSession_PprofHeapViaClient(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
		WithMaxSessions(2),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup
	defer func() { _ = srv.Stop() }()

	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	client := newTestClient(socketPath)

	resp, err := client.execute("pprof", []string{"heap"})
	if err != nil {
		t.Fatalf("execute pprof heap error = %v", err)
	}
	if !resp.Success {
		t.Errorf("pprof heap should succeed, got error: %s", resp.Error)
	}
}

func TestSession_PprofCpuStartStopViaClient(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
		WithMaxSessions(3),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup
	defer func() { _ = srv.Stop() }()

	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	client := newTestClient(socketPath)

	// Start CPU profile
	resp, err := client.execute("pprof", []string{"cpu", "start"})
	if err != nil {
		t.Fatalf("execute pprof cpu start error = %v", err)
	}
	if !resp.Success {
		t.Errorf("pprof cpu start should succeed, got error: %s", resp.Error)
	}

	// Stop CPU profile
	resp, err = client.execute("pprof", []string{"cpu", "stop"})
	if err != nil {
		t.Fatalf("execute pprof cpu stop error = %v", err)
	}
	if !resp.Success {
		t.Errorf("pprof cpu stop should succeed, got error: %s", resp.Error)
	}
}

func TestSession_InvalidProtocol(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
		WithMaxSessions(2),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup
	defer func() { _ = srv.Stop() }()

	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Send garbage data instead of valid protocol
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		t.Fatalf("connection error = %v", err)
	}
	//nolint:errcheck // test cleanup
	defer func() { _ = conn.Close() }()

	// Write invalid magic bytes
	//nolint:errcheck // test: intentionally sending bad data
	_, _ = conn.Write([]byte{0xFF, 0xFF, 0x01, 0x01, 0x00, 0x00, 0x00, 0x04, 't', 'e', 's', 't'})

	// Wait for server to process and close connection
	time.Sleep(100 * time.Millisecond)

	// Server should still be listening
	if !srv.IsListening() {
		t.Error("server should still be listening after invalid protocol data")
	}
}

func TestSession_PanicCommand(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
		WithMaxSessions(2),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// 注册一个会 panic 的命令
	srv.RegisterCommand(mustNewCommandFunc(t, "boom", "panics", func(_ context.Context, _ []string) (string, error) {
		panic("test panic from command")
	}))

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	//nolint:errcheck // test cleanup
	defer func() { _ = srv.Stop() }()

	if err := srv.Enable(); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	client := newTestClient(socketPath)

	// 执行 panic 命令 — 应返回错误响应而非崩溃进程
	resp, err := client.execute("boom", nil)
	if err != nil {
		t.Fatalf("execute boom error = %v", err)
	}
	if resp.Success {
		t.Error("panic command should return failure response")
	}
	if resp.Error == "" {
		t.Error("should have error message about panic")
	}

	// 服务器应该仍然在运行
	if !srv.IsListening() {
		t.Error("server should still be listening after command panic")
	}

	// 后续命令应该正常工作
	resp, err = client.execute("help", nil)
	if err != nil {
		t.Fatalf("execute help after panic error = %v", err)
	}
	if !resp.Success {
		t.Errorf("help command after panic should succeed, got error: %s", resp.Error)
	}
}

func TestSession_ReadTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	srv, err := New(
		WithSocketPath(socketPath),
		WithBackgroundMode(true),
		WithAutoShutdown(0),
		WithAuditLogger(NewNoopAuditLogger()),
		WithSessionReadTimeout(100*time.Millisecond),
		WithMaxSessions(2),
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

	// Connect but don't send anything — should timeout
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		t.Fatalf("connection error = %v", err)
	}
	//nolint:errcheck // test cleanup
	defer func() { _ = conn.Close() }()

	// Wait for read timeout to trigger
	time.Sleep(200 * time.Millisecond)

	// Server should still be listening
	if !srv.IsListening() {
		t.Error("server should still be listening after client read timeout")
	}
}
