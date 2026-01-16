package xretry

import (
	"context"
	"time"

	retry "github.com/avast/retry-go/v5"
)

// 以下是 avast/retry-go/v5 的类型别名，便于直接使用底层能力
// 用户可以直接使用这些类型，无需导入 retry-go 包
type (
	// Option 是 retry-go 的配置选项类型
	Option = retry.Option

	// OnRetryFunc 是重试回调函数类型
	// attempt: 当前尝试次数（从 0 开始）
	// err: 上次执行的错误
	OnRetryFunc = retry.OnRetryFunc

	// RetryIfFunc 是重试条件判断函数类型
	RetryIfFunc = retry.RetryIfFunc

	// DelayTypeFunc 是延迟类型函数
	DelayTypeFunc = retry.DelayTypeFunc

	// DelayContext 提供延迟计算所需的配置值
	DelayContext = retry.DelayContext

	// Timer 表示用于跟踪重试时间的计时器接口
	Timer = retry.Timer

	// Error 表示重试过程中的错误列表
	Error = retry.Error
)

// 以下是 retry-go 的配置选项函数
var (
	// Attempts 设置重试次数，设置为 0 表示无限重试
	// 默认值: 10
	Attempts = retry.Attempts

	// UntilSucceeded 无限重试直到成功，等同于 Attempts(0)
	UntilSucceeded = retry.UntilSucceeded

	// AttemptsForError 针对特定错误设置重试次数
	AttemptsForError = retry.AttemptsForError

	// Delay 设置重试间隔
	// 默认值: 100ms
	Delay = retry.Delay

	// MaxDelay 设置最大重试间隔
	MaxDelay = retry.MaxDelay

	// MaxJitter 设置最大抖动时间
	MaxJitter = retry.MaxJitter

	// DelayType 设置延迟类型
	// 默认值: CombineDelay(BackOffDelay, RandomDelay)
	DelayType = retry.DelayType

	// OnRetry 设置重试回调函数
	OnRetry = retry.OnRetry

	// RetryIf 设置重试条件判断函数
	RetryIf = retry.RetryIf

	// Context 设置上下文
	Context = retry.Context

	// WithTimer 设置自定义计时器（主要用于测试）
	WithTimer = retry.WithTimer

	// LastErrorOnly 只返回最后一个错误
	// 默认值: false（返回所有错误）
	LastErrorOnly = retry.LastErrorOnly

	// WrapContextErrorWithLastError 将上下文错误与最后一个错误包装在一起
	WrapContextErrorWithLastError = retry.WrapContextErrorWithLastError
)

// 以下是 retry-go 的延迟类型函数
var (
	// BackOffDelay 指数退避延迟
	BackOffDelay = retry.BackOffDelay

	// FixedDelay 固定延迟
	FixedDelay = retry.FixedDelay

	// RandomDelay 随机延迟
	RandomDelay = retry.RandomDelay

	// CombineDelay 组合多个延迟类型
	CombineDelay = retry.CombineDelay

	// FullJitterBackoffDelay 完全抖动的指数退避
	FullJitterBackoffDelay = retry.FullJitterBackoffDelay
)

// 以下是 retry-go 的错误处理函数
var (
	// Unrecoverable 将错误标记为不可恢复（不再重试）
	// 这是 retry-go 原生的不可恢复错误标记
	Unrecoverable = retry.Unrecoverable

	// IsRecoverable 检查错误是否可恢复
	IsRecoverable = retry.IsRecoverable
)

// Do 执行带重试的操作
//
// 这是对 retry-go 的薄包装，提供与 xretry 一致的 API 风格。
// 函数签名接受 context.Context 作为第一个参数，与 xretry.Retryer.Do 保持一致。
//
// 示例:
//
//	err := xretry.Do(ctx, func() error {
//	    return doSomething()
//	}, xretry.Attempts(3), xretry.Delay(100*time.Millisecond))
//
// 使用 PermanentError 跳过重试:
//
//	err := xretry.Do(ctx, func() error {
//	    if invalidInput {
//	        return xretry.NewPermanentError(errors.New("invalid input"))
//	    }
//	    return doSomething()
//	})
//
// 注意：如果调用方传入 RetryIf 选项，会覆盖内置的错误判断逻辑。
// 此时 PermanentError/TemporaryError/Unrecoverable 将不会自动生效，
// 调用方需要在自定义的 RetryIf 中处理这些情况。例如：
//
//	err := xretry.Do(ctx, fn, xretry.RetryIf(func(err error) bool {
//	    // 需要手动检查 xretry 的错误类型
//	    if !xretry.IsRecoverable(err) || !xretry.IsRetryable(err) {
//	        return false
//	    }
//	    // 自定义判断逻辑
//	    return !errors.Is(err, ErrFatal)
//	}))
func Do(ctx context.Context, fn func() error, opts ...Option) error {
	// 将 context 添加到选项中
	allOpts := make([]Option, 0, len(opts)+2)
	allOpts = append(allOpts, Context(ctx))
	allOpts = append(allOpts, RetryIf(func(err error) bool {
		// 先检查 retry-go 的 IsRecoverable（处理 Unrecoverable 包装的错误）
		if !IsRecoverable(err) {
			return false
		}
		// 再检查 xretry 的 IsRetryable（处理 PermanentError/TemporaryError）
		return IsRetryable(err)
	}))
	allOpts = append(allOpts, opts...)

	return retry.New(allOpts...).Do(fn)
}

// DoWithData 执行带重试的操作（有返回值）
//
// 这是泛型版本的 Do 函数，支持返回任意类型的值。
//
// 示例:
//
//	result, err := xretry.DoWithData(ctx, func() (string, error) {
//	    return fetchData()
//	}, xretry.Attempts(3))
//
// 注意：如果调用方传入 RetryIf 选项，会覆盖内置的错误判断逻辑。
// 此时 PermanentError/TemporaryError/Unrecoverable 将不会自动生效，
// 调用方需要在自定义的 RetryIf 中处理这些情况。详见 Do 函数的文档说明。
func DoWithData[T any](ctx context.Context, fn func() (T, error), opts ...Option) (T, error) {
	// 将 context 添加到选项中
	allOpts := make([]Option, 0, len(opts)+2)
	allOpts = append(allOpts, Context(ctx))
	allOpts = append(allOpts, RetryIf(func(err error) bool {
		// 先检查 retry-go 的 IsRecoverable（处理 Unrecoverable 包装的错误）
		if !IsRecoverable(err) {
			return false
		}
		// 再检查 xretry 的 IsRetryable（处理 PermanentError/TemporaryError）
		return IsRetryable(err)
	}))
	allOpts = append(allOpts, opts...)

	return retry.NewWithData[T](allOpts...).Do(fn)
}

// NewRetrier 创建一个底层的 retry.Retrier
//
// 这个函数直接返回 retry-go 的 Retrier，适合需要完全控制的场景。
// 与 NewRetryer 不同，这个函数不使用 RetryPolicy 和 BackoffPolicy 接口。
//
// 示例:
//
//	retrier := xretry.NewRetrier(
//	    xretry.Attempts(3),
//	    xretry.Delay(100*time.Millisecond),
//	    xretry.OnRetry(func(n uint, err error) {
//	        log.Printf("重试 #%d: %v", n, err)
//	    }),
//	)
//	err := retrier.Do(func() error {
//	    return doSomething()
//	})
func NewRetrier(opts ...Option) *retry.Retrier {
	return retry.New(opts...)
}

// NewRetrierWithData 创建一个带返回值的底层 retry.RetrierWithData
//
// 示例:
//
//	retrier := xretry.NewRetrierWithData[string](
//	    xretry.Attempts(3),
//	)
//	result, err := retrier.Do(func() (string, error) {
//	    return fetchData()
//	})
func NewRetrierWithData[T any](opts ...Option) *retry.RetrierWithData[T] {
	return retry.NewWithData[T](opts...)
}

// ToDelayType 将 BackoffPolicy 转换为 retry-go 的 DelayTypeFunc
//
// 这个函数用于将 xretry 的退避策略适配到 retry-go。
// 主要用于需要混合使用两种 API 的场景。
//
// 示例:
//
//	backoff := xretry.NewExponentialBackoff()
//	retrier := xretry.NewRetrier(
//	    xretry.Attempts(3),
//	    xretry.DelayType(xretry.ToDelayType(backoff)),
//	)
func ToDelayType(policy BackoffPolicy) DelayTypeFunc {
	return func(n uint, _ error, _ DelayContext) time.Duration {
		// #nosec G115 -- n 是重试次数，是有界的小正整数
		return policy.NextDelay(int(n))
	}
}

// ToRetryIfSimple 将仅基于错误类型的判断函数适配为 RetryIfFunc
//
// 用于简单场景，仅检查错误类型决定是否重试。
//
// 示例:
//
//	retryIf := xretry.ToRetryIfSimple(func(err error) bool {
//	    return !errors.Is(err, context.Canceled)
//	})
//	xretry.Do(ctx, fn, xretry.RetryIf(retryIf))
func ToRetryIfSimple(shouldRetry func(error) bool) RetryIfFunc {
	return shouldRetry
}
