package xmongo_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/omeyang/xkit/pkg/storage/xmongo"
)

// 注意：以下示例需要真实的 MongoDB 实例才能运行。
// 在没有 MongoDB 的环境中，这些示例仅作为文档参考。

func ExampleNew() {
	// 创建 MongoDB 客户端
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatal(err)
	}

	// 创建 xmongo 包装器
	m, err := xmongo.New(client)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := m.Close(context.Background()); err != nil {
			log.Printf("close error: %v", err)
		}
	}()

	// 健康检查
	ctx := context.Background()
	if err := m.Health(ctx); err != nil {
		log.Printf("MongoDB 不健康: %v", err)
	}

	// 直接使用底层客户端进行操作
	coll := m.Client().Database("testdb").Collection("users")
	_, err = coll.InsertOne(ctx, bson.M{"name": "Alice", "age": 30})
	if err != nil {
		log.Printf("insert error: %v", err)
	}
}

func ExampleNew_withOptions() {
	// 创建 MongoDB 客户端
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatal(err)
	}

	// 创建带选项的 xmongo 包装器
	m, err := xmongo.New(client,
		xmongo.WithHealthTimeout(10*time.Second),
		xmongo.WithSlowQueryThreshold(100*time.Millisecond),
		xmongo.WithSlowQueryHook(func(ctx context.Context, info xmongo.SlowQueryInfo) {
			log.Printf("慢查询: %s.%s %s 耗时 %v",
				info.Database, info.Collection, info.Operation, info.Duration)
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := m.Close(context.Background()); err != nil {
			log.Printf("close error: %v", err)
		}
	}()

	// 使用包装器
	ctx := context.Background()
	if err := m.Health(ctx); err != nil {
		log.Printf("health check failed: %v", err)
	}
}

func ExampleMongo_FindPage() {
	// 假设 m 是已创建的 xmongo.Mongo 实例
	// m, _ := xmongo.New(client)

	// 以下是使用示例（需要真实 MongoDB 才能运行）：
	//
	// ctx := context.Background()
	// coll := m.Client().Database("mydb").Collection("users")
	//
	// // 分页查询第一页，每页 10 条，按创建时间倒序
	// result, err := m.FindPage(ctx, coll, bson.M{"status": "active"}, xmongo.PageOptions{
	//     Page:     1,
	//     PageSize: 10,
	//     Sort:     bson.D{{"created_at", -1}},
	// })
	// if err != nil {
	//     log.Fatal(err)
	// }
	//
	// fmt.Printf("总数: %d, 总页数: %d\n", result.Total, result.TotalPages)
	// for _, doc := range result.Data {
	//     fmt.Printf("%v\n", doc)
	// }

	fmt.Println("分页查询示例")
	// Output: 分页查询示例
}

func ExampleMongo_BulkInsert() {
	// 假设 m 是已创建的 xmongo.Mongo 实例
	// m, _ := xmongo.New(client)

	// 以下是使用示例（需要真实 MongoDB 才能运行）：
	//
	// ctx := context.Background()
	// coll := m.Client().Database("mydb").Collection("logs")
	//
	// // 准备 1000 条文档
	// docs := make([]any, 1000)
	// for i := range docs {
	//     docs[i] = bson.M{
	//         "index":     i,
	//         "message":   fmt.Sprintf("log message %d", i),
	//         "timestamp": time.Now(),
	//     }
	// }
	//
	// // 批量插入，每批 100 条
	// result, err := m.BulkInsert(ctx, coll, docs, xmongo.BulkOptions{
	//     BatchSize: 100,
	//     Ordered:   false, // 无序写入，性能更高
	// })
	// if err != nil {
	//     log.Fatal(err)
	// }
	//
	// fmt.Printf("成功插入 %d 条记录\n", result.InsertedCount)
	// if len(result.Errors) > 0 {
	//     fmt.Printf("发生 %d 个错误\n", len(result.Errors))
	// }

	fmt.Println("批量插入示例")
	// Output: 批量插入示例
}

func ExampleMongo_Stats() {
	// 假设 m 是已创建的 xmongo.Mongo 实例
	// m, _ := xmongo.New(client)

	// 以下是使用示例（需要真实 MongoDB 才能运行）：
	//
	// // 获取统计信息
	// stats := m.Stats()
	// fmt.Printf("健康检查次数: %d\n", stats.PingCount)
	// fmt.Printf("健康检查失败: %d\n", stats.PingErrors)
	// fmt.Printf("慢查询次数: %d\n", stats.SlowQueries)
	// fmt.Printf("活跃连接数: %d\n", stats.Pool.InUseConnections)

	fmt.Println("统计信息示例")
	// Output: 统计信息示例
}

func Example_slowQueryMonitoring() {
	// 慢查询监控示例
	//
	// 配置慢查询检测，当查询耗时超过阈值时触发回调：
	//
	// client, _ := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	//
	// obs, _ := xmetrics.NewOTelObserver()
	//
	// m, _ := xmongo.New(client,
	//     xmongo.WithObserver(obs),
	//     xmongo.WithSlowQueryThreshold(100*time.Millisecond),
	//     xmongo.WithSlowQueryHook(func(ctx context.Context, info xmongo.SlowQueryInfo) {
	//         // 记录慢查询日志
	//         log.Printf("[SLOW] %s.%s %s filter=%v duration=%v",
	//             info.Database,
	//             info.Collection,
	//             info.Operation,
	//             info.Filter,
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
	// defer m.Close(context.Background())

	fmt.Println("慢查询监控示例")
	// Output: 慢查询监控示例
}
