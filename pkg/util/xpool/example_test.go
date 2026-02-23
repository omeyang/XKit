package xpool_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/omeyang/xkit/pkg/util/xpool"
)

func ExamplePool_Shutdown() {
	pool, err := xpool.New(2, 10, func(n int) {
		// 处理任务
	})
	if err != nil {
		panic(err)
	}

	for i := range 5 {
		if err := pool.Submit(i); err != nil {
			fmt.Println("Submit error:", err)
		}
	}

	// 带超时的优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Shutdown(ctx); err != nil {
		fmt.Println("Shutdown error:", err)
	}

	fmt.Println("shutdown complete")
	// Output:
	// shutdown complete
}

func Example() {
	var count atomic.Int32

	pool, err := xpool.New(2, 10, func(n int) {
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

	// Close 等待所有任务处理完成
	if err := pool.Close(); err != nil {
		panic(err)
	}

	fmt.Println("Processed:", count.Load())

	// Output:
	// Processed: 5
}

func Example_withLogTaskValue() {
	pool, err := xpool.New(2, 10, func(n int) {
		if n < 0 {
			panic("invalid task")
		}
	}, xpool.WithLogTaskValue())
	if err != nil {
		panic(err)
	}

	for i := range 3 {
		if err := pool.Submit(i); err != nil {
			fmt.Println("Submit error:", err)
		}
	}

	if err := pool.Close(); err != nil {
		panic(err)
	}

	fmt.Println("done")
	// Output:
	// done
}

func Example_gracefulShutdown() {
	var sum atomic.Int64

	pool, err := xpool.New(4, 100, func(n int) {
		sum.Add(int64(n))
	})
	if err != nil {
		panic(err)
	}

	// 提交任务
	for i := 1; i <= 10; i++ {
		if err := pool.Submit(i); err != nil {
			fmt.Println("Submit error:", err)
		}
	}

	// 优雅关闭：等待所有任务完成
	if err := pool.Close(); err != nil {
		panic(err)
	}

	fmt.Println("Sum:", sum.Load())

	// Output:
	// Sum: 55
}
