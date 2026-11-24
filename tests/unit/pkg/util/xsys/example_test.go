package xsys_test

import (
	"fmt"

	"github.com/omeyang/xkit/pkg/util/xsys"
)

// ExampleSetFileLimit 展示如何设置进程最大打开文件数。
// 不使用 // Output: 断言，因为输出取决于平台和权限（非 Unix 返回错误，
// Unix 下可能因权限不足返回 EPERM）。
func ExampleSetFileLimit() {
	err := xsys.SetFileLimit(65536)
	if err != nil {
		fmt.Println("设置文件限制失败:", err)
	}
}

// ExampleSetFileLimit_invalidParam 展示参数校验的错误返回。
// 零值在所有平台上均返回 ErrInvalidFileLimit，输出确定性可用作回归断言。
func ExampleSetFileLimit_invalidParam() {
	err := xsys.SetFileLimit(0)
	fmt.Println(err)
	// Output: xsys: file limit must be greater than 0
}

// ExampleGetFileLimit 展示如何查询进程当前文件限制。
// 不使用 // Output: 断言，因为 soft/hard limit 值取决于运行环境。
func ExampleGetFileLimit() {
	soft, hard, err := xsys.GetFileLimit()
	if err != nil {
		fmt.Println("查询文件限制失败:", err)
		return
	}
	fmt.Printf("soft=%d, hard=%d\n", soft, hard)
}
