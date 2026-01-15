// Package xbreaker 提供熔断器功能，保护系统免受级联故障影响。
//
// # 设计理念
//
// xbreaker 通过类型别名完全暴露 [sony/gobreaker/v2] 原生能力，
// 并提供 TripPolicy 抽象简化熔断策略配置。
//
// # 熔断器状态
//
//   - StateClosed（关闭）：正常状态，请求正常通过
//   - StateOpen（打开）：熔断状态，请求直接失败
//   - StateHalfOpen（半开）：探测状态，允许部分请求通过
//
// # 熔断策略
//
// 内置策略（TripPolicy）：
//   - ConsecutiveFailuresPolicy：连续失败 N 次后熔断
//   - FailureRatioPolicy：失败率超过阈值后熔断
//   - FailureCountPolicy：失败次数超过阈值后熔断
//   - CompositePolicy：组合多个策略
//   - SlowCallRatioPolicy：慢调用熔断
//
// # 快速开始
//
//	breaker := xbreaker.NewBreaker("my-service",
//	    xbreaker.WithTripPolicy(xbreaker.NewConsecutiveFailures(5)),
//	    xbreaker.WithTimeout(30*time.Second),
//	)
//
//	result, err := xbreaker.Execute(ctx, breaker, func() (string, error) {
//	    return callRemoteService()
//	})
//
//	if xbreaker.IsOpen(err) {
//	    log.Println("服务熔断中")
//	}
//
// # 与 xretry 组合
//
//	combo := xbreaker.NewBreakerRetryer(breaker, retryer)
//	result, err := xbreaker.ExecuteWithRetry(ctx, combo, operation)
//
// [sony/gobreaker/v2]: https://github.com/sony/gobreaker
package xbreaker
