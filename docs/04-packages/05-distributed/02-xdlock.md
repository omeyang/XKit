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

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/distributed/xdlock"

l, err := xdlock.NewRedis(redisClient, "lock:my-resource",
    xdlock.WithTTL(30*time.Second),
)
defer l.Close()

held, err := l.Lock(ctx)
if err != nil { /* ... */ }
if !held { return nil }
defer l.Unlock(ctx)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Locker` interface | `Lock / TryLock / Unlock / Renew / Close` |
| `NewRedis(client, key, opts...) Locker` | Redis 实现 |
| `NewEtcd(client, key, opts...) Locker` | etcd 实现 |
| `WithTTL / WithTries / WithRetryDelay / WithFailFast` | 选项 |

## 设计要点

- **Redis NX + Lua 释放**：避免误释放他人持有的锁
- **etcd v3.6 移除了 `ErrLockReleased`**：1.23 兼容分支需移除该分支
- **`Renew`** 由调用方主动续期，库内不内置心跳 goroutine

## 相关

- 模式：
  - [Distributed Lock](../../06-patterns/04-distributed-lock.md)
- API：[api.md#pkgdistributedxdlock](../../03-conventions/01-api.md#pkgdistributedxdlock)
