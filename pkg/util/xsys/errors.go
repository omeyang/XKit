package xsys

import "errors"

var (
	// ErrInvalidFileLimit 表示文件限制值无效。
	ErrInvalidFileLimit = errors.New("xsys: file limit must be greater than 0")

	// ErrUnsupportedPlatform 表示当前平台不支持此操作。
	ErrUnsupportedPlatform = errors.New("xsys: unsupported platform")

	// ErrFileLimitOverflow 表示文件限制值超出当前平台 rlimit 字段的表示范围。
	// 仅在 FreeBSD/DragonFly 等 Rlimit 字段为 int64 的平台上可能出现，
	// 当 limit > math.MaxInt64 时返回。
	ErrFileLimitOverflow = errors.New("xsys: file limit exceeds platform rlimit range")
)
