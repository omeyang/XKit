---
package: pkg/mq/xpulsar
stability: Stable
coverage: 99.5%
tags: [mq, pulsar, dlq, otel]
---

# pkg/mq/xpulsar

Pulsar 客户端。基于 `pulsar-client-go`，加 DLQ + OTel 链路追踪。

## 用途

- 多租户消息系统
- 延迟消息 / 定时消息
- 多订阅模式（Exclusive/Shared/Failover/KeyShared）

## 使用入口

示例文件中的 `ExampleNewClient`、`ExampleConsumerOptionsBuilder` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/mq/xpulsar` 查看。

[包说明](../../../pkg/mq/xpulsar/doc.go) · [使用示例](../../../tests/unit/pkg/mq/xpulsar/example_test.go)

## 设计要点

- **OTel propagator nil 守卫**：与 xkafka 同样（OTelTracer 零值）

## 相关

- 模式：[Fail-Fast 消费循环](../../06-patterns/05-fail-fast-consume-loop.md)
- 内部：[mqcore](../06-internal/02-mqcore.md)
- API：[api.md#pkgmqxpulsar](../../03-conventions/01-api.md#pkgmqxpulsar)
