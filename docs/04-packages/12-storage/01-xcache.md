---
package: pkg/storage/xcache
stability: Stable
coverage: 93.8%
tags: [storage, cache, redis, memory]
related:
  - ../../06-patterns/02-cache-aside.md
---

# pkg/storage/xcache

`NewRedis` 封装 Redis 客户端，`NewMemory` 创建进程内缓存。需要在 Redis 未命中时回源的调用方，通过 `NewLoader` 创建加载器，使用 `Load` 或 `LoadHash` 加载字节数据。

## 使用入口

示例文件中的 `ExampleNewRedis`、`ExampleNewLoader` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/storage/xcache` 查看。

[包说明](../../../pkg/storage/xcache/doc.go) · [使用示例](../../../tests/unit/pkg/storage/xcache/example_test.go)

## 相关

- 模式：[Cache-Aside](../../06-patterns/02-cache-aside.md)
- 包：[xlru](../14-util/05-xlru.md)（Memory 后端基础）
- API：[api.md#pkgstoragexcache](../../03-conventions/01-api.md#pkgstoragexcache)
