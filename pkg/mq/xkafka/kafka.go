package xkafka

import (
	"context"
	"fmt"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// =============================================================================
// Producer 接口
// =============================================================================

// Producer 定义 Kafka 生产者接口。
// 通过 Producer() 方法暴露底层 *kafka.Producer，可使用所有原生 API。
type Producer interface {
	// Producer 返回底层的 *kafka.Producer。
	// 用于执行所有 Kafka 生产者操作。
	Producer() *kafka.Producer

	// Health 执行健康检查。
	// 通过获取 Broker 元数据验证连接状态。
	Health(ctx context.Context) error

	// Stats 返回生产者统计信息。
	Stats() ProducerStats

	// Close 优雅关闭生产者。
	// 会等待所有消息发送完成（受 FlushTimeout 限制）。
	Close() error
}

// ProducerStats 包含 Kafka Producer 的统计信息。
type ProducerStats struct {
	// MessagesProduced 已发送的消息数量。
	MessagesProduced int64
	// BytesProduced 已发送的字节数。
	BytesProduced int64
	// Errors 发送失败的消息数量。
	Errors int64
	// QueueLength 当前队列中等待发送的消息数量。
	QueueLength int
}

// =============================================================================
// Consumer 接口
// =============================================================================

// Consumer 定义 Kafka 消费者接口。
// 通过 Consumer() 方法暴露底层 *kafka.Consumer，可使用所有原生 API。
type Consumer interface {
	// Consumer 返回底层的 *kafka.Consumer。
	// 用于执行所有 Kafka 消费者操作。
	Consumer() *kafka.Consumer

	// Health 执行健康检查。
	// 检查消费者是否已分配分区。
	Health(ctx context.Context) error

	// Stats 返回消费者统计信息。
	Stats() ConsumerStats

	// Close 优雅关闭消费者。
	// 会提交当前偏移量并取消订阅。
	Close() error
}

// ConsumerStats 包含 Kafka Consumer 的统计信息。
type ConsumerStats struct {
	// MessagesConsumed 已消费的消息数量。
	MessagesConsumed int64
	// BytesConsumed 已消费的字节数。
	BytesConsumed int64
	// Errors 未能成功处理的错误次数。
	// 注意：对于 ConsumerWithDLQ，成功重试或发送到 DLQ 的消息不计入此统计。
	// 要追踪 handler 失败次数，请使用 DLQStats。
	Errors int64
	// Lag 消费延迟（与最新偏移量的差值）。
	Lag int64
}

// =============================================================================
// 工厂函数
// =============================================================================

// NewProducer 创建 Kafka 生产者实例。
// config 必须包含 "bootstrap.servers" 配置项。
func NewProducer(config *kafka.ConfigMap, opts ...ProducerOption) (Producer, error) {
	wrapper, err := newProducerWrapper(config, opts...)
	if err != nil {
		return nil, err
	}
	return wrapper, nil
}

func newProducerWrapper(config *kafka.ConfigMap, opts ...ProducerOption) (*producerWrapper, error) {
	if config == nil {
		return nil, ErrNilConfig
	}

	options := defaultProducerOptions()
	for _, opt := range opts {
		opt(options)
	}

	producer, err := kafka.NewProducer(config)
	if err != nil {
		return nil, err
	}

	return &producerWrapper{
		producer: producer,
		options:  options,
	}, nil
}

// NewConsumer 创建 Kafka 消费者实例。
// config 必须包含 "bootstrap.servers" 和 "group.id" 配置项。
// topics 是要订阅的主题列表，不能为空。
func NewConsumer(config *kafka.ConfigMap, topics []string, opts ...ConsumerOption) (Consumer, error) {
	wrapper, err := newConsumerWrapper(config, topics, opts...)
	if err != nil {
		return nil, err
	}
	return wrapper, nil
}

func newConsumerWrapper(config *kafka.ConfigMap, topics []string, opts ...ConsumerOption) (*consumerWrapper, error) {
	if config == nil {
		return nil, ErrNilConfig
	}
	if len(topics) == 0 {
		return nil, ErrEmptyTopics
	}

	options := defaultConsumerOptions()
	for _, opt := range opts {
		opt(options)
	}

	// 强制设置 enable.auto.offset.store=false 以确保 at-least-once 语义
	// 这确保 offset 只在显式调用 StoreOffsets 后才会被存储
	if err := config.SetKey("enable.auto.offset.store", false); err != nil {
		return nil, fmt.Errorf("failed to set enable.auto.offset.store: %w", err)
	}

	consumer, err := kafka.NewConsumer(config)
	if err != nil {
		return nil, err
	}

	if err := consumer.SubscribeTopics(topics, nil); err != nil {
		closeErr := consumer.Close()
		if closeErr != nil {
			return nil, err // 返回原始错误
		}
		return nil, err
	}

	return &consumerWrapper{
		consumer: consumer,
		options:  options,
	}, nil
}

// =============================================================================
// 选项
// =============================================================================

// producerOptions 包含 Kafka Producer 的配置选项。
type producerOptions struct {
	Tracer        Tracer
	Observer      xmetrics.Observer
	FlushTimeout  time.Duration
	HealthTimeout time.Duration
}

func defaultProducerOptions() *producerOptions {
	return &producerOptions{
		Tracer:        NoopTracer{},
		Observer:      xmetrics.NoopObserver{},
		FlushTimeout:  10 * time.Second,
		HealthTimeout: 5 * time.Second,
	}
}

// ProducerOption 定义 Kafka Producer 的配置选项函数类型。
type ProducerOption func(*producerOptions)

// WithProducerTracer 设置链路追踪器。
func WithProducerTracer(tracer Tracer) ProducerOption {
	return func(o *producerOptions) {
		if tracer != nil {
			o.Tracer = tracer
		}
	}
}

// WithProducerObserver 设置统一观测接口。
func WithProducerObserver(observer xmetrics.Observer) ProducerOption {
	return func(o *producerOptions) {
		if observer != nil {
			o.Observer = observer
		}
	}
}

// WithProducerFlushTimeout 设置关闭时的刷新超时时间。
func WithProducerFlushTimeout(d time.Duration) ProducerOption {
	return func(o *producerOptions) {
		if d > 0 {
			o.FlushTimeout = d
		}
	}
}

// WithProducerHealthTimeout 设置健康检查超时时间。
func WithProducerHealthTimeout(d time.Duration) ProducerOption {
	return func(o *producerOptions) {
		if d > 0 {
			o.HealthTimeout = d
		}
	}
}

// consumerOptions 包含 Kafka Consumer 的配置选项。
type consumerOptions struct {
	Tracer        Tracer
	Observer      xmetrics.Observer
	PollTimeout   time.Duration
	HealthTimeout time.Duration
}

func defaultConsumerOptions() *consumerOptions {
	return &consumerOptions{
		Tracer:        NoopTracer{},
		Observer:      xmetrics.NoopObserver{},
		PollTimeout:   100 * time.Millisecond,
		HealthTimeout: 5 * time.Second,
	}
}

// ConsumerOption 定义 Kafka Consumer 的配置选项函数类型。
type ConsumerOption func(*consumerOptions)

// WithConsumerTracer 设置链路追踪器。
func WithConsumerTracer(tracer Tracer) ConsumerOption {
	return func(o *consumerOptions) {
		if tracer != nil {
			o.Tracer = tracer
		}
	}
}

// WithConsumerObserver 设置统一观测接口。
func WithConsumerObserver(observer xmetrics.Observer) ConsumerOption {
	return func(o *consumerOptions) {
		if observer != nil {
			o.Observer = observer
		}
	}
}

// WithConsumerPollTimeout 设置轮询超时时间。
func WithConsumerPollTimeout(d time.Duration) ConsumerOption {
	return func(o *consumerOptions) {
		if d > 0 {
			o.PollTimeout = d
		}
	}
}

// WithConsumerHealthTimeout 设置健康检查超时时间。
func WithConsumerHealthTimeout(d time.Duration) ConsumerOption {
	return func(o *consumerOptions) {
		if d > 0 {
			o.HealthTimeout = d
		}
	}
}
