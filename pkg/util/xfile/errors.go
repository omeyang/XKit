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
)
