---
title: Internal 包
tags: [moc, packages, internal]
---

# Internal 包

仅 XKit 项目内部使用。不在 `pkg/` 下，受 Go `internal/` 可见性规则保护，**业务方不可直接导入**。

## 包列表

| 包 | 用途 | 覆盖率 |
|---|---|---|
| [deploy](01-deploy.md) | 部署相关共享逻辑 | 100.0% |
| [mqcore](02-mqcore.md) | MQ 通用消费循环（xkafka/xpulsar 共享，fail-fast 设计） | 100.0% |
| [rediscompat](03-rediscompat.md) | Redis 代理脚本模式探测（DetectScriptMode / Bounded） | 100.0% |
| [storageopt](04-storageopt.md) | 存储层共享选项（连接池/超时等） | 100.0% |

## 设计要点

- 共享逻辑放 internal/ 是为了**避免重复**，但又不暴露不稳定 API
- 所有 internal 包覆盖率 100%，因为其变更影响多个 pkg/，必须严格守护

## 相关

- [Fail-Fast 消费循环模式](../../06-patterns/05-fail-fast-consume-loop.md) - mqcore 设计
- [API 清单](../../03-conventions/01-api.md) - 不列出 internal 内容
