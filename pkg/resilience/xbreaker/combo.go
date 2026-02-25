package xbreaker

import (
	"context"
	"fmt"

	"github.com/omeyang/xkit/pkg/resilience/xretry"
)

// BreakerRetryer 熔断器+重试组合执行器
//
// 组合熔断器和重试器，提供更强大的容错能力：
//   - 熔断器负责快速失败，防止级联故障
//   - 重试器负责处理瞬时故障，提高成功率
//   - 每次重试尝试都会被熔断器感知和统计
//
// 执行流程：
//  1. 请求进入重试器
//  2. 每次重试尝试都经过熔断器检查
//  3. 如果熔断器打开，直接返回错误，停止重试
//  4. 如果熔断器关闭/半开，执行操作
//  5. 每次尝试的结果（成功/失败）都被熔断器记录
//  6. 连续失败可能触发熔断，后续重试将被阻断
//
// 注意：与 RetryThenBreak 的区别：
//   - BreakerRetryer: 每次重试都经过熔断器，连续失败可能在重试过程中触发熔断
//   - RetryThenBreak: 重试期间不影响熔断器，只有最终结果才记录
type BreakerRetryer struct {
	breaker *Breaker
	retryer *xretry.Retryer
}

// NewBreakerRetryer 创建熔断器+重试组合执行器
//
// 如果 breaker 或 retryer 为 nil，返回对应的错误。
//
// 示例:
//
//	breaker := xbreaker.NewBreaker("my-service",
//	    xbreaker.WithTripPolicy(xbreaker.NewConsecutiveFailures(5)),
//	)
//	retryer := xretry.NewRetryer(
//	    xretry.WithRetryPolicy(xretry.NewFixedRetry(3)),
//	    xretry.WithBackoffPolicy(xretry.NewExponentialBackoff()),
//	)
//
//	combo, err := xbreaker.NewBreakerRetryer(breaker, retryer)
func NewBreakerRetryer(breaker *Breaker, retryer *xretry.Retryer) (*BreakerRetryer, error) {
	if breaker == nil {
		return nil, ErrNilBreaker
	}
	if retryer == nil {
		return nil, ErrNilRetryer
	}

	return &BreakerRetryer{
		breaker: breaker,
		retryer: retryer,
	}, nil
}

// DoWithRetry 执行带熔断和重试的操作
//
// 执行流程：
//  1. 重试器开始执行
//  2. 每次重试尝试都经过熔断器检查
//  3. 如果熔断器打开，返回 ErrOpenState，停止重试
//  4. 如果熔断器关闭/半开，执行操作
//  5. 每次尝试的结果都被熔断器记录
//  6. 如果在重试过程中触发熔断，后续重试将被阻断
func (br *BreakerRetryer) DoWithRetry(ctx context.Context, fn func(ctx context.Context) error) error {
	if br == nil {
		return ErrNilBreakerRetryer
	}
	if ctx == nil {
		return ErrNilContext
	}
	if fn == nil {
		return ErrNilFunc
	}
	return br.retryer.Do(ctx, func(ctx context.Context) error {
		// 每次重试尝试都经过熔断器
		return br.breaker.Do(ctx, func() error {
			return fn(ctx)
		})
	})
}

// Breaker 返回熔断器
func (br *BreakerRetryer) Breaker() *Breaker {
	return br.breaker
}

// Retryer 返回重试器
func (br *BreakerRetryer) Retryer() *xretry.Retryer {
	return br.retryer
}

// ExecuteWithRetry 执行带熔断和重试的操作（泛型版本）
//
// 这是 BreakerRetryer.DoWithRetry 的泛型版本，支持返回值。
// 每次重试尝试都会经过熔断器检查和记录。
//
// 注意：fn 不接收 context，context 取消仅在重试间隔时检测。
// 若需在操作内部响应取消，请在 fn 闭包中捕获 context 使用。
// br 不能为 nil，否则返回 ErrNilBreakerRetryer。
//
// 示例:
//
//	combo, err := xbreaker.NewBreakerRetryer(breaker, retryer)
//
//	result, err := xbreaker.ExecuteWithRetry(ctx, combo, func() (string, error) {
//	    return callRemoteService()
//	})
func ExecuteWithRetry[T any](ctx context.Context, br *BreakerRetryer, fn func() (T, error)) (T, error) {
	var zero T
	if br == nil {
		return zero, ErrNilBreakerRetryer
	}
	if ctx == nil {
		return zero, ErrNilContext
	}
	if fn == nil {
		return zero, ErrNilFunc
	}
	return xretry.DoWithResult(ctx, br.retryer, func(ctx context.Context) (T, error) {
		// 每次重试尝试都经过熔断器
		return Execute(ctx, br.breaker, fn)
	})
}

// DoWithRetrySimple 执行带熔断和重试的简单操作
//
// 这是一个便利函数，使用简化的函数签名。
// 与 DoWithRetry 不同，操作函数不接收 context。
// 每次重试尝试都会经过熔断器检查和记录。
func (br *BreakerRetryer) DoWithRetrySimple(ctx context.Context, fn func() error) error {
	if br == nil {
		return ErrNilBreakerRetryer
	}
	if ctx == nil {
		return ErrNilContext
	}
	if fn == nil {
		return ErrNilFunc
	}
	return br.retryer.Do(ctx, func(ctx context.Context) error {
		// 每次重试尝试都经过熔断器
		return br.breaker.Do(ctx, fn)
	})
}

// RetryThenBreak 先重试后熔断模式（保护模式）
//
// 与 BreakerRetryer 不同，这个模式：
//   - 在重试前先检查熔断器状态，如果熔断器打开则直接失败
//   - 重试期间不影响熔断器状态（不记录中间失败）
//   - 只有最终结果才记录到熔断器
//
// 适用场景：
//   - 希望重试期间的瞬时失败不影响熔断器统计
//   - 但仍需要熔断器的保护能力（阻断请求）
type RetryThenBreak struct {
	retryer *xretry.Retryer
	breaker *Breaker
	tscb    *TwoStepCircuitBreaker[any] // 用于 Allow/Done 模式
}

// NewRetryThenBreak 创建先重试后熔断执行器（保护模式）
//
// 与 BreakerRetryer 的区别：
//   - BreakerRetryer: 每次重试都经过熔断器，连续失败可能在重试过程中触发熔断
//   - RetryThenBreak: 重试期间不影响熔断器统计，只有最终结果才记录
//
// 两者共同点：
//   - 都会在执行前检查熔断器状态
//   - 熔断器打开时都会阻断请求
//
// 重要说明：
//   - 此构造函数只复用传入 Breaker 的【配置】，不复用其【状态】
//   - 内部会创建独立的 TwoStepCircuitBreaker，状态从 Closed 开始
//   - 如果传入的 Breaker 已经处于 Open 状态，RetryThenBreak 仍会允许请求
//   - 若需要独立的熔断器实例，建议使用 NewRetryThenBreakWithConfig
//
// 设计决策: 此函数只复用 Breaker 的配置（TripPolicy、SuccessPolicy、Timeout 等），
// 不复用其状态。Breaker() getter 返回的实例仅用于访问配置，
// 其 State()/Counts() 与 RetryThenBreak 内部的熔断器状态不同步。
// 如果需要更清晰的语义，建议使用 NewRetryThenBreakWithConfig。
func NewRetryThenBreak(retryer *xretry.Retryer, breaker *Breaker) (*RetryThenBreak, error) {
	if retryer == nil {
		return nil, ErrNilRetryer
	}
	if breaker == nil {
		return nil, ErrNilBreaker
	}

	// 创建与 Breaker 配置相同的 TwoStep 熔断器
	// 注意：只复用配置，不复用状态
	tscb := NewTwoStepCircuitBreaker[any](breaker.buildSettings())

	return &RetryThenBreak{
		retryer: retryer,
		breaker: breaker,
		tscb:    tscb,
	}, nil
}

// NewRetryThenBreakWithConfig 使用配置选项创建先重试后熔断执行器
//
// 与 NewRetryThenBreak 不同，此函数直接接受配置选项，
// 不依赖现有的 Breaker 实例，避免状态混淆。
//
// 示例:
//
//	rtb := xbreaker.NewRetryThenBreakWithConfig(
//	    "my-service",
//	    retryer,
//	    xbreaker.WithTripPolicy(xbreaker.NewConsecutiveFailures(5)),
//	    xbreaker.WithTimeout(30 * time.Second),
//	)
func NewRetryThenBreakWithConfig(name string, retryer *xretry.Retryer, opts ...BreakerOption) (*RetryThenBreak, error) {
	if retryer == nil {
		return nil, ErrNilRetryer
	}

	// 使用配置创建一个临时 Breaker（仅用于获取配置）
	breaker := NewBreaker(name, opts...)

	// 创建 TwoStep 熔断器
	tscb := NewTwoStepCircuitBreaker[any](breaker.buildSettings())

	return &RetryThenBreak{
		retryer: retryer,
		breaker: breaker,
		tscb:    tscb,
	}, nil
}

// Do 执行操作
//
// 执行流程：
//  1. 检查熔断器状态，如果打开则直接返回 ErrOpenState
//  2. 使用重试器执行操作（重试期间不记录到熔断器）
//  3. 将最终结果（成功或失败）记录到熔断器（使用 SuccessPolicy 判断）
//
// 注意：即使发生 panic，也会通过 defer 确保熔断器计数被正确更新（记为失败）。
func (rtb *RetryThenBreak) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if rtb == nil {
		return ErrNilRetryThenBreak
	}
	if ctx == nil {
		return ErrNilContext
	}
	if fn == nil {
		return ErrNilFunc
	}
	// 检查 context 是否已取消
	if err := ctx.Err(); err != nil {
		return err
	}

	// 先检查熔断器状态，使用 TwoStep 模式获取执行许可
	done, cbErr := rtb.tscb.Allow()
	if cbErr != nil {
		// 熔断器打开或请求过多，包装错误后返回
		// 包装后的错误实现 Retryable() 返回 false，避免不必要的重试
		return wrapBreakerError(cbErr, rtb.breaker.name, rtb.State())
	}

	// 使用 defer 确保 done 一定会被调用，即使发生 panic
	// 这避免了半开状态下请求计数不正确导致的 ErrTooManyRequests
	var err error
	defer func() {
		if r := recover(); r != nil {
			// panic 时记为失败，然后重新抛出
			done(fmt.Errorf("panic: %v", r))
			panic(r)
		}
		// gobreaker v2: done(nil) 表示成功，done(err) 表示失败
		done(rtb.toResultError(err))
	}()

	// 使用重试器执行（重试期间不记录到熔断器）
	err = rtb.retryer.Do(ctx, fn)

	return err
}

// Breaker 返回用于配置的 Breaker 实例
//
// 重要说明：
//   - 返回的 Breaker 仅用于访问配置（TripPolicy、SuccessPolicy 等）
//   - RetryThenBreak 内部使用独立的 TwoStepCircuitBreaker
//   - 返回的 Breaker 状态与内部熔断器状态不同步
//   - 获取实际熔断器状态请使用 State() 和 Counts() 方法
func (rtb *RetryThenBreak) Breaker() *Breaker {
	return rtb.breaker
}

// Retryer 返回重试器
func (rtb *RetryThenBreak) Retryer() *xretry.Retryer {
	return rtb.retryer
}

// State 返回熔断器当前状态
func (rtb *RetryThenBreak) State() State {
	return rtb.tscb.State()
}

// Counts 返回当前统计计数
func (rtb *RetryThenBreak) Counts() Counts {
	return rtb.tscb.Counts()
}

// toResultError 将策略判断结果转换为 gobreaker v2 期望的 error 值
//
// gobreaker v2 的 done 回调签名为 func(err error)：
//   - done(nil) 表示成功
//   - done(err) 表示失败（或被排除）
//
// 设计决策: 先检查 ExcludePolicy，再检查 SuccessPolicy，
// 与 gobreaker 内部 afterRequest 的优先级一致（先 isExcluded 再 isSuccessful）。
// 若顺序相反，同时匹配两个策略的错误会被 SuccessPolicy 转为 nil，
// 绕过 gobreaker 的 isExcluded 检查，错误地计入成功计数。
func (rtb *RetryThenBreak) toResultError(err error) error {
	// 被排除的错误原样传递，让 gobreaker 自行处理排除逻辑（不计入任何计数）
	if rtb.breaker.IsExcluded(err) {
		return err
	}
	if rtb.breaker.IsSuccessful(err) {
		return nil
	}
	// 操作被 SuccessPolicy 判定为失败
	if err != nil {
		return err
	}
	// 极端情况：err 为 nil 但 SuccessPolicy 返回 false
	// 这通常不应发生，但为安全起见返回一个占位错误
	return errFailedByPolicy
}

// ExecuteRetryThenBreak 执行先重试后熔断的操作（泛型版本）
//
// 注意：即使发生 panic，也会通过 defer 确保熔断器计数被正确更新（记为失败）。
// rtb 不能为 nil，否则返回 ErrNilRetryThenBreak。
func ExecuteRetryThenBreak[T any](ctx context.Context, rtb *RetryThenBreak, fn func() (T, error)) (T, error) {
	var zero T

	if rtb == nil {
		return zero, ErrNilRetryThenBreak
	}
	if ctx == nil {
		return zero, ErrNilContext
	}
	if fn == nil {
		return zero, ErrNilFunc
	}

	// 检查 context 是否已取消
	if err := ctx.Err(); err != nil {
		return zero, err
	}

	// 先检查熔断器状态，使用 TwoStep 模式获取执行许可
	done, cbErr := rtb.tscb.Allow()
	if cbErr != nil {
		// 熔断器打开或请求过多，包装错误后返回
		// 包装后的错误实现 Retryable() 返回 false，避免不必要的重试
		return zero, wrapBreakerError(cbErr, rtb.breaker.name, rtb.State())
	}

	// 使用 defer 确保 done 一定会被调用，即使发生 panic
	// 这避免了半开状态下请求计数不正确导致的 ErrTooManyRequests
	var result T
	var err error
	defer func() {
		if r := recover(); r != nil {
			// panic 时记为失败，然后重新抛出
			done(fmt.Errorf("panic: %v", r))
			panic(r)
		}
		// gobreaker v2: done(nil) 表示成功，done(err) 表示失败
		done(rtb.toResultError(err))
	}()

	// 使用重试器执行（重试期间不记录到熔断器）
	result, err = xretry.DoWithResult(ctx, rtb.retryer, func(_ context.Context) (T, error) {
		return fn()
	})

	return result, err
}
