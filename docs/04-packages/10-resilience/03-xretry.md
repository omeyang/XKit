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
    xretry.Attempts(5),
    xretry.Delay(100*time.Millisecond),
    xretry.MaxDelay(5*time.Second),
    xretry.DelayType(xretry.BackOffDelay),
    xretry.RetryIf(func(err error) bool {
        return xretry.IsRetryable(err)
    }),
)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Do(ctx, fn, opts...) error` | 入口 |
| `Attempts / Delay / MaxDelay` | 节奏 |
| `DelayType(strategy)` | `BackOffDelay / FixedDelay / FullJitterBackoffDelay` |
| `RetryIf(pred)` | 自定义判断 |
| `IsRetryable / IsPermanent` | 重试错误分类 |

## 设计要点

- **ctx 取消立即停**：在 sleep 期间也响应 ctx.Done()
- **分类重试**：默认永久性错误不重试

## 相关

- API：[api.md#pkgresiliencexretry](../../03-conventions/01-api.md#pkgresiliencexretry)
