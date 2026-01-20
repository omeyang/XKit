# 模块审查：pkg/mq（消息队列）

> **输出要求**：请用中文输出审查结果，不要直接修改代码。只需分析问题并提出改进建议即可。

## 项目背景

XKit 是深信服内部的 Go 基础库，供其他服务调用。Go 1.25.4，K8s 原生部署。

## 模块概览

```
pkg/mq/
├── xkafka/   # Kafka 客户端封装，基于 confluent-kafka-go
└── xpulsar/  # Pulsar 客户端封装，基于 pulsar-client-go
```

设计理念：
- 透明封装：通过 Producer()/Consumer()/Client() 暴露底层 API
- 增值功能：链路追踪、健康检查、统计信息、DLQ 支持
- 可选增强：TracingProducer/TracingConsumer 自动追踪注入/提取

---

## xkafka：Kafka 客户端封装

**职责**：基于 confluent-kafka-go 提供轻量级封装和增值功能。

**关键文件**：
- `producer.go` - Producer 包装器
- `consumer.go` - Consumer 包装器
- `tracing.go` - TracingProducer/TracingConsumer
- `dlq_consumer.go` - 死信队列支持
- `tracer.go` - 链路追踪接口

**核心组件**：
| 组件 | 说明 |
|------|------|
| Producer | 基础生产者，暴露底层 kafka.Producer |
| Consumer | 基础消费者，暴露底层 kafka.Consumer |
| TracingProducer | 自动注入追踪信息的生产者 |
| TracingConsumer | 自动提取追踪信息的消费者 |
| ConsumerWithDLQ | 支持死信队列和重试机制的消费者 |

**Offset 管理策略**：
```go
config := &kafka.ConfigMap{
    "bootstrap.servers":       "localhost:9092",
    "group.id":                "my-group",
    "enable.auto.commit":      true,        // 自动提交已存储的 offset
    "auto.commit.interval.ms": 5000,        // 5秒提交间隔
    // enable.auto.offset.store 由 xkafka 自动设置为 false
}
```

语义保证（at-least-once）：
- 只有成功处理的消息的 offset 才会被提交
- 处理失败时，消息可被重新消费
- 进程崩溃时，未提交的消息会被重新投递

**DLQ 实现**：
```go
policy := &xkafka.DLQPolicy{
    DLQTopic:      "my-topic-dlq",
    RetryPolicy:   xretry.NewFixedRetry(3),
    BackoffPolicy: xretry.NewExponentialBackoff(),
}
consumer, err := xkafka.NewConsumerWithDLQ(config, topics, policy)
```

DLQ 消息头：
- `x-retry-count`：重试次数
- `x-original-topic`：原始主题
- `x-original-partition`：原始分区
- `x-original-offset`：原始偏移量
- `x-first-fail-time`：首次失败时间
- `x-last-fail-time`：最近失败时间
- `x-failure-reason`：失败原因

**链路追踪**：
```go
tracer := xkafka.NewOTelTracer()
obs, _ := xmetrics.NewOTelObserver()

producer, err := xkafka.NewTracingProducer(config,
    xkafka.WithProducerTracer(tracer),
    xkafka.WithProducerObserver(obs),
)

consumer, err := xkafka.NewTracingConsumer(config, topics,
    xkafka.WithConsumerTracer(tracer),
    xkafka.WithConsumerObserver(obs),
)
```

---

## xpulsar：Pulsar 客户端封装

**职责**：基于 pulsar-client-go 提供封装和增值功能。

**关键文件**：
- `pulsar.go` - Client 包装器
- `tracing.go` - TracingProducer/TracingConsumer
- `dlq.go` - DLQBuilder
- `nack_backoff.go` - xretry 适配器
- `tracer.go` - 链路追踪接口

**核心组件**：
| 组件 | 说明 |
|------|------|
| Client | 客户端包装器，暴露底层 pulsar.Client |
| TracingProducer | 自动注入追踪信息的生产者 |
| TracingConsumer | 自动提取追踪信息的消费者 |
| DLQBuilder | 死信队列配置构建器 |

**基本使用**：
```go
client, err := xpulsar.NewClient(xpulsar.ClientOptions{
    URL: "pulsar://localhost:6650",
})
defer client.Close()

// 访问底层 API
producer, _ := client.Client().CreateProducer(pulsar.ProducerOptions{...})
consumer, _ := client.Client().Subscribe(pulsar.ConsumerOptions{...})
```

**DLQ 配置**（Pulsar 原生支持）：
```go
dlqBuilder := xpulsar.NewDLQBuilder().
    WithMaxDeliveries(5).
    WithDeadLetterTopic("my-topic-dlq").
    WithRetryLetterTopic("my-topic-retry")

opts := xpulsar.NewConsumerOptionsBuilder("my-topic", "my-subscription").
    WithType(pulsar.Shared).
    WithDLQBuilder(dlqBuilder).
    WithNackBackoff(xretry.NewExponentialBackoff()).
    WithRetryEnable(true).
    Build()
```

**NackBackoff 适配**：
```go
// 将 xretry.BackoffPolicy 适配为 pulsar.NackBackoffPolicy
pulsarBackoff := xpulsar.ToPulsarNackBackoff(xretry.NewExponentialBackoff())
```

计数差异处理：
- Pulsar `redeliveryCount` 从 0 开始（0 = 第一次重投递）
- xretry `NextDelay(attempt)` 从 1 开始
- xpulsar 内部将 `redeliveryCount+1` 传给 `NextDelay`

**链路追踪**：
```go
tracer := xpulsar.NewOTelTracer()

// 包装原生生产者/消费者
tracingProducer := xpulsar.WrapProducer(producer, xpulsar.WithProducerTracer(tracer))
tracingConsumer := xpulsar.WrapConsumer(consumer, xpulsar.WithConsumerTracer(tracer))
```

---

## 审查参考

以下是一些值得关注的技术细节，但不限于此：

**Kafka Offset 管理**：
- `enable.auto.offset.store=false` 是否由 xkafka 自动强制设置？
- 消息处理成功后是否调用 `StoreOffsets`？
- `Close()` 时是否同步提交所有已存储的 offset？
- TracingConsumer.Consume 是否在处理成功后自动调用 StoreOffsets？

**Kafka DLQ**：
- 消息经过重试队列时，`x-original-*` 头部是否保留最初的值？
- RetryPolicy 和 BackoffPolicy 与 xretry 的集成是否正确？
- DLQ 生产失败时如何处理？原始消息的 offset 是否被提交？

**Pulsar DLQ**：
- DLQBuilder 是否正确生成 Pulsar DLQ 配置？
- ConsumerOptionsBuilder 是否覆盖常用配置项？
- redeliveryCount 到 attempt 的转换是否正确？

**链路追踪**：
- Tracer.Inject 是否正确将 trace context 注入消息头？
- Tracer.Extract 是否正确从消息头提取 trace context？
- OTelTracer 是否正确使用 propagation.TraceContext？
- 消息头 key 命名是否符合 W3C Trace Context 规范？
- 异步场景下 span 的 parent 关系是否正确？
- 消息头中没有追踪信息时如何处理？是否创建新的根 span？

**资源管理与优雅关闭**：
- Producer/Consumer 的生命周期管理是否清晰？
- Close() 是否先完成所有待提交的 offset？
- 连接断开时的重连逻辑？
- 优雅关闭时进行中的消息如何处理？

**健康检查**：
- Health(ctx) 如何判断客户端健康？
- 健康检查是否有合理的超时？
- 网络抖动时健康检查是否过于敏感？

**Rebalance 处理**：
- rebalance 期间的 offset 提交时机？
- 是否正确处理 partition revoke？
- 是否会导致消息重复消费或丢失？

**并发安全**：
- Consumer 是否支持并发调用？
- StoreOffsets 是否线程安全？
- 是否会有 offset 乱序提交？

---

## 消息流转分析

**Kafka 正常流程**：
```
Producer.Produce()
    ↓
[Kafka Broker]
    ↓
Consumer.ReadMessage()
    ↓
processMessage() → 成功 → StoreOffsets() → auto.commit 提交
```

**Kafka DLQ 流程**：
```
Consumer.ReadMessage()
    ↓
processMessage() → 失败
    ↓
重试（根据 RetryPolicy）→ 仍失败
    ↓
发送到 DLQ Topic（携带 x-* 头部）
    ↓
StoreOffsets()（标记原消息已处理）
```

**Kafka TracingConsumer 流程**：
```
TracingConsumer.Consume(ctx, handler)
    ↓
ReadMessage()
    ↓
Tracer.Extract() → 从消息头提取 trace context
    ↓
创建 child span
    ↓
handler(ctx, msg) → 成功 → StoreOffsets() 自动调用
```

