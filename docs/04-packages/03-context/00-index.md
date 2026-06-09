---
title: Context 包
tags: [moc, packages, context]
---

# Context 包

`context.Context` 上增强：追踪 / 租户 / 平台 / 环境信息的注入与提取。所有跨包传递语义都从此处规定。

## 包列表

| 包 | 用途 | 稳定性 | 覆盖率 |
|---|---|---|---|
| [xctx](01-xctx.md) | Context 增强（追踪/租户/平台融合入口） | Stable | 97.7% |
| [xtenant](04-xtenant.md) | 租户信息 HTTP+gRPC 双协议中间件 | Stable | 96.8% |
| [xplatform](03-xplatform.md) | 平台信息（部署类型/平台 ID）管理 | Stable | 100.0% |
| [xenv](02-xenv.md) | 环境变量读取与解析 | Stable | 95.6% |

## 选型指南

| 我要... | 用 |
|---|---|
| 在请求处理链上传递 traceID/tenantID | [xctx](01-xctx.md) |
| HTTP/gRPC 服务端自动提取租户 | [xtenant](04-xtenant.md) |
| 知道当前部署环境是 dev/test/prod | [xplatform](03-xplatform.md) |
| 读取 ENV 变量（含类型转换、默认值） | [xenv](02-xenv.md) |

## 相关概念

- [Interface defined by consumer](../../05-concepts/04-interface-by-consumer.md)
- [Nil context defense](../../05-concepts/06-nil-context-defense.md)

## 相关 ADR

- [0002 接口由使用方定义](../../01-decisions/0002-interface-defined-by-consumer.md)

## 相关 API

- [API 清单 Context 段](../../03-conventions/01-api.md)
