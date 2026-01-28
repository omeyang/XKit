package xproc

import (
	"os"
	"path/filepath"
)

// ProcessID 返回当前进程 ID。
func ProcessID() int {
	return os.Getpid()
}

// ProcessName 返回当前进程名称（不含路径）。
// 在极端情况下（如 os.Args 为空或 os.Args[0] 为空）返回空字符串。
func ProcessName() string {
	if len(os.Args) == 0 || os.Args[0] == "" {
		return ""
	}
	return filepath.Base(os.Args[0])
}
