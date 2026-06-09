---
title: 可观测性栈
tags: [concept, observability, slog, otel, w3c]
---

# 可观测性栈

## 三件套

| 信号 | 栈 | 注入方式 |
|---|---|---|
| 日志 | `log/slog`（标准库，唯一） | `WithLogger(logger)` 选项 |
| 链路 | OpenTelemetry + W3C Trace Context | `WithTracerProvider(tp)` |
| 指标 | OpenTelemetry Meter | `WithMeterProvider(mp)` |

## 为什么 slog 唯一

- 标准库，无依赖
- 性能足够（不是 zap 极致优化但实用）
- 接口稳定（Go 1.21+）

不引入 zap/zerolog/logrus —— 见 [ADR-0005](../01-decisions/0005-slog-as-sole-log-backend.md)。

## OTel vendor-neutral

XKit 用 `go.opentelemetry.io/otel` 抽象接口，**不绑定**具体后端（Jaeger / Tempo / Prometheus）。业务方在自己的 main 函数装配 SDK：

```go
import "go.opentelemetry.io/otel/sdk/trace"

tp := trace.NewTracerProvider(...)
defer tp.Shutdown(ctx)

sem, _ := xsemaphore.New(client, "r",
    xsemaphore.WithTracerProvider(tp),
)
```

## W3C Trace Context

`xtrace` 严格遵循 [W3C Trace Context 规范](https://www.w3.org/TR/trace-context/)：

- `traceparent` header（version-traceID-spanID-flags）
- `tracestate` header（vendor-specific 追加）

跨语言/平台标准，可与任何符合规范的系统（Python OTel、Java OTel、Datadog 等）互通。

## util 包不内置可观测性

按 [ADR-0007](../01-decisions/0007-util-packages-no-builtin-observability.md)，`pkg/util/*` 不引入 slog/OTel 依赖，理由：
- util 应该可被任何项目导入而无额外依赖
- 业务方在装配时自己加日志/追踪即可

## 包内 Logger 接口

各 XKit 包定义自己最小化 Logger 接口（按 [接口由使用方定义](04-interface-by-consumer.md)）：

```go
// xsemaphore 内部
type Logger interface {
    Info(msg string, attrs ...slog.Attr)
    Error(msg string, attrs ...slog.Attr)
}
```

业务方用 `xlog.NewWithHandler(slog.NewJSONHandler(...))` 注入。

## typed-nil provider 守卫

OTel provider 可能是 typed-nil（见 [Typed-nil 陷阱](10-typed-nil-trap.md)），各包入口需守卫：

```go
func WithTracerProvider(tp trace.TracerProvider) Option {
    return func(o *options) {
        if tp == nil { return }      // ← 守卫
        // 或 typed-nil 检查
        o.tracerProvider = tp
    }
}
```

## 相关

- ADR：[0005 slog](../01-decisions/0005-slog-as-sole-log-backend.md)、[0007 util 不内置可观测性](../01-decisions/0007-util-packages-no-builtin-observability.md)
- 包：[xlog](../04-packages/09-observability/01-xlog.md)、[xtrace](../04-packages/09-observability/05-xtrace.md)、[xmetrics](../04-packages/09-observability/02-xmetrics.md)
- 概念：[Typed-nil 陷阱](10-typed-nil-trap.md)
