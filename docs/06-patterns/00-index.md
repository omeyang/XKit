---
title: 设计模式 MOC
tags: [moc, patterns]
---

# 设计模式（MOC）

XKit 中反复出现的模式。区别于 [概念](../05-concepts/00-index.md)（语法/语义层面）与 [ADR](../01-decisions/00-index.md)（决策层面），patterns 是**可复用的解题模板**。

## 缓存与去重

| 模式 | 摘要 |
|---|---|
| [Cache-Aside](02-cache-aside.md) | 业务读缓存 → miss 回源 → 写缓存。XKit 提供 `GetOrLoad` 助手 |
| [Singleflight](07-singleflight.md) | 同 key 并发回源去重。配合 Cache-Aside 防缓存击穿 |

## 互斥与协调

| 模式 | 摘要 |
|---|---|
| [Distributed Lock](04-distributed-lock.md) | 跨进程互斥；Redis NX+Lua 或 etcd concurrency.Mutex |
| [Leader Election](06-leader-election.md) | 多副本选一个 leader 长期持有；etcd lease |

## 并发与韧性

| 模式 | 摘要 |
|---|---|
| [Worker Pool](08-worker-pool.md) | 固定 worker + 有界队列；非阻塞提交 |
| [Circuit Breaker](03-circuit-breaker.md) | 三态机：快速失败防级联故障 |
| [Fail-Fast 消费循环](05-fail-fast-consume-loop.md) | MQ 消费 handler panic 不补 recover；与包内 goroutine recover 形成对照 |

## 工程流程

| 模式 | 摘要 |
|---|---|
| [对抗审查](01-adversarial-review.md) | 多路 AI 互审：Claude (CA/CB) + Codex (A/B) + 交叉裁决 (CC) |

## 相关

- [包索引](../04-packages/00-index.md)
- [概念笔记 MOC](../05-concepts/00-index.md)
