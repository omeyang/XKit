package xkafka

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// newTestTracingProducer creates a TracingProducer with a mock client for testing.
func newTestTracingProducer(ctrl *gomock.Controller) (*TracingProducer, *MockkafkaProducerClient) {
	mock := NewMockkafkaProducerClient(ctrl)
	pw := &producerWrapper{
		client:  mock,
		options: defaultProducerOptions(),
	}
	return &TracingProducer{producerWrapper: pw}, mock
}

// newTestTracingConsumer creates a TracingConsumer with a mock consumer client for testing.
func newTestTracingConsumer(ctrl *gomock.Controller) (*TracingConsumer, *MockkafkaConsumerClient) {
	mock := NewMockkafkaConsumerClient(ctrl)
	cw := &consumerWrapper{
		client:  mock,
		options: defaultConsumerOptions(),
		groupID: "test-group",
	}
	return &TracingConsumer{consumerWrapper: cw}, mock
}

// =============================================================================
// TracingProducer.Produce() Tests
// =============================================================================

func TestTracingProducer_Produce_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	tp, mock := newTestTracingProducer(ctrl)

	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("hello"),
	}

	mock.EXPECT().Produce(msg, gomock.Any()).Return(nil)
	mock.EXPECT().Len().Return(0).AnyTimes()

	err := tp.Produce(context.Background(), msg, nil)
	assert.NoError(t, err)

	stats := tp.Stats()
	assert.Equal(t, int64(1), stats.MessagesProduced)
	assert.Equal(t, int64(5), stats.BytesProduced)
	assert.Equal(t, int64(0), stats.Errors)
}

func TestTracingProducer_Produce_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	tp, mock := newTestTracingProducer(ctrl)

	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("hello"),
	}

	mock.EXPECT().Produce(msg, gomock.Any()).Return(errors.New("queue full"))
	mock.EXPECT().Len().Return(0).AnyTimes()

	err := tp.Produce(context.Background(), msg, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "queue full")

	stats := tp.Stats()
	assert.Equal(t, int64(0), stats.MessagesProduced)
	assert.Equal(t, int64(1), stats.Errors)
}

func TestTracingProducer_Produce_NilMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	tp, _ := newTestTracingProducer(ctrl)

	err := tp.Produce(context.Background(), nil, nil)
	assert.ErrorIs(t, err, ErrNilMessage)
}

func TestTracingProducer_Produce_NilContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	tp, mock := newTestTracingProducer(ctrl)

	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("data"),
	}

	mock.EXPECT().Produce(gomock.Any(), gomock.Any()).Return(nil)

	err := tp.Produce(nil, msg, nil) //nolint:staticcheck
	assert.NoError(t, err)
}

func TestTracingProducer_Produce_WithDeliveryChan(t *testing.T) {
	ctrl := gomock.NewController(t)
	tp, mock := newTestTracingProducer(ctrl)

	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("data"),
	}
	deliveryChan := make(chan kafka.Event, 1)

	mock.EXPECT().Produce(gomock.Any(), gomock.Any()).Return(nil)

	err := tp.Produce(context.Background(), msg, deliveryChan)
	assert.NoError(t, err)
}

// =============================================================================
// TracingConsumer.ReadMessage() Tests
// =============================================================================

func TestTracingConsumer_ReadMessage_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	tc, mock := newTestTracingConsumer(ctrl)

	topic := "test-topic"
	expectedMsg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: 0, Offset: 42},
		Value:          []byte("message-body"),
	}

	mock.EXPECT().ReadMessage(gomock.Any()).Return(expectedMsg, nil)

	ctx, msg, err := tc.ReadMessage(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, ctx)
	assert.Equal(t, expectedMsg, msg)

	// Verify stats counters directly (avoid Stats() which triggers calculateLag)
	assert.Equal(t, int64(1), tc.messagesConsumed.Load())
	assert.Equal(t, int64(12), tc.bytesConsumed.Load())
}

func TestTracingConsumer_ReadMessage_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	tc, mock := newTestTracingConsumer(ctrl)

	mock.EXPECT().ReadMessage(gomock.Any()).
		Return(nil, errors.New("fatal error"))

	_, _, err := tc.ReadMessage(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fatal error")
}

func TestTracingConsumer_ReadMessage_Timeout_ThenSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	tc, mock := newTestTracingConsumer(ctrl)

	topic := "test-topic"
	expectedMsg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("data"),
	}

	timedOut := kafka.NewError(kafka.ErrTimedOut, "timed out", false)

	gomock.InOrder(
		mock.EXPECT().ReadMessage(gomock.Any()).Return(nil, timedOut),
		mock.EXPECT().ReadMessage(gomock.Any()).Return(expectedMsg, nil),
	)

	_, msg, err := tc.ReadMessage(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, expectedMsg, msg)
}

func TestTracingConsumer_ReadMessage_Closed(t *testing.T) {
	ctrl := gomock.NewController(t)
	tc, _ := newTestTracingConsumer(ctrl)

	tc.closed.Store(true)

	_, _, err := tc.ReadMessage(context.Background())
	assert.ErrorIs(t, err, ErrClosed)
}

func TestTracingConsumer_ReadMessage_ContextCanceled(t *testing.T) {
	ctrl := gomock.NewController(t)
	tc, mock := newTestTracingConsumer(ctrl)

	timedOut := kafka.NewError(kafka.ErrTimedOut, "timed out", false)

	ctx, cancel := context.WithCancel(context.Background())
	mock.EXPECT().ReadMessage(gomock.Any()).DoAndReturn(func(_ time.Duration) (*kafka.Message, error) {
		cancel()
		return nil, timedOut
	}).AnyTimes()

	_, _, err := tc.ReadMessage(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestTracingConsumer_ReadMessage_NilContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	tc, mock := newTestTracingConsumer(ctrl)

	topic := "test-topic"
	mock.EXPECT().ReadMessage(gomock.Any()).Return(&kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("data"),
	}, nil)

	ctx, msg, err := tc.ReadMessage(nil) //nolint:staticcheck
	assert.NoError(t, err)
	assert.NotNil(t, ctx)
	assert.NotNil(t, msg)
}

// =============================================================================
// TracingConsumer.Consume() Tests
// =============================================================================

func TestTracingConsumer_Consume_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	tc, mock := newTestTracingConsumer(ctrl)

	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: 0, Offset: 10},
		Value:          []byte("data"),
	}

	mock.EXPECT().ReadMessage(gomock.Any()).Return(msg, nil)
	mock.EXPECT().StoreMessage(msg).Return(nil, nil)

	handlerCalled := false
	handler := func(_ context.Context, m *kafka.Message) error {
		handlerCalled = true
		assert.Equal(t, msg, m)
		return nil
	}

	err := tc.Consume(context.Background(), handler)
	assert.NoError(t, err)
	assert.True(t, handlerCalled)
}

func TestTracingConsumer_Consume_HandlerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	tc, mock := newTestTracingConsumer(ctrl)

	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("data"),
	}

	mock.EXPECT().ReadMessage(gomock.Any()).Return(msg, nil)

	handler := func(_ context.Context, _ *kafka.Message) error {
		return errors.New("handler failed")
	}

	err := tc.Consume(context.Background(), handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handler failed")
}

func TestTracingConsumer_Consume_NilHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	tc, _ := newTestTracingConsumer(ctrl)

	err := tc.Consume(context.Background(), nil)
	assert.ErrorIs(t, err, ErrNilHandler)
}

func TestTracingConsumer_Consume_StoreMessageError(t *testing.T) {
	ctrl := gomock.NewController(t)
	tc, mock := newTestTracingConsumer(ctrl)

	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("data"),
	}

	mock.EXPECT().ReadMessage(gomock.Any()).Return(msg, nil)
	mock.EXPECT().StoreMessage(msg).Return(nil, errors.New("store failed"))

	handler := func(_ context.Context, _ *kafka.Message) error {
		return nil
	}

	err := tc.Consume(context.Background(), handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "store offset failed")
}

func TestTracingConsumer_Consume_ClosedDuringConsume(t *testing.T) {
	ctrl := gomock.NewController(t)
	tc, mock := newTestTracingConsumer(ctrl)

	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("data"),
	}

	mock.EXPECT().ReadMessage(gomock.Any()).DoAndReturn(func(_ time.Duration) (*kafka.Message, error) {
		// Close while reading
		tc.closed.Store(true)
		return msg, nil
	})

	handler := func(_ context.Context, _ *kafka.Message) error {
		return nil
	}

	err := tc.Consume(context.Background(), handler)
	assert.ErrorIs(t, err, ErrClosed)
}

// =============================================================================
// TracingConsumer.ConsumeLoop() Tests
// =============================================================================

func TestTracingConsumer_ConsumeLoop_ContextCancel(t *testing.T) {
	ctrl := gomock.NewController(t)
	tc, mock := newTestTracingConsumer(ctrl)

	topic := "test-topic"
	count := 0
	mock.EXPECT().ReadMessage(gomock.Any()).DoAndReturn(func(_ time.Duration) (*kafka.Message, error) {
		count++
		return &kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic},
			Value:          []byte("data"),
		}, nil
	}).AnyTimes()
	mock.EXPECT().StoreMessage(gomock.Any()).Return(nil, nil).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())

	handler := func(_ context.Context, _ *kafka.Message) error {
		if count >= 3 {
			cancel()
		}
		return nil
	}

	err := tc.ConsumeLoop(ctx, handler)
	assert.ErrorIs(t, err, context.Canceled)
	assert.GreaterOrEqual(t, count, 3)
}

func TestTracingConsumer_ConsumeLoop_NilHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	tc, _ := newTestTracingConsumer(ctrl)

	err := tc.ConsumeLoop(context.Background(), nil)
	assert.ErrorIs(t, err, ErrNilHandler)
}

func TestTracingConsumer_ConsumeLoopWithPolicy_NilHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	tc, _ := newTestTracingConsumer(ctrl)

	err := tc.ConsumeLoopWithPolicy(context.Background(), nil, nil)
	assert.ErrorIs(t, err, ErrNilHandler)
}

func TestTracingConsumer_ConsumeLoopWithPolicy_NilContext(t *testing.T) {
	// The nil ctx path is trivially handled (defaults to Background).
	// Just verify the nil handler check works with nil ctx.
	ctrl := gomock.NewController(t)
	tc, _ := newTestTracingConsumer(ctrl)

	err := tc.ConsumeLoopWithPolicy(nil, nil, nil) //nolint:staticcheck
	assert.ErrorIs(t, err, ErrNilHandler)
}

// =============================================================================
// TracingConsumer.Close() Tests
// =============================================================================

func TestTracingConsumer_Close_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	tc, mock := newTestTracingConsumer(ctrl)

	mock.EXPECT().Commit().Return(nil, nil)
	mock.EXPECT().Close().Return(nil)

	err := tc.Close()
	assert.NoError(t, err)
}

func TestTracingConsumer_Close_Idempotent(t *testing.T) {
	ctrl := gomock.NewController(t)
	tc, mock := newTestTracingConsumer(ctrl)

	mock.EXPECT().Commit().Return(nil, nil)
	mock.EXPECT().Close().Return(nil)

	assert.NoError(t, tc.Close())
	assert.ErrorIs(t, tc.Close(), ErrClosed)
}
