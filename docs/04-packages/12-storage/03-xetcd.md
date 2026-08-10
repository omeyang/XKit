---
package: pkg/storage/xetcd
stability: Stable
coverage: 94.3%
tags: [storage, etcd, watch, informer]
---

# pkg/storage/xetcd

etcd 客户端封装。**含 Informer**（list + watch 缓存）：参照 K8s client-go 模式，提供 `Store / Indexer` 抽象。

## 用途

- 配置中心 / 服务发现 / 协调
- 大量小键的本地缓存（Informer + Watch 增量同步）

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/storage/xetcd"

// 基础 KV
c, _ := xetcd.New(xetcd.Config{
    Endpoints: []string{"localhost:2379"},
})
v, _ := c.Get(ctx, "/config/foo")

// Informer
informer, _ := xetcd.NewInformer(c, "/configs/",
    xetcd.WithKeyFunc(func(kv *clientv3.KeyValue) string {
        return string(kv.Key)
    }),
)
informer.Start(ctx)
defer informer.Stop()

obj, _ := informer.Store().GetByKey("/configs/foo")
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Client` / `New(cfg)` | KV 封装 |
| `Informer` / `NewInformer(c, prefix, opts...)` | list+watch 缓存 |
| `Store / Indexer` interface | 缓存抽象 |
| `SetForTest / RemoveForTest / ReplaceForTest` | 测试辅助（不要在生产用）|

## 设计要点

- **`Informer.applyEvents`** 增加 `ev.Kv` nil 守卫（etcd 异常可产生 nil Kv 导致 panic）
- **测试 helpers 公开**：xetcd 包外测试可操作 Store 内部状态

## 相关

- 包：[xetcdtest](../13-testkit/01-xetcdtest.md)（嵌入式 etcd）
- API：[api.md#pkgstoragexetcd](../../03-conventions/01-api.md#pkgstoragexetcd)
