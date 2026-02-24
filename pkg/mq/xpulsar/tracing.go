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
//
// 设计决策: 返回具体类型而非接口，因为 TracingProducer 嵌入了 pulsar.Producer，
// 用户需要直接访问所有原生 Producer 方法。返回接口会丢失嵌入类型的方法集。
type TracingProducer struct {
	pulsar.Producer
	tracer   Tracer
	observer xmetrics.Observer
	topic    string
}

// WrapProducer 包装 Producer 以注入追踪信息。
// producer 不能为 nil，否则返回 ErrNilProducer。
// topic 为空时自动从 producer.Topic() 获取。
func WrapProducer(producer pulsar.Producer, topic string, tracer Tracer, observer xmetrics.Observer) (*TracingProducer, error) {
	if producer == nil {
		return nil, ErrNilProducer
	}
	if observer == nil {
		observer = xmetrics.NoopObserver{}
	}
	if tracer == nil {
		tracer = NoopTracer{}
	}
	if topic == "" {
		topic = producer.Topic()
	}
	return &TracingProducer{
		Producer: producer,
		tracer:   tracer,
		observer: observer,
		topic:    topic,
	}, nil
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
	return WrapProducer(producer, options.Topic, tracer, observer)
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
		result := xmetrics.Result{Err: err}
		if id != nil {
			result.Attrs = []xmetrics.Attr{
				xmetrics.String("messaging.message.id", id.String()),
			}
		}
		span.End(result)
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
//
// 设计决策: 返回具体类型而非接口，因为 TracingConsumer 嵌入了 pulsar.Consumer，
// 用户需要直接访问所有原生 Consumer 方法。返回接口会丢失嵌入类型的方法集。
type TracingConsumer struct {
	pulsar.Consumer
	tracer   Tracer
	observer xmetrics.Observer
	topic    string
}

// WrapConsumer 包装 Consumer 以提取追踪信息。
// consumer 不能为 nil，否则返回 ErrNilConsumer。
//
// 设计决策: 与 WrapProducer 不同，topic 为空时不自动回填。
// pulsar.Consumer 可订阅多个 topic（Topics/TopicsPattern），无单一 Topic() 方法。
// 使用 NewTracingConsumer 时会通过 topicFromConsumerOptions 自动推导。
func WrapConsumer(consumer pulsar.Consumer, topic string, tracer Tracer, observer xmetrics.Observer) (*TracingConsumer, error) {
	if consumer == nil {
		return nil, ErrNilConsumer
	}
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
	}, nil
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
	return WrapConsumer(consumer, topic, tracer, observer)
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
// 成功时自动 Ack，失败时自动 Nack。
func (c *TracingConsumer) Consume(ctx context.Context, handler MessageHandler) (err error) {
	if handler == nil {
		return ErrNilHandler
	}

	msgCtx, msg, err := c.ReceiveWithContext(ctx)
	if err != nil {
		return err
	}
	// 设计决策: 防御性检查。正常情况下 Receive 要么返回消息要么返回错误，
	// msg == nil && err == nil 不应出现。此分支仅作为安全兜底。
	if msg == nil {
		return nil
	}

	attrs := pulsarAttrs(c.topic)
	if sub := c.Subscription(); sub != "" {
		attrs = append(attrs, xmetrics.String("messaging.consumer.group.name", sub))
	}
	if id := msg.ID(); id != nil {
		attrs = append(attrs, xmetrics.String("messaging.message.id", id.String()))
	}

	var ackErr error

	msgCtx, span := xmetrics.Start(msgCtx, c.observer, xmetrics.SpanOptions{
		Component: componentName,
		Operation: "consume",
		Kind:      xmetrics.KindConsumer,
		Attrs:     attrs,
	})
	defer func() {
		result := xmetrics.Result{Err: err}
		if ackErr != nil {
			result.Attrs = []xmetrics.Attr{
				xmetrics.String("messaging.pulsar.ack_error", ackErr.Error()),
			}
		}
		span.End(result)
	}()

	err = handler(msgCtx, msg)
	if err != nil {
		// 处理失败，Nack 消息以便重试
		c.Nack(msg)
		return err
	}

	// 设计决策: Ack 失败不应阻止已成功处理的消息。消息已被 handler 成功消费，
	// 返回 Ack 错误会导致调用方误认为处理失败而重复处理。
	// Pulsar 客户端会在后台重试 Ack。Ack 错误通过 span 属性记录，确保可观测性。
	ackErr = c.Ack(msg)
	return nil
}

// ConsumeLoop 循环消费消息直到 ctx 取消。
// 使用默认退避配置避免错误时 CPU 100%。
func (c *TracingConsumer) ConsumeLoop(ctx context.Context, handler MessageHandler) error {
	return c.ConsumeLoopWithPolicy(ctx, handler, nil)
}

// ConsumeLoopWithPolicy 启动带退避策略的消费循环。
// 使用 xretry.BackoffPolicy 接口，支持灵活的退避策略配置。
//
// 参数：
//   - ctx: 上下文，取消时退出循环
//   - handler: 消息处理函数
//   - backoff: 退避策略，nil 时使用默认 xretry.ExponentialBackoff
func (c *TracingConsumer) ConsumeLoopWithPolicy(ctx context.Context, handler MessageHandler, backoff BackoffPolicy) error {
	if handler == nil {
		return ErrNilHandler
	}

	consume := func(ctx context.Context) error {
		return c.Consume(ctx, handler)
	}

	var opts []mqcore.ConsumeLoopOption
	if backoff != nil {
		opts = append(opts, mqcore.WithBackoff(backoff))
	}

	return mqcore.RunConsumeLoop(ctx, consume, opts...)
}
