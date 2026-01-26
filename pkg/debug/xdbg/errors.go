package xdbg

import "errors"

// 预定义错误。
var (
	// ErrNotRunning 表示调试服务未运行。
	ErrNotRunning = errors.New("xdbg: debug server is not running")

	// ErrAlreadyRunning 表示调试服务已在运行。
	ErrAlreadyRunning = errors.New("xdbg: debug server is already running")

	// ErrCommandNotFound 表示命令未找到。
	ErrCommandNotFound = errors.New("xdbg: command not found")

	// ErrCommandForbidden 表示命令被禁止执行（不在白名单中）。
	ErrCommandForbidden = errors.New("xdbg: command is forbidden")

	// ErrTimeout 表示命令执行超时。
	ErrTimeout = errors.New("xdbg: command execution timeout")

	// ErrTooManySessions 表示已达到最大会话数限制。
	ErrTooManySessions = errors.New("xdbg: too many concurrent sessions")

	// ErrTooManyCommands 表示已达到最大并发命令数限制。
	ErrTooManyCommands = errors.New("xdbg: too many concurrent commands")

	// ErrInvalidMessage 表示消息格式无效。
	ErrInvalidMessage = errors.New("xdbg: invalid message format")

	// ErrMessageTooLarge 表示消息过大。
	ErrMessageTooLarge = errors.New("xdbg: message too large")

	// ErrConnectionClosed 表示连接已关闭。
	ErrConnectionClosed = errors.New("xdbg: connection closed")

	// ErrOutputTruncated 表示输出被截断。
	ErrOutputTruncated = errors.New("xdbg: output truncated")

	// ErrInvalidState 表示服务器状态无效，无法执行此操作。
	ErrInvalidState = errors.New("xdbg: invalid server state for this operation")
)
