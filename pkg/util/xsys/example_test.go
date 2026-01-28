package xsys_test

import (
	"fmt"

	"github.com/omeyang/xkit/pkg/util/xsys"
)

func ExampleSetFileLimit() {
	// SetFileLimit 设置进程最大打开文件数（Unix 平台生效，非 Unix 返回 ErrUnsupportedPlatform）。
	// 通常在进程启动时调用一次。
	err := xsys.SetFileLimit(65536)
	if err != nil {
		fmt.Println("设置文件限制失败:", err)
	}
}

func ExampleGetFileLimit() {
	soft, hard, err := xsys.GetFileLimit()
	if err != nil {
		fmt.Println("查询文件限制失败:", err)
		return
	}
	fmt.Printf("soft=%d, hard=%d\n", soft, hard)
}
