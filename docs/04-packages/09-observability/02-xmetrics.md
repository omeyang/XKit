---
package: pkg/observability/xmetrics
stability: Stable
coverage: 100.0%
tags: [observability, metrics, otel]
related:
  - ../../05-concepts/10-typed-nil-trap.md
---

# pkg/observability/xmetrics

统一可观测性接口（OTel 抽象）。提供 Meter 工厂 + span 传播，vendor-neutral。

## 用途

- XKit 各包通过此处获取 Meter / Tracer
- 业务方注入自己的 `metric.MeterProvider` / `trace.TracerProvider`

## 快速上手

```go
import (
    "github.com/omeyang/xkit/pkg/observability/xmetrics"
    "go.opentelemetry.io/otel/metric"
)

// 在创建 XKit 组件时注入
sem, _ := xsemaphore.New(redisClient, "r",
    xsemaphore.WithMeterProvider(meterProvider),
    xsemaphore.WithTracerProvider(tracerProvider),
)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `NewMeter(provider, name) (metric.Meter, error)` | 安全构造（nil 检查）|
| `StartSpan(ctx, tracer, name) (ctx, span)` | 带 nil ctx 回退保留 span 传播 |

## 设计要点

- **`NewMetrics` nil provider 返 `(nil, nil)`**（设计决策）：调用方检查 nil `*Metrics`
- **typed-nil 守卫**：`provider` 是 `typed-nil` 时返回 nil ctx 防御（见 [Typed-nil 陷阱](../../05-concepts/10-typed-nil-trap.md)）
- **`attr.Value` 过滤**：typed-nil attr 在 Add 前过滤

## 相关

- 概念：
  - [可观测性栈](../../05-concepts/07-observability-stack.md)
  - [Typed-nil 陷阱](../../05-concepts/10-typed-nil-trap.md)
- ADR：
  - [0007 util 包不内置可观测性](../../01-decisions/0007-util-packages-no-builtin-observability.md)
- API：[api.md#pkgobservabilityxmetrics](../../03-conventions/01-api.md#pkgobservabilityxmetrics)
