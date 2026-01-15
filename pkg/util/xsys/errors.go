package xsys

import "errors"

var (
	// ErrInvalidFileLimit 表示文件限制值无效。
	ErrInvalidFileLimit = errors.New("xsys: file limit must be greater than 0")

	// ErrUnsupportedPlatform 表示当前平台不支持此操作。
	ErrUnsupportedPlatform = errors.New("xsys: unsupported platform")
)
