---
title: Business 包
tags: [moc, packages, business]
---

# Business 包

业务公共能力封装：与具体业务无关、但服务于业务的复用层（Token/认证客户端等）。

## 包列表

| 包 | 用途 | 稳定性 | 覆盖率 |
|---|---|---|---|
| [xauth](01-xauth.md) | 认证服务客户端（Token + 平台信息 + 双层缓存） | Beta | 95.0% |

## 相关

- [Storage 包](../12-storage/00-index.md) - xcache 是 xauth 的双层缓存底层
- [Context 包](../03-context/00-index.md) - 平台信息通过 xplatform 注入 ctx
- [API 清单](../../03-conventions/01-api.md) - 公开 API 列表
