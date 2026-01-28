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
	fmt.Println(name != "")
	// Output:
	// true
}
