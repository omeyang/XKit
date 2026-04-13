package xdbg

import (
	"context"
	"net"
)

// DefaultSocketPath 默认的 Unix Socket 路径。
const DefaultSocketPath = "/var/run/xdbg.sock"

// DefaultSocketPerm 默认的 Socket 文件权限。
const DefaultSocketPerm = 0600

// Transport 传输层接口。
type Transport interface {
	// Listen 开始监听连接。
	Listen(ctx context.Context) error

	// Accept 接受新连接。
	// 返回连接和对端身份信息。
	Accept() (net.Conn, *PeerIdentity, error)

	// Close 关闭传输层。
	Close() error

	// Addr 返回监听地址。
	Addr() string
}

// PeerIdentity 对端身份信息。
type PeerIdentity struct {
	// UID 用户 ID。
	UID uint32 `json:"uid"`

	// GID 组 ID。
	GID uint32 `json:"gid"`

	// PID 进程 ID。
	PID int32 `json:"pid"`
}
