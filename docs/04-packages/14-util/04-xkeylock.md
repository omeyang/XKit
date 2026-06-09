---
package: pkg/util/xkeylock
stability: Beta
coverage: 100.0%
tags: [util, lock, concurrency]
related:
  - ../../05-concepts/01-concurrency-safety.md
---

# pkg/util/xkeylock

按 key 的进程内互斥锁。同 key 互斥、不同 key 并发，分片 channel 实现，**非可重入**。

## 用途

- 缓存击穿防护：同 key 的多个 goroutine 只让一个去回源
- 资源串行化：按 key 序列化某种操作（如同一文件的写入）

## 不适用场景

- 跨进程：用 [xdlock](../05-distributed/02-xdlock.md)
- 可重入需求：本包刻意不支持（设计决策）

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/util/xkeylock"

kl, err := xkeylock.New(xkeylock.WithShards(64))
if err != nil { /* ... */ }
defer kl.Close()

h, err := kl.Lock(ctx, "user:123")
if err != nil { /* ctx 取消或已关闭 */ }
defer h.Unlock()

// 此时同 key "user:123" 的并发会阻塞,不同 key 并发
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Locker` interface | 主接口（按 ADR-0002 由使用方角度命名）|
| `New(opts...) (Locker, error)` | 构造 |
| `WithShards(n int)` | 分片数（默认 16，最大 `1<<16`）|
| `Lock(ctx, key) (handle, error)` | 获取，handle.Unlock() 释放 |
| `TryLock(ctx, key) (handle, ok, error)` | 非阻塞尝试 |
| `Close() error` | 关闭（之后 Lock 返回 `ErrClosed`）|
| `Keys() []string` | 当前活跃 key（非原子，文档化）|
| `ErrNilContext / ErrClosed` | 错误变量 |

## 设计要点

- **分片机制**：FNV 哈希 → shard index；shard mutex 保护单个 shard 的 entry map
- **refcnt + ch**：`lockEntry{refcnt, ch}`；refcnt≥1 保证 entry 不被 GC，ch 是阻塞通道
- **非可重入**：同 goroutine 重入会死锁，文档化设计决策
- **shardPayload 抽出**：架构可移植的 `unsafe.Sizeof` padding，修 32-bit 构建
- **Keys() 非原子**：每 shard 持锁遍历，但 shards 间无全局快照（文档化）
- **CAS 单执行**：Unlock 用 CAS 保证恰好一次

## 相关

- 概念：
  - [并发安全](../../05-concepts/01-concurrency-safety.md)
- ADR：
  - [0002 接口由使用方定义](../../01-decisions/0002-interface-defined-by-consumer.md)
- API：[api.md#pkgutilxkeylock](../../03-conventions/01-api.md#pkgutilxkeylock)
