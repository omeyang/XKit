package xfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultDirPerm 默认目录权限
//
// 0750 权限说明：
//   - 所有者：读写执行 (7)
//   - 组：读执行 (5)
//   - 其他：无权限 (0)
//
// 符合 gosec G301 安全建议
const DefaultDirPerm = 0750

// EnsureDir 确保文件的父目录存在
//
// 使用默认权限 0750 创建目录。
// 如果目录已存在，不会报错。
//
// 安全注意：底层使用 os.MkdirAll，会跟随符号链接。如果路径中包含指向外部的
// 符号链接，目录可能被创建在符号链接目标位置。如需防护此风险，请先使用
// SafeJoinWithOptions（启用 ResolveSymlinks）验证路径。
//
// 本函数不会拒绝包含 ".." 的路径段。若 filename 来自不可信输入，应先做路径约束：
//   - 仅做格式校验：使用 SanitizePath
//   - 需要限制在固定目录内：使用 SafeJoin 或 SafeJoinWithOptions
func EnsureDir(filename string) error {
	return EnsureDirWithPerm(filename, DefaultDirPerm)
}

// EnsureDirWithPerm 确保文件的父目录存在，使用指定权限
//
// 参数：
//   - filename: 文件路径（不是目录路径），不能为空，不能包含空字节
//   - perm: 目录权限，必须包含所有者执行位（0100），否则目录无法遍历
//
// 如果目录已存在，不会修改其权限。
//
// 安全注意：底层使用 os.MkdirAll，会跟随符号链接。参见 [EnsureDir] 的安全注意事项。
// 本函数同样不会拒绝 ".." 路径段；不可信输入应先经 [SanitizePath] 或
// [SafeJoin]/[SafeJoinWithOptions] 校验后再调用。
func EnsureDirWithPerm(filename string, perm os.FileMode) error {
	if filename == "" {
		return fmt.Errorf("filename is required: %w", ErrEmptyPath)
	}
	if containsNullByte(filename) {
		return fmt.Errorf("filename contains null byte: %w", ErrNullByte)
	}
	// 目录必须包含所有者执行位（0100），否则无法进入和遍历
	if perm&0100 == 0 {
		return fmt.Errorf("directory permission %04o missing owner execute bit: %w", perm, ErrInvalidPerm)
	}
	dir := filepath.Dir(filename)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, perm)
}
