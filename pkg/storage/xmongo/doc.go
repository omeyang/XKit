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
//   - FindPage()：分页查询
//   - BulkInsert()：批量插入（支持 context 取消）
//   - 慢查询检测：支持同步（SlowQueryHook）和异步（AsyncSlowQueryHook）回调
//
// Close() 可安全重复调用，首次关闭执行断连，后续调用返回 ErrClosed。
// 所有方法在 Close() 后调用均返回 ErrClosed。并发安全。
package xmongo
