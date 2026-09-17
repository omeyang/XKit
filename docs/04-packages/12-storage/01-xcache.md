---
package: pkg/storage/xcache
stability: Stable
coverage: 93.8%
tags: [storage, cache, redis, memory]
related:
  - ../../06-patterns/02-cache-aside.md
---

# pkg/storage/xcache

缓存抽象层。Redis / Memory 双后端 + Cache-Aside 助手。

## 用途

- 业务代码不写 Redis client 直接调用
- 测试时切换 Memory 后端
- 标准 Cache-Aside 流程

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/storage/xcache"

c := xcache.NewRedis(redisClient, "myapp")

// Cache-Aside
val, err := xcache.GetOrLoad(ctx, c, "user:123",
    5*time.Minute,
    func(ctx context.Context) (*User, error) {
        return loadFromDB(ctx, "123")
    },
)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Cache` interface | `Get / Set / Del / GetMulti / SetMulti` |
| `NewRedis(client, namespace) Cache` | Redis 后端 |
| `NewMemory(maxEntries, ttl) Cache` | 内存后端（基于 xlru）|
| `GetOrLoad[T](ctx, c, key, ttl, loader)` | Cache-Aside 助手 |

## 设计要点

- **接口由使用方定义**：业务方可写自家 mock
- **namespace 前缀**：避免多服务同 Redis 撞 key

## 相关

- 模式：[Cache-Aside](../../06-patterns/02-cache-aside.md)
- 包：[xlru](../14-util/05-xlru.md)（Memory 后端基础）
- API：[api.md#pkgstoragexcache](../../03-conventions/01-api.md#pkgstoragexcache)
