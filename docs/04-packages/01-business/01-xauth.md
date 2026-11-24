---
package: pkg/business/xauth
stability: Stable
coverage: 95.0%
tags: [business, auth, token, cache]
---

# pkg/business/xauth

认证服务客户端。封装 Token / 平台信息查询 + 双层缓存（Memory + Redis）。

## 用途

- 业务后端校验客户 Token
- 获取用户/租户/平台信息
- 高频查询场景下避免重复打 auth 服务

## 使用入口

示例文件中的 `ExampleNewClient`、`ExampleClient_GetToken` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/business/xauth` 查看。

[包说明](../../../pkg/business/xauth/doc.go) · [使用示例](../../../tests/unit/pkg/business/xauth/example_test.go)

## 设计要点

- **路径与配置分离**：`Path*` 常量与 `DefaultPaths()` 只是开源版本的示例路由；实际部署通过 `Config.Paths` 覆盖认证服务的真实路径，代码库中不保留任何环境专属的值。未设置的字段自动回填示例值，已设置的值必须以 `/` 开头（否则 `Validate` 返回 `ErrInvalidPath`）。

  ```go
  cfg := &xauth.Config{
      Host: "https://auth.example.com",
      Paths: xauth.Paths{
          TokenObtain: "/your/oauth/token",
          TokenVerify: "/your/oauth/token/verify",
      },
  }
  ```

- **双层缓存**：Memory（xlru）→ Redis（xcache）→ auth 服务
- **`Request()` 浅拷贝 AuthRequest** 避免修改调用方 Body
- **nil ctx 防御**：HTTPClient.Do/request 入口

## 相关

- 包：[xcache](../12-storage/01-xcache.md) / [xlru](../14-util/05-xlru.md)
- 概念：[Nil context 防御](../../05-concepts/06-nil-context-defense.md)
- API：[api.md#pkgbusinessxauth](../../03-conventions/01-api.md#pkgbusinessxauth)
