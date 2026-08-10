---
package: pkg/resilience/xretry
stability: Stable
coverage: 96.3%
tags: [resilience, retry]
---

# pkg/resilience/xretry

重试策略。指数退避 + 可中断 + 可分类重试。

## 用途

- 调用瞬时失败时自动重试
- ctx 取消立刻停止

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/resilience/xretry"

err := xretry.Do(ctx, func() error {
    return callAPI(ctx)
},
    xretry.WithMaxAttempts(5),
    xretry.WithInitialDelay(100*time.Millisecond),
    xretry.WithMaxDelay(5*time.Second),
    xretry.WithBackoff(xretry.ExponentialBackoff),
    xretry.WithRetryIf(func(err error) bool {
        return xretry.IsTransient(err)
    }),
)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Do(ctx, fn, opts...) error` | 入口 |
| `WithMaxAttempts / WithInitialDelay / WithMaxDelay` | 节奏 |
| `WithBackoff(strategy)` | `ExponentialBackoff / LinearBackoff` |
| `WithRetryIf(pred)` | 自定义判断 |
| `IsTransient(err)` | 内置网络瞬时错判断 |

## 设计要点

- **ctx 取消立即停**：在 sleep 期间也响应 ctx.Done()
- **分类重试**：默认非瞬时错不重试

## 相关

- API：[api.md#pkgresiliencexretry](../../03-conventions/01-api.md#pkgresiliencexretry)
