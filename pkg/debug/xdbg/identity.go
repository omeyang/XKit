package xdbg

import (
	"fmt"
	"os/user"
)

// IdentityInfo 身份信息（包含用户名）。
type IdentityInfo struct {
	*PeerIdentity

	// Username 用户名（可能为空，如果无法查找）。
	Username string

	// Groupname 组名（可能为空，如果无法查找）。
	Groupname string
}

// ResolveIdentity 解析身份信息，查找用户名和组名。
// 如果 peer 为 nil，返回一个空的身份信息。
func ResolveIdentity(peer *PeerIdentity) *IdentityInfo {
	info := &IdentityInfo{
		PeerIdentity: peer,
	}

	// 如果 peer 为 nil，返回空身份信息
	if peer == nil {
		return info
	}

	// 尝试查找用户名
	if u, err := user.LookupId(fmt.Sprintf("%d", peer.UID)); err == nil {
		info.Username = u.Username
	}

	// 尝试查找组名
	if g, err := user.LookupGroupId(fmt.Sprintf("%d", peer.GID)); err == nil {
		info.Groupname = g.Name
	}

	return info
}

// String 返回身份信息的字符串表示。
func (i *IdentityInfo) String() string {
	// 处理 nil 情况
	if i == nil || i.PeerIdentity == nil {
		return "unknown"
	}

	username := i.Username
	if username == "" {
		username = fmt.Sprintf("uid=%d", i.UID)
	}

	groupname := i.Groupname
	if groupname == "" {
		groupname = fmt.Sprintf("gid=%d", i.GID)
	}

	return fmt.Sprintf("%s(%s) pid=%d", username, groupname, i.PID)
}
