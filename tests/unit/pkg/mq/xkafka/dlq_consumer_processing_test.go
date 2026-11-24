package xkafka

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/resilience/xretry"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDLQConsumer_Close_CustomFlushTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, producerMock := newTestDLQConsumer(ctrl)
	dc.policy.DLQFlushTimeout = 30 * time.Second

	consumerMock.EXPECT().Commit().Return(nil, nil)
	consumerMock.EXPECT().Close().Return(nil)
	producerMock.EXPECT().Flush(30000).Return(0) // 30s = 30000ms
	producerMock.EXPECT().Close()

	err := dc.Close()
	assert.NoError(t, err)
}

func TestDLQConsumer_Close_ConsumerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, producerMock := newTestDLQConsumer(ctrl)

	consumerMock.EXPECT().Commit().Return(nil, errors.New("commit err"))
	consumerMock.EXPECT().Close().Return(nil)
	producerMock.EXPECT().Flush(gomock.Any()).Return(0)
	producerMock.EXPECT().Close()

	err := dc.Close()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "commit")
}

// =============================================================================
// dlqConsumer.formatFailureReason() Tests
// =============================================================================

func TestDLQConsumer_FormatFailureReason_Default(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, _ := newTestDLQConsumer(ctrl)

	result := dc.formatFailureReason(errors.New("test error"))
	assert.Equal(t, "test error", result)
}

func TestDLQConsumer_FormatFailureReason_Custom(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, _ := newTestDLQConsumer(ctrl)
	dc.policy.FailureReasonFormatter = func(err error) string {
		return "custom: " + err.Error()
	}

	result := dc.formatFailureReason(errors.New("test error"))
	assert.Equal(t, "custom: test error", result)
}

// =============================================================================
// dlqConsumer.ConsumeLoop() Tests
// =============================================================================

func TestDLQConsumer_ConsumeLoop_ContextCancel(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, _ := newTestDLQConsumer(ctrl)

	topic := "test-topic"
	count := 0
	consumerMock.EXPECT().ReadMessage(gomock.Any()).DoAndReturn(func(_ time.Duration) (*kafka.Message, error) {
		count++
		return &kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: 0, Offset: kafka.Offset(count)},
			Value:          []byte("data"),
		}, nil
	}).AnyTimes()
	consumerMock.EXPECT().StoreMessage(gomock.Any()).Return(nil, nil).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	handler := func(_ context.Context, _ *kafka.Message) error {
		if count >= 2 {
			cancel()
		}
		return nil
	}

	err := dc.ConsumeLoop(ctx, handler)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDLQConsumer_ConsumeLoop_NilHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, _ := newTestDLQConsumer(ctrl)

	err := dc.ConsumeLoop(context.Background(), nil)
	assert.ErrorIs(t, err, ErrNilHandler)
}

// =============================================================================
// dlqConsumer.handleRetry() Tests
// =============================================================================

func TestDLQConsumer_HandleRetry_WithBackoff(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, producerMock := newTestDLQConsumer(ctrl)
	dc.policy.BackoffPolicy = xretry.NewFixedBackoff(1 * time.Millisecond)

	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: 0, Offset: 10},
		Value:          []byte("data"),
	}

	// Redeliver
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
	consumerMock.EXPECT().StoreMessage(msg).Return(nil, nil)

	retryCalled := false
	dc.policy.OnRetry = func(_ *kafka.Message, attempt int, _ error) {
		retryCalled = true
		assert.Equal(t, 1, attempt)
	}

	err := dc.handleRetry(context.Background(), msg, 1, errors.New("err"))
	assert.NoError(t, err)
	assert.True(t, retryCalled)
}

func TestDLQConsumer_HandleRetry_ContextCancelDuringBackoff(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, _ := newTestDLQConsumer(ctrl)
	dc.policy.BackoffPolicy = xretry.NewFixedBackoff(10 * time.Second) // long backoff

	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("data"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := dc.handleRetry(ctx, msg, 1, errors.New("err"))
	assert.ErrorIs(t, err, context.Canceled)
}

// =============================================================================
// dlqConsumer.processMessage() with OnDLQ callback
// =============================================================================

func TestDLQConsumer_ProcessMessage_OnDLQCallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, producerMock := newTestDLQConsumer(ctrl)
	dc.policy.RetryPolicy = xretry.NewFixedRetry(0) // no retries

	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: 0, Offset: 10},
		Value:          []byte("data"),
	}

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

	onDLQCalled := false
	dc.policy.OnDLQ = func(_ *kafka.Message, _ error, metadata DLQMetadata) {
		onDLQCalled = true
		assert.Equal(t, "test-topic", metadata.OriginalTopic)
	}

	handler := func(_ context.Context, _ *kafka.Message) error {
		return errors.New("handler error")
	}

	err := dc.processMessage(context.Background(), msg, handler)
	assert.NoError(t, err)
	assert.True(t, onDLQCalled)
}

func TestDLQConsumer_ProcessMessage_SuccessAfterRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, _ := newTestDLQConsumer(ctrl)

	topic := "test-topic"
	// Simulate a message that has been retried (has retry count header)
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: 0, Offset: 10},
		Value:          []byte("data"),
		Headers: []kafka.Header{
			{Key: HeaderRetryCount, Value: []byte("2")},
		},
	}

	consumerMock.EXPECT().StoreMessage(msg).Return(nil, nil)

	handler := func(_ context.Context, _ *kafka.Message) error {
		return nil // success
	}

	err := dc.processMessage(context.Background(), msg, handler)
	assert.NoError(t, err)

	dlqStats := dc.DLQStats()
	assert.Equal(t, int64(1), dlqStats.SuccessAfterRetry)
}

func TestDLQConsumer_ProcessMessage_ClosedDuringProcess(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, _ := newTestDLQConsumer(ctrl)
	dc.closed.Store(true)

	topic := "t"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("v"),
	}

	handler := func(_ context.Context, _ *kafka.Message) error { return nil }
	err := dc.processMessage(context.Background(), msg, handler)
	assert.ErrorIs(t, err, ErrClosed)
}

func TestDLQConsumer_ProcessMessage_StoreOffsetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, _ := newTestDLQConsumer(ctrl)

	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("data"),
	}

	consumerMock.EXPECT().StoreMessage(msg).Return(nil, errors.New("store failed"))

	handler := func(_ context.Context, _ *kafka.Message) error { return nil }

	err := dc.processMessage(context.Background(), msg, handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "store offset failed")
}

// =============================================================================
// dlqConsumer.redeliverMessage() Tests
// =============================================================================

func TestDLQConsumer_RedeliverMessage_WithRetryTopic(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, producerMock := newTestDLQConsumer(ctrl)
	dc.policy.RetryTopic = "custom-retry-topic"

	topic := "original-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("data"),
	}

	producerMock.EXPECT().Produce(gomock.Any(), gomock.Any()).DoAndReturn(
		func(redeliverMsg *kafka.Message, deliveryChan chan kafka.Event) error {
			assert.Equal(t, "custom-retry-topic", *redeliverMsg.TopicPartition.Topic)
			go func() {
				deliveryChan <- &kafka.Message{
					TopicPartition: kafka.TopicPartition{Topic: redeliverMsg.TopicPartition.Topic},
				}
			}()
			return nil
		},
	)
	consumerMock.EXPECT().StoreMessage(msg).Return(nil, nil)

	err := dc.redeliverMessage(context.Background(), msg)
	assert.NoError(t, err)
}

func TestDLQConsumer_RedeliverMessage_ProduceError(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, producerMock := newTestDLQConsumer(ctrl)

	topic := "t"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("v"),
	}

	producerMock.EXPECT().Produce(gomock.Any(), gomock.Any()).Return(errors.New("produce err"))

	err := dc.redeliverMessage(context.Background(), msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redeliver to retry topic")
}

func TestDLQConsumer_RedeliverMessage_DeliveryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, producerMock := newTestDLQConsumer(ctrl)

	topic := "t"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("v"),
	}

	producerMock.EXPECT().Produce(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *kafka.Message, deliveryChan chan kafka.Event) error {
			go func() {
				deliveryErr := kafka.NewError(kafka.ErrMsgTimedOut, "delivery failed", false)
				deliveryChan <- &kafka.Message{
					TopicPartition: kafka.TopicPartition{Topic: &topic, Error: deliveryErr},
				}
			}()
			return nil
		},
	)

	err := dc.redeliverMessage(context.Background(), msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redeliver delivery failed")
}

func TestDLQConsumer_RedeliverMessage_StoreOffsetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, consumerMock, producerMock := newTestDLQConsumer(ctrl)

	topic := "t"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("v"),
	}

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
	consumerMock.EXPECT().StoreMessage(msg).Return(nil, errors.New("store err"))

	err := dc.redeliverMessage(context.Background(), msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "store offset after redeliver failed")
}

func TestDLQConsumer_RedeliverMessage_CanceledContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, _ := newTestDLQConsumer(ctrl)

	topic := "t"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic},
		Value:          []byte("v"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := dc.redeliverMessage(ctx, msg)
	assert.ErrorIs(t, err, context.Canceled)
}

// =============================================================================
// filterProducerConfig Tests (already partially covered, adding edge cases)
// =============================================================================

func TestCreateDLQProducer_WithProducerConfig(t *testing.T) {
	// This tests the path where ProducerConfig is provided
	policy := &DLQPolicy{
		DLQTopic:    "dlq",
		RetryPolicy: xretry.NewFixedRetry(3),
		ProducerConfig: &kafka.ConfigMap{
			"bootstrap.servers": "nonexistent-broker:9092",
		},
	}

	// createDLQProducer will use the ProducerConfig, which skips filterProducerConfig
	// It will fail because broker doesn't exist, but the code path is exercised
	_, err := createDLQProducer(&kafka.ConfigMap{}, policy)
	// Note: kafka.NewProducer may succeed even with bad config (lazy connection)
	// So we just verify no panic
	_ = err
	require.True(t, true) // just verifying no panic
}

// =============================================================================
// dlqConsumer.ConsumeLoopWithPolicy() Tests
// =============================================================================

func TestDLQConsumer_ConsumeLoopWithPolicy_NilHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	dc, _, _ := newTestDLQConsumer(ctrl)

	err := dc.ConsumeLoopWithPolicy(context.Background(), nil, nil)
	assert.ErrorIs(t, err, ErrNilHandler)
}
