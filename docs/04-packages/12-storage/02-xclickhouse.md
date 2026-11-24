---
package: pkg/storage/xclickhouse
stability: Stable
coverage: 96.7%
tags: [storage, clickhouse, olap]
---

# pkg/storage/xclickhouse

基于 `ClickHouse/clickhouse-go/v2` 的客户端封装，提供分页查询和分批写入。

## 用途

- OLAP 分析、报表
- 按批次写入多行数据

## 使用入口

示例文件中的 `ExampleNew`、`ExampleClickHouse_BatchInsert` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/storage/xclickhouse` 查看。

[包说明](../../../pkg/storage/xclickhouse/doc.go) · [使用示例](../../../tests/unit/pkg/storage/xclickhouse/example_test.go)

## 设计要点

- 追加行失败或发送前发现上下文取消时会中止当前批次。先前发送成功的批次会保留。
- **LIMIT 变体**：完整支持 `LIMIT m, n` / `LIMIT n OFFSET m`
- 分页总数先按 `uint64` 读取，转为 `int64` 前检查是否溢出。

## 相关

- API：[api.md#pkgstoragexclickhouse](../../03-conventions/01-api.md#pkgstoragexclickhouse)
