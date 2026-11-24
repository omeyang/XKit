---
package: pkg/debug/xdbg
stability: Stable
coverage: 90.5%
tags: [debug, unix-socket]
---

# pkg/debug/xdbg

运行时调试服务。通过 **Unix Socket**（不开 HTTP 端口）暴露 goroutine / heap / stack / metrics 等内省接口。

## 用途

- 生产环境排障：无需重启即可访问进程内部状态
- 安全：Unix Socket 仅本机可访问，不外露
- 配套 CLI：`cmd/xdbgctl`

## 使用入口

示例文件中的 `Example`、`Example_withCustomOptions` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/debug/xdbg` 查看。

[包说明](../../../pkg/debug/xdbg/doc.go) · [使用示例](../../../tests/unit/pkg/debug/xdbg/example_test.go)

## 设计要点

- **Unix Socket 默认权限 0600**：仅 owner 可访问
- **`safeCallback` 嵌套 recover**：与 xpool 同样防 logger panic 二次崩溃
- **配套 `cmd/xdbgctl`** CLI 简化访问

## 相关

- 包：[xrun](../07-lifecycle/02-xrun.md)（main 中通常和 xrun 一起启动）
- API：[api.md#pkgdebugxdbg](../../03-conventions/01-api.md#pkgdebugxdbg)
