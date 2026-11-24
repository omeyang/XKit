package xrotate

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain 在所有测试完成后检测 goroutine 泄漏。
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// 设计决策: lumberjack 的 millRun goroutine 通过 sync.Once 启动，
		// 循环从 millCh channel 接收压缩/清理任务。lumberjack 的 Close()
		// 不关闭 millCh，导致 millRun goroutine 在 Logger 生命周期结束后仍驻留。
		// 这是上游已知限制，无法在 wrapper 层修复。
		goleak.IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun"),
	)
}
