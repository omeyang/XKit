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
//
// # 已知限制
//
// ## FORMAT/SETTINGS 检测
//
// QueryPage 使用正则表达式检测 FORMAT 和 SETTINGS 子句。
// 此方法是有意的设计权衡，而非 bug：
//   - 正则检测可能对字符串字面量产生误判（如 WHERE name = 'FORMAT'）
//   - 这是已知限制，复杂 SQL 解析成本过高
//   - 遇到误判时，请使用 Conn() 直接执行查询
//
// 相关错误：ErrQueryContainsFormat, ErrQueryContainsSettings
package xclickhouse
