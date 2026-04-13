package xbreaker

import (
	"errors"
	"fmt"

	"github.com/sony/gobreaker/v2"
)

// errFailedByPolicy 当 SuccessPolicy 判定 nil error 为失败时使用的占位错误。
// 这是一个极端情况：操作未返回错误，但 SuccessPolicy 仍判定为失败。
var errFailedByPolicy = errors.New("xbreaker: operation marked as failed by success policy")

// 参数校验错误
var (
	// ErrNilBreaker 传入的 Breaker 为 nil
	ErrNilBreaker = errors.New("xbreaker: breaker cannot be nil")

	// ErrNilRetryer 传入的 Retryer 为 nil
	ErrNilRetryer = errors.New("xbreaker: retryer cannot be nil")

	// ErrNilBreakerRetryer 传入的 BreakerRetryer 为 nil
	ErrNilBreakerRetryer = errors.New("xbreaker: breaker-retryer cannot be nil")

	// ErrNilRetryThenBreak 传入的 RetryThenBreak 为 nil
	ErrNilRetryThenBreak = errors.New("xbreaker: retry-then-break cannot be nil")

	// ErrNilContext 传入的 context 为 nil
	ErrNilContext = errors.New("xbreaker: context cannot be nil")

	// ErrNilFunc 传入的操作函数为 nil
	ErrNilFunc = errors.New("xbreaker: function cannot be nil")

	// ErrNilManagedBreaker 传入的 ManagedBreaker 为 nil
	ErrNilManagedBreaker = errors.New("xbreaker: managed breaker cannot be nil")
)

// BreakerError 熔断器错误包装类型
//
// 包装 gobreaker 的错误（ErrOpenState、ErrTooManyRequests），
// 并实现 Retryable() 方法返回 false，让 xretry 不再重试这些错误。
//
// 这解决了熔断器打开时，重试器仍然会继续退避重试的问题。
// 熔断器错误应该立即返回（快速失败），而不是继续重试。
// 设计决策: Err/Name/State 保留为导出字段，便于调用方在日志和监控中直接读取。
// 与 xretry 的未导出字段风格不同，是因为 BreakerError 通常用于外部诊断（日志/告警），
// 而 xretry 的错误主要用于内部错误链传递。
type BreakerError struct {
	Err   error  // 原始错误（ErrOpenState 或 ErrTooManyRequests）
	Name  string // 熔断器名称（可选，用于日志）
	State State  // 熔断器状态
}

// Error 实现 error 接口
func (e *BreakerError) Error() string {
	if e.Name != "" {
		return fmt.Sprintf("breaker %s: %v", e.Name, e.Err)
	}
	return e.Err.Error()
}

// Unwrap 实现 errors.Unwrap 接口
func (e *BreakerError) Unwrap() error {
	return e.Err
}

// Retryable 实现 xretry.RetryableError 接口
//
// 熔断器错误不应该重试，因为：
//   - ErrOpenState: 熔断器打开，说明下游服务不可用，重试无意义
//   - ErrTooManyRequests: 半开状态下请求过多，应该等待熔断器状态变化
//
// 返回 false 表示这是不可重试的错误。
func (e *BreakerError) Retryable() bool {
	return false
}

// newBreakerError 创建熔断器错误
func newBreakerError(err error, name string, state State) *BreakerError {
	return &BreakerError{
		Err:   err,
		Name:  name,
		State: state,
	}
}

// wrapBreakerError 如果是熔断器错误则包装，否则原样返回
//
// 注意：此函数只包装直接的 sentinel error（ErrOpenState、ErrTooManyRequests），
// 不使用 errors.Is 遍历整个错误链。这样可以避免在嵌套熔断器场景下，
// 将内层熔断器的错误错误地归因到外层熔断器。
//
// 如果错误已经是 BreakerError，则保留原始错误不再包装，
// 以保持正确的错误来源信息。
//
// 设计决策: 从错误类型推导状态（ErrOpenState→StateOpen, ErrTooManyRequests→StateHalfOpen），
// 而非依赖调用方传入的实时 State() 查询。这避免了 TOCTOU 竞态——
// cb.Execute 返回后到调用 State() 之间，其他 goroutine 可能触发状态变化，
// 导致 BreakerError.State 字段与错误发生时的实际状态不一致。
func wrapBreakerError(err error, name string) error {
	if err == nil {
		return nil
	}

	// 如果已经是 BreakerError，保留原始错误，避免重复包装
	// 这确保了嵌套熔断器场景下错误来源的正确性
	var be *BreakerError
	if errors.As(err, &be) {
		return err
	}

	// 只检查直接的 sentinel error，不使用 errors.Is
	// 这避免了将错误链中的熔断器错误错误地归因到当前熔断器
	if err == gobreaker.ErrOpenState {
		return newBreakerError(err, name, StateOpen)
	}
	if err == gobreaker.ErrTooManyRequests {
		return newBreakerError(err, name, StateHalfOpen)
	}

	return err
}

// IsOpen 检查错误是否是熔断器打开错误
//
// 当熔断器处于 Open 状态时，调用会返回此错误。
// 可用于判断是否应该快速失败或使用降级逻辑。
//
// 示例:
//
//	result, err := xbreaker.Execute(ctx, breaker, fn)
//	if xbreaker.IsOpen(err) {
//	    return fallbackValue, nil // 使用降级值
//	}
func IsOpen(err error) bool {
	return errors.Is(err, gobreaker.ErrOpenState)
}

// IsTooManyRequests 检查错误是否是请求过多错误
//
// 当熔断器处于 HalfOpen 状态且已达到最大请求数时，
// 新的请求会返回此错误。
//
// 示例:
//
//	result, err := xbreaker.Execute(ctx, breaker, fn)
//	if xbreaker.IsTooManyRequests(err) {
//	    // 等待一段时间后重试
//	    time.Sleep(100 * time.Millisecond)
//	    return retry()
//	}
func IsTooManyRequests(err error) bool {
	return errors.Is(err, gobreaker.ErrTooManyRequests)
}

// IsBreakerError 检查错误是否是熔断器相关错误
//
// 包括 ErrOpenState 和 ErrTooManyRequests。
// 可用于区分熔断器错误和业务错误。
//
// 示例:
//
//	result, err := xbreaker.Execute(ctx, breaker, fn)
//	if xbreaker.IsBreakerError(err) {
//	    log.Warn("熔断器拦截请求", "error", err)
//	    return fallbackValue, nil
//	}
//	if err != nil {
//	    log.Error("业务错误", "error", err)
//	    return nil, err
//	}
func IsBreakerError(err error) bool {
	return IsOpen(err) || IsTooManyRequests(err)
}
