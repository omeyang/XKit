package xrotate_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/omeyang/xkit/pkg/observability/xrotate"
)

func ExampleNewLumberjack() {
	tmpDir, err := os.MkdirTemp("", "xrotate-example-*")
	if err != nil {
		fmt.Println("创建临时目录失败:", err)
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	filename := filepath.Join(tmpDir, "app.log")

	r, err := xrotate.NewLumberjack(filename,
		xrotate.WithMaxSize(100),     // 100MB 触发轮转
		xrotate.WithMaxBackups(7),    // 保留 7 个备份
		xrotate.WithMaxAge(30),       // 保留 30 天
		xrotate.WithCompress(true),   // 压缩备份
		xrotate.WithLocalTime(false), // 使用 UTC 时间
	)
	if err != nil {
		fmt.Println("创建失败:", err)
		return
	}
	defer r.Close()

	_, _ = r.Write([]byte("hello xrotate\n"))
	fmt.Println("写入成功")
	// Output: 写入成功
}

func ExampleNewLumberjack_withOnError() {
	tmpDir, err := os.MkdirTemp("", "xrotate-example-*")
	if err != nil {
		fmt.Println("创建临时目录失败:", err)
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	filename := filepath.Join(tmpDir, "app.log")

	r, err := xrotate.NewLumberjack(filename,
		xrotate.WithFileMode(0644),
		xrotate.WithOnError(func(err error) {
			// 注意：不要向同一 Rotator 写入，避免递归
			fmt.Fprintf(os.Stderr, "xrotate error: %v\n", err)
		}),
	)
	if err != nil {
		fmt.Println("创建失败:", err)
		return
	}
	defer r.Close()

	_, _ = r.Write([]byte("hello\n"))
	fmt.Println("写入成功")
	// Output: 写入成功
}

func ExampleNewLumberjack_withFileMode() {
	tmpDir, err := os.MkdirTemp("", "xrotate-example-*")
	if err != nil {
		fmt.Println("创建临时目录失败:", err)
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	filename := filepath.Join(tmpDir, "app.log")

	r, err := xrotate.NewLumberjack(filename,
		xrotate.WithFileMode(0644), // 自定义文件权限
	)
	if err != nil {
		fmt.Println("创建失败:", err)
		return
	}
	defer r.Close()

	_, _ = r.Write([]byte("hello\n"))
	fmt.Println("写入成功")
	// Output: 写入成功
}
