package xretry

import (
	"context"
	"math"
	"sync/atomic"
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

// 确保 *Retryer 实现 Executor 接口
var _ Executor = (*Retryer)(nil)

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

// WithOnRetry 设置重试回调函数。
// 传入 nil 会被静默忽略（与 WithRetryPolicy/WithBackoffPolicy 保持一致）。
func WithOnRetry(f func(attempt int, err error)) RetryerOption {
	return func(r *Retryer) {
		if f != nil {
			r.onRetry = f
		}
	}
}

// NewRetryer 创建重试执行器
// 默认使用 FixedRetry(3) 和 ExponentialBackoff
//
// 设计决策: 返回 *Retryer 而非 Executor 接口，因为泛型函数 DoWithResult
// 需要访问内部方法。如需 mock，请在调用方使用 Executor 接口作为参数类型。
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
// 如果接收者为 nil，返回 ErrNilRetryer。
func (r *Retryer) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if r == nil {
		return ErrNilRetryer
	}
	if ctx == nil {
		return ErrNilContext
	}
	if fn == nil {
		return ErrNilFunc
	}
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
// 如果 r 为 nil，返回零值和 ErrNilRetryer。
func DoWithResult[T any](ctx context.Context, r *Retryer, fn func(ctx context.Context) (T, error)) (T, error) {
	if r == nil {
		var zero T
		return zero, ErrNilRetryer
	}
	if ctx == nil {
		var zero T
		return zero, ErrNilContext
	}
	if fn == nil {
		var zero T
		return zero, ErrNilFunc
	}
	// 构建 retry-go 的选项
	opts := r.buildOptions(ctx)

	// 执行重试
	return retry.NewWithData[T](opts...).Do(func() (T, error) {
		return fn(ctx)
	})
}

// buildOptions 构建 retry-go 的选项
// 设计决策: 每次 Do 调用重建选项切片（约 440 B/op, 13 allocs/op），对于重试场景完全可接受。
// 预构建不变选项可减少分配，但增加并发安全复杂度，收益微乎其微。
func (r *Retryer) buildOptions(ctx context.Context) []Option {
	opts := make([]Option, 0, 6)

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
	// 设计决策: Attempts(maxAttempts) 设置 retry-go 的硬上限，RetryIf 中的
	// ShouldRetry 提供更灵活的逐次判断。两者共同生效——ShouldRetry 可提前终止，
	// 但不会超过 Attempts 上限。attemptCount 表示"已失败次数"（1-based），
	// 与 RetryPolicy.ShouldRetry 的 attempt 参数语义一致。
	// 设计决策: 使用 atomic.Int64 而非普通 int，确保通过 Retrier() 逃逸的
	// *retry.Retrier 即使被并发调用也不会触发数据竞争（Go 规范中数据竞争是未定义行为）。
	// 对 Retryer.Do() 路径（每次创建独立闭包）无额外影响。
	var attemptCount atomic.Int64
	opts = append(opts, RetryIf(func(err error) bool {
		count := int(attemptCount.Add(1))
		// 先检查 retry-go 的 Unrecoverable（处理 xretry.Unrecoverable 包装的错误）
		if !IsRecoverable(err) {
			return false
		}
		// 委托给 RetryPolicy.ShouldRetry，传递完整的 ctx 和 attempt 参数
		return retryPolicy.ShouldRetry(ctx, count, err)
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
// 如果接收者为 nil，使用默认配置创建实例。
//
// 重要: 返回的实例为一次性使用（类比 strings.Builder）。
// 内部 RetryIf 闭包维护了 attemptCount 状态，对同一实例多次调用 Do()
// 会导致计数累积，产生非预期的重试行为（重试次数异常减少）。
// 每次需要重试时应重新调用 Retrier() 获取新实例。
// 并发调用同一实例的 Do() 不会触发数据竞争（attemptCount 使用原子操作），
// 但计数累积会导致各并发调用的重试预算互相干扰。
//
// 设计决策: 未改为返回工厂函数，因为 *retry.Retrier 是 retry-go 的原生类型，
// 保持类型一致性比防止误用更重要。
// 设计决策: nil ctx 归一化为 context.Background() 而非返回错误，
// 因为此方法不返回 error（保持 API 兼容性），且与 nil 接收者的
// "提供可用默认值"语义一致。
func (r *Retryer) Retrier(ctx context.Context) *retry.Retrier {
	if ctx == nil {
		ctx = context.Background()
	}
	if r == nil {
		return retry.New(Context(ctx))
	}
	return retry.New(r.buildOptions(ctx)...)
}

// RetrierWithData 返回底层的 retry.RetrierWithData
//
// 与 Retrier() 类似，但用于需要返回值的场景。
// 如果 r 为 nil，使用默认配置创建实例。
//
// 重要: 返回的实例为一次性使用，详见 Retrier 方法的文档说明。
func RetrierWithData[T any](ctx context.Context, r *Retryer) *retry.RetrierWithData[T] {
	if ctx == nil {
		ctx = context.Background()
	}
	if r == nil {
		return retry.NewWithData[T](Context(ctx))
	}
	return retry.NewWithData[T](r.buildOptions(ctx)...)
}

// RetryPolicy 返回当前重试策略。
// nil 接收者返回 nil。
func (r *Retryer) RetryPolicy() RetryPolicy {
	if r == nil {
		return nil
	}
	return r.retryPolicy
}

// BackoffPolicy 返回当前退避策略。
// nil 接收者返回 nil。
func (r *Retryer) BackoffPolicy() BackoffPolicy {
	if r == nil {
		return nil
	}
	return r.backoffPolicy
}
