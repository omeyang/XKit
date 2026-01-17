package xpulsar

import (
	"context"

	"github.com/omeyang/xkit/internal/mqcore"
	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/apache/pulsar-client-go/pulsar"
)

// MessageHandler 定义 Pulsar 消息处理函数。
type MessageHandler func(ctx context.Context, msg pulsar.Message) error

// TracingProducer 是带追踪注入能力的 Producer。
type TracingProducer struct {
	pulsar.Producer
	tracer   Tracer
	observer xmetrics.Observer
	topic    string
}

// WrapProducer 包装 Producer 以注入追踪信息。
func WrapProducer(producer pulsar.Producer, topic string, tracer Tracer, observer xmetrics.Observer) *TracingProducer {
	if observer == nil {
		observer = xmetrics.NoopObserver{}
	}
	if tracer == nil {
		tracer = NoopTracer{}
	}
	return &TracingProducer{
		Producer: producer,
		tracer:   tracer,
		observer: observer,
		topic:    topic,
	}
}

// NewTracingProducer 创建带追踪注入能力的 Producer。
func NewTracingProducer(client Client, options pulsar.ProducerOptions, tracer Tracer, observer xmetrics.Observer) (*TracingProducer, error) {
	if client == nil {
		return nil, ErrNilClient
	}
	producer, err := client.CreateProducer(options)
	if err != nil {
		return nil, err
	}
	return WrapProducer(producer, options.Topic, tracer, observer), nil
}

// Send 发送消息并注入追踪信息。
func (p *TracingProducer) Send(ctx context.Context, msg *pulsar.ProducerMessage) (id pulsar.MessageID, err error) {
	if msg == nil {
		return nil, ErrNilMessage
	}
	if ctx == nil {
		ctx = context.Background()
	}

	injectPulsarTrace(ctx, p.tracer, msg)

	ctx, span := xmetrics.Start(ctx, p.observer, xmetrics.SpanOptions{
		Component: componentName,
		Operation: "produce",
		Kind:      xmetrics.KindProducer,
		Attrs:     pulsarAttrs(p.topic),
	})
	defer func() {
		span.End(xmetrics.Result{Err: err})
	}()

	id, err = p.Producer.Send(ctx, msg)
	return id, err
}

// SendAsync 异步发送消息并注入追踪信息。
func (p *TracingProducer) SendAsync(ctx context.Context, msg *pulsar.ProducerMessage, callback func(pulsar.MessageID, *pulsar.ProducerMessage, error)) {
	if msg == nil {
		if callback != nil {
			callback(nil, nil, ErrNilMessage)
		}
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	injectPulsarTrace(ctx, p.tracer, msg)

	ctx, span := xmetrics.Start(ctx, p.observer, xmetrics.SpanOptions{
		Component: componentName,
		Operation: "produce",
		Kind:      xmetrics.KindProducer,
		Attrs:     pulsarAttrs(p.topic),
	})

	wrappedCallback := func(id pulsar.MessageID, m *pulsar.ProducerMessage, err error) {
		span.End(xmetrics.Result{Err: err})
		if callback != nil {
			callback(id, m, err)
		}
	}

	p.Producer.SendAsync(ctx, msg, wrappedCallback)
}

// TracingConsumer 是带追踪提取能力的 Consumer。
type TracingConsumer struct {
	pulsar.Consumer
	tracer   Tracer
	observer xmetrics.Observer
	topic    string
}

// WrapConsumer 包装 Consumer 以提取追踪信息。
func WrapConsumer(consumer pulsar.Consumer, topic string, tracer Tracer, observer xmetrics.Observer) *TracingConsumer {
	if observer == nil {
		observer = xmetrics.NoopObserver{}
	}
	if tracer == nil {
		tracer = NoopTracer{}
	}
	return &TracingConsumer{
		Consumer: consumer,
		tracer:   tracer,
		observer: observer,
		topic:    topic,
	}
}

// NewTracingConsumer 创建带追踪提取能力的 Consumer。
func NewTracingConsumer(client Client, options pulsar.ConsumerOptions, tracer Tracer, observer xmetrics.Observer) (*TracingConsumer, error) {
	if client == nil {
		return nil, ErrNilClient
	}
	consumer, err := client.Subscribe(options)
	if err != nil {
		return nil, err
	}

	topic := topicFromConsumerOptions(options)
	return WrapConsumer(consumer, topic, tracer, observer), nil
}

// ReceiveWithContext 接收消息并提取追踪信息。
func (c *TracingConsumer) ReceiveWithContext(ctx context.Context) (context.Context, pulsar.Message, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	msg, err := c.Receive(ctx)
	if err != nil {
		return ctx, nil, err
	}

	msgCtx := extractPulsarTrace(ctx, c.tracer, msg)
	return msgCtx, msg, nil
}

// Consume 消费一条消息并执行处理函数。
func (c *TracingConsumer) Consume(ctx context.Context, handler MessageHandler) (err error) {
	if handler == nil {
		return ErrNilHandler
	}

	msgCtx, msg, err := c.ReceiveWithContext(ctx)
	if err != nil {
		return err
	}
	if msg == nil {
		return nil
	}

	msgCtx, span := xmetrics.Start(msgCtx, c.observer, xmetrics.SpanOptions{
		Component: componentName,
		Operation: "consume",
		Kind:      xmetrics.KindConsumer,
		Attrs:     pulsarAttrs(c.topic),
	})
	defer func() {
		span.End(xmetrics.Result{Err: err})
	}()

	err = handler(msgCtx, msg)
	return err
}

// ConsumeLoop 循环消费消息直到 ctx 取消。
// 使用默认退避配置避免错误时 CPU 100%。
func (c *TracingConsumer) ConsumeLoop(ctx context.Context, handler MessageHandler) error {
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
func (c *TracingConsumer) ConsumeLoopWithPolicy(ctx context.Context, handler MessageHandler, backoff BackoffPolicy) error {
	consume := func(ctx context.Context) error {
		return c.Consume(ctx, handler)
	}

	opts := []mqcore.ConsumeLoopOption{}
	if backoff != nil {
		opts = append(opts, mqcore.WithBackoff(backoff))
	}

	return mqcore.RunConsumeLoop(ctx, consume, opts...)
}

// ConsumeLoopWithBackoff 带退避的消费循环。
//
// Deprecated: 请使用 ConsumeLoopWithPolicy 替代，它支持更灵活的 xretry.BackoffPolicy 接口。
// 此方法保留用于向后兼容。
func (c *TracingConsumer) ConsumeLoopWithBackoff(ctx context.Context, handler MessageHandler, config BackoffConfig) error {
	return c.ConsumeLoopWithPolicy(ctx, handler, config.ToBackoffPolicy())
}
