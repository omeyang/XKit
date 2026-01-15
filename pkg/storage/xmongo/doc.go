// Package xmongo 提供 MongoDB 客户端包装器和增值功能。
//
// # 设计理念
//
// xmongo 不包装底层客户端的所有 API，而是提供：
//   - 统一的工厂方法（New）
//   - 底层客户端直接暴露（Client() 方法）
//   - 增值功能（健康检查、统计、分页查询、批量写入、慢查询检测）
//
// 通过 Client() 直接执行的操作不会进入统计和慢查询检测。
//
// # 核心功能
//
//   - Client()：暴露底层 mongo.Client
//   - Health()：健康检查
//   - Stats()：统计信息
//   - FindPage()：分页查询
//   - BulkWrite()：批量写入（支持 context 取消）
//
// # 快速开始
//
//	m, err := xmongo.New(client,
//	    xmongo.WithSlowQueryThreshold(100*time.Millisecond),
//	)
//	defer m.Close(context.Background())
//
//	// 直接使用底层客户端
//	coll := m.Client().Database("mydb").Collection("users")
//	_, err = coll.InsertOne(ctx, bson.M{"name": "test"})
//
//	// 分页查询
//	result, _ := m.FindPage(ctx, coll, bson.M{"status": "active"}, xmongo.PageOptions{
//	    Page: 1, PageSize: 10,
//	})
package xmongo
