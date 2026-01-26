# MQ 模块审查

> 通用审查方法见 [README.md](README.md)

## 审查范围

```
pkg/mq/
├── xkafka/   # Kafka 客户端封装，基于 confluent-kafka-go
└── xpulsar/  # Pulsar 客户端封装，基于 pulsar-client-go

internal/
└── （相关内部实现）
```

## 模块职责

**设计原则**：
- 透明封装：通过 Producer()/Consumer()/Client() 暴露底层 API
- 增值功能：链路追踪、健康检查、统计信息、DLQ 支持
- 可选增强：TracingProducer/TracingConsumer 自动追踪注入/提取

**包职责**：
- **xkafka**：
  - Producer/Consumer 基础封装
  - TracingProducer/TracingConsumer 自动追踪
  - ConsumerWithDLQ 死信队列和重试机制
  - Offset 管理：at-least-once 语义（enable.auto.offset.store=false）

- **xpulsar**：
  - Client 包装器
  - TracingProducer/TracingConsumer
  - DLQBuilder 死信队列配置
  - NackBackoff 适配 xretry.BackoffPolicy

**包间关系**：
- xkafka/xpulsar 的 DLQ 功能依赖 xretry
- Tracing 功能依赖 xmetrics
