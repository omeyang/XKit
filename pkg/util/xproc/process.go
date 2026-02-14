package xproc

import (
	"os"
	"path/filepath"
)

// osExecutable 是 os.Executable 的包级变量，支持测试中 mock。
var osExecutable = os.Executable

// ProcessID 返回当前进程 ID。
func ProcessID() int {
	return os.Getpid()
}

// baseName 提取路径的基础文件名。
// 对 [filepath.Base] 返回的特殊值（"."、".."、路径分隔符）返回空字符串。
func baseName(path string) string {
	name := filepath.Base(path)
	if name == "." || name == ".." || name == string(filepath.Separator) {
		return ""
	}
	return name
}

// ProcessName 返回当前进程名称（不含路径）。
// 优先使用 [os.Executable] 获取可执行文件路径（不受 os.Args 修改影响），
// 失败时回退到 os.Args[0]。
// 对 [filepath.Base] 返回的特殊值（"."、".."、路径分隔符）统一返回空字符串。
// 在极端情况下（所有来源均无效）返回空字符串。
func ProcessName() string {
	if exe, err := osExecutable(); err == nil && exe != "" {
		if name := baseName(exe); name != "" {
			return name
		}
	}
	if len(os.Args) == 0 || os.Args[0] == "" {
		return ""
	}
	return baseName(os.Args[0])
}
