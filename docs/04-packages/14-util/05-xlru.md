---
package: pkg/util/xlru
stability: Stable
coverage: 100.0%
tags: [util, cache, lru, ttl, generic]
related:
  - ../../06-patterns/02-cache-aside.md
---

# pkg/util/xlru

LRU 缓存（泛型 + TTL）。包装 `hashicorp/golang-lru/v2/expirable`，加 Close 控制与 OnEvicted 死锁文档。

## 用途

- 进程内热点数据缓存（如配置、平台信息）
- 带 TTL 自动过期
- 容量上限触发 LRU 淘汰

## 不适用场景

- 跨进程：用 [xcache](../12-storage/01-xcache.md)
- 需要持久化：本包纯内存

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/util/xlru"

c, err := xlru.New[string, *User](
    1000,                  // 容量
    5*time.Minute,         // TTL，0 表示不过期
    xlru.WithOnEvicted(func(k string, v *User) {
        slog.Info("evicted", "key", k)
    }),
)
defer c.Close()

c.Add("k1", &User{...})
if v, ok := c.Get("k1"); ok { /* ... */ }
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Cache[K, V]` | 泛型缓存 |
| `New[K, V](size, ttl, opts...) (*Cache, error)` | 构造 |
| `Add / Get / Peek / Remove / Contains` | 标准 LRU 操作 |
| `Len / Keys / Purge / Close` | 管理操作 |
| `WithOnEvicted(fn)` | 淘汰回调（⚠️ 在 sync.Mutex 内运行，禁止再操作本 cache）|

## 设计要点

- **`closed atomic.Bool`** 守护所有方法
- **Contains 用 Peek 实现**：上游 Contains 不过滤过期 entry
- **TTL=0 透传上游 noEvictionTTL=10 年哨兵**
- **OnEvicted 死锁风险**：在上游 mutex 内执行，文档化约束
- **Close TOCTOU 窗口**：可接受设计取舍，残留 entry 不可见且会被 GC
- **`stopCleanupGoroutine`** 用 reflect+unsafe，有测试 `TestStopCleanupGoroutine_UpstreamStructAssert` 守护上游结构变化
- **Close 后 deleteExpired 可能滞留 TTL/100**：上游不可中断 sleep，文档化

## 相关

- 模式：
  - [Cache-Aside](../../06-patterns/02-cache-aside.md)
- ADR：
  - [0007 util 包不内置可观测性](../../01-decisions/0007-util-packages-no-builtin-observability.md)
- API：[api.md#pkgutilxlru](../../03-conventions/01-api.md#pkgutilxlru)
