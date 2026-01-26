//go:build darwin || freebsd

package xdbg

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// getPeerIdentity 通过 LOCAL_PEERCRED 获取对端身份（macOS/FreeBSD）。
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

	var cred *unix.Xucred
	var credErr error

	err = rawConn.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	})
	if err != nil {
		return nil, fmt.Errorf("control syscall conn: %w", err)
	}
	if credErr != nil {
		return nil, fmt.Errorf("getsockopt LOCAL_PEERCRED: %w", credErr)
	}

	var gid uint32
	if len(cred.Groups) > 0 {
		gid = cred.Groups[0] // 主组 ID
	}

	return &PeerIdentity{
		UID: cred.Uid,
		GID: gid,
		PID: 0, // LOCAL_PEERCRED 不返回 PID
	}, nil
}
