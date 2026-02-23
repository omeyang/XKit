//go:build !windows

package xdbg

// ServerState 服务器状态。
//
// 状态转换图（所有转换通过 CAS 原子操作实现）：
//
//	Created ──Start()──→ Started ──Enable()──→ Listening
//	                         ↑                    │
//	                         └───Disable()────────┘
//	                         │                    │
//	                         └───Stop()──→ Stopped ←──Stop()──┘
//
//	Created ──Stop()──→ Stopped
//
// 注意: Stopped 是终态，不可转换到其他状态。
type ServerState int32

const (
	// ServerStateCreated 已创建，未启动。
	ServerStateCreated ServerState = iota

	// ServerStateStarted 已启动，等待触发。
	ServerStateStarted

	// ServerStateListening 正在监听连接。
	ServerStateListening

	// ServerStateStopped 已停止。
	ServerStateStopped
)

// String 返回状态的字符串表示。
func (s ServerState) String() string {
	switch s {
	case ServerStateCreated:
		return "Created"
	case ServerStateStarted:
		return "Started"
	case ServerStateListening:
		return "Listening"
	case ServerStateStopped:
		return "Stopped"
	default:
		return "Unknown"
	}
}
