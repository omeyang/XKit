package xclickhouse_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"github.com/omeyang/xkit/pkg/storage/xclickhouse"
)

// 注意：以下示例需要真实的 ClickHouse 实例才能运行。
// 在没有 ClickHouse 的环境中，这些示例仅作为文档参考。

func ExampleNew() {
	// 创建 ClickHouse 连接
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{"localhost:9000"},
		Auth: clickhouse.Auth{
			Database: "default",
			Username: "default",
			Password: "",
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	// 创建 xclickhouse 包装器
	ch, err := xclickhouse.New(conn)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := ch.Close(); err != nil {
			log.Printf("close error: %v", err)
		}
	}()

	// 健康检查
	ctx := context.Background()
	if err := ch.Health(ctx); err != nil {
		log.Printf("ClickHouse 不健康: %v", err)
	}

	// 直接使用底层连接进行操作
	rows, err := ch.Conn().Query(ctx, "SELECT 1")
	if err != nil {
		log.Printf("query error: %v", err)
	}
	if rows != nil {
		defer func() { _ = rows.Close() }() //nolint:errcheck // defer cleanup
	}
}

func ExampleNew_withOptions() {
	// 创建 ClickHouse 连接
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{"localhost:9000"},
	})
	if err != nil {
		log.Fatal(err)
	}

	// 创建带选项的 xclickhouse 包装器
	ch, err := xclickhouse.New(conn,
		xclickhouse.WithHealthTimeout(10*time.Second),
		xclickhouse.WithSlowQueryThreshold(100*time.Millisecond),
		xclickhouse.WithSlowQueryHook(func(ctx context.Context, info xclickhouse.SlowQueryInfo) {
			log.Printf("慢查询: %s 耗时 %v", info.Query, info.Duration)
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := ch.Close(); err != nil {
			log.Printf("close error: %v", err)
		}
	}()

	// 使用包装器
	ctx := context.Background()
	if err := ch.Health(ctx); err != nil {
		log.Printf("health check failed: %v", err)
	}
}

func ExampleClickHouse_QueryPage() {
	// 假设 ch 是已创建的 xclickhouse.ClickHouse 实例
	// ch, _ := xclickhouse.New(conn)

	// 以下是使用示例（需要真实 ClickHouse 才能运行）：
	//
	// ctx := context.Background()
	//
	// // 分页查询第一页，每页 10 条
	// result, err := ch.QueryPage(ctx, "SELECT id, name FROM users WHERE status = 1", xclickhouse.PageOptions{
	//     Page:     1,
	//     PageSize: 10,
	// })
	// if err != nil {
	//     log.Fatal(err)
	// }
	//
	// fmt.Printf("总数: %d, 总页数: %d\n", result.Total, result.TotalPages)
	// fmt.Printf("列: %v\n", result.Columns)
	// for _, row := range result.Rows {
	//     fmt.Printf("%v\n", row)
	// }

	fmt.Println("分页查询示例")
	// Output: 分页查询示例
}

func ExampleClickHouse_BatchInsert() {
	// 假设 ch 是已创建的 xclickhouse.ClickHouse 实例
	// ch, _ := xclickhouse.New(conn)

	// 以下是使用示例（需要真实 ClickHouse 才能运行）：
	//
	// type LogEntry struct {
	//     Timestamp time.Time `ch:"timestamp"`
	//     Level     string    `ch:"level"`
	//     Message   string    `ch:"message"`
	// }
	//
	// ctx := context.Background()
	//
	// // 准备 1000 条日志
	// logs := make([]any, 1000)
	// for i := range logs {
	//     logs[i] = &LogEntry{
	//         Timestamp: time.Now(),
	//         Level:     "INFO",
	//         Message:   fmt.Sprintf("log message %d", i),
	//     }
	// }
	//
	// // 批量写入，每批 100 条
	// result, err := ch.BatchInsert(ctx, "logs", logs, xclickhouse.BatchOptions{
	//     BatchSize: 100,
	// })
	// if err != nil {
	//     log.Fatal(err)
	// }
	//
	// fmt.Printf("成功插入 %d 条记录\n", result.InsertedCount)
	// if len(result.Errors) > 0 {
	//     fmt.Printf("发生 %d 个错误\n", len(result.Errors))
	// }

	fmt.Println("批量写入示例")
	// Output: 批量写入示例
}

func ExampleClickHouse_Stats() {
	// 假设 ch 是已创建的 xclickhouse.ClickHouse 实例
	// ch, _ := xclickhouse.New(conn)

	// 以下是使用示例（需要真实 ClickHouse 才能运行）：
	//
	// // 获取统计信息
	// stats := ch.Stats()
	// fmt.Printf("健康检查次数: %d\n", stats.PingCount)
	// fmt.Printf("健康检查失败: %d\n", stats.PingErrors)
	// fmt.Printf("查询次数: %d\n", stats.QueryCount)
	// fmt.Printf("查询失败: %d\n", stats.QueryErrors)
	// fmt.Printf("慢查询次数: %d\n", stats.SlowQueries)

	fmt.Println("统计信息示例")
	// Output: 统计信息示例
}

func Example_slowQueryMonitoring() {
	// 慢查询监控示例
	//
	// 配置慢查询检测，当查询耗时超过阈值时触发回调：
	//
	// conn, _ := clickhouse.Open(&clickhouse.Options{
	//     Addr: []string{"localhost:9000"},
	// })
	//
	// obs, _ := xmetrics.NewOTelObserver()
	//
	// ch, _ := xclickhouse.New(conn,
	//     xclickhouse.WithObserver(obs),
	//     xclickhouse.WithSlowQueryThreshold(100*time.Millisecond),
	//     xclickhouse.WithSlowQueryHook(func(ctx context.Context, info xclickhouse.SlowQueryInfo) {
	//         // 记录慢查询日志
	//         log.Printf("[SLOW] %s args=%v duration=%v",
	//             info.Query,
	//             info.Args,
	//             info.Duration,
	//         )
	//
	//         // 可以发送告警
	//         // alerting.Send("慢查询告警", info)
	//
	//         // Observer 会自动记录统一指标与 Trace
	//         // 如需自定义告警/埋点，可在此补充
	//     }),
	// )
	//
	// defer ch.Close()

	fmt.Println("慢查询监控示例")
	// Output: 慢查询监控示例
}
