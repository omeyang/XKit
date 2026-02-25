//go:build !windows

package xdbg

import (
	"context"
	"testing"

	"go.uber.org/goleak"
)

// mustNewCommandFunc 是测试辅助函数，创建 CommandFunc 并在失败时终止测试。
func mustNewCommandFunc(tb testing.TB, name, help string, fn func(ctx context.Context, args []string) (string, error)) *CommandFunc {
	tb.Helper()
	cmd, err := NewCommandFunc(name, help, fn)
	if err != nil {
		tb.Fatalf("NewCommandFunc(%q) error = %v", name, err)
	}
	return cmd
}

// TestMain 在所有测试完成后检测 goroutine 泄漏。
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// 忽略已知的后台 goroutine
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"),
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
	)
}
