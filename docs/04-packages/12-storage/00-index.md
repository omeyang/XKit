---
title: Storage 包
tags: [moc, packages, storage]
---

# Storage 包

数据存储客户端封装：缓存、KV、文档库、列式分析。每个包均有 OTel 链路追踪集成。

## 包列表

| 包 | 用途 | 稳定性 | 覆盖率 |
|---|---|---|---|
| [xcache](01-xcache.md) | 缓存抽象层（Redis / Memory 双后端） | Stable | 93.8% |
| [xetcd](03-xetcd.md) | etcd 客户端 + Informer（list+watch 缓存） | Stable | 94.3% |
| [xmongo](04-xmongo.md) | MongoDB 客户端（mongo-driver v2） | Stable | 96.0% |
| [xclickhouse](02-xclickhouse.md) | ClickHouse 客户端（批次原子性 + COUNT UInt64） | Stable | 96.7% |

## 选型指南

| 场景 | 用 |
|---|---|
| 内存或 Redis 缓存（接口统一） | [xcache](01-xcache.md) |
| 配置中心 / 服务发现 / 协调 | [xetcd](03-xetcd.md) |
| 文档型业务数据存储 | [xmongo](04-xmongo.md) |
| 大规模 OLAP 分析 | [xclickhouse](02-xclickhouse.md) |

## 共享底层

- `internal/storageopt`：存储层共享选项（连接池、超时等）

## 相关

- [缓存模式](../../06-patterns/02-cache-aside.md)
- [API 清单 Storage 段](../../03-conventions/01-api.md)
