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

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/business/xauth"

c, _ := xauth.New(xauth.Config{
    BaseURL: "https://auth.internal/v1",
    Cache: xauth.CacheConfig{
        MemoryTTL: 30*time.Second,
        RedisTTL:  5*time.Minute,
        RedisClient: redisClient,
    },
})

info, err := c.ValidateToken(ctx, token)
if err != nil { /* 失败或过期 */ }
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Client / New(cfg)` | 入口 |
| `Config / CacheConfig` | 配置 |
| `ValidateToken(ctx, token) (Info, error)` | 校验 + 拉信息 |
| `HTTPClient.Do / request` | nil ctx 返 `ErrNilContext`（原 panic，FG-H fix）|

## 设计要点

- **双层缓存**：Memory（xlru）→ Redis（xcache）→ auth 服务
- **`Request()` 浅拷贝 AuthRequest** 避免修改调用方 Body（FG-M fix）
- **nil ctx 防御**：HTTPClient.Do/request 入口（FG-H fix）

## 相关

- 包：[xcache](../12-storage/01-xcache.md) / [xlru](../14-util/05-xlru.md)
- 概念：[Nil context 防御](../../05-concepts/06-nil-context-defense.md)
- API：[api.md#pkgbusinessxauth](../../03-conventions/01-api.md#pkgbusinessxauth)
