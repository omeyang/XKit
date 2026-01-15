package xfile

import (
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
func EnsureDir(filename string) error {
	return EnsureDirWithPerm(filename, DefaultDirPerm)
}

// EnsureDirWithPerm 确保文件的父目录存在，使用指定权限
//
// 参数：
//   - filename: 文件路径（不是目录路径）
//   - perm: 目录权限
//
// 如果目录已存在，不会修改其权限。
func EnsureDirWithPerm(filename string, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, perm)
}
