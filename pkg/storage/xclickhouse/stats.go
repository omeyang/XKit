package xclickhouse

// Stats 包含 ClickHouse 包装器的统计信息。
type Stats struct {
	// PingCount 是健康检查次数。
	PingCount int64

	// PingErrors 是健康检查失败次数。
	PingErrors int64

	// QueryCount 是查询执行次数。
	QueryCount int64

	// QueryErrors 是查询失败次数。
	QueryErrors int64

	// SlowQueries 是慢查询次数。
	SlowQueries int64

	// Pool 是连接池状态。
	// 注意：当前 clickhouse-go/v2 驱动未直接暴露连接池详细信息，
	// 此字段暂时返回空值。如需连接池监控，请使用驱动的 Stats() 方法。
	Pool PoolStats
}

// PoolStats 包含连接池状态信息。
// 注意：当前实现暂不提供连接池详细信息，所有字段均为零值。
// clickhouse-go/v2 驱动的连接池信息需通过其他方式获取。
type PoolStats struct {
	// Open 是打开的连接数（当前未实现）。
	Open int

	// Idle 是空闲连接数（当前未实现）。
	Idle int

	// InUse 是使用中连接数（当前未实现）。
	InUse int
}
