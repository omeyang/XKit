package xretry

import (
	"context"
	"math"
	"time"

	retry "github.com/avast/retry-go/v5"
)

// safeIntToUint 将 int 安全转换为 uint。
// 负数返回 0，正数直接转换。
// 用于将 MaxAttempts (int) 传递给 retry-go 的 Attempts (uint)。
func safeIntToUint(n int) uint {
	if n <= 0 {
		return 0
	}
	return uint(n)
}

// safeUintToInt 将 uint 安全转换为 int。
// 超过 MaxInt 的值会被截断到 MaxInt。
// 用于将 retry-go 的重试次数 (uint) 传递给用户回调 (int)。
func safeUintToInt(n uint) int {
	if n > uint(math.MaxInt) {
		return math.MaxInt
	}
	return int(n)
}

// Retryer 重试执行器
//
// Retryer 组合了 RetryPolicy（重试策略）和 BackoffPolicy（退避策略），
// 提供统一的重试执行能力。
//
// 底层使用 avast/retry-go/v5 实现。
// 如需使用 retry-go 的完整功能，可以通过 Retrier() 方法获取底层实例。
type Retryer struct {
	retryPolicy   RetryPolicy
	backoffPolicy BackoffPolicy
	onRetry       func(attempt int, err error)
}

// RetryerOption 执行器配置选项
type RetryerOption func(*Retryer)

// WithRetryPolicy 设置重试策略
func WithRetryPolicy(p RetryPolicy) RetryerOption {
	return func(r *Retryer) {
		if p != nil {
			r.retryPolicy = p
		}
	}
}

// WithBackoffPolicy 设置退避策略
func WithBackoffPolicy(p BackoffPolicy) RetryerOption {
	return func(r *Retryer) {
		if p != nil {
			r.backoffPolicy = p
		}
	}
}

// WithOnRetry 设置重试回调函数
func WithOnRetry(f func(attempt int, err error)) RetryerOption {
	return func(r *Retryer) {
		r.onRetry = f
	}
}

// NewRetryer 创建重试执行器
// 默认使用 FixedRetry(3) 和 ExponentialBackoff
func NewRetryer(opts ...RetryerOption) *Retryer {
	r := &Retryer{
		retryPolicy:   NewFixedRetry(3),
		backoffPolicy: NewExponentialBackoff(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Do 执行带重试的操作
//
// 底层使用 avast/retry-go/v5 实现重试逻辑，
// 同时兼容 xretry 的 RetryPolicy 和 BackoffPolicy 接口。
func (r *Retryer) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	// 构建 retry-go 的选项
	opts := r.buildOptions(ctx)

	// 执行重试
	return retry.New(opts...).Do(func() error {
		return fn(ctx)
	})
}

// DoWithResult 执行带重试的操作（有返回值）
//
// 这是泛型函数，必须作为包级函数使用。
func DoWithResult[T any](ctx context.Context, r *Retryer, fn func(ctx context.Context) (T, error)) (T, error) {
	// 构建 retry-go 的选项
	opts := r.buildOptions(ctx)

	// 执行重试
	return retry.NewWithData[T](opts...).Do(func() (T, error) {
		return fn(ctx)
	})
}

// buildOptions 构建 retry-go 的选项
func (r *Retryer) buildOptions(ctx context.Context) []Option {
	opts := make([]Option, 0, 5)

	// 设置上下文
	opts = append(opts, Context(ctx))

	// 防止零值 Retryer 使用时 panic
	retryPolicy := r.retryPolicy
	if retryPolicy == nil {
		retryPolicy = NewFixedRetry(3)
	}
	backoffPolicy := r.backoffPolicy
	if backoffPolicy == nil {
		backoffPolicy = NewExponentialBackoff()
	}

	// 设置重试次数
	// maxAttempts <= 0 视为无限重试
	maxAttempts := retryPolicy.MaxAttempts()
	if maxAttempts <= 0 {
		opts = append(opts, UntilSucceeded())
	} else {
		opts = append(opts, Attempts(safeIntToUint(maxAttempts)))
	}

	// 设置重试条件
	opts = append(opts, RetryIf(func(err error) bool {
		// 先检查 retry-go 的 Unrecoverable（处理 xretry.Unrecoverable 包装的错误）
		if !IsRecoverable(err) {
			return false
		}
		// 再检查 xretry 的 IsRetryable（处理 PermanentError/TemporaryError）
		// 注意：RetryPolicy.ShouldRetry 的 ctx/attempt 参数无法在此使用，
		// 因为 retry-go 的 RetryIf 不提供这些参数。
		// 实际的 attempt 限制已经通过 Attempts 选项设置。
		return IsRetryable(err)
	}))

	// 设置延迟类型（使用 BackoffPolicy）
	opts = append(opts, DelayType(func(n uint, _ error, _ DelayContext) time.Duration {
		// 注意：retry-go v5 中 DelayType 的 n 从 1 开始，与 BackoffPolicy.NextDelay 一致
		return backoffPolicy.NextDelay(safeUintToInt(n))
	}))

	// 设置重试回调
	if r.onRetry != nil {
		opts = append(opts, OnRetry(func(n uint, err error) {
			// 注意：retry-go v5 中 OnRetry 的 n 从 0 开始，需要 +1 转换为 1-based
			r.onRetry(safeUintToInt(n)+1, err)
		}))
	}

	// 只返回最后一个错误，简化调用方的错误处理
	opts = append(opts, LastErrorOnly(true))

	return opts
}

// Retrier 返回底层的 retry.Retrier
//
// 通过此方法可以获取 retry-go 的原生 Retrier 实例，
// 使用 retry-go 的完整功能。
//
// 注意：每次调用都会创建新的 Retrier 实例。
func (r *Retryer) Retrier(ctx context.Context) *retry.Retrier {
	return retry.New(r.buildOptions(ctx)...)
}

// RetrierWithData 返回底层的 retry.RetrierWithData
//
// 与 Retrier() 类似，但用于需要返回值的场景。
func RetrierWithData[T any](ctx context.Context, r *Retryer) *retry.RetrierWithData[T] {
	return retry.NewWithData[T](r.buildOptions(ctx)...)
}

// RetryPolicy 返回当前重试策略
func (r *Retryer) RetryPolicy() RetryPolicy {
	return r.retryPolicy
}

// BackoffPolicy 返回当前退避策略
func (r *Retryer) BackoffPolicy() BackoffPolicy {
	return r.backoffPolicy
}
