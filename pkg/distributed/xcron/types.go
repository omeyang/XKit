package xcron

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/omeyang/xkit/pkg/resilience/xretry"
)

// JobID 任务唯一标识，直接复用 cron.EntryID。
// 用于后续移除任务或查询任务状态。
type JobID = cron.EntryID

// Job 定时任务接口。
// 实现此接口以定义任务执行逻辑。
type Job interface {
	// Run 执行任务。
	// ctx 包含超时控制和追踪信息，任务应响应 ctx.Done()。
	// 返回 error 表示任务执行失败，会被记录到日志。
	Run(ctx context.Context) error
}

// JobFunc 函数适配器，将普通函数转换为 [Job] 接口。
//
// 用法：
//
//	var job Job = JobFunc(func(ctx context.Context) error {
//	    return doSomething(ctx)
//	})
type JobFunc func(ctx context.Context) error

// Run 实现 [Job] 接口。
func (f JobFunc) Run(ctx context.Context) error {
	return f(ctx)
}

// Logger 日志接口，兼容 xlog.Logger。
// 如果不设置，使用标准库 log 输出。
type Logger interface {
	// Debug 记录调试日志
	Debug(ctx context.Context, msg string, args ...any)
	// Info 记录信息日志
	Info(ctx context.Context, msg string, args ...any)
	// Warn 记录警告日志
	Warn(ctx context.Context, msg string, args ...any)
	// Error 记录错误日志
	Error(ctx context.Context, msg string, args ...any)
}

// RetryPolicy 重试策略接口，兼容 xretry.RetryPolicy。
// 用于配置任务失败后的重试行为。
type RetryPolicy interface {
	// ShouldRetry 判断是否应该重试
	// attempt: 当前重试次数（从 1 开始）
	// err: 上次执行的错误
	ShouldRetry(attempt int, err error) bool
}

// BackoffPolicy 退避策略接口，直接复用 xretry.BackoffPolicy。
// 用于配置重试之间的等待时间。
//
// 推荐使用 xretry 包中的实现：
//   - xretry.NewConstantBackoff(delay)
//   - xretry.NewExponentialBackoff(opts...)
//   - xretry.NewLinearBackoff(opts...)
type BackoffPolicy = xretry.BackoffPolicy

// Observer 可观测性接口，兼容 xmetrics.Observer。
// 用于任务执行的链路追踪。
type Observer interface {
	// Start 开始一个新的 span
	Start(ctx context.Context, spanName string, opts ...any) (context.Context, Span)
}

// Span 追踪 span 接口，兼容 xmetrics.Span。
type Span interface {
	// End 结束 span，记录执行结果
	End(opts ...any)
	// SetAttributes 设置属性
	SetAttributes(attrs ...any)
	// RecordError 记录错误
	RecordError(err error, opts ...any)
}

// Hook 任务执行钩子接口。
//
// 用于在任务执行前后注入自定义逻辑，如日志、指标、告警、恢复处理等。
// 可以通过 [WithHook] 或 [WithHooks] 配置多个钩子，按添加顺序执行。
//
// 执行时机：
//   - BeforeJob: 在获取锁之后、执行任务之前调用
//   - AfterJob: 在任务执行完成后调用（无论成功或失败）
//
// # 典型用途
//
//   - 日志记录: 记录任务开始/结束，便于审计
//   - 指标上报: 上报执行次数、耗时等到监控系统
//   - 告警通知: 任务失败时发送告警
//   - 恢复处理: 捕获 panic 并执行清理逻辑
//
// # 示例
//
//	type metricsHook struct {
//	    counter *prometheus.Counter
//	}
//
//	func (h *metricsHook) BeforeJob(ctx context.Context, name string) context.Context {
//	    return ctx // 可以在 context 中注入跟踪信息
//	}
//
//	func (h *metricsHook) AfterJob(ctx context.Context, name string, duration time.Duration, err error) {
//	    h.counter.WithLabelValues(name, errorLabel(err)).Inc()
//	}
type Hook interface {
	// BeforeJob 在任务执行前调用。
	//
	// ctx: 执行上下文（已包含超时控制）
	// name: 任务名
	//
	// 返回的 context 将传递给任务执行和后续钩子。
	// 可用于注入请求 ID、跟踪信息等。
	BeforeJob(ctx context.Context, name string) context.Context

	// AfterJob 在任务执行后调用。
	//
	// ctx: 执行上下文
	// name: 任务名
	// duration: 执行耗时（包含重试时间）
	// err: 执行错误，nil 表示成功
	//
	// 即使任务 panic，此方法也会被调用（err 会是 panic 包装的错误）。
	AfterJob(ctx context.Context, name string, duration time.Duration, err error)
}

// HookFunc 函数适配器，将函数对转换为 [Hook] 接口。
//
// 用于快速创建简单的钩子，无需定义完整的结构体。
//
// 用法：
//
//	hook := xcron.HookFunc{
//	    Before: func(ctx context.Context, name string) context.Context {
//	        log.Printf("job %s starting", name)
//	        return ctx
//	    },
//	    After: func(ctx context.Context, name string, d time.Duration, err error) {
//	        log.Printf("job %s finished in %v, error: %v", name, d, err)
//	    },
//	}
type HookFunc struct {
	// Before 任务执行前调用，可为 nil
	Before func(ctx context.Context, name string) context.Context
	// After 任务执行后调用，可为 nil
	After func(ctx context.Context, name string, duration time.Duration, err error)
}

// BeforeJob 实现 [Hook] 接口。
func (h HookFunc) BeforeJob(ctx context.Context, name string) context.Context {
	if h.Before != nil {
		return h.Before(ctx, name)
	}
	return ctx
}

// AfterJob 实现 [Hook] 接口。
func (h HookFunc) AfterJob(ctx context.Context, name string, duration time.Duration, err error) {
	if h.After != nil {
		h.After(ctx, name, duration, err)
	}
}
