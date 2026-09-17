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

## 快速上手

```go
import "github.com/omeyang/xkit/pkg/mq/xkafka"

// 生产
p, _ := xkafka.NewProducer(xkafka.ProducerConfig{
    Brokers: []string{"localhost:9092"},
})
defer p.Close()
_ = p.Produce(ctx, "topic-1", []byte(payload))

// 消费（fail-fast：handler panic 不补 recover）
c, _ := xkafka.NewConsumer(xkafka.ConsumerConfig{
    Brokers: []string{"localhost:9092"},
    Topic:   "topic-1",
    Group:   "my-group",
    DLQ:     xkafka.DLQConfig{Topic: "topic-1-dlq", MaxRetries: 3},
})
_ = c.ConsumeLoop(ctx, func(ctx context.Context, msg *xkafka.Message) error {
    return process(msg)
})
```

## 关键类型与函数

| 名称 | 说明 |
|---|---|
| `Producer / NewProducer(cfg)` | 生产 |
| `Consumer / NewConsumer(cfg)` | 消费 |
| `ConsumeLoop(ctx, handler) error` | 阻塞消费（fail-fast，**handler panic 暴露不补 recover**）|
| `Message` | 消息封装 |
| `TracingProducer / TracingConsumer` | OTel 装饰器 |
| `DLQConfig` | DLQ topic + MaxRetries |

## 设计要点

- **`ctx` 取消前置检查**（FG-M fix）：`sendToDLQInternal/redeliverMessage` 在 Produce 前检查 `ctx.Err()`，防止 ctx 已取消时仍入队但不提交 offset
- **`setHeader` 重复 Header 修复**（FG-M fix）：删除所有同名 header 后追加新值，避免重复 traceparent
- **`TracingProducer/Consumer` 生命周期竞态**（对抗审查 fix）：避免 close 后还在 publish
- **Fail-Fast**：`ConsumeLoop` handler panic 不补 recover（共享设计 `internal/mqcore`）

## 相关

- 模式：[Fail-Fast 消费循环](../../06-patterns/05-fail-fast-consume-loop.md)
- 内部：[mqcore](../06-internal/02-mqcore.md)（共享底层）
- API：[api.md#pkgmqxkafka](../../03-conventions/01-api.md#pkgmqxkafka)
