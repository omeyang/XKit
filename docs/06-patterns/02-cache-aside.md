---
title: Cache-Aside 模式
tags: [pattern, cache]
---

# Cache-Aside

## 问题

业务读热点数据，每次打数据库压力大。

## 解法

```
Client → Cache.Get(k)
            ├── hit:  return val
            └── miss: val := DB.Get(k); Cache.Set(k, val, ttl); return val
```

## XKit 助手

[xcache.GetOrLoad](../04-packages/12-storage/01-xcache.md) 封装这个流程：

```go
val, err := xcache.GetOrLoad(ctx, c, "user:123", 5*time.Minute,
    func(ctx context.Context) (*User, error) {
        return loadFromDB(ctx, "123")
    },
)
```

## 进阶：防缓存击穿（同 key 并发回源）

热点 key 失效瞬间多个 goroutine 同时回源，配合 [Singleflight](07-singleflight.md)：

```go
import "golang.org/x/sync/singleflight"

var g singleflight.Group

val, _, _ := g.Do(key, func() (interface{}, error) {
    return xcache.GetOrLoad(ctx, c, key, ttl, loader)
})
```

XKit 中 [xauth](../04-packages/01-business/01-xauth.md) 用此模式。

## 防雪崩

大量 key 同时过期会导致全量回源。两种缓解：

1. **随机 TTL**：`ttl := base + rand.Duration(0, jitter)`
2. **双层缓存**：Memory（短 TTL）+ Redis（长 TTL），见 [xauth](../04-packages/01-business/01-xauth.md)

## 防穿透

恶意请求不存在的 key 导致每次都打 DB：

1. **空值缓存**：DB 返回 nil 时也缓存（短 TTL）
2. **布隆过滤器**：前置过滤"肯定不存在"的 key

## 相关

- 包：[xcache](../04-packages/12-storage/01-xcache.md)、[xlru](../04-packages/14-util/05-xlru.md)、[xauth](../04-packages/01-business/01-xauth.md)
- 模式：[Singleflight](07-singleflight.md)
