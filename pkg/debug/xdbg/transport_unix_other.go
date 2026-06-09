//go:build !windows && !linux && !darwin && !freebsd

package xdbg

import (
	"fmt"
	"net"
)

// getPeerIdentity 在不支持 SO_PEERCRED / LOCAL_PEERCRED 的平台上返回 nil 身份。
//
// 设计决策: 此前返回当前进程身份作为"降级"，会使审计日志把调用者伪造为服务端自身，
// 造成错误归因。改为返回 nil：ResolveIdentity(nil) 会生成字符串 "unknown"，
// 审计日志能清晰反映"对端身份不可知"，不掩盖事实。
func getPeerIdentity(conn net.Conn) (*PeerIdentity, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("not a unix connection")
	}

	// 验证连接有效性
	if unixConn.LocalAddr() == nil {
		return nil, fmt.Errorf("invalid unix connection")
	}

	// 平台不支持 peer credentials — 返回 nil 身份（审计显示 "unknown"）。
	return nil, nil
}
