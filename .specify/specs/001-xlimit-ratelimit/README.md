# 001-xlimit-ratelimit

## 概述

XKit 分布式限流模块（xlimit）的规格文档目录。

## 文档索引

| 文档 | 状态 | 说明 |
|------|------|------|
| [spec.md](./spec.md) | ✅ 完成 | 需求规格文档 |
| [plan.md](./plan.md) | ✅ 完成 | 技术实现计划 |
| [tasks.md](./tasks.md) | ✅ 完成 | 任务拆解清单 |

## 模块定位

`xlimit` 是 XKit 弹性防护体系的一部分，与现有模块的关系：

```
pkg/resilience/
├── xbreaker/   # 熔断器 - 防止级联故障
├── xretry/     # 重试策略 - 瞬态故障恢复
└── xlimit/     # 限流器 - 流量控制保护 (本模块)
```

## 核心特性

- **分布式限流**：基于 Redis，多 Pod 共享配额
- **多租户原生**：与 xtenant 集成，支持默认配额 + 租户覆盖
- **层级限流**：全局 → 租户 → API 多层叠加
- **动态配置**：支持 xconf/etcd 热更新
- **智能降级**：Redis 故障时自动降级到本地限流

## 技术选型

- 底层实现：[go-redis/redis_rate](https://github.com/go-redis/redis_rate)
- 限流算法：GCRA（通用信元速率算法）

## 相关资源

- Feature ID: `feature-001-xlimit-ratelimit`
- 创建日期: 2026-01-16
- 状态: Draft
