package xkafka

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// TracingProducer 是带追踪注入能力的 Producer。
type TracingProducer struct {
	*producerWrapper
}

// NewTracingProducer 创建带追踪注入能力的 Producer。
func NewTracingProducer(config *kafka.ConfigMap, opts ...ProducerOption) (*TracingProducer, error) {
	producer, err := newProducerWrapper(config, opts...)
	if err != nil {
		return nil, err
	}
	return &TracingProducer{producerWrapper: producer}, nil
}

// Produce 发送消息并注入追踪信息。
func (w *TracingProducer) Produce(ctx context.Context, msg *kafka.Message, deliveryChan chan kafka.Event) (err error) {
	if msg == nil {
		return ErrNilMessage
	}
	if ctx == nil {
		ctx = context.Background()
	}

	injectKafkaTrace(ctx, w.options.Tracer, msg)

	_, span := xmetrics.Start(ctx, w.options.Observer, xmetrics.SpanOptions{
		Component: componentName,
		Operation: "produce",
		Kind:      xmetrics.KindProducer,
		Attrs:     kafkaAttrs(topicFromKafkaMessage(msg)),
	})
	defer func() {
		span.End(xmetrics.Result{Err: err})
	}()

	err = w.producer.Produce(msg, deliveryChan)
	if err != nil {
		w.errors.Add(1)
		return fmt.Errorf("kafka produce: %w", err)
	}

	w.messagesProduced.Add(1)
	w.bytesProduced.Add(int64(len(msg.Value)))
	return nil
}

// TracingConsumer 是带追踪提取能力的 Consumer。
type TracingConsumer struct {
	*consumerWrapper

	// closeMu 保护 Consume 与 Close 的并发。
	// Consume 持有读锁，Close 持有写锁。
	// 确保 Close 等待进行中的 Consume（含 StoreMessage）完成后再关闭资源，
	// 避免消息已处理但 offset 未提交的竞态条件。
	closeMu sync.RWMutex
}

// NewTracingConsumer 创建带追踪提取能力的 Consumer。
func NewTracingConsumer(config *kafka.ConfigMap, topics []string, opts ...ConsumerOption) (*TracingConsumer, error) {
	consumer, err := newConsumerWrapper(config, topics, opts...)
	if err != nil {
		return nil, err
	}
	return &TracingConsumer{consumerWrapper: consumer}, nil
}

// ReadMessage 读取消息并提取追踪信息。
func (w *TracingConsumer) ReadMessage(ctx context.Context) (context.Context, *kafka.Message, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	for {
		if w.closed.Load() {
			return ctx, nil, ErrClosed
		}
		if ctx.Err() != nil {
			return ctx, nil, ctx.Err()
		}

		msg, err := w.consumer.ReadMessage(w.options.PollTimeout)
		if err != nil {
			var kafkaErr kafka.Error
			if errors.As(err, &kafkaErr) && kafkaErr.Code() == kafka.ErrTimedOut {
				continue
			}
			// 设计决策: 不在此处递增 errorsCount，由 ConsumeLoopWithPolicy 统一计数，
			// 避免 ReadMessage → Consume → ConsumeLoop 链路上的双重计数。
			return ctx, nil, err
		}

		w.messagesConsumed.Add(1)
		w.bytesConsumed.Add(int64(len(msg.Value)))

		msgCtx := extractKafkaTrace(ctx, w.options.Tracer, msg)
		return msgCtx, msg, nil
	}
}

// Consume 消费一条消息并执行处理函数。
// closeMu 读锁保护 handler 执行和 StoreMessage 的原子性，
// 确保 Close 等待进行中的消费完成后再关闭资源。
func (w *TracingConsumer) Consume(ctx context.Context, handler MessageHandler) (err error) {
	if handler == nil {
		return ErrNilHandler
	}

	msgCtx, msg, err := w.ReadMessage(ctx)
	if err != nil {
		return err
	}
	if msg == nil {
		return nil
	}

	w.closeMu.RLock()
	defer w.closeMu.RUnlock()
	if w.closed.Load() {
		return ErrClosed
	}

	msgCtx, span := xmetrics.Start(msgCtx, w.options.Observer, xmetrics.SpanOptions{
		Component: componentName,
		Operation: "consume",
		Kind:      xmetrics.KindConsumer,
		Attrs:     kafkaConsumerMessageAttrs(msg, w.groupID),
	})
	defer func() {
		span.End(xmetrics.Result{Err: err})
	}()

	err = handler(msgCtx, msg)
	if err != nil {
		return err
	}

	// 成功处理后存储 offset，确保 at-least-once 语义
	// 设计决策: 使用 StoreMessage 而非 StoreOffsets，因为 StoreMessage 内部会将 offset+1，
	// 表示"下次从此 offset 之后的位置开始消费"。直接使用 StoreOffsets 传递当前 offset
	// 会导致重启后重复消费已处理的消息。
	if _, storeErr := w.consumer.StoreMessage(msg); storeErr != nil {
		return fmt.Errorf("store offset failed: %w", storeErr)
	}

	return nil
}

// Close 优雅关闭消费者。
// 获取写锁等待进行中的 Consume 完成后再关闭底层消费者，
// 避免消息已处理但 offset 未提交的竞态条件。
// 重复调用 Close 安全返回 ErrClosed。
func (w *TracingConsumer) Close() error {
	w.closeMu.Lock()
	defer w.closeMu.Unlock()
	return w.consumerWrapper.Close()
}

// ConsumeLoop 循环消费消息直到 ctx 取消。
// 使用默认退避配置避免错误时 CPU 100%。
func (w *TracingConsumer) ConsumeLoop(ctx context.Context, handler MessageHandler) error {
	return w.ConsumeLoopWithPolicy(ctx, handler, nil)
}

// ConsumeLoopWithPolicy 启动带退避策略的消费循环。
// 使用 xretry.BackoffPolicy 接口，支持灵活的退避策略配置。
//
// 参数：
//   - ctx: 上下文，取消时退出循环
//   - handler: 消息处理函数
//   - backoff: 退避策略，nil 时使用默认 xretry.ExponentialBackoff
func (w *TracingConsumer) ConsumeLoopWithPolicy(ctx context.Context, handler MessageHandler, backoff BackoffPolicy) error {
	if handler == nil {
		return ErrNilHandler
	}
	if ctx == nil {
		ctx = context.Background()
	}
	consume := func(ctx context.Context) error {
		return w.Consume(ctx, handler)
	}
	return runConsumeLoop(ctx, consume, &w.errorsCount, backoff)
}
