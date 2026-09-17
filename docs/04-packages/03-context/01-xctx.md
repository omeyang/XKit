---
package: pkg/context/xctx
stability: Stable
coverage: 97.7%
tags: [context, trace, tenant, platform]
---

# pkg/context/xctx

`context.Context` 增强。统一管理追踪 / 租户 / 平台信息的注入与提取，是其他三个 context 包的**汇总入口**。

## 用途

- 跨包传递 traceID / spanID / requestID
- 跨包传递 tenantID / tenantName
- 跨包传递 platformID / deploymentType

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/context/xctx"

ctx = xctx.WithTraceID(ctx, "abc123")
ctx = xctx.WithTenantID(ctx, "tenant-A")
ctx = xctx.EnsureTrace(ctx)  // 若无 traceID 自动生成

tid := xctx.TraceID(ctx)
ten := xctx.TenantID(ctx)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `WithTraceID / WithSpanID / WithRequestID` | 注入追踪 |
| `TraceID / SpanID / RequestID` | 提取 |
| `EnsureTrace(ctx)` | 无则生成 |
| `WithTenantID / WithTenantName` | 注入租户 |
| `TenantID / TenantName` | 提取 |
| `WithPlatformID / WithDeploymentType` | 注入平台 |
| `PlatformID / DeploymentType` | 提取 |

## 设计要点

- **typed key**：每类信息用未导出的 typed key，避免外部冲突
- **零值容忍**：所有 getter 在缺失时返空串/零值，不 panic

## 相关

- 包：[xtenant](04-xtenant.md) / [xplatform](03-xplatform.md) / [xenv](02-xenv.md)
- ADR：[0002 接口由使用方定义](../../01-decisions/0002-interface-defined-by-consumer.md)
- API：[api.md#pkgcontextxctx](../../03-conventions/01-api.md#pkgcontextxctx)
