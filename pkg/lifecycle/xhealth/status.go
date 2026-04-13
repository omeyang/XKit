package xhealth

import (
	"encoding/json"
	"time"
)

// Status 表示健康检查状态。
type Status string

const (
	// StatusUp 表示服务完全正常。
	StatusUp Status = "up"

	// StatusDegraded 表示服务可用但有降级（存在 SkipOnErr=true 的检查失败）。
	StatusDegraded Status = "degraded"

	// StatusDown 表示服务不可用（存在 SkipOnErr=false 的检查失败）。
	StatusDown Status = "down"
)

// IsHealthy 返回状态是否表示服务可用（Up 或 Degraded）。
func (s Status) IsHealthy() bool {
	return s == StatusUp || s == StatusDegraded
}

// CheckResult 表示单个检查项的结果。
type CheckResult struct {
	// Status 是检查结果状态。
	Status Status `json:"status"`

	// Error 是检查失败时的错误信息（成功时为空）。
	Error string `json:"error,omitempty"`

	// Duration 是检查执行耗时。
	Duration time.Duration `json:"-"`
}

// MarshalJSON 实现自定义 JSON 序列化，将 Duration 格式化为人类可读字符串。
func (r CheckResult) MarshalJSON() ([]byte, error) {
	type alias CheckResult
	return json.Marshal(struct {
		alias
		Duration string `json:"duration"`
	}{
		alias:    alias(r),
		Duration: r.Duration.String(),
	})
}

// Result 表示一个端点的聚合检查结果。
type Result struct {
	// Status 是聚合状态。
	Status Status `json:"status"`

	// Checks 是各检查项的结果，key 为检查项名称。
	Checks map[string]CheckResult `json:"checks,omitempty"`
}
