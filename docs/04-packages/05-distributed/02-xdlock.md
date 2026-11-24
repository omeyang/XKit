---
package: pkg/distributed/xdlock
stability: Stable
coverage: 94.6%
tags: [distributed, lock, redis, etcd]
related:
  - ../../06-patterns/04-distributed-lock.md
---

# pkg/distributed/xdlock

分布式锁。两套后端：Redis（NX + Lua 释放）与 etcd（concurrency.Mutex）。

## 用途

- 短期互斥（操作幂等保护、初始化串行化）
- 多副本中只让一个执行某操作（一次性任务）

## 不适用场景

- 选主（长期持有）：用 [xelection](03-xelection.md)
- 限流/限并发：用 [xsemaphore](04-xsemaphore.md)

## 使用入口

示例文件中的 `Example_redisBasic`、`Example_redlock` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/distributed/xdlock` 查看。

[包说明](../../../pkg/distributed/xdlock/doc.go) · [使用示例](../../../tests/unit/pkg/distributed/xdlock/example_test.go)

## 设计要点

- **Redis NX + Lua 释放**：避免误释放他人持有的锁
- **etcd v3.6 移除了 `ErrLockReleased`**：实现不得依赖该错误
- 调用方通过 `LockHandle.Extend` 主动续期。

## 相关

- 模式：
  - [Distributed Lock](../../06-patterns/04-distributed-lock.md)
- API：[api.md#pkgdistributedxdlock](../../03-conventions/01-api.md#pkgdistributedxdlock)
