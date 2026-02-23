package storageopt

import (
	"sync/atomic"
	"time"
)

// =============================================================================
// 通用统计计数器
// =============================================================================

// HealthCounter 健康检查计数器。
// 提供原子计数器用于追踪健康检查状态。
type HealthCounter struct {
	pingCount  atomic.Int64
	pingErrors atomic.Int64
}

// IncPing 增加 ping 计数。
func (h *HealthCounter) IncPing() {
	h.pingCount.Add(1)
}

// IncPingError 增加 ping 错误计数。
func (h *HealthCounter) IncPingError() {
	h.pingErrors.Add(1)
}

// PingCount 返回 ping 计数。
func (h *HealthCounter) PingCount() int64 {
	return h.pingCount.Load()
}

// PingErrors 返回 ping 错误计数。
func (h *HealthCounter) PingErrors() int64 {
	return h.pingErrors.Load()
}

// SlowQueryCounter 慢查询计数器。
// 提供原子计数器用于追踪慢查询数量。
type SlowQueryCounter struct {
	count atomic.Int64
}

// Inc 增加慢查询计数。
func (s *SlowQueryCounter) Inc() {
	s.count.Add(1)
}

// Count 返回慢查询计数。
func (s *SlowQueryCounter) Count() int64 {
	return s.count.Load()
}

// QueryCounter 查询计数器。
// 提供原子计数器用于追踪查询状态。
//
// 设计决策: 当前仅被 xclickhouse 使用，保留在 storageopt 而非移入 xclickhouse，
// 与 HealthCounter/SlowQueryCounter 保持类型族一致性。xmongo 未使用是因为
// MongoDB driver 的操作粒度不同（FindOne/InsertMany 等），统一 Query 维度无意义。
type QueryCounter struct {
	queryCount  atomic.Int64
	queryErrors atomic.Int64
}

// IncQuery 增加查询计数。
func (q *QueryCounter) IncQuery() {
	q.queryCount.Add(1)
}

// IncQueryError 增加查询错误计数。
func (q *QueryCounter) IncQueryError() {
	q.queryErrors.Add(1)
}

// QueryCount 返回查询计数。
func (q *QueryCounter) QueryCount() int64 {
	return q.queryCount.Load()
}

// QueryErrors 返回查询错误计数。
func (q *QueryCounter) QueryErrors() int64 {
	return q.queryErrors.Load()
}

// =============================================================================
// 通用辅助函数
// =============================================================================

// MeasureOperation 测量操作耗时。
//
// 设计决策: 虽然当前实现等同于 time.Since(start)，保留此函数作为 storage 子包的
// 统一度量入口点，便于未来扩展（如自动记录 metrics）且不破坏调用方。
//
// 使用方式：
//
//	start := time.Now()
//	// ... 操作 ...
//	duration := storageopt.MeasureOperation(start)
func MeasureOperation(start time.Time) time.Duration {
	return time.Since(start)
}
