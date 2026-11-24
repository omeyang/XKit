---
package: pkg/mq/xkafka
stability: Stable
coverage: 88.5%
tags: [mq, kafka, dlq, otel]
related:
  - ../../06-patterns/05-fail-fast-consume-loop.md
---

# pkg/mq/xkafka

Kafka 客户端。基于 `confluent-kafka-go`，加 DLQ + OTel 链路追踪。

## 用途

- 生产 / 消费 Kafka 消息
- DLQ 自动投递无法处理的消息
- 链路追踪自动传播

## 使用入口

示例文件中的 `ExampleDLQPolicy_Validate`、`ExampleConsumerStats` 展示实际调用方式。完整签名可在仓库根目录用 `go doc ./pkg/mq/xkafka` 查看。

[包说明](../../../pkg/mq/xkafka/doc.go) · [使用示例](../../../tests/unit/pkg/mq/xkafka/example_test.go)

## 设计要点

- **`ctx` 取消前置检查**：`sendToDLQInternal/redeliverMessage` 在 Produce 前检查 `ctx.Err()`，防止 ctx 已取消时仍入队但不提交 offset
- **`setHeader` 重复 Header 修复**：删除所有同名 header 后追加新值，避免重复 traceparent
- **`TracingProducer/Consumer` 生命周期竞态**：避免 close 后还在 publish
- **Fail-Fast**：`ConsumeLoop` handler panic 不补 recover（共享设计 `internal/mqcore`）

## 相关

- 模式：[Fail-Fast 消费循环](../../06-patterns/05-fail-fast-consume-loop.md)
- 内部：[mqcore](../06-internal/02-mqcore.md)（共享底层）
- API：[api.md#pkgmqxkafka](../../03-conventions/01-api.md#pkgmqxkafka)
