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
	// 设计决策: 此错误供客户端代码使用（如 xdbgctl 判断响应是否被截断），
	// 服务端通过 Response.Truncated 布尔字段传递截断状态，不直接使用此错误。
	ErrOutputTruncated = errors.New("xdbg: output truncated")

	// ErrInvalidState 表示服务器状态无效，无法执行此操作。
	ErrInvalidState = errors.New("xdbg: invalid server state for this operation")

	// ErrNilContext 表示传入的 context 为 nil。
	ErrNilContext = errors.New("xdbg: context must not be nil")

	// ErrEmptyCommandName 表示命令名不能为空。
	ErrEmptyCommandName = errors.New("xdbg: command name must not be empty")

	// ErrNilCommandFunc 表示命令函数不能为 nil。
	ErrNilCommandFunc = errors.New("xdbg: command function must not be nil")

	// ErrNilCommand 表示命令不能为 nil。
	ErrNilCommand = errors.New("xdbg: command must not be nil")
)
