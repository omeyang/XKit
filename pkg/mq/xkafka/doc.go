// Package xkafka 提供 Kafka 客户端封装。
//
// 本包基于 confluent-kafka-go 提供轻量级封装，核心设计原则是：
//   - 透明封装：通过 Producer()/Consumer() 方法暴露底层 API
//   - 增值功能：提供链路追踪、健康检查、统计信息等增强能力
//   - 可选增强：TracingProducer/TracingConsumer 提供自动追踪注入/提取
//   - DLQ 支持：ConsumerWithDLQ 提供死信队列和重试机制
//
// # 基本使用
//
// 使用 NewProducer/NewConsumer 创建客户端，通过 Producer()/Consumer() 方法访问底层 API。
//
// # 链路追踪
//
// 使用 TracingProducer/TracingConsumer 实现自动追踪注入/提取。
//
// # 死信队列
//
// 使用 ConsumerWithDLQ 结合 DLQPolicy 实现消息重试和死信处理。
//
// # 并发安全
//
// 所有 Health() 和 Stats() 方法在 Close() 后安全返回 ErrClosed 或零值，
// 不会在已关闭的底层句柄上执行操作。Close() 可安全地与 Health()/Stats() 并发调用。
package xkafka
