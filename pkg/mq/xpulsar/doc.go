// Package xpulsar 提供 Apache Pulsar 消息队列的封装。
//
// 本包对 github.com/apache/pulsar-client-go/pulsar 进行封装，提供：
//   - 统一的客户端管理：连接管理、健康检查、统计信息
//   - 链路追踪集成：自动注入和提取追踪上下文
//   - 可观测性支持：集成 xmetrics 统一观测接口
//   - DLQ 辅助工具：便捷的死信队列配置构建器
//   - xretry 集成：将 xretry.BackoffPolicy 适配为 Pulsar NackBackoffPolicy
//
// # 基本使用
//
// 使用 NewClient 创建客户端，通过 CreateProducer/Subscribe 创建生产者/消费者。
// 客户端关闭后，CreateProducer/Subscribe/Health 均返回 ErrClosed。
// 使用 Client() 方法可访问底层 pulsar.Client 调用原生 API。
//
// # 链路追踪
//
// 使用 WrapProducer/WrapConsumer 包装原生生产者/消费者，自动注入/提取追踪信息。
// 两者均要求非 nil 的 producer/consumer 参数，否则返回 ErrNilProducer/ErrNilConsumer。
// WrapProducer 的 topic 参数为空时自动从 producer.Topic() 获取。
//
// # DLQ 配置
//
// 使用 DLQBuilder 构建死信队列策略，通过 WithMaxDeliveries/WithDeadLetterTopic 等方法配置。
//
// # 配置选项
//
// 使用 WithTracer/WithObserver/WithConnectionTimeout 等选项配置客户端行为。
//
// # 与原生 API 的关系
//
// 本包采用"透明包装"设计：
//   - Client() 方法返回底层的 pulsar.Client，可使用所有原生 API
//   - CreateProducer/Subscribe 返回原生的 pulsar.Producer/Consumer
//   - TracingProducer/TracingConsumer 嵌入原生类型，可访问所有原生方法
//   - Schema 管理（Avro/JSON/Protobuf）超出本包范围，通过 Client() 使用原生 Schema API
package xpulsar
