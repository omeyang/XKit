---
package: pkg/storage/xetcd
stability: Stable
coverage: 94.3%
tags: [storage, etcd, watch, informer]
---

# pkg/storage/xetcd

通过 `NewClient` 获取 etcd 客户端。业务可读写键值、监听变更，也可使用 Informer 维护本地缓存。

## 使用入口

示例文件中的 `ExampleNewClient`、`ExampleClient_WatchWithRetry` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/storage/xetcd` 查看。

[包说明](../../../pkg/storage/xetcd/doc.go) · [使用示例](../../../tests/unit/pkg/storage/xetcd/example_test.go)

## 设计要点

- **`Informer.applyEvents`** 增加 `ev.Kv` nil 守卫（etcd 异常可产生 nil Kv 导致 panic）
- **测试 helpers 公开**：xetcd 包外测试可操作 Store 内部状态

## 相关

- 包：[xetcdtest](../13-testkit/01-xetcdtest.md)（嵌入式 etcd）
- API：[api.md#pkgstoragexetcd](../../03-conventions/01-api.md#pkgstoragexetcd)
