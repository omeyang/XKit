package xkafka

import (
	"context"
	"errors"
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
	// MessagesProduced 已成功入队的消息数量。
	// 设计决策: kafka.Producer.Produce() 是异步的，入队成功不等于发送到 Broker 成功。
	// 实际发送结果需通过 deliveryChan 确认。此字段统计入队数，非最终投递数。
	MessagesProduced int64
	// BytesProduced 已成功入队的消息字节数。
	BytesProduced int64
	// Errors 入队失败的消息数量。
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
	// Errors 消费循环中未能恢复的错误次数。
	// 仅在通过 ConsumeLoop/ConsumeLoopWithPolicy 消费时递增。
	// 直接调用 ReadMessage/Consume 的错误不计入此统计。
	// 对于 ConsumerWithDLQ，成功重试或发送到 DLQ 的消息不计入此统计，
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

	// 复制配置，避免修改调用方传入的 ConfigMap，与 newConsumerWrapper 保持一致
	clonedConfig := &kafka.ConfigMap{}
	for k, v := range *config {
		if err := clonedConfig.SetKey(k, v); err != nil {
			return nil, fmt.Errorf("clone config key %q: %w", k, err)
		}
	}

	producer, err := kafka.NewProducer(clonedConfig)
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

	// 复制配置，避免修改调用方传入的 ConfigMap
	clonedConfig := &kafka.ConfigMap{}
	for k, v := range *config {
		if err := clonedConfig.SetKey(k, v); err != nil {
			return nil, fmt.Errorf("clone config key %q: %w", k, err)
		}
	}

	// 强制设置 enable.auto.offset.store=false 以确保 at-least-once 语义
	// 这确保 offset 只在显式调用 StoreOffsets 后才会被存储
	if err := clonedConfig.SetKey("enable.auto.offset.store", false); err != nil {
		return nil, fmt.Errorf("failed to set enable.auto.offset.store: %w", err)
	}

	consumer, err := kafka.NewConsumer(clonedConfig)
	if err != nil {
		return nil, err
	}

	if err := consumer.SubscribeTopics(topics, nil); err != nil {
		return nil, errors.Join(err, consumer.Close())
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
