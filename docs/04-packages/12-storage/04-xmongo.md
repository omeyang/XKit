---
package: pkg/storage/xmongo
stability: Stable
coverage: 96.0%
tags: [storage, mongo]
---

# pkg/storage/xmongo

MongoDB 客户端封装。基于 `mongo-driver/v2`，加慢查询钩子 + 错误处理。

## 用途

- 文档型业务存储
- 复杂查询（aggregate pipeline）

## 使用入口

示例文件中的 `ExampleNew`、`ExampleMongo_FindPage` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/storage/xmongo` 查看。

[包说明](../../../pkg/storage/xmongo/doc.go) · [使用示例](../../../tests/unit/pkg/storage/xmongo/example_test.go)

## 设计要点

- **无序模式 ctx 取消时保留原始 MongoDB 错误**：不用 ctx.Err() 替换
- **`maybeSlowQuery`** 对 Filter 做 fmt.Sprintf 字符串快照，防止异步 hook 与调用方竞态读写
- **`maybeSlowQuery` 快路径**：threshold>0 且 duration<threshold 时跳过 Filter Sprintf 避免无效分配
- **Close TOCTOU**：与 xlru 一致的设计取舍，mongo driver post-Disconnect 不 panic

## 相关

- API：[api.md#pkgstoragexmongo](../../03-conventions/01-api.md#pkgstoragexmongo)
