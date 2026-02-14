package xpool_test

import (
	"fmt"
	"sync/atomic"

	"github.com/omeyang/xkit/pkg/util/xpool"
)

func Example() {
	var count atomic.Int32

	pool, err := xpool.NewWorkerPool(2, 10, func(n int) {
		count.Add(1)
	})
	if err != nil {
		panic(err)
	}

	for i := range 5 {
		if err := pool.Submit(i); err != nil {
			fmt.Println("Submit error:", err)
		}
	}

	// Stop 等待所有任务处理完成
	pool.Stop()

	fmt.Println("Processed:", count.Load())

	// Output:
	// Processed: 5
}

func Example_gracefulShutdown() {
	var sum atomic.Int64

	pool, err := xpool.NewWorkerPool(4, 100, func(n int) {
		sum.Add(int64(n))
	})
	if err != nil {
		panic(err)
	}

	// 提交任务
	for i := 1; i <= 10; i++ {
		pool.Submit(i) //nolint:errcheck // 示例中简化错误处理
	}

	// 优雅关闭：等待所有任务完成
	pool.Stop()

	fmt.Println("Sum:", sum.Load())

	// Output:
	// Sum: 55
}
