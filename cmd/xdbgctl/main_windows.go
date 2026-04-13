//go:build windows

// 设计决策: xdbgctl 不支持 Windows 平台。
// xdbgctl 依赖 Unix Domain Socket（进程间通信）和 POSIX 信号（SIGUSR1 toggle），
// 这些是 Unix/Linux 特性，在 Windows 上没有直接等价实现。
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "xdbgctl: 不支持 Windows 平台（依赖 Unix Domain Socket 和 POSIX 信号）")
	os.Exit(1)
}
