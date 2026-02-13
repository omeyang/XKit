package xretry

import (
	"context"
	"time"
)

// RetryPolicy 定义重试策略接口
// 判断是否应该继续重试
//
// 通过 Retryer 使用时：
//   - MaxAttempts() 设置 retry-go 的 Attempts 上限
//   - ShouldRetry() 在每次失败后被调用，可实现自定义的重试判断逻辑
//   - Unrecoverable 错误会在 ShouldRetry 之前被短路拦截
type RetryPolicy interface {
	// MaxAttempts 返回最大尝试次数（包含首次尝试）
	// 返回 0 表示无限重试
	MaxAttempts() int

	// ShouldRetry 判断是否应该重试
	//
	// ctx: 上下文，可用于取消
	// attempt: 当前尝试次数（从 1 开始）
	// err: 上次执行的错误
	ShouldRetry(ctx context.Context, attempt int, err error) bool
}

// BackoffPolicy 定义退避策略接口
// 计算重试间隔时间
type BackoffPolicy interface {
	// NextDelay 返回下次重试的延迟时间
	// attempt: 当前尝试次数（从 1 开始）
	NextDelay(attempt int) time.Duration
}

// ResettableBackoff 可重置的退避策略接口。
// 实现此接口的 BackoffPolicy 可通过 Reset() 重置内部状态。
//
// 设计决策: Retryer 当前不自动调用 Reset()，因为唯一的实现
// ExponentialBackoff.Reset() 是空操作（crypto/rand 无状态）。
// 此接口保留为扩展点，供有状态的自定义 BackoffPolicy 使用。
// 如需在成功后重置状态，调用方应手动执行类型断言调用 Reset()。
type ResettableBackoff interface {
	BackoffPolicy
	Reset()
}

// Executor 重试执行器接口
//
// 设计决策: NewRetryer 返回 *Retryer 而非 Executor 接口，因为泛型函数
// DoWithResult 需要访问 *Retryer 的内部方法。调用方如需 mock 重试执行器，
// 可在自身代码中使用此接口作为函数参数类型。
type Executor interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}
