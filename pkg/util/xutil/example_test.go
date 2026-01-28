package xutil_test

import (
	"fmt"

	"github.com/omeyang/xkit/pkg/util/xutil"
)

func ExampleIf() {
	name := xutil.If(true, "Alice", "Bob")
	fmt.Println(name)

	port := xutil.If(false, 8080, 80)
	fmt.Println(port)
	// Output:
	// Alice
	// 80
}
