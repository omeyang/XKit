package xid_test

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/omeyang/xkit/pkg/util/xid"
)

func Example_basic() {
	// 推荐：使用 WithRetry 方法，自动处理时钟回拨
	id, err := xid.NewStringWithRetry(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	// ID 长度通常在 11-13 个字符之间（取决于时间戳）
	fmt.Printf("Generated ID length in range: %v\n", len(id) >= 10 && len(id) <= 13)
	fmt.Printf("ID is not empty: %v\n", id != "")

	// Output:
	// Generated ID length in range: true
	// ID is not empty: true
}

func Example_withErrorHandling() {
	// 带错误处理的方式
	id, err := xid.NewString()
	if err != nil {
		log.Printf("Failed to generate ID: %v", err)
		return
	}
	fmt.Printf("ID generated successfully: %v\n", id != "")

	// Output:
	// ID generated successfully: true
}

func Example_parseAndDecompose() {
	// 生成 ID
	id, err := xid.New()
	if err != nil {
		log.Fatal(err)
	}

	// 分解 ID 查看各部分（纯函数，无需初始化）
	parts, err := xid.Decompose(id)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Has time component: %v\n", parts.Time > 0)
	fmt.Printf("Has machine: %v\n", parts.Machine >= 0)
	fmt.Printf("Has sequence: %v\n", parts.Sequence >= 0)

	// Output:
	// Has time component: true
	// Has machine: true
	// Has sequence: true
}

func Example_concurrent() {
	// 并发生成 ID
	var wg sync.WaitGroup
	ids := make(chan string, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids <- xid.MustNewString()
		}()
	}

	wg.Wait()
	close(ids)

	// 收集所有 ID
	uniqueIDs := make(map[string]bool)
	for id := range ids {
		uniqueIDs[id] = true
	}

	fmt.Printf("Generated %d unique IDs\n", len(uniqueIDs))

	// Output:
	// Generated 10 unique IDs
}
