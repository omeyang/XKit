package xfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// =============================================================================
// SanitizePath 示例
// =============================================================================

func ExampleSanitizePath() {
	// 正常路径
	path, err := SanitizePath("/var/log/app.log")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(path)

	// 路径穿越会被拒绝
	_, err = SanitizePath("../../../etc/passwd")
	if err != nil {
		fmt.Println("路径穿越被阻止")
	}
	// Output:
	// /var/log/app.log
	// 路径穿越被阻止
}

func ExampleSanitizePath_normalize() {
	// 路径会被规范化
	path, err := SanitizePath("/var/./log/../log/app.log")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(path)
	// Output: /var/log/app.log
}

// =============================================================================
// SafeJoin 示例
// =============================================================================

func ExampleSafeJoin() {
	// 正常使用
	path, err := SafeJoin("/var/log", "app.log")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(path)
	// Output: /var/log/app.log
}

func ExampleSafeJoin_pathTraversal() {
	// 路径穿越会被阻止
	_, err := SafeJoin("/var/log", "../etc/passwd")
	if err != nil {
		fmt.Println("路径穿越被阻止")
	}
	// Output: 路径穿越被阻止
}

func ExampleSafeJoin_absolutePath() {
	// 绝对路径会被拒绝
	_, err := SafeJoin("/var/log", "/etc/passwd")
	if err != nil {
		fmt.Println("绝对路径被拒绝")
	}
	// Output: 绝对路径被拒绝
}

// =============================================================================
// EnsureDir 示例
// =============================================================================

func ExampleEnsureDir() {
	tmpDir, err := os.MkdirTemp("", "xfile-example-*")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck // 示例清理

	// 确保日志文件的父目录存在
	err = EnsureDir(filepath.Join(tmpDir, "myapp", "app.log"))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("目录已创建")
	// Output: 目录已创建
}

func ExampleEnsureDirWithPerm() {
	tmpDir, err := os.MkdirTemp("", "xfile-example-*")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck // 示例清理

	// 使用自定义权限创建目录
	err = EnsureDirWithPerm(filepath.Join(tmpDir, "myapp", "app.log"), 0700)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("目录已创建")
	// Output: 目录已创建
}
