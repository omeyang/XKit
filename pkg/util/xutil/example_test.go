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

func ExampleIf_zeroValue() {
	// 零值也是有效结果——与 cmp.Or（零值判定）不同，
	// If 基于布尔条件选择，不会跳过零值。
	result := xutil.If(false, "hello", "")
	fmt.Printf("%q\n", result)
	// Output:
	// ""
}
