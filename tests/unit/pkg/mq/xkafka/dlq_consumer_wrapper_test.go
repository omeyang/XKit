package xkafka

import (
	"context"
	"errors"
	"testing"

	"github.com/omeyang/xkit/pkg/resilience/xretry"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// newTestDLQConsumer creates a dlqConsumer with mock clients for testing.
func newTestDLQConsumer(ctrl *gomock.Controller) (*dlqConsumer, *MockkafkaConsumerClient, *MockkafkaProducerClient) {
	consumerMock := NewMockkafkaConsumerClient(ctrl)
	producerMock := NewMockkafkaProducerClient(ctrl)

	cw := &consumerWrapper{
		client:  consumerMock,
		options: defaultConsumerOptions(),
		groupID: "test-group",
	}

	return &dlqConsumer{
		consumerWrapper: cw,
		policy: &DLQPolicy{
			DLQTopic:    "test-dlq",
			RetryPolicy: xretry.NewFixedRetry(3),
		},
		dlqProducer: producerMock,
		stats:       newDLQStatsCollector(),
	}, consumerMock, producerMock
}

// =============================================================================
// dlqConsumer.ConsumeWithRetry() Tests
// =============================================================================

func TestDLQConsumer_ConsumeWithRetry_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, _ := newTestDLQConsumer(ctrl)

	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: 0, Offset: 10},
		Value:          []byte("data"),
	}

	consumerMock.EXPECT().ReadMessage(gomock.Any()).Return(msg, nil)
	consumerMock.EXPECT().StoreMessage(msg).Return(nil, nil)

	handlerCalled := false
	handler := func(_ context.Context, m *kafka.Message) error {
		handlerCalled = true
		return nil
	}

	err := dc.ConsumeWithRetry(context.Background(), handler)
	assert.NoError(t, err)
	assert.True(t, handlerCalled)

	dlqStats := dc.DLQStats()
	assert.Equal(t, int64(1), dlqStats.TotalMessages)
}

func TestDLQConsumer_ConsumeWithRetry_Timeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, _ := newTestDLQConsumer(ctrl)

	timedOut := kafka.NewError(kafka.ErrTimedOut, "timed out", false)
	consumerMock.EXPECT().ReadMessage(gomock.Any()).Return(nil, timedOut)

	handler := func(_ context.Context, _ *kafka.Message) error {
		t.Fatal("handler should not be called on timeout")
		return nil
	}

	err := dc.ConsumeWithRetry(context.Background(), handler)
	assert.NoError(t, err)
}

func TestDLQConsumer_ConsumeWithRetry_NilHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, _ := newTestDLQConsumer(ctrl)

	err := dc.ConsumeWithRetry(context.Background(), nil)
	assert.ErrorIs(t, err, ErrNilHandler)
}

func TestDLQConsumer_ConsumeWithRetry_Closed(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, _ := newTestDLQConsumer(ctrl)
	dc.closed.Store(true)

	handler := func(_ context.Context, _ *kafka.Message) error { return nil }

	err := dc.ConsumeWithRetry(context.Background(), handler)
	assert.ErrorIs(t, err, ErrClosed)
}

func TestDLQConsumer_ConsumeWithRetry_ReadError(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, _ := newTestDLQConsumer(ctrl)

	consumerMock.EXPECT().ReadMessage(gomock.Any()).Return(nil, errors.New("read error"))

	handler := func(_ context.Context, _ *kafka.Message) error { return nil }

	err := dc.ConsumeWithRetry(context.Background(), handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read error")
}

func TestDLQConsumer_ConsumeWithRetry_HandlerFail_SendToDLQ(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, producerMock := newTestDLQConsumer(ctrl)

	// Set max retries to 0 so handler failure immediately goes to DLQ
	dc.policy.RetryPolicy = xretry.NewFixedRetry(0)

	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: 0, Offset: 10},
		Value:          []byte("data"),
	}

	consumerMock.EXPECT().ReadMessage(gomock.Any()).Return(msg, nil)

	// DLQ produce
	producerMock.EXPECT().Produce(gomock.Any(), gomock.Any()).DoAndReturn(
		func(dlqMsg *kafka.Message, deliveryChan chan kafka.Event) error {
			// Simulate successful delivery
			go func() {
				deliveryChan <- &kafka.Message{
					TopicPartition: kafka.TopicPartition{
						Topic: &dc.policy.DLQTopic,
					},
				}
			}()
			return nil
		},
	)

	// StoreMessage after DLQ success
	consumerMock.EXPECT().StoreMessage(msg).Return(nil, nil)

	handler := func(_ context.Context, _ *kafka.Message) error {
		return errors.New("permanent failure")
	}

	err := dc.ConsumeWithRetry(context.Background(), handler)
	assert.NoError(t, err)

	dlqStats := dc.DLQStats()
	assert.Equal(t, int64(1), dlqStats.TotalMessages)
	assert.Equal(t, int64(1), dlqStats.DeadLetterMessages)
}

func TestDLQConsumer_ConsumeWithRetry_HandlerFail_Retry_ThenDLQ(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, producerMock := newTestDLQConsumer(ctrl)

	// Max 2 retries, so attempt 1 triggers retry
	dc.policy.RetryPolicy = xretry.NewFixedRetry(2)

	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: 0, Offset: 10},
		Value:          []byte("data"),
	}

	consumerMock.EXPECT().ReadMessage(gomock.Any()).Return(msg, nil)

	// First call: handler fails, should retry (redeliver)
	producerMock.EXPECT().Produce(gomock.Any(), gomock.Any()).DoAndReturn(
		func(redeliverMsg *kafka.Message, deliveryChan chan kafka.Event) error {
			go func() {
				deliveryChan <- &kafka.Message{
					TopicPartition: kafka.TopicPartition{Topic: redeliverMsg.TopicPartition.Topic},
				}
			}()
			return nil
		},
	)
	// StoreMessage after redeliver
	consumerMock.EXPECT().StoreMessage(msg).Return(nil, nil)

	handler := func(_ context.Context, _ *kafka.Message) error {
		return errors.New("transient failure")
	}

	err := dc.ConsumeWithRetry(context.Background(), handler)
	assert.NoError(t, err)

	dlqStats := dc.DLQStats()
	assert.Equal(t, int64(1), dlqStats.RetriedMessages)
}

// =============================================================================
// dlqConsumer.SendToDLQ() Tests
// =============================================================================

func TestDLQConsumer_SendToDLQ_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, producerMock := newTestDLQConsumer(ctrl)

	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: 0, Offset: 10},
		Value:          []byte("data"),
	}

	producerMock.EXPECT().Produce(gomock.Any(), gomock.Any()).DoAndReturn(
		func(dlqMsg *kafka.Message, deliveryChan chan kafka.Event) error {
			go func() {
				deliveryChan <- &kafka.Message{
					TopicPartition: kafka.TopicPartition{Topic: &dc.policy.DLQTopic},
				}
			}()
			return nil
		},
	)
	consumerMock.EXPECT().StoreMessage(msg).Return(nil, nil)

	err := dc.SendToDLQ(context.Background(), msg, errors.New("manual dlq"))
	assert.NoError(t, err)

	dlqStats := dc.DLQStats()
	assert.Equal(t, int64(1), dlqStats.DeadLetterMessages)
}

func TestDLQConsumer_SendToDLQ_NilMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, _ := newTestDLQConsumer(ctrl)

	err := dc.SendToDLQ(context.Background(), nil, errors.New("reason"))
	assert.ErrorIs(t, err, ErrNilMessage)
}

func TestDLQConsumer_SendToDLQ_Closed(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, _ := newTestDLQConsumer(ctrl)
	dc.closed.Store(true)

	topic := "t"
	msg := &kafka.Message{TopicPartition: kafka.TopicPartition{Topic: &topic}}

	err := dc.SendToDLQ(context.Background(), msg, errors.New("reason"))
	assert.ErrorIs(t, err, ErrClosed)
}

func TestDLQConsumer_SendToDLQ_ProduceError(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, producerMock := newTestDLQConsumer(ctrl)

	topic := "t"
	msg := &kafka.Message{TopicPartition: kafka.TopicPartition{Topic: &topic}, Value: []byte("v")}

	producerMock.EXPECT().Produce(gomock.Any(), gomock.Any()).Return(errors.New("produce failed"))

	err := dc.SendToDLQ(context.Background(), msg, errors.New("reason"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "produce to DLQ topic")
}

func TestDLQConsumer_SendToDLQ_DeliveryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, producerMock := newTestDLQConsumer(ctrl)

	topic := "t"
	msg := &kafka.Message{TopicPartition: kafka.TopicPartition{Topic: &topic}, Value: []byte("v")}

	producerMock.EXPECT().Produce(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *kafka.Message, deliveryChan chan kafka.Event) error {
			go func() {
				deliveryErr := kafka.NewError(kafka.ErrMsgTimedOut, "delivery timed out", false)
				deliveryChan <- &kafka.Message{
					TopicPartition: kafka.TopicPartition{
						Topic: &dc.policy.DLQTopic,
						Error: deliveryErr,
					},
				}
			}()
			return nil
		},
	)

	err := dc.SendToDLQ(context.Background(), msg, errors.New("reason"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DLQ delivery failed")
}

func TestDLQConsumer_SendToDLQ_NilContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, producerMock := newTestDLQConsumer(ctrl)

	topic := "t"
	msg := &kafka.Message{TopicPartition: kafka.TopicPartition{Topic: &topic}, Value: []byte("v")}

	producerMock.EXPECT().Produce(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *kafka.Message, deliveryChan chan kafka.Event) error {
			go func() {
				deliveryChan <- &kafka.Message{
					TopicPartition: kafka.TopicPartition{Topic: &dc.policy.DLQTopic},
				}
			}()
			return nil
		},
	)
	consumerMock.EXPECT().StoreMessage(msg).Return(nil, nil)

	err := dc.SendToDLQ(nil, msg, errors.New("reason")) //nolint:staticcheck
	assert.NoError(t, err)
}

func TestDLQConsumer_SendToDLQ_CanceledContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, _ := newTestDLQConsumer(ctrl)

	topic := "t"
	msg := &kafka.Message{TopicPartition: kafka.TopicPartition{Topic: &topic}, Value: []byte("v")}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := dc.SendToDLQ(ctx, msg, errors.New("reason"))
	assert.ErrorIs(t, err, context.Canceled)
}

// =============================================================================
// dlqConsumer.DLQStats() Tests
// =============================================================================

func TestDLQConsumer_DLQStats_Accuracy(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, _ := newTestDLQConsumer(ctrl)

	dc.stats.incTotal()
	dc.stats.incTotal()
	dc.stats.incRetried()
	dc.stats.incDeadLetter("topic-a")
	dc.stats.incSuccessAfterRetry()

	stats := dc.DLQStats()
	assert.Equal(t, int64(2), stats.TotalMessages)
	assert.Equal(t, int64(1), stats.RetriedMessages)
	assert.Equal(t, int64(1), stats.DeadLetterMessages)
	assert.Equal(t, int64(1), stats.SuccessAfterRetry)
	assert.Equal(t, int64(1), stats.ByTopic["topic-a"])
}

// =============================================================================
// dlqConsumer.Close() Tests
// =============================================================================

func TestDLQConsumer_Close_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, producerMock := newTestDLQConsumer(ctrl)

	consumerMock.EXPECT().Commit().Return(nil, nil)
	consumerMock.EXPECT().Close().Return(nil)
	producerMock.EXPECT().Flush(gomock.Any()).Return(0)
	producerMock.EXPECT().Close()

	err := dc.Close()
	assert.NoError(t, err)
}

func TestDLQConsumer_Close_FlushesAndClosesDLQProducer(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, producerMock := newTestDLQConsumer(ctrl)

	consumerMock.EXPECT().Commit().Return(nil, nil)
	consumerMock.EXPECT().Close().Return(nil)
	producerMock.EXPECT().Flush(gomock.Any()).Return(0)
	producerMock.EXPECT().Close()

	err := dc.Close()
	assert.NoError(t, err)
}

func TestDLQConsumer_Close_DLQFlushTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, producerMock := newTestDLQConsumer(ctrl)

	consumerMock.EXPECT().Commit().Return(nil, nil)
	consumerMock.EXPECT().Close().Return(nil)
	producerMock.EXPECT().Flush(gomock.Any()).Return(3) // 3 remaining
	producerMock.EXPECT().Close()

	err := dc.Close()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrFlushTimeout)
}

func TestDLQConsumer_Close_Idempotent(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, producerMock := newTestDLQConsumer(ctrl)

	consumerMock.EXPECT().Commit().Return(nil, nil)
	consumerMock.EXPECT().Close().Return(nil)
	producerMock.EXPECT().Flush(gomock.Any()).Return(0)
	producerMock.EXPECT().Close()

	assert.NoError(t, dc.Close())
	assert.ErrorIs(t, dc.Close(), ErrClosed)
}
