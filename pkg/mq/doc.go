// Package mq 提供消息队列相关的子包。
//
// 子包列表：
//   - xkafka: Kafka 客户端封装，支持生产者、消费者、DLQ
//   - xpulsar: Pulsar 客户端封装
//
// 内部包：
//   - internal/mqcore: 共享的追踪和可观测性代码
//
// 设计原则：
//   - 提供统一的生产者/消费者接口
//   - 内置追踪上下文传播（W3C Trace Context）
//   - 支持死信队列（DLQ）和重试策略
//   - 内置可观测性（指标、日志、追踪）
package mq
