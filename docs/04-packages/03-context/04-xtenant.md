---
package: pkg/context/xtenant
stability: Stable
coverage: 96.8%
tags: [context, tenant, http, grpc, middleware]
---

# pkg/context/xtenant

租户信息 HTTP + gRPC 双协议中间件。从 header/metadata 自动提取 tenantID 注入 ctx。

## 用途

- SaaS 多租户系统的统一租户传播
- 跨服务调用保留租户上下文
- 业务方读 `xctx.TenantID(ctx)` 无需关心传输层

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/context/xtenant"

// HTTP 服务端
mux.Use(xtenant.HTTPMiddleware())

// gRPC 服务端
server := grpc.NewServer(
    grpc.UnaryInterceptor(xtenant.UnaryServerInterceptor()),
    grpc.StreamInterceptor(xtenant.StreamServerInterceptor()),
)

// gRPC 客户端
conn, _ := grpc.Dial(addr,
    grpc.WithUnaryInterceptor(xtenant.UnaryClientInterceptor()),
)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `HTTPMiddleware() func(http.Handler) http.Handler` | HTTP 自动提取 |
| `UnaryServerInterceptor / StreamServerInterceptor` | gRPC 服务端 |
| `UnaryClientInterceptor / StreamClientInterceptor` | gRPC 客户端 |
| `InjectToRequest(ctx, req)` / `InjectToOutgoingContext(ctx)` | 显式注入（跨服务调用）|

## 设计要点

- **HTTP / gRPC 命名一致**：header `X-Tenant-Id`，metadata `x-tenant-id`
- **缺失容忍**：无 header 时 ctx 不注入，下游 getter 返空
- **空白校验**：TrimSpace 后注入

## 相关

- 包：[xctx](01-xctx.md)（getter 在那里）
- 概念：[多租户传播](../../06-patterns/04-distributed-lock.md)
- API：[api.md#pkgcontextxtenant](../../03-conventions/01-api.md#pkgcontextxtenant)
