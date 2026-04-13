package xdlock_test

import (
	"fmt"
	"os"
	"testing"

	"go.uber.org/goleak"
)

// TestMain 先运行用例，再清理共享嵌入式 etcd，最后 goleak 检测。
// 设计决策：共享 Mock 在 etcd_xetcdtest_test.go 中惰性初始化，这里统一 teardown
// 以避免每个用例重启 embed.Etcd 的开销。
func TestMain(m *testing.M) {
	code := m.Run()
	if sharedEtcdMock != nil {
		sharedEtcdMock.Close()
	}
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}
