package xfile

import "errors"

var (
	// ErrEmptyPath 表示必需的路径参数为空。
	ErrEmptyPath = errors.New("xfile: path is required")

	// ErrInvalidPath 表示路径格式无效（如目录路径、非绝对路径等）。
	ErrInvalidPath = errors.New("xfile: invalid path")

	// ErrPathTraversal 表示检测到路径穿越攻击（".." 路径段）。
	ErrPathTraversal = errors.New("xfile: path traversal detected")

	// ErrPathEscaped 表示路径超出了指定的基准目录范围。
	ErrPathEscaped = errors.New("xfile: path escapes base directory")

	// ErrPathTooDeep 表示路径层级过深，无法完成符号链接解析。
	ErrPathTooDeep = errors.New("xfile: path too deep")

	// ErrSymlinkResolution 表示符号链接解析失败。
	ErrSymlinkResolution = errors.New("xfile: symlink resolution failed")

	// ErrNullByte 表示路径中包含空字节（\x00），Linux 内核会在空字节处截断路径，
	// 导致 Go 代码与操作系统看到的路径不一致。
	ErrNullByte = errors.New("xfile: path contains null byte")

	// ErrInvalidPerm 表示目录权限无效（如缺少所有者执行位，目录无法遍历）。
	ErrInvalidPerm = errors.New("xfile: invalid directory permission")
)
