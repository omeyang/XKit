//go:build linux

package xdbg

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// getPeerIdentity 通过 SO_PEERCRED 获取对端身份（Linux）。
// 使用 SyscallConn 避免 File() 调用将连接切换到阻塞模式的副作用。
func getPeerIdentity(conn net.Conn) (*PeerIdentity, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("not a unix connection")
	}

	// 使用 SyscallConn 而不是 File() 来避免副作用
	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("get syscall conn: %w", err)
	}

	var cred *unix.Ucred
	var credErr error

	err = rawConn.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if err != nil {
		return nil, fmt.Errorf("control syscall conn: %w", err)
	}
	if credErr != nil {
		return nil, fmt.Errorf("getsockopt SO_PEERCRED: %w", credErr)
	}

	return &PeerIdentity{
		UID: cred.Uid,
		GID: cred.Gid,
		PID: cred.Pid,
	}, nil
}
