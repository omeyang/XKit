package xhealth

import "errors"

// 参数校验错误。
var (
	// ErrNilContext 表示传入了 nil context。
	ErrNilContext = errors.New("xhealth: context cannot be nil")

	// ErrNilCheck 表示 CheckConfig.Check 为 nil。
	ErrNilCheck = errors.New("xhealth: check function cannot be nil")

	// ErrEmptyName 表示检查项名称为空。
	ErrEmptyName = errors.New("xhealth: check name cannot be empty")

	// ErrDuplicateCheck 表示同一端点下注册了重名的检查项。
	ErrDuplicateCheck = errors.New("xhealth: duplicate check name")

	// ErrInvalidInterval 表示异步检查的 Interval 无效（必须为正数）。
	ErrInvalidInterval = errors.New("xhealth: async check interval must be positive")

	// ErrAlreadyStarted 表示 Health 已经启动，不能重复调用 Run。
	ErrAlreadyStarted = errors.New("xhealth: already started")

	// ErrNotStarted 表示 Health 尚未启动。
	ErrNotStarted = errors.New("xhealth: not started")

	// ErrShutdown 表示 Health 已关闭，不能执行操作。
	ErrShutdown = errors.New("xhealth: already shut down")

	// ErrInvalidAddr 表示 HTTP 监听地址无效。
	ErrInvalidAddr = errors.New("xhealth: invalid listen address")

	// ErrCheckNotFound 表示查询的检查项不存在。
	ErrCheckNotFound = errors.New("xhealth: check not found")
)
