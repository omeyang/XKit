---
package: pkg/observability/xlog
stability: Stable
coverage: 96.8%
tags: [observability, log, slog]
related:
  - ../../01-decisions/0005-slog-as-sole-log-backend.md
---

# pkg/observability/xlog

结构化日志。**唯一日志后端是 `log/slog`**。提供薄薄的 Logger 接口 + 工厂构造。

## 用途

- 所有 XKit 包通过此接口注入日志
- 业务方可注入自定义 `*slog.Logger`（如带 OTel handler）

## 不适用场景

- 不引入 zap/zerolog/logrus（见 ADR-0005）

## 使用入口

示例文件中的 `Example`、`Example_withAttrs` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/observability/xlog` 查看。

[包说明](../../../pkg/observability/xlog/doc.go) · [使用示例](../../../tests/unit/pkg/observability/xlog/example_test.go)

## 设计要点

- **接口由使用方定义**（ADR-0002）：XKit 各包定义自己最小化的 Logger 接口，xlog.Logger 是个统一可选实现
- **slog 唯一**（ADR-0005）：不引入其他日志栈

## 相关

- ADR：
  - [0005 slog 作为唯一日志后端](../../01-decisions/0005-slog-as-sole-log-backend.md)
- API：[api.md#pkgobservabilityxlog](../../03-conventions/01-api.md#pkgobservabilityxlog)
