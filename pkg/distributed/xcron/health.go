package xcron

import (
	"context"
	"fmt"
	"time"
)

// HealthStatus 表示调度器的健康状态。
type HealthStatus string

const (
	// HealthStatusHealthy 表示调度器完全健康，所有组件正常运行。
	HealthStatusHealthy HealthStatus = "healthy"
	// HealthStatusDegraded 表示调度器部分功能受损，但核心功能正常。
	// 例如：失败率过高但调度器仍在运行。
	HealthStatusDegraded HealthStatus = "degraded"
	// HealthStatusUnhealthy 表示调度器不健康，可能无法正常调度任务。
	// 例如：调度器未启动、分布式锁连接失败等。
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// HealthCheck 健康检查结果。
//
// 包含调度器的运行状态、统计摘要和潜在问题。
type HealthCheck struct {
	// Status 总体健康状态
	Status HealthStatus `json:"status"`
	// HasJobs 是否有已注册的任务。
	// 注意：此字段不表示调度器是否已启动，仅表示是否有任务注册。
	HasJobs bool `json:"has_jobs"`
	// RegisteredJobs 已注册的任务数量
	RegisteredJobs int `json:"registered_jobs"`
	// TotalExecutions 总执行次数
	TotalExecutions int64 `json:"total_executions"`
	// SuccessCount 成功次数
	SuccessCount int64 `json:"success_count"`
	// FailureCount 失败次数
	FailureCount int64 `json:"failure_count"`
	// SuccessRate 成功率（0.0-1.0）
	SuccessRate float64 `json:"success_rate"`
	// LastExecTime 最后执行时间
	LastExecTime time.Time `json:"last_exec_time,omitempty"`
	// LastError 最后一次错误信息
	LastError string `json:"last_error,omitempty"`
	// Message 附加说明
	Message string `json:"message,omitempty"`
	// CheckTime 检查时间
	CheckTime time.Time `json:"check_time"`
	// Details 详细信息（用于调试）
	Details map[string]any `json:"details,omitempty"`
}

// HealthChecker 健康检查接口。
//
// 用于检查调度器和相关依赖的健康状态。
// 可以集成到 Kubernetes liveness/readiness 探针或监控系统。
type HealthChecker interface {
	// Check 执行健康检查。
	//
	// ctx 用于超时控制，检查应在合理时间内完成。
	// 返回 HealthCheck 包含详细的健康状态信息。
	Check(ctx context.Context) *HealthCheck
}

// HealthCheckOption 健康检查配置选项。
type HealthCheckOption func(*healthCheckOptions)

type healthCheckOptions struct {
	// 失败率阈值，超过此值状态变为 degraded
	degradedThreshold float64
	// 检查分布式锁健康状态
	checkLocker bool
	// 最小执行次数，低于此值不计算失败率
	minExecutions int64
}

func defaultHealthCheckOptions() *healthCheckOptions {
	return &healthCheckOptions{
		degradedThreshold: 0.5, // 50% 失败率触发 degraded
		checkLocker:       false,
		minExecutions:     10, // 至少 10 次执行后才计算失败率
	}
}

// WithDegradedThreshold 设置降级阈值。
//
// 当失败率超过此阈值时，健康状态变为 degraded。
// 值范围 0.0-1.0，默认 0.5（50%）。
func WithDegradedThreshold(threshold float64) HealthCheckOption {
	return func(o *healthCheckOptions) {
		if threshold >= 0 && threshold <= 1 {
			o.degradedThreshold = threshold
		}
	}
}

// WithCheckLocker 启用分布式锁健康检查。
//
// 检查 Locker 的 Health 方法（如果实现了 LockerHealthChecker 接口）。
func WithCheckLocker() HealthCheckOption {
	return func(o *healthCheckOptions) {
		o.checkLocker = true
	}
}

// WithMinExecutions 设置最小执行次数。
//
// 低于此次数不会基于失败率判断 degraded 状态。
// 默认 10 次。
func WithMinExecutions(n int64) HealthCheckOption {
	return func(o *healthCheckOptions) {
		if n >= 0 {
			o.minExecutions = n
		}
	}
}

// LockerHealthChecker 分布式锁健康检查接口。
//
// 如果 Locker 实现了此接口，健康检查时会调用 Health 方法。
type LockerHealthChecker interface {
	// Health 检查锁服务健康状态。
	// 返回 nil 表示健康，返回 error 表示不健康。
	Health(ctx context.Context) error
}

// schedulerHealthChecker 调度器健康检查实现
type schedulerHealthChecker struct {
	scheduler *cronScheduler
	opts      *healthCheckOptions
}

// NewHealthChecker 创建健康检查器。
//
// 用法：
//
//	scheduler := xcron.New()
//	checker := xcron.NewHealthChecker(scheduler)
//	result := checker.Check(context.Background())
//	if result.Status != xcron.HealthStatusHealthy {
//	    log.Printf("scheduler unhealthy: %s", result.Message)
//	}
func NewHealthChecker(scheduler Scheduler, opts ...HealthCheckOption) HealthChecker {
	cronSched, ok := scheduler.(*cronScheduler)
	if !ok {
		// 返回一个始终返回 unhealthy 的检查器
		return &fallbackHealthChecker{message: "unsupported scheduler type"}
	}

	hcOpts := defaultHealthCheckOptions()
	for _, opt := range opts {
		opt(hcOpts)
	}

	return &schedulerHealthChecker{
		scheduler: cronSched,
		opts:      hcOpts,
	}
}

// Check 执行健康检查
func (c *schedulerHealthChecker) Check(ctx context.Context) *HealthCheck {
	stats := c.scheduler.stats
	numEntries := len(c.scheduler.cron.Entries())

	result := c.buildBaseResult(stats, numEntries)
	c.checkFailureRate(stats, result)
	c.addDetailStats(stats, result)
	c.checkLockerHealth(ctx, result)
	c.checkNoJobs(numEntries, result)

	return result
}

// buildBaseResult 构建基础健康检查结果
func (c *schedulerHealthChecker) buildBaseResult(stats *Stats, numEntries int) *HealthCheck {
	result := &HealthCheck{
		Status:          HealthStatusHealthy,
		HasJobs:         numEntries > 0,
		RegisteredJobs:  numEntries,
		TotalExecutions: stats.TotalExecutions(),
		SuccessCount:    stats.SuccessCount(),
		FailureCount:    stats.FailureCount(),
		SuccessRate:     stats.SuccessRate(),
		LastExecTime:    stats.LastExecTime(),
		CheckTime:       time.Now(),
		Details:         make(map[string]any),
	}
	if lastErr := stats.LastError(); lastErr != nil {
		result.LastError = lastErr.Error()
	}
	return result
}

// checkFailureRate 检查失败率
func (c *schedulerHealthChecker) checkFailureRate(stats *Stats, result *HealthCheck) {
	if stats.TotalExecutions() < c.opts.minExecutions {
		return
	}
	failureRate := 1 - stats.SuccessRate()
	if failureRate > c.opts.degradedThreshold {
		result.Status = HealthStatusDegraded
		result.Message = fmt.Sprintf("high failure rate: %.1f%% (threshold: %.1f%%)",
			failureRate*100, c.opts.degradedThreshold*100)
	}
}

// addDetailStats 添加详细统计信息
func (c *schedulerHealthChecker) addDetailStats(stats *Stats, result *HealthCheck) {
	result.Details["skip_count"] = stats.SkipCount()
	result.Details["min_duration"] = stats.MinDuration().String()
	result.Details["max_duration"] = stats.MaxDuration().String()
	result.Details["avg_duration"] = stats.AvgDuration().String()
}

// checkLockerHealth 检查分布式锁健康状态
func (c *schedulerHealthChecker) checkLockerHealth(ctx context.Context, result *HealthCheck) {
	if !c.opts.checkLocker || c.scheduler.locker == nil {
		return
	}
	checker, ok := c.scheduler.locker.(LockerHealthChecker)
	if !ok {
		return
	}
	if err := checker.Health(ctx); err != nil {
		result.Status = HealthStatusUnhealthy
		appendMessage(result, fmt.Sprintf("locker unhealthy: %v", err))
		result.Details["locker_error"] = err.Error()
	}
}

// checkNoJobs 检查是否无任务注册
func (c *schedulerHealthChecker) checkNoJobs(numEntries int, result *HealthCheck) {
	if numEntries == 0 {
		appendMessage(result, "no jobs registered")
	}
}

// appendMessage 追加消息
func appendMessage(result *HealthCheck, msg string) {
	if result.Message != "" {
		result.Message += "; "
	}
	result.Message += msg
}

// fallbackHealthChecker 回退健康检查器
type fallbackHealthChecker struct {
	message string
}

func (c *fallbackHealthChecker) Check(_ context.Context) *HealthCheck {
	return &HealthCheck{
		Status:    HealthStatusUnhealthy,
		HasJobs:   false,
		Message:   c.message,
		CheckTime: time.Now(),
	}
}

// 确保类型实现了接口
var _ HealthChecker = (*schedulerHealthChecker)(nil)
var _ HealthChecker = (*fallbackHealthChecker)(nil)
