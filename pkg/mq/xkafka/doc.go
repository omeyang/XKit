// Package xkafka 提供 Kafka 客户端封装。
//
// 本包基于 confluent-kafka-go 提供轻量级封装，核心设计原则是：
//   - 透明封装：通过 Producer()/Consumer() 方法暴露底层 API，不限制高级特性
//   - 增值功能：提供链路追踪、健康检查、统计信息等增强能力
//   - 可选增强：TracingProducer/TracingConsumer 提供自动追踪注入/提取
//   - DLQ 支持：ConsumerWithDLQ 提供死信队列和重试机制
//
// # 三层架构
//
// 基础层（producerWrapper/consumerWrapper）透明封装 confluent-kafka-go；
// 装饰层（TracingProducer/TracingConsumer）增加 trace inject/extract + metrics span；
// 扩展层（dlqConsumer）增加 DLQ/Retry 流程 + closeMu 并发保护。
//
// # 基本使用
//
// 使用 NewProducer/NewConsumer 创建客户端，通过 Producer()/Consumer() 方法访问底层 API。
//
// # 链路追踪
//
// 使用 TracingProducer/TracingConsumer 实现自动追踪注入/提取。
// [NewTracingProducer] 和 [NewTracingConsumer] 返回具体类型（*TracingProducer/*TracingConsumer）
// 而非接口，因为 Tracing 类型通过嵌入添加了额外方法（如带追踪的 Produce/Consume），
// 返回接口会丢失这些方法。
//
// # 死信队列
//
// 使用 ConsumerWithDLQ 结合 DLQPolicy 实现消息重试和死信处理。
// 推荐使用 [DefaultDLQTopic] 生成 DLQ Topic 名称以保持命名一致性。
//
// DLQ 遵循 at-least-once 语义：消息投递到 DLQ 成功后才存储 offset。
// 如果 StoreMessage 在 DLQ 投递成功后失败，消费者重启会重新消费该消息，
// 可能导致 DLQ 中出现重复。建议使用方在 DLQ 消费端基于
// x-original-topic + x-original-partition + x-original-offset 做幂等处理。
//
// 失败原因写入 x-failure-reason Header 时默认截断至 1024 字符，
// 防止敏感信息泄露。可通过 [DLQPolicy].FailureReasonFormatter 自定义格式化。
//
// # 统计信息
//
// [ProducerStats] 和 [ConsumerStats] 中的 MessagesProduced/MessagesConsumed 等计数
// 仅在使用 [TracingProducer]、[TracingConsumer] 或 [ConsumerWithDLQ] 时被递增。
// 直接通过 [NewProducer]/[NewConsumer] 创建的基础实例，Stats() 返回零值计数。
//
// [ConsumerStats].Lag 通过对每个分区执行 Committed + QueryWatermarkOffsets RPC 计算，
// 在分区数较多时可能持有锁数秒。不建议在高频路径（如秒级健康检查）中调用 Stats()。
// Lag RPC 超时复用 [WithConsumerHealthTimeout] 配置，设置较短值可能导致 lag 返回 0。
//
// # Offset 提交模型
//
// 本包强制设置 enable.auto.offset.store=false 和 enable.auto.commit=true
// 以确保 at-least-once 语义。Offset 仅在成功处理后通过 StoreMessage 存储，
// 由 auto-commit 机制定期提交。Close() 时会执行一次显式 Commit。
// 用户配置的 enable.auto.commit 或 enable.auto.offset.store 值会被覆盖。
//
// 设计决策: SubscribeTopics 未注册 rebalance 回调，分区撤销时 offset 提交
// 依赖 auto-commit 窗口（默认 5s）。扩缩容时最近窗口内已处理消息可能被重复消费。
// 如需更精确的 rebalance 处理，建议通过 Consumer() 获取底层 API 自行注册回调。
//
// # 并发安全
//
// 所有 Health() 和 Stats() 方法在 Close() 后安全返回 ErrClosed 或零值，
// 不会在已关闭的底层句柄上执行操作。Close() 可安全地与 Health()/Stats() 并发调用。
// TracingConsumer.Consume 和 dlqConsumer.processMessage 通过 closeMu RWMutex
// 与 Close 协调，确保消息处理完成后才关闭资源。
package xkafka
