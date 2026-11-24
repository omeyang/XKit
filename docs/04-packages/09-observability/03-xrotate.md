---
package: pkg/observability/xrotate
stability: Stable
coverage: 96.9%
tags: [observability, log, rotation]
---

# pkg/observability/xrotate

使用 `NewLumberjack` 创建可写入日志的轮转器。日志按大小轮转，保留策略可限制备份数量和保存时间。

## 使用入口

示例文件中的 `ExampleNewLumberjack`、`ExampleNewLumberjack_withOnError` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/observability/xrotate` 查看。

[包说明](../../../pkg/observability/xrotate/doc.go) · [使用示例](../../../tests/unit/pkg/observability/xrotate/example_test.go)

## 设计要点

- **lumberjack 久经考验**：选它不写自家轮转
- **不耦合日志框架**：返 io.Writer，slog/zap/任何 logger 都能用

## 相关

- API：[api.md#pkgobservabilityxrotate](../../03-conventions/01-api.md#pkgobservabilityxrotate)
