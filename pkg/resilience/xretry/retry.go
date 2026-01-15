package xretry

import (
	"context"
	"time"
)

// RetryPolicy 定义重试策略接口
// 判断是否应该继续重试
//
// 注意：当通过 Retryer 使用时，由于底层 retry-go 库的限制：
//   - MaxAttempts() 会被正确使用
//   - ShouldRetry() 的 ctx 和 attempt 参数无法传递（retry-go 的 RetryIf 不提供）
//   - 错误判断通过 IsRetryable() 和 IsRecoverable() 函数实现
//
// 如需基于 ctx/attempt 的复杂重试逻辑，建议直接使用 retry-go 的原生 API。
type RetryPolicy interface {
	// MaxAttempts 返回最大尝试次数（包含首次尝试）
	// 返回 0 表示无限重试
	MaxAttempts() int

	// ShouldRetry 判断是否应该重试
	//
	// 注意：通过 Retryer 使用时，此方法不会被直接调用。
	// 错误判断改为使用 IsRetryable()/IsRecoverable() 函数。
	// 此方法主要用于直接使用 RetryPolicy 接口的场景。
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

// ResettableBackoff 可重置的退避策略
type ResettableBackoff interface {
	BackoffPolicy
	Reset()
}
