---
package: pkg/lifecycle/xhealth
stability: Beta
coverage: 92.4%
tags: [lifecycle, health, kubernetes]
related:
  - ../../05-concepts/08-panic-recover-discipline.md
---

# pkg/lifecycle/xhealth

Kubernetes 健康探针。HTTP 监听暴露 `/healthz`、`/readyz`、`/startupz` 端点，可注册多检查器。

## 用途

- K8s liveness / readiness / startup probe
- 检查内部依赖（DB ping / DNS / 自定义）

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/lifecycle/xhealth"

h, _ := xhealth.New(
    xhealth.WithAddr(":8081"),
    xhealth.WithCacheTTL(2*time.Second),
)

h.AddCheck("db", xhealth.DatabasePingCheck(db), xhealth.CheckConfig{
    Timeout: 500*time.Millisecond,
    Endpoint: xhealth.Readiness,
})
h.AddCheck("goroutines", xhealth.GoroutineCountCheck(10000), xhealth.CheckConfig{
    Endpoint: xhealth.Liveness,
})

go h.Start()
defer h.Stop(context.Background())
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Health` / `New(opts...) (*Health, error)` | 入口 |
| `CheckFunc` / `CheckConfig` / `CheckResult / Result` | 类型 |
| `Status` 枚举 | Up / Down / Unknown |
| `StatusListenerFunc` | 状态变更回调（**必须有 recover 防御**）|
| 内置：`GoroutineCountCheck / TCPDialCheck / DatabasePingCheck / DNSResolveCheck / HTTPGetCheck` | |
| 选项：`WithAddr / WithBasePath / WithCacheTTL / WithStatusListener / WithShutdownTimeout / WithDetailOnQueryParam` | |

## 设计要点

- **`statusListener` 回调补 recover** 防御（FG-M fix，与 `CheckFunc` 对齐）
- **缓存 TTL**：避免高频探针打爆下游
- **detail 仅在带 query 参数时返**：避免泄漏内部信息

## 相关

- 概念：[Panic recover 纪律](../../05-concepts/08-panic-recover-discipline.md)
- API：[api.md#pkglifecyclexhealth](../../03-conventions/01-api.md#pkglifecyclexhealth)
