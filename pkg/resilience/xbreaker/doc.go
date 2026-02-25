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
//   - SlowCallRatioPolicy：慢调用熔断（基于 FailureRatioPolicy，需配合 SuccessPolicy 使用）
//
// # 组合模式
//
// 提供两种熔断器+重试的组合模式：
//   - BreakerRetryer：每次重试都经过熔断器检查和记录
//   - RetryThenBreak：重试期间不影响熔断器统计，只有最终结果才记录
//
// 组合构造函数（NewBreakerRetryer、NewRetryThenBreak、NewRetryThenBreakWithConfig）
// 对 nil 参数返回错误（ErrNilBreaker、ErrNilRetryer），不会 panic。
//
// # 错误排除
//
// 若需将特定错误（如 context.Canceled）从熔断统计中排除（不影响任何计数），
// 可通过 WithExcludePolicy 设置错误排除策略。
// 若需将特定错误标记为"成功"（计入成功计数），请使用 WithSuccessPolicy。
//
// # 状态变化回调
//
// WithOnStateChange 注册的回调通过 goroutine 异步执行，
// 避免与 gobreaker 内部 mutex 产生死锁。回调中可安全调用
// Breaker 的 State()/Counts()/Do() 等方法。
// 回调中的 panic 会被自动捕获并通过 slog.Error 记录，不会导致进程崩溃。
// 注意回调执行顺序不保证，且获取的状态可能已是更新后的值。
//
// 设计决策: 回调必须为轻量级操作（建议 <1ms），不应在回调中执行 I/O 操作
// （如数据库写入、HTTP 请求）。每次状态变化会启动一个新 goroutine 执行回调，
// 不设背压机制。若回调执行缓慢且状态快速振荡，goroutine 可能累积。
// 需要在回调中执行 I/O 的场景，请自行实现有界队列+消费者模式。
//
// [sony/gobreaker/v2]: https://github.com/sony/gobreaker
package xbreaker
