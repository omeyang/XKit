package xid_test

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/omeyang/xkit/pkg/util/xid"
)

func Example_basic() {
	// 推荐：使用 NewStringWithRetry 并显式处理错误
	id, err := xid.NewStringWithRetry(context.Background())
	if err != nil {
		log.Printf("Failed to generate ID: %v", err)
		return
	}
	// ID 长度通常为 12-13 个字符（取决于时间戳大小）
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
	fmt.Printf("Machine in range: %v\n", parts.Machine >= 0 && parts.Machine <= 65535)
	fmt.Printf("Sequence in range: %v\n", parts.Sequence >= 0 && parts.Sequence <= 255)

	// Output:
	// Has time component: true
	// Machine in range: true
	// Sequence in range: true
}

func Example_concurrent() {
	// 并发生成 ID
	var wg sync.WaitGroup
	type result struct {
		id  string
		err error
	}
	ids := make(chan result, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := xid.NewStringWithRetry(context.Background())
			ids <- result{id: id, err: err}
		}()
	}

	wg.Wait()
	close(ids)

	// 收集所有 ID
	uniqueIDs := make(map[string]bool)
	for result := range ids {
		if result.err != nil {
			log.Printf("Failed to generate ID: %v", result.err)
			return
		}
		uniqueIDs[result.id] = true
	}

	fmt.Printf("Generated %d unique IDs\n", len(uniqueIDs))

	// Output:
	// Generated 10 unique IDs
}
