package xpool

import (
	"math"
	"testing"
)

func FuzzSubmit(f *testing.F) {
	f.Add(1, 1)
	f.Add(0, 0)
	f.Add(-1, -1)
	f.Add(100, 100)
	f.Add(math.MaxInt, 1)           // 极端 workers
	f.Add(1, math.MaxInt)           // 极端 queueSize
	f.Add(math.MaxInt, math.MaxInt) // 双极端
	f.Add(maxWorkers, maxQueueSize) // 上限边界
	f.Add(maxWorkers+1, 1)          // 超上限 workers
	f.Add(1, maxQueueSize+1)        // 超上限 queueSize

	f.Fuzz(func(t *testing.T, workers, queueSize int) {
		pool, err := New(workers, queueSize, func(_ int) {})
		if err != nil {
			// 参数无效时应返回错误而非 panic
			return
		}
		defer pool.Close() // fuzz 测试清理

		// 提交任务不应 panic
		for i := range min(queueSize, 10) {
			pool.Submit(i) // fuzz 测试中忽略队列满
		}
	})
}
