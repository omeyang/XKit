//go:build darwin || freebsd

// 文件名不得带 _darwin / _freebsd 等 GOOS 后缀：Go 会据文件名施加隐式约束，
// 与 //go:build 行取交集。原名 transport_unix_darwin.go 使本文件在 freebsd 上
// 被排除，getPeerIdentity 无定义，GOOS=freebsd 编译长期失败而门禁全绿。
// 故取 _bsd（非 GOOS 名，不产生隐式约束），平台集合只由上面的 build 行决定。

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
