---
package: pkg/lifecycle/xrun
stability: Stable
coverage: 98.2%
tags: [lifecycle, errgroup, signal]
---

# pkg/lifecycle/xrun

进程生命周期管理。`errgroup` 加信号处理，统一管理多 goroutine 的启动与优雅退出。

## 用途

- main 函数里 spawn 多个长期服务（HTTP/gRPC/cron/MQ）统一管理
- SIGTERM / SIGINT 触发优雅 shutdown
- 任一服务失败带动整体退出

## 使用入口

示例文件中的 `ExampleRun`、`ExampleGroup` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/lifecycle/xrun` 查看。

[包说明](../../../pkg/lifecycle/xrun/doc.go) · [使用示例](../../../tests/unit/pkg/lifecycle/xrun/example_test.go)

## 设计要点

- **errgroup 语义**：任一返错触发 ctx cancel
- **信号 → ctx cancel**：统一信号处理

## 相关

- API：[api.md#pkglifecyclexrun](../../03-conventions/01-api.md#pkglifecyclexrun)
