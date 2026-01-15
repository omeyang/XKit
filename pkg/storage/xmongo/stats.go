package xmongo

// =============================================================================
// 统计信息
// =============================================================================

// Stats 包含 MongoDB 包装器的统计信息。
type Stats struct {
	// PingCount 健康检查次数。
	PingCount int64

	// PingErrors 健康检查失败次数。
	PingErrors int64

	// SlowQueries 慢查询次数。
	SlowQueries int64

	// Pool 连接池状态。
	Pool PoolStats
}

// PoolStats 连接池状态信息。
// 注意：MongoDB driver v2 未直接暴露完整的连接池详细信息。
// 当前仅通过 NumberSessionsInProgress() 提供 InUseConnections（活跃会话数）。
// TotalConnections 和 AvailableConnections 暂未实现，始终为零值。
type PoolStats struct {
	// TotalConnections 总连接数（当前未实现，始终为 0）。
	TotalConnections int

	// AvailableConnections 可用连接数（当前未实现，始终为 0）。
	AvailableConnections int

	// InUseConnections 使用中连接数。
	// 通过 mongo.Client.NumberSessionsInProgress() 获取，表示活跃会话数。
	InUseConnections int
}
