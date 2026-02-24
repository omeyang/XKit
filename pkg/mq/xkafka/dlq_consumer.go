package xkafka

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// dlqConsumer 实现 ConsumerWithDLQ 接口
type dlqConsumer struct {
	*consumerWrapper
	policy      *DLQPolicy
	dlqProducer *kafka.Producer

	stats *dlqStatsCollector

	// closeMu 保护 processMessage/SendToDLQ 与 Close 的并发。
	// processMessage/SendToDLQ 持有读锁，Close 持有写锁。
	// 确保 Close 等待所有进行中的消息处理完成后再关闭资源，
	// 避免 DLQ 消息已投递但 offset 未提交的竞态条件。
	closeMu sync.RWMutex
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

	// 直接使用内部构造函数，避免类型断言
	wrapper, err := newConsumerWrapper(config, topics, opts...)
	if err != nil {
		return nil, err
	}

	// 创建 DLQ Producer
	dlqProducer, err := createDLQProducer(config, dlqPolicy)
	if err != nil {
		closeErr := wrapper.Close()
		return nil, errors.Join(err, closeErr)
	}

	return &dlqConsumer{
		consumerWrapper: wrapper,
		policy:          dlqPolicy,
		dlqProducer:     dlqProducer,
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

// ConsumeWithRetry 消费单条消息，自动处理重试和 DLQ
func (c *dlqConsumer) ConsumeWithRetry(ctx context.Context, handler MessageHandler) error {
	if handler == nil {
		return ErrNilHandler
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if c.closed.Load() {
		return ErrClosed
	}

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
// 使用 xretry.BackoffPolicy 接口，支持灵活的退避策略配置。
//
// 参数：
//   - ctx: 上下文，取消时退出循环
//   - handler: 消息处理函数
//   - backoff: 退避策略，nil 时使用默认 xretry.ExponentialBackoff
func (c *dlqConsumer) ConsumeLoopWithPolicy(ctx context.Context, handler MessageHandler, backoff BackoffPolicy) error {
	if handler == nil {
		return ErrNilHandler
	}
	if ctx == nil {
		ctx = context.Background()
	}
	consume := func(ctx context.Context) error {
		return c.ConsumeWithRetry(ctx, handler)
	}
	return runConsumeLoop(ctx, consume, &c.errorsCount, backoff)
}

// processMessage 处理单条消息。
// 设计决策: messagesConsumed/bytesConsumed 在此处递增，而非在 ReadMessage 中，
// 因为 dlqConsumer 直接调用底层 consumer.ReadMessage（不经过 TracingConsumer.ReadMessage），
// 避免双重计数。如果未来重构为使用 TracingConsumer，需移除此处的统计递增。
func (c *dlqConsumer) processMessage(ctx context.Context, msg *kafka.Message, handler MessageHandler) (resultErr error) {
	c.closeMu.RLock()
	defer c.closeMu.RUnlock()
	if c.closed.Load() {
		return ErrClosed
	}

	c.stats.incTotal()

	// 更新消费统计
	c.messagesConsumed.Add(1)
	c.bytesConsumed.Add(int64(len(msg.Value)))

	// 获取当前重试次数
	retryCount := getRetryCount(msg)
	attempt := retryCount + 1

	msgCtx := extractKafkaTrace(ctx, c.options.Tracer, msg)
	msgCtx, span := xmetrics.Start(msgCtx, c.options.Observer, xmetrics.SpanOptions{
		Component: componentName,
		Operation: "consume",
		Kind:      xmetrics.KindConsumer,
		Attrs:     kafkaConsumerMessageAttrs(msg, c.groupID),
	})
	defer func() {
		span.End(xmetrics.Result{Err: resultErr})
	}()

	// 执行处理
	resultErr = handler(msgCtx, msg)
	if resultErr == nil {
		// 处理成功，存储 offset
		if retryCount > 0 {
			c.stats.incSuccessAfterRetry()
		}
		// 设计决策: 使用 StoreMessage 而非 StoreOffsets，StoreMessage 内部执行 offset+1，
		// 表示"下次从此 offset 之后开始消费"，避免重启后重复消费已处理消息。
		if _, storeErr := c.consumer.StoreMessage(msg); storeErr != nil {
			resultErr = fmt.Errorf("store offset failed: %w", storeErr)
			return resultErr
		}
		return nil
	}

	// 处理失败，检查是否需要重试
	if !c.policy.RetryPolicy.ShouldRetry(msgCtx, attempt, resultErr) {
		// 超过重试次数或遇到永久性错误，发送到 DLQ
		resultErr = c.sendToDLQInternal(msgCtx, msg, resultErr, retryCount)
		return resultErr
	}

	resultErr = c.handleRetry(msgCtx, msg, attempt, resultErr)
	return resultErr
}

// handleRetry 处理消息重试逻辑：触发回调、等待退避延迟、重新投递。
func (c *dlqConsumer) handleRetry(ctx context.Context, msg *kafka.Message, attempt int, err error) error {
	c.stats.incRetried()
	if c.policy.OnRetry != nil {
		c.policy.OnRetry(msg, attempt, err)
	}

	// 设计决策: 退避延迟同步阻塞当前 goroutine，期间不消费新消息。
	// 这是同步重试模型的固有行为——简单可靠，适合低吞吐场景。
	// 高吞吐场景应配置 RetryTopic 将重试消息异步投递到重试队列。
	if c.policy.BackoffPolicy != nil {
		delay := c.policy.BackoffPolicy.NextDelay(attempt)
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}

	// 更新 Header 并重新投递
	c.incrementRetryCount(msg, err)
	return c.redeliverMessage(ctx, msg)
}

// SendToDLQ 手动发送消息到 DLQ
func (c *dlqConsumer) SendToDLQ(ctx context.Context, msg *kafka.Message, reason error) error {
	if msg == nil {
		return ErrNilMessage
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.closeMu.RLock()
	defer c.closeMu.RUnlock()
	if c.closed.Load() {
		return ErrClosed
	}
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

	// DLQ 发送成功，存储 offset（StoreMessage 内部 offset+1）
	if _, storeErr := c.consumer.StoreMessage(msg); storeErr != nil {
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

	// 重新投递成功，存储 offset（StoreMessage 内部 offset+1）
	if _, storeErr := c.consumer.StoreMessage(msg); storeErr != nil {
		return fmt.Errorf("store offset after redeliver failed: %w", storeErr)
	}

	return nil
}

// Close 关闭消费者和 DLQ Producer。
// 关闭顺序：先获取写锁等待进行中的消息处理完成，再关闭消费者（提交 offset），
// 最后刷新并关闭 DLQ Producer。
// 重复调用 Close 安全返回 ErrClosed。
func (c *dlqConsumer) Close() error {
	// 获取写锁，等待所有进行中的 processMessage/SendToDLQ 完成
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	// 先关闭消费者，确保 offset 被提交
	consumerErr := c.consumerWrapper.Close()
	if errors.Is(consumerErr, ErrClosed) {
		// 已关闭过，跳过 dlqProducer 操作避免对已关闭的 producer 重复调用
		return ErrClosed
	}

	// 刷新 DLQ Producer 队列中的消息，避免丢失
	flushTimeout := c.policy.DLQFlushTimeout
	if flushTimeout <= 0 {
		flushTimeout = defaultFlushTimeout
	}
	remaining := c.dlqProducer.Flush(int(flushTimeout.Milliseconds()))
	c.dlqProducer.Close()

	if remaining > 0 {
		flushErr := fmt.Errorf("%w: %d DLQ messages still in queue", ErrFlushTimeout, remaining)
		return errors.Join(consumerErr, flushErr)
	}
	return consumerErr
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
