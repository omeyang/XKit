// Package xclickhouse 提供 ClickHouse 客户端包装器和增值功能。
//
// # 设计理念
//
// xclickhouse 不包装底层客户端的所有 API，而是提供：
//   - 统一的工厂方法（New）
//   - 底层连接直接暴露（Client() 方法）
//   - 增值功能（健康检查、统计、分页查询、批量插入、慢查询检测）
//
// 通过 Client() 直接执行的操作不会进入统计和慢查询检测。
// 异步插入（AsyncInsert）、Exec 等未封装的操作也应通过 Client() 使用。
//
// # 核心功能
//
//   - Client()：暴露底层 driver.Conn（关闭后仍可调用，底层操作返回驱动层错误）
//   - Health()：健康检查（关闭后返回 ErrClosed）
//   - Stats()：统计信息
//   - QueryPage()：分页查询（关闭后返回 ErrClosed，统计为 2 次查询，PageSize 上限 MaxPageSize）
//   - BatchInsert()：批量插入（关闭后返回 ErrClosed，context 取消时中止当前批次，不发送部分数据，BatchSize 上限 MaxBatchSize）
//   - Close()：幂等关闭（多次调用安全，第二次起返回 ErrClosed）
//
// # 已知限制
//
// ## OFFSET 分页
//
// QueryPage 使用 LIMIT/OFFSET 分页。在 ClickHouse 中，大偏移量会导致
// 扫描放大和性能下降。如需大数据量分页，请使用 Client() 实现游标分页。
// PageSize 受 MaxPageSize（默认 10000）限制，超过时返回 ErrPageSizeTooLarge。
// Offset 受 MaxOffset（默认 100000）限制，超过时返回 ErrOffsetTooLarge。
// QueryPage 会检测查询末尾的 LIMIT/OFFSET 子句并返回 ErrQueryContainsLimitOffset。
//
// ## FORMAT/SETTINGS 检测
//
// 设计决策: QueryPage 使用正则表达式检测 FORMAT 和 SETTINGS 子句。
// 此方法是有意的设计权衡，而非 bug：
//   - 正则检测可能对字符串字面量产生误判（如 WHERE name = 'FORMAT'）
//   - 这是已知限制，复杂 SQL 解析成本过高
//   - 遇到误判时，请使用 Client() 直接执行查询
//
// 相关错误：ErrQueryContainsFormat, ErrQueryContainsSettings, ErrQueryContainsLimitOffset
//
// ## 批量插入限制
//
// BatchSize 受 MaxBatchSize（默认 100000）限制，超过时返回 ErrBatchSizeTooLarge。
// 如需更大的批次，请分多次调用或使用 Client() 直接操作。
//
// ## 超时策略
//
// 设计决策: QueryPage 和 BatchInsert 不内置默认超时，超时由调用方通过 context 控制。
// 仅 Health 提供 HealthTimeout（默认 5 秒），因为健康检查预期快速完成。
// 查询和写入的耗时因场景而异，内置默认超时可能导致合理的长查询被意外中断。
//
// ## Close() 不带 context
//
// 设计决策: Close() error 不接受 context 参数，因为底层 clickhouse-go
// driver.Conn.Close() 不接受 context。与 xmongo.Close(ctx) 签名不同（xmongo 底层
// mongo.Client.Disconnect 支持 ctx）。若 clickhouse-go 未来支持 Close(ctx)，
// 将同步更新接口签名。极端场景下（如网络分区），Close() 可能阻塞直到 TCP 超时。
//
// ## 接口命名
//
// 设计决策: 接口名为 ClickHouse 与 xmongo.Mongo 保持同一模式（使用技术名称），
// 虽然 xclickhouse.ClickHouse 存在命名重复（stuttering），但与同级包一致。
// 方法名 Client() 和错误名 ErrNilClient 已与 xmongo、xcache 统一。
//
// ## 方法命名
//
// 设计决策: QueryPage/BatchInsert 遵循 SQL 领域惯用语（query、batch），
// 而非与 xmongo 的 FindPage/BulkInsert 强制统一。各存储包遵循自身领域命名
// 以降低领域切换的认知负担：ClickHouse 使用 Query/Batch 概念，MongoDB 使用 Find/Bulk 概念。
package xclickhouse
