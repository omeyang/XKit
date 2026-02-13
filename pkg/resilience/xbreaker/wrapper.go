package xbreaker

import (
	"github.com/sony/gobreaker/v2"
)

// 以下是 sony/gobreaker/v2 的类型别名，便于直接使用底层能力
// 用户可以直接使用这些类型，无需导入 gobreaker 包

type (
	// Settings 熔断器配置
	Settings = gobreaker.Settings

	// Counts 统计计数，用于熔断判定
	Counts = gobreaker.Counts

	// State 熔断器状态
	State = gobreaker.State

	// CircuitBreaker 泛型熔断器
	CircuitBreaker[T any] = gobreaker.CircuitBreaker[T]

	// TwoStepCircuitBreaker 两阶段熔断器
	// 用于需要手动报告成功/失败的场景
	TwoStepCircuitBreaker[T any] = gobreaker.TwoStepCircuitBreaker[T]
)

// 熔断器状态常量
const (
	// StateClosed 关闭状态（正常）
	// 请求正常通过，失败会被统计
	StateClosed = gobreaker.StateClosed

	// StateHalfOpen 半开状态（探测）
	// 允许有限请求通过以检测服务是否恢复
	StateHalfOpen = gobreaker.StateHalfOpen

	// StateOpen 打开状态（熔断）
	// 请求直接失败，不会调用后端服务
	StateOpen = gobreaker.StateOpen
)

// 熔断器错误
var (
	// ErrTooManyRequests 半开状态下请求过多
	ErrTooManyRequests = gobreaker.ErrTooManyRequests

	// ErrOpenState 熔断器处于打开状态
	ErrOpenState = gobreaker.ErrOpenState
)

// NewCircuitBreaker 创建泛型熔断器
//
// 这是对 gobreaker.NewCircuitBreaker 的直接封装。
// 适用于需要完全控制熔断器配置的场景。
//
// 示例:
//
//	cb := xbreaker.NewCircuitBreaker[string](xbreaker.Settings{
//	    Name:        "my-service",
//	    MaxRequests: 3,
//	    Timeout:     30 * time.Second,
//	    ReadyToTrip: func(counts xbreaker.Counts) bool {
//	        return counts.ConsecutiveFailures >= 5
//	    },
//	})
//
//	result, err := cb.Execute(func() (string, error) {
//	    return callRemoteService()
//	})
func NewCircuitBreaker[T any](st Settings) *CircuitBreaker[T] {
	return gobreaker.NewCircuitBreaker[T](st)
}

// NewTwoStepCircuitBreaker 创建两阶段熔断器
//
// 两阶段熔断器适用于需要手动报告成功/失败的场景，
// 例如异步操作或需要自定义成功判定的场景。
//
// 示例:
//
//	cb := xbreaker.NewTwoStepCircuitBreaker[string](xbreaker.Settings{
//	    Name: "async-service",
//	})
//
//	// 第一阶段：获取执行许可
//	done, err := cb.Allow()
//	if err != nil {
//	    return err // 熔断器打开，不允许执行
//	}
//
//	// 执行操作
//	result, err := doAsyncOperation()
//
//	// 第二阶段：报告结果（nil 表示成功，非 nil 表示失败）
//	done(err)
func NewTwoStepCircuitBreaker[T any](st Settings) *TwoStepCircuitBreaker[T] {
	return gobreaker.NewTwoStepCircuitBreaker[T](st)
}

// StateString 返回状态的字符串表示
//
// 设计决策: 虽然 State 类型自身有 String() 方法，此函数保留用于
// Go 模板（text/template）等不能直接调用方法的场景。
func StateString(s State) string {
	return s.String()
}
