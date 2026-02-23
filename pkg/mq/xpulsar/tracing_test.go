package xpulsar

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// WrapProducer Tests
// =============================================================================

func TestWrapProducer_NilProducer(t *testing.T) {
	wrapper, err := WrapProducer(nil, "test-topic", NoopTracer{}, nil)

	assert.Nil(t, wrapper)
	assert.ErrorIs(t, err, ErrNilProducer)
}

func TestWrapProducer_NilObserver(t *testing.T) {
	mp := &mockProducer{}
	wrapper, err := WrapProducer(mp, "test-topic", NoopTracer{}, nil)

	require.NoError(t, err)
	assert.NotNil(t, wrapper)
	assert.Equal(t, "test-topic", wrapper.topic)
	assert.NotNil(t, wrapper.observer)
	assert.IsType(t, xmetrics.NoopObserver{}, wrapper.observer)
}

func TestWrapProducer_NilTracer(t *testing.T) {
	mp := &mockProducer{}
	wrapper, err := WrapProducer(mp, "test-topic", nil, xmetrics.NoopObserver{})

	require.NoError(t, err)
	assert.NotNil(t, wrapper)
	assert.NotNil(t, wrapper.tracer)
	assert.IsType(t, NoopTracer{}, wrapper.tracer)
}

func TestWrapProducer_AllNilOptional(t *testing.T) {
	mp := &mockProducer{}
	wrapper, err := WrapProducer(mp, "test-topic", nil, nil)

	require.NoError(t, err)
	assert.NotNil(t, wrapper)
	assert.NotNil(t, wrapper.tracer)
	assert.NotNil(t, wrapper.observer)
}

func TestWrapProducer_WithValues(t *testing.T) {
	mp := &mockProducer{}
	tracer := NewOTelTracer()
	observer := xmetrics.NoopObserver{}

	wrapper, err := WrapProducer(mp, "my-topic", tracer, observer)

	require.NoError(t, err)
	assert.NotNil(t, wrapper)
	assert.Equal(t, "my-topic", wrapper.topic)
	assert.NotNil(t, wrapper.tracer)
	assert.Equal(t, observer, wrapper.observer)
}

func TestWrapProducer_EmptyTopicFallback(t *testing.T) {
	mp := &mockProducer{} // Topic() returns "test-topic"
	wrapper, err := WrapProducer(mp, "", NoopTracer{}, nil)

	require.NoError(t, err)
	assert.Equal(t, "test-topic", wrapper.topic)
}

// =============================================================================
// WrapConsumer Tests
// =============================================================================

func TestWrapConsumer_NilConsumer(t *testing.T) {
	wrapper, err := WrapConsumer(nil, "test-topic", NoopTracer{}, nil)

	assert.Nil(t, wrapper)
	assert.ErrorIs(t, err, ErrNilConsumer)
}

func TestWrapConsumer_NilObserver(t *testing.T) {
	mc := &mockConsumer{}
	wrapper, err := WrapConsumer(mc, "test-topic", NoopTracer{}, nil)

	require.NoError(t, err)
	assert.NotNil(t, wrapper)
	assert.Equal(t, "test-topic", wrapper.topic)
	assert.NotNil(t, wrapper.observer)
	assert.IsType(t, xmetrics.NoopObserver{}, wrapper.observer)
}

func TestWrapConsumer_NilTracer(t *testing.T) {
	mc := &mockConsumer{}
	wrapper, err := WrapConsumer(mc, "test-topic", nil, xmetrics.NoopObserver{})

	require.NoError(t, err)
	assert.NotNil(t, wrapper)
	assert.NotNil(t, wrapper.tracer)
	assert.IsType(t, NoopTracer{}, wrapper.tracer)
}

func TestWrapConsumer_AllNilOptional(t *testing.T) {
	mc := &mockConsumer{}
	wrapper, err := WrapConsumer(mc, "test-topic", nil, nil)

	require.NoError(t, err)
	assert.NotNil(t, wrapper)
	assert.NotNil(t, wrapper.tracer)
	assert.NotNil(t, wrapper.observer)
}

func TestWrapConsumer_WithValues(t *testing.T) {
	mc := &mockConsumer{}
	tracer := NewOTelTracer()
	observer := xmetrics.NoopObserver{}

	wrapper, err := WrapConsumer(mc, "my-topic", tracer, observer)

	require.NoError(t, err)
	assert.NotNil(t, wrapper)
	assert.Equal(t, "my-topic", wrapper.topic)
	assert.NotNil(t, wrapper.tracer)
	assert.Equal(t, observer, wrapper.observer)
}

// =============================================================================
// NewTracingProducer Tests
// =============================================================================

func TestNewTracingProducer_NilClient(t *testing.T) {
	producer, err := NewTracingProducer(nil, pulsar.ProducerOptions{}, NoopTracer{}, nil)

	assert.Nil(t, producer)
	assert.ErrorIs(t, err, ErrNilClient)
}

// =============================================================================
// NewTracingConsumer Tests
// =============================================================================

func TestNewTracingConsumer_NilClient(t *testing.T) {
	consumer, err := NewTracingConsumer(nil, pulsar.ConsumerOptions{}, NoopTracer{}, nil)

	assert.Nil(t, consumer)
	assert.ErrorIs(t, err, ErrNilClient)
}

// =============================================================================
// Type alias check
// =============================================================================

func TestTracingTypes(t *testing.T) {
	// 验证类型结构
	producer := &TracingProducer{}
	assert.Nil(t, producer.Producer)

	consumer := &TracingConsumer{}
	assert.Nil(t, consumer.Consumer)
}

// =============================================================================
// TracingProducer.Send Validation Tests
// =============================================================================

func TestTracingProducer_Send_NilMessage(t *testing.T) {
	mp := &mockProducer{}
	producer, err := WrapProducer(mp, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	id, sendErr := producer.Send(context.Background(), nil)

	assert.Nil(t, id)
	assert.ErrorIs(t, sendErr, ErrNilMessage)
}

// =============================================================================
// TracingProducer.SendAsync Validation Tests
// =============================================================================

func TestTracingProducer_SendAsync_NilMessage(t *testing.T) {
	mp := &mockProducer{}
	producer, err := WrapProducer(mp, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	var callbackCalled bool
	var callbackErr error

	producer.SendAsync(context.Background(), nil, func(id pulsar.MessageID, msg *pulsar.ProducerMessage, err error) {
		callbackCalled = true
		callbackErr = err
	})

	assert.True(t, callbackCalled)
	assert.ErrorIs(t, callbackErr, ErrNilMessage)
}

func TestTracingProducer_SendAsync_NilMessage_NilCallback(t *testing.T) {
	mp := &mockProducer{}
	producer, err := WrapProducer(mp, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	// 不应 panic
	assert.NotPanics(t, func() {
		producer.SendAsync(context.Background(), nil, nil)
	})
}

// =============================================================================
// TracingConsumer.Consume Validation Tests
// =============================================================================

func TestTracingConsumer_Consume_NilHandler(t *testing.T) {
	mc := &mockConsumer{}
	consumer, err := WrapConsumer(mc, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	consumeErr := consumer.Consume(context.Background(), nil)

	assert.ErrorIs(t, consumeErr, ErrNilHandler)
}

// =============================================================================
// mockClient - 实现 Client 接口用于测试
// =============================================================================

type mockClient struct {
	createProducerErr error
	subscribeErr      error
}

func (m *mockClient) Client() pulsar.Client            { return nil }
func (m *mockClient) Health(ctx context.Context) error { return nil }
func (m *mockClient) CreateProducer(options pulsar.ProducerOptions) (pulsar.Producer, error) {
	if m.createProducerErr != nil {
		return nil, m.createProducerErr
	}
	return &mockProducer{}, nil
}
func (m *mockClient) Subscribe(options pulsar.ConsumerOptions) (pulsar.Consumer, error) {
	if m.subscribeErr != nil {
		return nil, m.subscribeErr
	}
	return &mockConsumer{}, nil
}
func (m *mockClient) Stats() Stats { return Stats{} }
func (m *mockClient) Close() error { return nil }

// =============================================================================
// NewTracingProducer Error Path Tests
// =============================================================================

func TestNewTracingProducer_CreateProducerError(t *testing.T) {
	expectedErr := errors.New("connection failed")
	client := &mockClient{createProducerErr: expectedErr}

	producer, err := NewTracingProducer(client, pulsar.ProducerOptions{Topic: "test"}, NoopTracer{}, nil)

	assert.Nil(t, producer)
	assert.ErrorIs(t, err, expectedErr)
}

func TestNewTracingProducer_Success(t *testing.T) {
	client := &mockClient{}

	producer, err := NewTracingProducer(client, pulsar.ProducerOptions{Topic: "test"}, NoopTracer{}, nil)

	assert.NoError(t, err)
	assert.NotNil(t, producer)
	assert.Equal(t, "test", producer.topic)
}

// =============================================================================
// NewTracingConsumer Error Path Tests
// =============================================================================

func TestNewTracingConsumer_SubscribeError(t *testing.T) {
	expectedErr := errors.New("subscription failed")
	client := &mockClient{subscribeErr: expectedErr}

	consumer, err := NewTracingConsumer(client, pulsar.ConsumerOptions{Topic: "test"}, NoopTracer{}, nil)

	assert.Nil(t, consumer)
	assert.ErrorIs(t, err, expectedErr)
}

func TestNewTracingConsumer_Success(t *testing.T) {
	client := &mockClient{}

	consumer, err := NewTracingConsumer(client, pulsar.ConsumerOptions{Topic: "test", SubscriptionName: "sub"}, NoopTracer{}, nil)

	assert.NoError(t, err)
	assert.NotNil(t, consumer)
	assert.Equal(t, "test", consumer.topic)
}

// =============================================================================
// mockProducer - 实现 pulsar.Producer 接口用于测试
// =============================================================================

type mockProducer struct {
	sendErr  error
	sendID   pulsar.MessageID
	asyncErr error
}

func (m *mockProducer) Topic() string { return "test-topic" }
func (m *mockProducer) Name() string  { return "mock-producer" }
func (m *mockProducer) Send(ctx context.Context, msg *pulsar.ProducerMessage) (pulsar.MessageID, error) {
	if m.sendErr != nil {
		return nil, m.sendErr
	}
	return m.sendID, nil
}
func (m *mockProducer) SendAsync(ctx context.Context, msg *pulsar.ProducerMessage, callback func(pulsar.MessageID, *pulsar.ProducerMessage, error)) {
	callback(m.sendID, msg, m.asyncErr)
}
func (m *mockProducer) LastSequenceID() int64                  { return 0 }
func (m *mockProducer) Flush() error                           { return nil }
func (m *mockProducer) FlushWithCtx(ctx context.Context) error { return nil }
func (m *mockProducer) Close()                                 {}

// =============================================================================
// mockConsumer - 实现 pulsar.Consumer 接口用于测试
// =============================================================================

type mockConsumer struct {
	receiveErr error
	receiveMsg pulsar.Message
	ackErr     error
	nackCalled bool
}

func (m *mockConsumer) Subscription() string                                { return "test-sub" }
func (m *mockConsumer) Unsubscribe() error                                  { return nil }
func (m *mockConsumer) UnsubscribeForce() error                             { return nil }
func (m *mockConsumer) GetLastMessageIDs() ([]pulsar.TopicMessageID, error) { return nil, nil }
func (m *mockConsumer) Receive(ctx context.Context) (pulsar.Message, error) {
	if m.receiveErr != nil {
		return nil, m.receiveErr
	}
	return m.receiveMsg, nil
}
func (m *mockConsumer) Chan() <-chan pulsar.ConsumerMessage                         { return nil }
func (m *mockConsumer) Ack(msg pulsar.Message) error                                { return m.ackErr }
func (m *mockConsumer) AckID(id pulsar.MessageID) error                             { return nil }
func (m *mockConsumer) AckIDList(ids []pulsar.MessageID) error                      { return nil }
func (m *mockConsumer) AckWithTxn(msg pulsar.Message, txn pulsar.Transaction) error { return nil }
func (m *mockConsumer) AckCumulative(msg pulsar.Message) error                      { return nil }
func (m *mockConsumer) AckIDCumulative(id pulsar.MessageID) error                   { return nil }
func (m *mockConsumer) ReconsumeLater(msg pulsar.Message, delay time.Duration)      {}
func (m *mockConsumer) ReconsumeLaterWithCustomProperties(msg pulsar.Message, props map[string]string, delay time.Duration) {
}
func (m *mockConsumer) Nack(msg pulsar.Message)           { m.nackCalled = true }
func (m *mockConsumer) NackID(id pulsar.MessageID)        {}
func (m *mockConsumer) Close()                            {}
func (m *mockConsumer) Seek(msgID pulsar.MessageID) error { return nil }
func (m *mockConsumer) SeekByTime(time time.Time) error   { return nil }
func (m *mockConsumer) Name() string                      { return "mock-consumer" }

// =============================================================================
// TracingProducer.Send Tests with Mock
// =============================================================================

func TestTracingProducer_Send_Success(t *testing.T) {
	mp := &mockProducer{}
	producer, err := WrapProducer(mp, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)
	msg := &pulsar.ProducerMessage{Payload: []byte("test")}

	id, sendErr := producer.Send(context.Background(), msg)

	assert.NoError(t, sendErr)
	assert.Nil(t, id) // mockProducer 返回 nil ID
}

func TestTracingProducer_Send_NilContext(t *testing.T) {
	var nilCtx context.Context
	mp := &mockProducer{}
	producer, err := WrapProducer(mp, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)
	msg := &pulsar.ProducerMessage{Payload: []byte("test")}

	// 传入 nil context，应该内部替换为 context.Background()
	id, sendErr := producer.Send(nilCtx, msg)

	assert.NoError(t, sendErr)
	assert.Nil(t, id)
}

func TestTracingProducer_Send_Error(t *testing.T) {
	expectedErr := errors.New("send failed")
	mp := &mockProducer{sendErr: expectedErr}
	producer, err := WrapProducer(mp, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)
	msg := &pulsar.ProducerMessage{Payload: []byte("test")}

	id, sendErr := producer.Send(context.Background(), msg)

	assert.Nil(t, id)
	assert.ErrorIs(t, sendErr, expectedErr)
}

// =============================================================================
// TracingProducer.SendAsync Tests with Mock
// =============================================================================

func TestTracingProducer_SendAsync_Success(t *testing.T) {
	mp := &mockProducer{}
	producer, err := WrapProducer(mp, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)
	msg := &pulsar.ProducerMessage{Payload: []byte("test")}

	var called bool
	var resultErr error
	producer.SendAsync(context.Background(), msg, func(id pulsar.MessageID, m *pulsar.ProducerMessage, err error) {
		called = true
		resultErr = err
	})

	assert.True(t, called)
	assert.NoError(t, resultErr)
}

func TestTracingProducer_SendAsync_NilContext(t *testing.T) {
	var nilCtx context.Context
	mp := &mockProducer{}
	producer, err := WrapProducer(mp, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)
	msg := &pulsar.ProducerMessage{Payload: []byte("test")}

	var called bool
	producer.SendAsync(nilCtx, msg, func(id pulsar.MessageID, m *pulsar.ProducerMessage, err error) {
		called = true
	})

	assert.True(t, called)
}

func TestTracingProducer_SendAsync_Error(t *testing.T) {
	expectedErr := errors.New("async send failed")
	mp := &mockProducer{asyncErr: expectedErr}
	producer, err := WrapProducer(mp, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)
	msg := &pulsar.ProducerMessage{Payload: []byte("test")}

	var resultErr error
	producer.SendAsync(context.Background(), msg, func(id pulsar.MessageID, m *pulsar.ProducerMessage, err error) {
		resultErr = err
	})

	assert.ErrorIs(t, resultErr, expectedErr)
}

func TestTracingProducer_SendAsync_NilCallback(t *testing.T) {
	mp := &mockProducer{}
	producer, err := WrapProducer(mp, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)
	msg := &pulsar.ProducerMessage{Payload: []byte("test")}

	// 不应 panic
	assert.NotPanics(t, func() {
		producer.SendAsync(context.Background(), msg, nil)
	})
}

// =============================================================================
// TracingConsumer.ReceiveWithContext Tests
// =============================================================================

func TestTracingConsumer_ReceiveWithContext_Success(t *testing.T) {
	msg := &mockMessage{properties: map[string]string{"key": "value"}}
	mc := &mockConsumer{receiveMsg: msg}
	consumer, err := WrapConsumer(mc, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	ctx, receivedMsg, recvErr := consumer.ReceiveWithContext(context.Background())

	assert.NoError(t, recvErr)
	assert.NotNil(t, ctx)
	assert.Equal(t, msg, receivedMsg)
}

func TestTracingConsumer_ReceiveWithContext_NilContext(t *testing.T) {
	var nilCtx context.Context
	msg := &mockMessage{properties: map[string]string{}}
	mc := &mockConsumer{receiveMsg: msg}
	consumer, err := WrapConsumer(mc, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	ctx, receivedMsg, recvErr := consumer.ReceiveWithContext(nilCtx)

	assert.NoError(t, recvErr)
	assert.NotNil(t, ctx)
	assert.Equal(t, msg, receivedMsg)
}

func TestTracingConsumer_ReceiveWithContext_Error(t *testing.T) {
	expectedErr := errors.New("receive failed")
	mc := &mockConsumer{receiveErr: expectedErr}
	consumer, err := WrapConsumer(mc, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	ctx, msg, recvErr := consumer.ReceiveWithContext(context.Background())

	assert.NotNil(t, ctx)
	assert.Nil(t, msg)
	assert.ErrorIs(t, recvErr, expectedErr)
}

// =============================================================================
// TracingConsumer.Consume Tests
// =============================================================================

func TestTracingConsumer_Consume_Success(t *testing.T) {
	msg := &mockMessage{properties: map[string]string{}}
	mc := &mockConsumer{receiveMsg: msg}
	consumer, err := WrapConsumer(mc, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	var handlerCalled bool
	consumeErr := consumer.Consume(context.Background(), func(ctx context.Context, m pulsar.Message) error {
		handlerCalled = true
		return nil
	})

	assert.NoError(t, consumeErr)
	assert.True(t, handlerCalled)
}

func TestTracingConsumer_Consume_ReceiveError(t *testing.T) {
	expectedErr := errors.New("receive failed")
	mc := &mockConsumer{receiveErr: expectedErr}
	consumer, err := WrapConsumer(mc, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	consumeErr := consumer.Consume(context.Background(), func(ctx context.Context, m pulsar.Message) error {
		return nil
	})

	assert.ErrorIs(t, consumeErr, expectedErr)
}

func TestTracingConsumer_Consume_HandlerError(t *testing.T) {
	msg := &mockMessage{properties: map[string]string{}}
	mc := &mockConsumer{receiveMsg: msg}
	consumer, err := WrapConsumer(mc, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	expectedErr := errors.New("handler failed")
	consumeErr := consumer.Consume(context.Background(), func(ctx context.Context, m pulsar.Message) error {
		return expectedErr
	})

	assert.ErrorIs(t, consumeErr, expectedErr)
}

func TestTracingConsumer_Consume_NilMessage(t *testing.T) {
	mc := &mockConsumer{receiveMsg: nil}
	consumer, err := WrapConsumer(mc, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	var handlerCalled bool
	consumeErr := consumer.Consume(context.Background(), func(ctx context.Context, m pulsar.Message) error {
		handlerCalled = true
		return nil
	})

	assert.NoError(t, consumeErr)
	assert.False(t, handlerCalled)
}

func TestTracingConsumer_Consume_AckError(t *testing.T) {
	// 验证 Ack 失败时：handler 返回 nil（处理成功），Ack 错误通过 span 属性记录
	msg := &mockMessage{properties: map[string]string{}}
	mc := &mockConsumer{receiveMsg: msg, ackErr: errors.New("ack failed")}
	consumer, err := WrapConsumer(mc, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	var handlerCalled bool
	consumeErr := consumer.Consume(context.Background(), func(ctx context.Context, m pulsar.Message) error {
		handlerCalled = true
		return nil
	})

	assert.NoError(t, consumeErr, "Ack 错误不应影响返回值")
	assert.True(t, handlerCalled)
}

func TestTracingConsumer_Consume_HandlerError_Nack(t *testing.T) {
	msg := &mockMessage{properties: map[string]string{}}
	mc := &mockConsumer{receiveMsg: msg}
	consumer, err := WrapConsumer(mc, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	expectedErr := errors.New("handler failed")
	consumeErr := consumer.Consume(context.Background(), func(ctx context.Context, m pulsar.Message) error {
		return expectedErr
	})

	assert.ErrorIs(t, consumeErr, expectedErr)
	assert.True(t, mc.nackCalled, "handler 失败时应调用 Nack")
}

// =============================================================================
// TracingConsumer.ConsumeLoop Tests
// =============================================================================

func TestTracingConsumer_ConsumeLoop_ContextCanceled(t *testing.T) {
	mc := &mockConsumer{}
	consumer, err := WrapConsumer(mc, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	loopErr := consumer.ConsumeLoop(ctx, func(ctx context.Context, m pulsar.Message) error {
		return nil
	})

	assert.ErrorIs(t, loopErr, context.Canceled)
}

func TestTracingConsumer_ConsumeLoopWithPolicy_NilHandler(t *testing.T) {
	mc := &mockConsumer{}
	consumer, err := WrapConsumer(mc, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	// nil handler 应立即返回 ErrNilHandler，而非进入无限重试循环
	loopErr := consumer.ConsumeLoopWithPolicy(context.Background(), nil, nil)
	assert.ErrorIs(t, loopErr, ErrNilHandler)
}

func TestTracingConsumer_ConsumeLoop_NilHandler(t *testing.T) {
	mc := &mockConsumer{}
	consumer, err := WrapConsumer(mc, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	// ConsumeLoop 委托 ConsumeLoopWithPolicy，同样应 fail-fast
	loopErr := consumer.ConsumeLoop(context.Background(), nil)
	assert.ErrorIs(t, loopErr, ErrNilHandler)
}

// =============================================================================
// TracingConsumer.Consume subscription attribute test
// =============================================================================

func TestTracingConsumer_Consume_IncludesSubscription(t *testing.T) {
	// 验证 Consume 方法正常工作（subscription 属性通过 span 记录，此处验证不 panic）
	msg := &mockMessage{properties: map[string]string{}}
	mc := &mockConsumer{receiveMsg: msg}
	consumer, err := WrapConsumer(mc, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	consumeErr := consumer.Consume(context.Background(), func(ctx context.Context, m pulsar.Message) error {
		return nil
	})

	assert.NoError(t, consumeErr)
}
