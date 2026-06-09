---
package: pkg/resilience/xbreaker
stability: Beta
coverage: 99.4%
tags: [resilience, circuit-breaker]
related:
  - ../../06-patterns/03-circuit-breaker.md
---

# pkg/resilience/xbreaker

熔断器。基于 `sony/gobreaker`，提供状态机封装与可观测性钩子。

## 用途

- 防止级联故障：下游异常时快速失败，避免堆积
- 给下游恢复时间

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/resilience/xbreaker"

cb := xbreaker.New("downstream-service",
    xbreaker.WithMaxRequests(3),
    xbreaker.WithInterval(30*time.Second),
    xbreaker.WithTimeout(10*time.Second),
    xbreaker.WithReadyToTrip(func(counts gobreaker.Counts) bool {
        return counts.ConsecutiveFailures > 5
    }),
)

result, err := cb.Execute(ctx, func() (any, error) {
    return callDownstream(ctx)
})
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Breaker` | 熔断器实例 |
| `New(name, opts...) *Breaker` | 构造 |
| `Execute(ctx, fn) (result, error)` | 包裹调用 |
| `WithMaxRequests / WithInterval / WithTimeout / WithReadyToTrip` | 选项 |

## 设计要点

- **sony/gobreaker**：成熟库，不自造轮子
- **三态机**：Closed / Open / Half-Open

## 相关

- 模式：[Circuit Breaker](../../06-patterns/03-circuit-breaker.md)
- API：[api.md#pkgresiliencexbreaker](../../03-conventions/01-api.md#pkgresiliencexbreaker)
