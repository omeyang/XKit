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
	// 数据来自 clickhouse-go/v2 驱动的 Stats() 方法。
	Pool PoolStats
}

// PoolStats 包含连接池状态信息。
// 数据来自 clickhouse-go/v2 驱动的 Conn.Stats()。
type PoolStats struct {
	// Open 是打开的连接数。
	Open int

	// Idle 是空闲连接数。
	Idle int

	// InUse 是使用中连接数（Open - Idle）。
	InUse int
}
