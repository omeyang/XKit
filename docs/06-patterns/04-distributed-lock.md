---
title: Distributed Lock 模式
tags: [pattern, distributed, lock]
---

# Distributed Lock

## 问题

跨进程互斥：多副本同时只能一个执行某操作。

## XKit 实现

[xdlock](../04-packages/05-distributed/02-xdlock.md) 提供两种后端：

| 后端 | 适用 |
|---|---|
| Redis NX + Lua | 已有 Redis；高频短期锁 |
| etcd concurrency.Mutex | 强一致；带 lease |

## 使用 Redis 锁

从 `xdlock.NewRedisFactory` 创建工厂，再按资源名获取锁句柄。临界区结束时用该句柄释放锁。多节点 Redlock 可向工厂传入多个 Redis 客户端。

[加锁、释放和多节点示例](../../tests/unit/pkg/distributed/xdlock/example_test.go)

## 关键陷阱

### 1. 误释放他人的锁

A 持锁，TTL 30s，A 卡了 35s，TTL 过期 → 锁自动释放 → B 获得锁。此时 A 恢复，调 Unlock → **释放了 B 的锁**。

**XKit 解法**：每次 Lock 生成唯一 token，Unlock 用 Lua 比对 token 才删 key：

```lua
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
```

### 2. TTL 选择

- 太短：临界区还没结束就过期
- 太长：进程崩了之后久不释放

对需要延长持有时间的操作，调用锁句柄的 `Extend(ctx)` 并检查返回的错误。

## 何时选 xdlock vs 其他

| 场景 | 用 |
|---|---|
| 短期临界区互斥 | [xdlock](../04-packages/05-distributed/02-xdlock.md) |
| 长期持有 leader 身份 | [xelection](../04-packages/05-distributed/03-xelection.md) → [Leader Election](06-leader-election.md) |
| 限并发（非互斥） | [xsemaphore](../04-packages/05-distributed/04-xsemaphore.md) |
| 定时任务去重 | [xcron](../04-packages/05-distributed/01-xcron.md)（内置锁） |

## 相关

- 包：[xdlock](../04-packages/05-distributed/02-xdlock.md)、[xsemaphore](../04-packages/05-distributed/04-xsemaphore.md)、[xelection](../04-packages/05-distributed/03-xelection.md)、[xcron](../04-packages/05-distributed/01-xcron.md)
- 模式：[Leader Election](06-leader-election.md)
