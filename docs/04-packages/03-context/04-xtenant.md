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

## 使用入口

示例文件中的 `ExampleHTTPMiddleware`、`ExampleInjectToRequest` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/context/xtenant` 查看。

[包说明](../../../pkg/context/xtenant/doc.go) · [使用示例](../../../tests/unit/pkg/context/xtenant/example_test.go)

## 设计要点

- **HTTP / gRPC 命名一致**：header `X-Tenant-Id`，metadata `x-tenant-id`
- **缺失容忍**：无 header 时 ctx 不注入，下游 getter 返空
- **空白校验**：TrimSpace 后注入

## 相关

- 包：[xctx](01-xctx.md)（getter 在那里）
- 概念：[多租户传播](../../06-patterns/04-distributed-lock.md)
- API：[api.md#pkgcontextxtenant](../../03-conventions/01-api.md#pkgcontextxtenant)
