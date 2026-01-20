# 模块审查：pkg/resilience（弹性容错）

> **输出要求**：请用中文输出审查结果，不要直接修改代码。只需分析问题并提出改进建议即可。

## 项目背景

XKit 是深信服内部的 Go 基础库，供其他服务调用。Go 1.25.4，K8s 原生部署。

## 模块概览

```
pkg/resilience/
├── xretry/    # 重试策略，基于 avast/retry-go/v5
└── xbreaker/  # 熔断器，基于 sony/gobreaker/v2
```

设计原则：
- 接口驱动设计（依赖倒置）
- 类型别名暴露底层库完整能力
- 配置与执行分离

包间关系：
- xbreaker 可与 xretry 组合使用（BreakerRetryer）
- xmq 的 DLQ 功能依赖 xretry 接口

---

## xretry：重试策略

**职责**：提供通用的重试策略和退避策略接口及实现。

**关键文件**：
- `policy.go` - RetryPolicy/BackoffPolicy 接口定义
- `retry.go` - Retryer 执行器
- `backoff.go` - 退避策略实现
- `errors.go` - 错误类型定义

**核心接口**：
```go
type RetryPolicy interface {
    MaxAttempts() int
    ShouldRetry(ctx context.Context, attempt int, err error) bool
}
type BackoffPolicy interface {
    NextDelay(attempt int) time.Duration
}
```

**重试策略**：
| 策略 | 说明 |
|------|------|
| FixedRetryPolicy | 固定次数重试 |
| AlwaysRetryPolicy | 无限重试（慎用） |
| NeverRetryPolicy | 永不重试 |

**退避策略**：
| 策略 | 说明 | 安全特性 |
|------|------|----------|
| FixedBackoff | 固定延迟 | - |
| ExponentialBackoff | 指数退避（带抖动） | crypto/rand 失败自动回退 math/rand/v2 |
| LinearBackoff | 线性退避 | 整数溢出保护，极端参数返回 maxDelay |
| NoBackoff | 无延迟 | - |

**两种使用方式**：
```go
// 方式一：Retryer（接口抽象）
retryer := xretry.NewRetryer(
    xretry.WithRetryPolicy(xretry.NewFixedRetry(3)),
    xretry.WithBackoffPolicy(xretry.NewExponentialBackoff()),
)
err := retryer.Do(ctx, fn)

// 方式二：retry-go 风格（简单场景）
err := xretry.Do(ctx, fn, xretry.Attempts(3), xretry.Delay(100*time.Millisecond))
```

**错误类型**：
- `NewPermanentError(err)` - 永久性错误，不重试
- `NewTemporaryError(err)` - 临时性错误，应重试
- `Unrecoverable(err)` - retry-go 不可恢复错误

**注意**：通过 Retryer 使用时，仅使用 RetryPolicy.MaxAttempts()，ShouldRetry() 不被调用（底层 retry-go 限制）。

---

## xbreaker：熔断器

**职责**：保护系统免受级联故障影响。

**关键文件**：
- `breaker.go` - Breaker 接口和实现
- `policy.go` - TripPolicy 熔断策略
- `combo.go` - BreakerRetryer 组合执行器
- `alias.go` - gobreaker 类型别名

**状态机**：
```
StateClosed ──[ReadyToTrip=true]──> StateOpen
     ↑                                  │
     │                            [Timeout]
     │                                  ↓
     └────[探测成功]──── StateHalfOpen ──[探测失败]──→ StateOpen
```

**熔断策略**：
| 策略 | 说明 | 安全特性 |
|------|------|----------|
| ConsecutiveFailuresPolicy | 连续失败 N 次 | - |
| FailureRatioPolicy | 失败率阈值 | 除零保护（Requests=0 返回 false） |
| FailureCountPolicy | 失败次数阈值 | - |
| SlowCallRatioPolicy | 慢调用比例 | 除零保护 |
| CompositePolicy | 组合策略（任一满足） | - |
| NeverTripPolicy / AlwaysTripPolicy | 测试用 | - |

**核心接口**：
```go
type TripPolicy interface {
    ReadyToTrip(counts Counts) bool
}
```

**两种使用方式**：
```go
// 方式一：Breaker（接口抽象）
breaker := xbreaker.NewBreaker("my-service",
    xbreaker.WithTripPolicy(xbreaker.NewConsecutiveFailures(5)),
    xbreaker.WithTimeout(30*time.Second),
)
result, err := xbreaker.Execute(ctx, breaker, fn)

// 方式二：直接使用 gobreaker
cb := xbreaker.NewCircuitBreaker[string](xbreaker.Settings{
    Name:        "my-service",
    MaxRequests: 3,
    ReadyToTrip: func(counts xbreaker.Counts) bool {
        return counts.ConsecutiveFailures >= 5
    },
})
```

**与 xretry 组合**：
```go
combo := xbreaker.NewBreakerRetryer(breaker, retryer)
result, err := xbreaker.ExecuteWithRetry(ctx, combo, fn)
// 执行顺序：熔断检查 → 执行 → 失败重试
```

**高性能场景**：
```go
// ManagedBreaker 避免类型断言开销
managed := xbreaker.NewManagedBreaker[*http.Response](breaker)
resp, err := managed.Execute(fn)
```

---

## 审查参考

以下是一些值得关注的技术细节，但不限于此：

**xretry 底层集成**：
- retry-go 的 Context 支持是否正确传递？
- ShouldRetry() 不被调用是否在代码中清晰体现？
- Retryer 零值安全是否真正实现？
- PermanentError/TemporaryError/Unrecoverable 是否都正确处理？

**退避策略安全性**：
- LinearBackoff 溢出保护在所有边界条件下是否生效？
- ExponentialBackoff 的 crypto/rand 回退逻辑是否正确？

**xbreaker 底层集成**：
- TripPolicy 到 gobreaker.Settings.ReadyToTrip 的映射是否正确？
- 类型别名是否完整暴露底层类型？
- ManagedBreaker 泛型是否正确避免类型断言开销？

**状态机正确性**：
- MaxRequests 在 HalfOpen 状态的作用是否清晰？
- OnStateChange 回调是否在所有状态转换时触发？
- IsOpen(err) 是否正确识别熔断错误？

**BreakerRetryer 组合**：
- 执行顺序是否正确？
- 熔断打开时是否跳过重试？
- 重试期间熔断器状态变化如何处理？

**并发安全**：
- Retryer 是否线程安全？OnRetry 回调并发执行？
- HalfOpen 状态多请求同时到达时的行为？
- gobreaker 的并发安全由底层保证，xbreaker 包装层是否引入额外共享状态？

**Context 取消**：
- ctx 超时时是否立即停止重试？
- 返回的错误是 context.DeadlineExceeded 还是被包装？

**错误处理**：
- 重试耗尽后返回最后一次错误还是聚合错误？
- retry-go 的错误包装行为是否被正确处理？
