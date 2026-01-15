package xkafka

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/omeyang/xkit/internal/mqcore"
	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// dlqConsumer 实现 ConsumerWithDLQ 接口
type dlqConsumer struct {
	*consumerWrapper
	policy      *DLQPolicy
	dlqProducer *kafka.Producer
	config      *kafka.ConfigMap

	stats *dlqStatsCollector
}

// NewConsumerWithDLQ 创建支持 DLQ 的消费者。
// dlqPolicy 必须配置 DLQTopic、RetryPolicy 等。
func NewConsumerWithDLQ(
	config *kafka.ConfigMap,
	topics []string,
	dlqPolicy *DLQPolicy,
	opts ...ConsumerOption,
) (ConsumerWithDLQ, error) {
	// 验证参数
	if dlqPolicy == nil {
		return nil, ErrDLQPolicyRequired
	}
	if err := dlqPolicy.Validate(); err != nil {
		return nil, err
	}

	// 创建基础消费者
	baseConsumer, err := NewConsumer(config, topics, opts...)
	if err != nil {
		return nil, err
	}

	// 创建 DLQ Producer
	dlqProducer, err := createDLQProducer(config, dlqPolicy)
	if err != nil {
		return nil, errors.Join(err, closeConsumer(baseConsumer))
	}

	wrapper, ok := baseConsumer.(*consumerWrapper)
	if !ok {
		dlqProducer.Close()
		typeErr := fmt.Errorf("unexpected consumer type: %T", baseConsumer)
		return nil, errors.Join(typeErr, closeConsumer(baseConsumer))
	}

	return &dlqConsumer{
		consumerWrapper: wrapper,
		policy:          dlqPolicy,
		dlqProducer:     dlqProducer,
		config:          config,
		stats:           newDLQStatsCollector(),
	}, nil
}

// createDLQProducer 创建 DLQ Producer。
func createDLQProducer(config *kafka.ConfigMap, dlqPolicy *DLQPolicy) (*kafka.Producer, error) {
	producerConfig := dlqPolicy.ProducerConfig
	if producerConfig == nil {
		// 从 consumer config 派生 producer config，过滤 consumer-only 配置项
		var err error
		producerConfig, err = filterProducerConfig(config)
		if err != nil {
			return nil, err
		}
	}
	return kafka.NewProducer(producerConfig)
}

// closeConsumer 安全关闭消费者，返回关闭错误（如果有）。
func closeConsumer(c Consumer) error {
	if err := c.Close(); err != nil {
		return fmt.Errorf("close kafka consumer failed: %w", err)
	}
	return nil
}

// ConsumeWithRetry 消费单条消息，自动处理重试和 DLQ
func (c *dlqConsumer) ConsumeWithRetry(ctx context.Context, handler MessageHandler) error {
	msg, err := c.consumer.ReadMessage(c.options.PollTimeout)
	if err != nil {
		var kafkaErr kafka.Error
		if errors.As(err, &kafkaErr) && kafkaErr.Code() == kafka.ErrTimedOut {
			return nil // 超时不是错误
		}
		return err
	}

	return c.processMessage(ctx, msg, handler)
}

// ConsumeLoop 启动消费循环。
// 在持续错误情况下会使用指数退避，避免 CPU 100% 的问题。
func (c *dlqConsumer) ConsumeLoop(ctx context.Context, handler MessageHandler) error {
	return c.ConsumeLoopWithPolicy(ctx, handler, nil)
}

// ConsumeLoopWithPolicy 启动带退避策略的消费循环。
// 使用 xretry.BackoffPolicy 接口，支持更灵活的退避策略配置。
//
// 参数：
//   - ctx: 上下文，取消时退出循环
//   - handler: 消息处理函数
//   - backoff: 退避策略，nil 时使用默认 xretry.ExponentialBackoff
//
// 推荐使用此方法替代 ConsumeLoopWithBackoff。
func (c *dlqConsumer) ConsumeLoopWithPolicy(ctx context.Context, handler MessageHandler, backoff BackoffPolicy) error {
	consume := func(ctx context.Context) error {
		return c.ConsumeWithRetry(ctx, handler)
	}

	onError := func(_ error) {
		c.errorsCount.Add(1)
	}

	opts := []mqcore.ConsumeLoopOption{
		mqcore.WithOnError(onError),
	}
	if backoff != nil {
		opts = append(opts, mqcore.WithBackoff(backoff))
	}

	return mqcore.RunConsumeLoop(ctx, consume, opts...)
}

// ConsumeLoopWithBackoff 启动带退避的消费循环。
//
// Deprecated: 请使用 ConsumeLoopWithPolicy 替代，它支持更灵活的 xretry.BackoffPolicy 接口。
// 此方法保留用于向后兼容。
func (c *dlqConsumer) ConsumeLoopWithBackoff(ctx context.Context, handler MessageHandler, config BackoffConfig) error {
	return c.ConsumeLoopWithPolicy(ctx, handler, config.ToBackoffPolicy())
}

// processMessage 处理单条消息
func (c *dlqConsumer) processMessage(ctx context.Context, msg *kafka.Message, handler MessageHandler) error {
	c.stats.incTotal()

	// 更新消费统计
	c.messagesConsumed.Add(1)
	c.bytesConsumed.Add(int64(len(msg.Value)))

	// 获取当前重试次数
	retryCount := getRetryCount(msg)
	attempt := retryCount + 1

	topic := topicFromKafkaMessage(msg)
	msgCtx := extractKafkaTrace(ctx, c.options.Tracer, msg)
	msgCtx, span := xmetrics.Start(msgCtx, c.options.Observer, xmetrics.SpanOptions{
		Component: componentName,
		Operation: "consume",
		Kind:      xmetrics.KindConsumer,
		Attrs:     kafkaAttrs(topic),
	})
	// 执行处理
	err := handler(msgCtx, msg)
	span.End(xmetrics.Result{Err: err})
	if err == nil {
		// 处理成功，存储 offset
		if retryCount > 0 {
			c.stats.incSuccessAfterRetry()
		}
		// 存储 offset，配合 enable.auto.commit=true 自动提交
		// 或在 Close() 时统一提交
		if _, storeErr := c.consumer.StoreOffsets([]kafka.TopicPartition{msg.TopicPartition}); storeErr != nil {
			return fmt.Errorf("store offset failed: %w", storeErr)
		}
		return nil
	}

	// 处理失败，检查是否需要重试
	if !c.policy.RetryPolicy.ShouldRetry(msgCtx, attempt, err) {
		// 超过重试次数或遇到永久性错误，发送到 DLQ
		return c.sendToDLQInternal(msgCtx, msg, err, retryCount)
	}

	// 需要重试
	c.stats.incRetried()
	if c.policy.OnRetry != nil {
		c.policy.OnRetry(msg, attempt, err)
	}

	// 计算退避延迟
	if c.policy.BackoffPolicy != nil {
		delay := c.policy.BackoffPolicy.NextDelay(attempt)
		if delay > 0 {
			select {
			case <-msgCtx.Done():
				return msgCtx.Err()
			case <-time.After(delay):
			}
		}
	}

	// 更新 Header 并重新投递
	c.incrementRetryCount(msg, err)
	return c.redeliverMessage(msgCtx, msg)
}

// SendToDLQ 手动发送消息到 DLQ
func (c *dlqConsumer) SendToDLQ(ctx context.Context, msg *kafka.Message, reason error) error {
	retryCount := getRetryCount(msg)
	return c.sendToDLQInternal(ctx, msg, reason, retryCount)
}

// sendToDLQInternal 内部发送消息到 DLQ
func (c *dlqConsumer) sendToDLQInternal(ctx context.Context, msg *kafka.Message, reason error, retryCount int) error {
	topic := ""
	if msg.TopicPartition.Topic != nil {
		topic = *msg.TopicPartition.Topic
	}

	// 构建 DLQ 消息
	dlqMsg := c.buildDLQMessage(msg, reason, retryCount)

	// 发送到 DLQ Topic
	// 使用缓冲 channel 避免 ctx 取消时 producer 发送阻塞
	deliveryChan := make(chan kafka.Event, 1)
	if err := c.dlqProducer.Produce(dlqMsg, deliveryChan); err != nil {
		return err
	}

	// 等待确认
	select {
	case <-ctx.Done():
		return ctx.Err()
	case e := <-deliveryChan:
		if m, ok := e.(*kafka.Message); ok && m.TopicPartition.Error != nil {
			return m.TopicPartition.Error
		}
	}

	// 投递成功后再递增统计，确保统计准确性
	c.stats.incDeadLetter(topic)

	// DLQ 发送成功，存储 offset
	// 确保原消息不会被重复消费
	if _, storeErr := c.consumer.StoreOffsets([]kafka.TopicPartition{msg.TopicPartition}); storeErr != nil {
		return fmt.Errorf("store offset after DLQ failed: %w", storeErr)
	}

	// 触发回调
	if c.policy.OnDLQ != nil {
		metadata := c.buildDLQMetadata(msg, reason, retryCount)
		c.policy.OnDLQ(msg, reason, metadata)
	}

	return nil
}

// DLQStats 返回 DLQ 统计信息
func (c *dlqConsumer) DLQStats() DLQStats {
	return c.stats.get()
}

// incrementRetryCount 增加重试次数并更新相关 Header
func (c *dlqConsumer) incrementRetryCount(msg *kafka.Message, err error) {
	updateRetryHeaders(msg, err)
}

// buildDLQMessage 构建 DLQ 消息
func (c *dlqConsumer) buildDLQMessage(original *kafka.Message, reason error, retryCount int) *kafka.Message {
	return buildDLQMessageFromPolicy(original, c.policy.DLQTopic, reason, retryCount)
}

// buildDLQMetadata 构建 DLQ 元数据
func (c *dlqConsumer) buildDLQMetadata(msg *kafka.Message, reason error, retryCount int) DLQMetadata {
	return buildDLQMetadataFromMessage(msg, reason, retryCount)
}

// redeliverMessage 重新投递消息
func (c *dlqConsumer) redeliverMessage(ctx context.Context, msg *kafka.Message) error {
	// 确定目标 Topic
	targetTopic := c.policy.RetryTopic
	if targetTopic == "" && msg.TopicPartition.Topic != nil {
		targetTopic = *msg.TopicPartition.Topic
	}

	redeliverMsg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &targetTopic,
			Partition: kafka.PartitionAny,
		},
		Key:     msg.Key,
		Value:   msg.Value,
		Headers: msg.Headers,
	}

	// 使用缓冲 channel 避免 ctx 取消时 producer 发送阻塞
	deliveryChan := make(chan kafka.Event, 1)
	if err := c.dlqProducer.Produce(redeliverMsg, deliveryChan); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case e := <-deliveryChan:
		if m, ok := e.(*kafka.Message); ok && m.TopicPartition.Error != nil {
			return m.TopicPartition.Error
		}
	}

	// 重新投递成功，存储 offset
	// 原消息已被处理（重新投递），应该提交其 offset
	if _, storeErr := c.consumer.StoreOffsets([]kafka.TopicPartition{msg.TopicPartition}); storeErr != nil {
		return fmt.Errorf("store offset after redeliver failed: %w", storeErr)
	}

	return nil
}

// Close 关闭消费者和 DLQ Producer
func (c *dlqConsumer) Close() error {
	c.dlqProducer.Close()
	return c.consumerWrapper.Close()
}

// filterProducerConfig 从 consumer config 派生 producer config
// 保留公共配置（如 bootstrap.servers），过滤 consumer-only 配置项
func filterProducerConfig(consumerConfig *kafka.ConfigMap) (*kafka.ConfigMap, error) {
	producerConfig := &kafka.ConfigMap{}

	// Consumer-only 配置项列表（基于 librdkafka 文档）
	consumerOnlyKeys := map[string]bool{
		"group.id":                      true,
		"group.instance.id":             true,
		"auto.offset.reset":             true,
		"enable.auto.commit":            true,
		"auto.commit.interval.ms":       true,
		"enable.auto.offset.store":      true,
		"partition.assignment.strategy": true,
		"session.timeout.ms":            true,
		"heartbeat.interval.ms":         true,
		"max.poll.interval.ms":          true,
		"fetch.min.bytes":               true,
		"fetch.max.bytes":               true,
		"fetch.wait.max.ms":             true,
		"max.partition.fetch.bytes":     true,
		"isolation.level":               true,
		"check.crcs":                    true,
		"queued.min.messages":           true,
		"queued.max.messages.kbytes":    true,
		"fetch.message.max.bytes":       true,
	}

	// 复制非 consumer-only 配置
	for key, value := range *consumerConfig {
		if !consumerOnlyKeys[key] {
			if err := producerConfig.SetKey(key, value); err != nil {
				return nil, fmt.Errorf("set producer config key %q: %w", key, err)
			}
		}
	}

	return producerConfig, nil
}

// 确保实现接口
var _ ConsumerWithDLQ = (*dlqConsumer)(nil)
