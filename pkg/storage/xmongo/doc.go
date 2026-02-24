// Package xmongo 提供 MongoDB 客户端包装器和增值功能。
//
// # 设计理念
//
// xmongo 不包装底层客户端的所有 API，而是提供：
//   - 统一的工厂方法（New）
//   - 底层客户端直接暴露（Client() 方法）
//   - 增值功能（健康检查、统计、分页查询、批量插入、慢查询检测）
//
// 通过 Client() 直接执行的操作不会进入统计和慢查询检测。
//
// # 核心功能
//
//   - Client()：暴露底层 mongo.Client
//   - Health()：健康检查
//   - Stats()：统计信息
//   - FindPage()：分页查询（支持排序、字段投影）
//   - BulkInsert()：批量插入（支持 context 取消）
//   - 慢查询检测：支持同步（SlowQueryHook）和异步（AsyncSlowQueryHook）回调
//
// Close() 可安全重复调用，首次关闭执行断连，后续调用返回 ErrClosed。
// 除 Client() 外的所有方法在 Close() 后调用均返回 ErrClosed。并发安全。
//
// # Close(ctx) 签名说明
//
// 设计决策: Close 接受 context.Context 参数，与同模块的 xetcd/xcache（Close() error）签名不同。
// 原因：mongo.Client.Disconnect 支持 context 超时控制，在网络不可达时允许调用方设置关闭截止时间，
// 避免无限阻塞。如需统一 Closer 接口，可传入 context.Background()。
//
// # Write Concern / Read Preference
//
// xmongo 不提供 Write Concern 和 Read Preference 的配置入口。
// 这些属性应在创建 Collection 时通过 Client() 设置：
//
//	coll := m.Client().Database("mydb",
//	    options.Database().SetWriteConcern(writeconcern.Majority()),
//	).Collection("users",
//	    options.Collection().SetReadPreference(readpref.SecondaryPreferred()),
//	)
//	// 传入带有自定义配置的 coll 给 FindPage/BulkInsert
package xmongo
