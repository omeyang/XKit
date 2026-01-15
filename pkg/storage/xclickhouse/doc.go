// Package xclickhouse 提供 ClickHouse 客户端包装器和增值功能。
//
// # 设计理念
//
// xclickhouse 不包装底层客户端的所有 API，而是提供：
//   - 统一的工厂方法（New）
//   - 底层连接直接暴露（Conn() 方法）
//   - 增值功能（健康检查、统计、分页查询、批量插入、慢查询检测）
//
// 通过 Conn() 直接执行的操作不会进入统计和慢查询检测。
//
// # 核心功能
//
//   - Conn()：暴露底层 driver.Conn
//   - Health()：健康检查
//   - Stats()：统计信息
//   - QueryPage()：分页查询（统计为 2 次查询）
//   - BatchInsert()：批量插入
//
// # 快速开始
//
//	ch, err := xclickhouse.New(conn,
//	    xclickhouse.WithSlowQueryThreshold(100*time.Millisecond),
//	)
//	defer ch.Close()
//
//	// 直接使用底层连接
//	rows, err := ch.Conn().Query(ctx, "SELECT * FROM users")
//
//	// 分页查询
//	result, _ := ch.QueryPage(ctx, "SELECT id, name FROM users", xclickhouse.PageOptions{
//	    Page: 1, PageSize: 10,
//	})
package xclickhouse
