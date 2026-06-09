---
package: pkg/storage/xclickhouse
stability: Beta
coverage: 96.7%
tags: [storage, clickhouse, olap]
---

# pkg/storage/xclickhouse

ClickHouse 客户端封装。基于 `ClickHouse/clickhouse-go/v2`，加批次原子性 + 类型安全。

## 用途

- OLAP 分析、报表
- 批量插入大量行（异步 + 原子）

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/storage/xclickhouse"

c, _ := xclickhouse.Connect(ctx, xclickhouse.Config{
    Addr: []string{"localhost:9000"},
})
defer c.Close()

batch, _ := c.PrepareBatch(ctx, "INSERT INTO events")
for _, ev := range events {
    _ = batch.Append(ev.Time, ev.Type, ev.Payload)
}
_ = batch.Send()
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Client` / `Connect(ctx, cfg)` | 入口 |
| `PrepareBatch` | 批量插入 |
| `QueryRow / Query` | 查询 |

## 设计要点

- **批次原子性**（FG-M fix）：批次失败回滚，避免半写
- **LIMIT 变体**：完整支持 `LIMIT m, n` / `LIMIT n OFFSET m`
- **`COUNT(*)` UInt64 溢出修复**：扫描到 int 容器时 ClickHouse 默认 UInt64 → int 在 64 位无问题但 32 位会溢出

## 相关

- API：[api.md#pkgstoragexclickhouse](../../03-conventions/01-api.md#pkgstoragexclickhouse)
