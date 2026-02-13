package xrotate

import "errors"

// 配置校验错误
var (
	// ErrEmptyFilename 文件名为空
	ErrEmptyFilename = errors.New("xrotate: filename is required")

	// ErrInvalidMaxSize MaxSizeMB 值无效（必须在 1~10240 范围内）
	ErrInvalidMaxSize = errors.New("xrotate: invalid MaxSizeMB")

	// ErrInvalidMaxBackups MaxBackups 值无效（必须在 0~1024 范围内）
	ErrInvalidMaxBackups = errors.New("xrotate: invalid MaxBackups")

	// ErrInvalidMaxAge MaxAgeDays 值无效（必须在 0~3650 范围内）
	ErrInvalidMaxAge = errors.New("xrotate: invalid MaxAgeDays")

	// ErrNoCleanupPolicy MaxBackups 和 MaxAgeDays 不能同时为 0
	ErrNoCleanupPolicy = errors.New("xrotate: no cleanup policy configured")

	// ErrInvalidFileMode FileMode 包含非权限位（仅允许低 9 位 0000~0777）
	ErrInvalidFileMode = errors.New("xrotate: invalid FileMode")

	// ErrClosed 轮转器已关闭
	ErrClosed = errors.New("xrotate: rotator is closed")
)
