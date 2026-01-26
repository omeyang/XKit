//go:build !windows

package xdbg

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain 在所有测试完成后检测 goroutine 泄漏。
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// 忽略已知的后台 goroutine
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"),
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
	)
}
