//go:build !windows && !linux && !darwin && !freebsd

package xdbg

import (
	"fmt"
	"net"
	"os"
)

// getPeerIdentity 返回当前进程身份作为降级方案（非 Linux/macOS/FreeBSD）。
// 在不支持 SO_PEERCRED 或 LOCAL_PEERCRED 的平台上，我们无法获取对端身份，
// 因此返回当前进程的身份信息。调用方应注意此限制。
func getPeerIdentity(conn net.Conn) (*PeerIdentity, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("not a unix connection")
	}

	// 验证连接有效性
	if unixConn.LocalAddr() == nil {
		return nil, fmt.Errorf("invalid unix connection")
	}

	// 降级：返回当前进程身份
	// 注意：这意味着在此平台上无法真正识别对端身份
	return &PeerIdentity{
		UID: uint32(os.Getuid()),
		GID: uint32(os.Getgid()),
		PID: int32(os.Getpid()),
	}, nil
}
