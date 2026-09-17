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

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/observability/xtrace"

// HTTP 服务端
mux.Use(xtrace.HTTPMiddleware(tracerProvider))

// gRPC 服务端
server := grpc.NewServer(
    grpc.UnaryInterceptor(xtrace.UnaryServerInterceptor(tracerProvider)),
)

// 跨服务调用注入
req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
xtrace.InjectToRequest(ctx, req)
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `OTelTracer` | OTel SDK 封装（零值有 propagator nil 守卫）|
| `HTTPMiddleware / UnaryServerInterceptor` | 服务端 |
| `InjectToRequest / InjectToOutgoingContext` | 客户端注入 |
| `ExtractFromHTTPHeader / ExtractFromMetadata` | 显式提取 |

## 设计要点

- **W3C 严格**：`traceparent` 校验 version/采样位/格式；非法格式跳过不报错
- **`tracestate` 孤立时丢弃**（FG-M fix）：W3C 规范要求 traceparent 缺失时不能保留 tracestate
- **`OTelTracer` 零值守卫**：`propagator==nil` 时 Inject/Extract 降级为 no-op（FG-H fix）

## 相关

- 概念：
  - [可观测性栈](../../05-concepts/07-observability-stack.md)
- API：[api.md#pkgobservabilityxtrace](../../03-conventions/01-api.md#pkgobservabilityxtrace)
