//go:build !windows

package xdbg

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
)

// UnixTransport Unix Socket 传输层实现。
type UnixTransport struct {
	socketPath string
	socketPerm os.FileMode
	listener   net.Listener
	mu         sync.Mutex
	closed     bool
}

// NewUnixTransport 创建 Unix Socket 传输层。
func NewUnixTransport(socketPath string, socketPerm os.FileMode) *UnixTransport {
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	if socketPerm == 0 {
		socketPerm = DefaultSocketPerm
	}
	return &UnixTransport{
		socketPath: socketPath,
		socketPerm: socketPerm,
	}
}

// Listen 开始监听 Unix Socket。
func (t *UnixTransport) Listen(_ context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrNotRunning
	}

	// 清理可能残留的 socket 文件
	info, err := os.Stat(t.socketPath)
	if err == nil {
		// 文件存在，检查类型
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("path exists but is not a socket: %s", t.socketPath)
		}
		// 是 socket，安全删除
		if err := os.Remove(t.socketPath); err != nil {
			return fmt.Errorf("remove existing socket: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check existing socket: %w", err)
	}

	// 创建 Unix Socket
	listener, err := net.Listen("unix", t.socketPath)
	if err != nil {
		return fmt.Errorf("listen unix socket: %w", err)
	}

	// 设置文件权限
	if err := os.Chmod(t.socketPath, t.socketPerm); err != nil {
		if closeErr := listener.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "[XDBG] failed to close listener during cleanup: %v\n", closeErr)
		}
		return fmt.Errorf("chmod socket: %w", err)
	}

	t.listener = listener
	return nil
}

// Accept 接受新连接并获取对端身份。
func (t *UnixTransport) Accept() (net.Conn, *PeerIdentity, error) {
	t.mu.Lock()
	listener := t.listener
	t.mu.Unlock()

	if listener == nil {
		return nil, nil, ErrNotRunning
	}

	conn, err := listener.Accept()
	if err != nil {
		return nil, nil, fmt.Errorf("accept: %w", err)
	}

	// 获取对端身份
	identity, err := getPeerIdentity(conn)
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "[XDBG] failed to close connection during cleanup: %v\n", closeErr)
		}
		return nil, nil, fmt.Errorf("get peer identity: %w", err)
	}

	return conn, identity, nil
}

// Close 关闭传输层。
func (t *UnixTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	var err error
	if t.listener != nil {
		err = t.listener.Close()
		t.listener = nil
	}

	// 清理 socket 文件
	if removeErr := os.Remove(t.socketPath); removeErr != nil && !os.IsNotExist(removeErr) {
		fmt.Fprintf(os.Stderr, "[XDBG] failed to remove socket file: %v\n", removeErr)
	}

	return err
}

// Addr 返回监听地址。
func (t *UnixTransport) Addr() string {
	return t.socketPath
}
