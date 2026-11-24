package xproc_test

import (
	"fmt"

	"github.com/omeyang/xkit/pkg/util/xproc"
)

func ExampleProcessID() {
	pid := xproc.ProcessID()
	fmt.Println(pid > 0)
	// Output:
	// true
}

func ExampleProcessName() {
	name := xproc.ProcessName()

	// ProcessName 在极端情况下可能返回空字符串，调用方应做兜底处理。
	if name == "" {
		name = "unknown"
	}
	fmt.Println(name != "")
	// Output:
	// true
}
