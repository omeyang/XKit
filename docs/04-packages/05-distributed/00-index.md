---
title: Distributed 包
tags: [moc, packages, distributed]
---

# Distributed 包

分布式协调：锁、定时任务、选主、信号量。Redis 与 etcd 双后端策略。

## 包列表

| 包 | 用途 | 稳定性 | 覆盖率 |
|---|---|---|---|
| [xdlock](02-xdlock.md) | 分布式锁（Redis/etcd 双实现） | Beta | 94.6% |
| [xcron](01-xcron.md) | 分布式定时任务（cron + 锁防重复执行） | Beta | 94.1% |
| [xelection](03-xelection.md) | 基于 etcd 的分布式选主 | Beta | 95.4% |
| [xsemaphore](04-xsemaphore.md) | Redis 分布式信号量（Lua + Fallback，多轮对抗审查稳定） | Beta | 94.1% |

## 选型指南

| 场景 | 用 |
|---|---|
| 互斥执行（多实例同时只能一个跑） | [xdlock](02-xdlock.md) |
| 周期性任务，多副本去重 | [xcron](01-xcron.md) |
| 多副本选出一个 leader 长期持有 | [xelection](03-xelection.md) |
| 限制并发数（如 5 个 worker 同时跑） | [xsemaphore](04-xsemaphore.md) |

## 后端依赖

- Redis：xdlock（一种实现）、xsemaphore
- etcd：xelection、xcron 的锁后端之一、xdlock（另一种实现）

## 相关概念

- [分布式锁模式](../../06-patterns/04-distributed-lock.md)
- [选主模式](../../06-patterns/06-leader-election.md)

## 相关 ADR

- [0001 构造函数返回 error](../../01-decisions/0001-constructor-returns-error-not-panic.md)
- [0003 函数选项模式](../../01-decisions/0003-functional-options-for-config.md)
