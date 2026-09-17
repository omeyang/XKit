---
package: pkg/context/xplatform
stability: Stable
coverage: 100.0%
tags: [context, platform]
---

# pkg/context/xplatform

平台信息管理。封装 platformID 与 deploymentType（如 dev/test/prod），以及对应的注入与提取。

## 用途

- 根据部署环境做条件逻辑（如 prod 不开 verbose 日志）
- 多平台 SaaS 中区分平台

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/context/xplatform"

ctx = xplatform.WithPlatformID(ctx, "platform-A")
ctx = xplatform.WithDeploymentType(ctx, xplatform.Prod)

if xplatform.DeploymentTypeOf(ctx) == xplatform.Prod {
    // ...
}
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `WithPlatformID / PlatformIDOf` | 平台 ID 注入/提取 |
| `WithDeploymentType / DeploymentTypeOf` | 部署类型注入/提取 |
| `DeploymentType` 枚举 | `Dev / Test / Prod / Stage` 等 |

## 设计要点

- **typed key**：避免冲突
- **枚举类型**：编译时检查部署类型，避免字符串拼写错

## 相关

- 包：[xctx](01-xctx.md)（同样有 platform getter）
- API：[api.md#pkgcontextxplatform](../../03-conventions/01-api.md#pkgcontextxplatform)
