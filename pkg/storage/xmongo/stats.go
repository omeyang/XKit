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
//
// 限制说明：
// MongoDB driver v2 不直接暴露连接池详细信息（TotalConnections、AvailableConnections）。
// 这是 driver 的设计决策，因为 MongoDB 使用连接池复用和会话管理，
// 传统的"连接数"概念不能准确反映资源使用情况。
//
// 当前返回值：
//   - InUseConnections: 活跃会话数（通过 NumberSessionsInProgress 获取）
//   - TotalConnections: 始终为 0（driver 未暴露）
//   - AvailableConnections: 始终为 0（driver 未暴露）
//
// 获取详细连接池信息的替代方案：
//  1. MongoDB serverStatus 命令: db.runCommand({serverStatus: 1}).connections
//  2. 监控 MongoDB 服务端指标（推荐用于生产环境）
//  3. 使用 driver 的事件监控功能（PoolEvent）统计连接创建/关闭
type PoolStats struct {
	// TotalConnections 总连接数。
	// 当前未实现，始终为 0。
	// 如需此信息，请使用 MongoDB serverStatus 命令或事件监控。
	TotalConnections int

	// AvailableConnections 可用连接数。
	// 当前未实现，始终为 0。
	// 如需此信息，请使用 MongoDB serverStatus 命令或事件监控。
	AvailableConnections int

	// InUseConnections 使用中连接数。
	// 通过 mongo.Client.NumberSessionsInProgress() 获取，表示活跃会话数。
	// 这是当前 driver 提供的最接近"使用中连接数"的指标。
	InUseConnections int
}
