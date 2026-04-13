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
// ConsumerOptionsBuilder 默认订阅类型为 Shared（非 Pulsar 原生默认的 Exclusive），
// 因为 Shared 更适合多实例微服务部署。如需 Exclusive 模式请显式调用 WithType。
//
// # 健康检查
//
// Health() 通过创建临时 Reader 验证与 Broker 的连接状态。
// 默认使用 non-persistent://public/default/__health_check__ 作为探测 Topic。
// 在启用 ACL 或非 public/default 命名空间的集群中，
// 使用 WithHealthCheckTopic 配置为客户端有权限访问的 Topic。
//
// Health() 超时后会启动后台清理 goroutine，Close() 会等待其完成，不会泄漏 goroutine。
//
// Stats().Connected 仅表示客户端未调用 Close()，不反映实际网络连接状态。
// 若需检测连接健康，请使用 Health() 方法。
//
// # Topic 解析
//
// 使用 ParseTopic 从 Pulsar URI 解析 Topic 结构体，或使用 NewTopic 从字段构造。
// Topic 类型提供严格的命名规则校验（字母数字开头，仅允许字母/数字/连字符/下划线/点号），
// 支持 persistent:// 和 non-persistent:// 两种 scheme。
// Topic.URI() 返回标准 Pulsar URI，可直接用于 CreateProducer/Subscribe。
//
// # 认证
//
// 使用 WithAuth 配合认证工厂函数配置客户端认证：
//   - Token/TokenFromFile/TokenFromEnv/TokenFromSupplier: JWT Token 认证
//   - TLSCert/TLSCertFromSupplier: mTLS 双向证书认证
//   - OAuth2: OAuth2 客户端凭证认证（类型安全参数）
//   - Athenz: Athenz 认证（map 参数）
//
// 所有工厂函数返回 (AuthMethod, error)，在创建时校验必要参数。
// 字符串参数（token、路径、URL、envKey）前后空白会被去除，
// 避免复制粘贴引入的空白在运行时以认证失败形式暴露。
// 必须先接住 error 再交给 WithAuth：
//
//	auth, err := xpulsar.Token("my-token")
//	if err != nil {
//	    return err
//	}
//	client, err := xpulsar.NewClient("pulsar://localhost:6650", xpulsar.WithAuth(auth))
//
// 如需直接传入 pulsar.Authentication，请使用 WithAuthentication。
//
// # 配置选项
//
// 使用 WithTracer/WithObserver/WithConnectionTimeout/WithAuth 等选项配置客户端行为。
//
// # DLQ 与 xkafka 的差异
//
// 设计决策: xpulsar 将 DLQ 逻辑委托给 Pulsar 原生支持（DLQPolicy + NackBackoffPolicy），
// 而 xkafka 提供完整的 ConsumerWithDLQ 实现。原因是 Pulsar 原生 DLQ 比 Kafka 更成熟，
// 包括自动重试投递、死信 Topic 管理等。如需自定义 DLQ 元数据追踪，
// 请通过 Client() 获取原生客户端实现。
//
// # 与原生 API 的关系
//
// 本包采用"透明包装"设计：
//   - Client() 方法返回底层的 pulsar.Client，可使用所有原生 API
//   - CreateProducer/Subscribe 返回原生的 pulsar.Producer/Consumer
//   - TracingProducer/TracingConsumer 嵌入原生类型，可访问所有原生方法
//   - Schema 管理（Avro/JSON/Protobuf）超出本包范围，通过 Client() 使用原生 Schema API
package xpulsar
