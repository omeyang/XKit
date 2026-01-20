# 模块审查：pkg/storage（存储封装）

> **输出要求**：请用中文输出审查结果，不要直接修改代码。只需分析问题并提出改进建议即可。

## 项目背景

XKit 是深信服内部的 Go 基础库，供其他服务调用。Go 1.25.4，K8s 原生部署。

## 模块概览

```
pkg/storage/
├── xcache/       # 缓存统一封装，Redis + 内存（ristretto）
├── xmongo/       # MongoDB 包装器
├── xclickhouse/  # ClickHouse 包装器
└── xetcd/        # etcd 客户端封装
```

设计理念：
- 工厂方法创建（New / NewRedis / NewMemory）
- 底层客户端直接暴露（Client() / Conn()）
- 增值功能层叠加（健康检查、统计、慢查询检测、分页等）

关键点：通过 Client()/Conn() 直接执行的操作不计入统计和慢查询检测。

---

## xcache：缓存统一封装

**职责**：提供统一的缓存工厂和增值功能（Redis、内存缓存）。

**关键文件**：
- `redis.go` - Redis 缓存包装器
- `memory.go` - 内存缓存包装器（基于 ristretto）
- `loader.go` - Cache-Aside 模式加载器
- `options.go` - 配置选项

**核心组件**：
| 组件 | 说明 | 底层 |
|------|------|------|
| Redis | Redis 缓存包装器 | go-redis UniversalClient |
| Memory | 内存缓存包装器 | dgraph-io/ristretto |
| Loader | Cache-Aside 加载器 | singleflight + 分布式锁 |

**Loader 防击穿机制**：
```go
loader := xcache.NewLoader(redisCache,
    xcache.WithSingleflight(true),      // 单机防击穿
    xcache.WithDistributedLock(true),   // 分布式防击穿
)
result, err := loader.Load(ctx, "key", loadFunc, time.Hour)
```

**Context 处理**：
- singleflight 合并并发请求时，使用独立 context（保留 trace/values，独立超时）
- 默认超时 30 秒（可通过 WithLoadTimeout 配置）
- 即使 caller 的 ctx 被 cancel，底层加载仍会完成

**分布式锁 Key 格式**：
```
lock:{prefix}{key}
```
- "lock:" 由 Redis.Lock() 自动添加
- {prefix} 默认 "loader:"，可通过 WithDistributedLockKeyPrefix 配置

**与 xdlock 集成**：
```go
loader := xcache.NewLoader(redisCache,
    xcache.WithDistributedLock(true),
    xcache.WithExternalLock(func(ctx context.Context, key string, ttl time.Duration) (xcache.Unlocker, error) {
        locker := factory.NewMutex(key, xdlock.WithExpiry(ttl))
        if err := locker.Lock(ctx); err != nil {
            return nil, err
        }
        return locker.Unlock, nil
    }),
)
```

**内置锁 vs 外部锁**：
| 特性 | 内置锁 | xdlock (Redis) | xdlock (etcd) |
|------|--------|----------------|---------------|
| 复杂度 | 简单 | 中等 | 中等 |
| 多节点 | ❌ | ✅ Redlock | ✅ etcd 集群 |
| 续期 | ❌ | ✅ Extend() | ✅ 自动续期 |
| 适用场景 | 简单缓存防击穿 | 高可用分布式锁 | 强一致性场景 |

---

## xmongo：MongoDB 包装器

**职责**：MongoDB 客户端包装器，提供增值功能。

**关键文件**：
- `mongo.go` - 核心包装器
- `page.go` - 分页查询
- `bulk.go` - 批量写入
- `observer.go` - 可观测性集成

**核心功能**：
- `Client()`：暴露底层 mongo.Client
- `Health()`：健康检查（Ping）
- `Stats()`：统计信息
- `FindPage()`：分页查询
- `BulkWrite()`：批量写入

**分页查询**：
```go
result, err := m.FindPage(ctx, coll, bson.M{"status": "active"}, xmongo.PageOptions{
    Page:     1,
    PageSize: 10,
    Sort:     bson.D{{"created_at", -1}},
})
fmt.Printf("总数: %d, 总页数: %d\n", result.Total, result.TotalPages)
```

**批量写入**：
```go
result, err := m.BulkWrite(ctx, coll, docs, xmongo.BulkOptions{
    BatchSize: 100,
    Ordered:   false,
})
```

**慢查询检测**：
```go
m, _ := xmongo.New(client,
    xmongo.WithObserver(obs),
    xmongo.WithSlowQueryThreshold(100*time.Millisecond),
    xmongo.WithSlowQueryHook(func(ctx context.Context, info xmongo.SlowQueryInfo) {
        log.Printf("慢查询: %s.%s %s 耗时 %v",
            info.Database, info.Collection, info.Operation, info.Duration)
    }),
)
```

**Context 取消处理**：
- 每个批次前检查 ctx.Err()
- 无序模式下单批次失败后检查 context
- 取消时返回已完成的插入数量和错误

---

## xclickhouse：ClickHouse 包装器

**职责**：ClickHouse 客户端包装器，提供增值功能。

**关键文件**：
- `wrapper.go` - 核心包装器
- `page.go` - 分页查询
- `batch.go` - 批量插入
- `observer.go` - 可观测性集成

**核心功能**：
- `Conn()`：暴露底层 driver.Conn
- `Health()`：健康检查（Ping）
- `Stats()`：统计信息
- `QueryPage()`：分页查询（统计为 2 次查询）
- `BatchInsert()`：批量插入

**分页查询**：
```go
result, err := ch.QueryPage(ctx, "SELECT id, name FROM users WHERE status = 1", xclickhouse.PageOptions{
    Page:     1,
    PageSize: 10,
})
```

**批量插入**：
```go
result, err := ch.BatchInsert(ctx, "users", users, xclickhouse.BatchOptions{
    BatchSize: 100,
})
```

**统计说明**：
- QueryPage 内部执行 2 次查询（COUNT + 分页数据），计数 +2
- 通过 Conn() 直接执行的操作不计入统计

---

## xetcd：etcd 客户端封装

**职责**：提供简化的 etcd KV 操作和 Watch 功能。

**关键文件**：
- `client.go` - 核心客户端
- `options.go` - 配置选项

**核心功能**：
- 简化的 KV 操作（Get/Put/Delete/List）
- Watch 功能，监听键值变化
- 与 xdlock 分布式锁的配置复用

**快速开始**：
```go
config := &xetcd.Config{
    Endpoints: []string{"localhost:2379"},
}
client, err := xetcd.NewClient(config)
defer client.Close()

// KV 操作
err = client.Put(ctx, "key", []byte("value"))
value, err := client.Get(ctx, "key")
```

**Watch**：
```go
events, _ := client.Watch(ctx, "/prefix/", xetcd.WithPrefix())
for event := range events {
    fmt.Printf("%s: %s\n", event.Key, event.Value)
}
```

**与 xdlock 集成**：
```go
config := &xetcd.Config{Endpoints: []string{"localhost:2379"}}

// 用于 KV 操作
kvClient, _ := xetcd.NewClient(config)

// 用于分布式锁（xdlock 内部引用 xetcd）
lockFactory, _, _ := xdlock.NewEtcdFactoryFromConfig(config, nil)
```

---

## 审查参考

以下是一些值得关注的技术细节，但不限于此：

**xcache Loader**：
- singleflight 是否使用 `golang.org/x/sync/singleflight`？
- 独立 context 的创建是否正确保留 trace/values？
- 分布式锁 key 格式是否正确？
- WithExternalLock 与内置锁的互斥逻辑？

**xcache 内存缓存**：
- ristretto 的异步写入特性是否被正确处理？
- Wait() 方法是否暴露给用户？
- MaxCost 配置的默认值和边界？

**分页查询**：
- 分页参数边界值处理（Page=0, PageSize=0）？
- 总数查询和数据查询的原子性（两次查询）？
- xclickhouse QueryPage 统计计数 +2 是否正确？

**批量操作**：
- 每个批次前是否检查 ctx.Err()？
- context 取消时返回值是否正确反映已完成数量？
- 无序模式部分失败的处理逻辑？
- cursor 在批量操作中是否正确关闭？

**慢查询检测**：
- SlowQueryHook 是否会阻塞查询？
- SlowQueryInfo 包含哪些信息？
- 阈值设置的合理性？

**资源生命周期**：
- Close() 是否正确关闭所有资源？
- Loader 是否需要 Close()？singleflight.Group 的生命周期？
- Watch channel 的关闭时机？

**分布式锁**：
- 内置锁是否有 owner 检查（防止删除他人的锁）？
- 锁过期后 unlock 的行为？
- Lua 脚本原子操作的实现？

**xetcd 集成**：
- Config 类型与 xdlock.EtcdConfig 的兼容性？
- Watch 断线重连的处理？
- etcd 集群故障时的行为？

