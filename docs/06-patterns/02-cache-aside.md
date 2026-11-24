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

## Redis 缓存回源

用 `xcache.NewLoader` 创建加载器。调用 `loader.Load(ctx, key, loadFn, ttl)` 时，Redis 命中则返回缓存的字节，未命中则调用 `loadFn` 并缓存结果。同一加载器会合并同一 key 的并发回源。批量键的过期时间可通过 `WithTTLJitter` 分散。

[Redis 加载与 TTL 抖动示例](../../tests/unit/pkg/storage/xcache/example_test.go) · [Singleflight](07-singleflight.md)

## 防穿透

恶意请求不存在的 key 导致每次都打 DB：

1. **空值缓存**：DB 返回 nil 时也缓存（短 TTL）
2. **布隆过滤器**：前置过滤"肯定不存在"的 key

## 相关

- 包：[xcache](../04-packages/12-storage/01-xcache.md)、[xlru](../04-packages/14-util/05-xlru.md)、[xauth](../04-packages/01-business/01-xauth.md)
- 模式：[Singleflight](07-singleflight.md)
