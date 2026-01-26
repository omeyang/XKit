//go:build !windows

package xdbg

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestUnixTransport_ListenAccept(t *testing.T) {
	// 使用临时目录
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	transport := NewUnixTransport(socketPath, 0600)
	//nolint:errcheck // test cleanup: 测试传输层关闭失败不影响测试结果
	defer func() { _ = transport.Close() }()

	// 测试监听
	err := transport.Listen(context.Background())
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	// 验证 socket 文件存在
	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("socket file not found: %v", err)
	}

	// 验证文件权限
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("socket permission = %o, want 0600", perm)
	}

	// 验证地址
	if transport.Addr() != socketPath {
		t.Errorf("Addr() = %q, want %q", transport.Addr(), socketPath)
	}
}

func TestUnixTransport_CleanupExistingSocket(t *testing.T) {
	// 使用临时目录
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// 使用底层 syscall 创建一个 Unix socket 文件
	// Go 的 net.Listen("unix") 会在 Close() 时自动删除 socket 文件
	// 所以需要用 syscall 手动创建以模拟残留的 socket 文件
	fd, err := createSocketFile(socketPath)
	if err != nil {
		t.Fatalf("create socket file error = %v", err)
	}
	// 关闭 fd 但不删除 socket 文件
	//nolint:errcheck // test setup: syscall.Close 失败不影响测试
	_ = syscall.Close(fd)

	// 验证 socket 文件存在且是 socket 类型
	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("socket file should exist: %v", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("file should be a socket, got mode %v", info.Mode())
	}

	transport := NewUnixTransport(socketPath, 0600)
	//nolint:errcheck // test cleanup: 测试传输层关闭失败不影响测试结果
	defer func() { _ = transport.Close() }()

	// 应该能成功监听（清理了旧 socket 文件）
	err = transport.Listen(context.Background())
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
}

// createSocketFile 使用 syscall 创建一个 Unix socket 文件。
// 返回的 fd 应该被调用者关闭。socket 文件会保留在文件系统中。
func createSocketFile(path string) (int, error) {
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return 0, err
	}

	addr := &syscall.SockaddrUnix{Name: path}
	if err := syscall.Bind(fd, addr); err != nil {
		//nolint:errcheck // cleanup: best effort close
		_ = syscall.Close(fd)
		return 0, err
	}

	return fd, nil
}

func TestUnixTransport_RejectsNonSocketFile(t *testing.T) {
	// 使用临时目录
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// 创建一个普通文件（不是 socket）
	f, err := os.Create(socketPath)
	if err != nil {
		t.Fatalf("create file error = %v", err)
	}
	//nolint:errcheck // test cleanup: 临时文件关闭失败不影响测试结果
	_ = f.Close()

	transport := NewUnixTransport(socketPath, 0600)
	//nolint:errcheck // test cleanup: 测试传输层关闭失败不影响测试结果
	defer func() { _ = transport.Close() }()

	// 应该拒绝覆盖非 socket 文件
	err = transport.Listen(context.Background())
	if err == nil {
		t.Fatal("Listen() should fail when path is not a socket")
	}

	// 验证错误信息
	expectedErrMsg := "path exists but is not a socket"
	if !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("Listen() error = %v, want to contain %q", err, expectedErrMsg)
	}
}

func TestUnixTransport_Close(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	transport := NewUnixTransport(socketPath, 0600)

	err := transport.Listen(context.Background())
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	// 第一次关闭
	err = transport.Close()
	if err != nil {
		t.Errorf("first Close() error = %v", err)
	}

	// socket 文件应该被删除
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("socket file should be deleted after Close()")
	}

	// 第二次关闭应该是幂等的
	err = transport.Close()
	if err != nil {
		t.Errorf("second Close() error = %v", err)
	}
}

func TestUnixTransport_AcceptNotListening(t *testing.T) {
	transport := NewUnixTransport("/tmp/nonexistent.sock", 0600)

	_, _, err := transport.Accept()
	if err != ErrNotRunning {
		t.Errorf("Accept() error = %v, want ErrNotRunning", err)
	}
}

func TestUnixTransport_DefaultValues(t *testing.T) {
	transport := NewUnixTransport("", 0)

	if transport.socketPath != DefaultSocketPath {
		t.Errorf("socketPath = %q, want %q", transport.socketPath, DefaultSocketPath)
	}

	if transport.socketPerm != DefaultSocketPerm {
		t.Errorf("socketPerm = %o, want %o", transport.socketPerm, DefaultSocketPerm)
	}
}

func TestUnixTransport_GetPeerIdentity(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "identity.sock")

	transport := NewUnixTransport(socketPath, 0600)
	//nolint:errcheck // test cleanup: 测试传输层关闭失败不影响测试结果
	defer func() { _ = transport.Close() }()

	err := transport.Listen(context.Background())
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	// 在后台连接
	testDone := make(chan struct{})
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			t.Logf("client dial error = %v", err)
			return
		}
		//nolint:errcheck // test cleanup: 测试连接关闭失败不影响测试结果
		defer func() { _ = conn.Close() }()
		// 保持连接直到测试完成
		<-testDone
	}()
	defer func() {
		close(testDone)
		<-clientDone
	}()

	// 接受连接
	conn, identity, err := transport.Accept()
	if err != nil {
		t.Fatalf("Accept() error = %v", err)
	}
	//nolint:errcheck // test cleanup: 测试连接关闭失败不影响测试结果
	defer func() { _ = conn.Close() }()

	// 验证身份信息
	if identity == nil {
		t.Fatal("identity should not be nil")
	}

	// UID 应该是当前用户
	//nolint:gosec // G115: os.Getuid() 在 Linux 上返回的值始终在 uint32 范围内
	currentUID := uint32(os.Getuid())
	if identity.UID != currentUID {
		t.Errorf("UID = %d, want %d", identity.UID, currentUID)
	}

	// GID 应该是当前用户组
	//nolint:gosec // G115: os.Getgid() 在 Linux 上返回的值始终在 uint32 范围内
	currentGID := uint32(os.Getgid())
	if identity.GID != currentGID {
		t.Errorf("GID = %d, want %d", identity.GID, currentGID)
	}

	// PID 应该是有效的（大于 0）
	if identity.PID <= 0 {
		t.Errorf("PID = %d, want > 0", identity.PID)
	}
}

func TestUnixTransport_ListenClosed(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "closed.sock")

	transport := NewUnixTransport(socketPath, 0600)

	// 先关闭
	//nolint:errcheck // test setup: 此关闭操作用于测试设置，确保传输层处于已关闭状态
	_ = transport.Close()

	// 尝试监听应该失败
	err := transport.Listen(context.Background())
	if err != ErrNotRunning {
		t.Errorf("Listen() after Close() error = %v, want ErrNotRunning", err)
	}
}
