---
package: pkg/storage/xmongo
stability: Beta
coverage: 96.0%
tags: [storage, mongo]
---

# pkg/storage/xmongo

MongoDB 客户端封装。基于 `mongo-driver/v2`，加慢查询钩子 + 错误处理。

## 用途

- 文档型业务存储
- 复杂查询（aggregate pipeline）

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/storage/xmongo"

c, err := xmongo.Connect(ctx, xmongo.Config{
    URI: "mongodb://localhost:27017",
    SlowQueryThreshold: 200*time.Millisecond,
    OnSlowQuery: func(filter string, duration time.Duration) {
        slog.Warn("slow query", "filter", filter, "dur", duration)
    },
})
defer c.Close(ctx)

coll := c.Database("app").Collection("users")
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Client` / `Connect(ctx, cfg)` | 入口 |
| `Config` | URI / 慢查询阈值 / 回调 |
| `Close` | 关闭（含 TOCTOU 文档化窗口）|

## 设计要点

- **无序模式 ctx 取消时保留原始 MongoDB 错误**（FG-M fix）：不用 ctx.Err() 替换
- **`maybeSlowQuery`** 对 Filter 做 fmt.Sprintf 字符串快照，防止异步 hook 与调用方竞态读写
- **`maybeSlowQuery` 快路径**：threshold>0 且 duration<threshold 时跳过 Filter Sprintf 避免无效分配
- **Close TOCTOU**：与 xlru 一致的设计取舍，mongo driver post-Disconnect 不 panic

## 相关

- API：[api.md#pkgstoragexmongo](../../03-conventions/01-api.md#pkgstoragexmongo)
