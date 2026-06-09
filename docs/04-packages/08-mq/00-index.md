---
title: MQ 包
tags: [moc, packages, mq]
---

# MQ 包

消息队列封装。Kafka 与 Pulsar 两种后端，统一的生产者/消费者抽象 + DLQ + OTel 链路追踪。

## 包列表

| 包 | 用途 | 稳定性 | 覆盖率 |
|---|---|---|---|
| [xkafka](01-xkafka.md) | Kafka 客户端（confluent-kafka-go，DLQ + OTel） | Beta | 88.5% |
| [xpulsar](02-xpulsar.md) | Pulsar 客户端（pulsar-client-go，DLQ + OTel） | Beta | 99.5% |

## 共享底层

- `internal/mqcore`：通用消费循环 `RunConsumeLoop`，**handler panic 不补 recover，刻意 fail-fast**（见 [Fail-Fast 消费循环](../../06-patterns/05-fail-fast-consume-loop.md)）

## 选型指南

| 场景 | 用 |
|---|---|
| 已有 Kafka 集群、需要兼容现有生态 | [xkafka](01-xkafka.md) |
| 多租户、延迟队列、灵活订阅模式 | [xpulsar](02-xpulsar.md) |

## 相关

- [Fail-Fast 消费循环模式](../../06-patterns/05-fail-fast-consume-loop.md)
- [可观测性栈](../../05-concepts/07-observability-stack.md)
- [API 清单 MQ 段](../../03-conventions/01-api.md)
