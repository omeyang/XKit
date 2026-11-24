---
package: pkg/observability/xtrace
stability: Stable
coverage: 92.6%
tags: [observability, trace, w3c, otel]
---

# pkg/observability/xtrace

链路追踪中间件。**W3C Trace Context 标准**：从 HTTP/gRPC 协议头自动提取/注入 traceparent + tracestate。

## 用途

- 跨服务链路传播
- 与 OTel Collector 对接
- 标准化跨语言/平台

## 使用入口

示例文件中的 `ExampleHTTPMiddleware`、`ExampleInjectToOutgoingContext` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/observability/xtrace` 查看。

[包说明](../../../pkg/observability/xtrace/doc.go) · [使用示例](../../../tests/unit/pkg/observability/xtrace/example_test.go)

## 设计要点

- **W3C 严格**：`traceparent` 校验 version/采样位/格式；非法格式跳过不报错
- **`tracestate` 孤立时丢弃**：W3C 规范要求 traceparent 缺失时不能保留 tracestate
- **`OTelTracer` 零值守卫**：`propagator==nil` 时 Inject/Extract 降级为 no-op

## 相关

- 概念：
  - [可观测性栈](../../05-concepts/07-observability-stack.md)
- API：[api.md#pkgobservabilityxtrace](../../03-conventions/01-api.md#pkgobservabilityxtrace)
