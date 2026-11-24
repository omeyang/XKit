---
package: pkg/context/xenv
stability: Stable
coverage: 95.6%
tags: [context, env]
---

# pkg/context/xenv

服务启动时通过 `Init()` 读取 `DEPLOYMENT_TYPE`，或用 `InitWith` 显式指定本地部署或 SaaS 部署。调用方可通过 `IsLocal()`、`IsSaaS()` 或 `RequireType()` 查询进程级部署类型。

## 使用入口

示例文件中的 `Example_quickStart`、`ExampleRequireType` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/context/xenv` 查看。

[包说明](../../../pkg/context/xenv/doc.go) · [使用示例](../../../tests/unit/pkg/context/xenv/example_test.go)

## 相关

- API：[api.md#pkgcontextxenv](../../03-conventions/01-api.md#pkgcontextxenv)
