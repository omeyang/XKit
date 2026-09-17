package xkafka

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newTestConsumerWrapper creates a consumerWrapper with a mock client for testing.
func newTestConsumerWrapper(ctrl *gomock.Controller) (*consumerWrapper, *MockkafkaConsumerClient) {
	mock := NewMockkafkaConsumerClient(ctrl)
	return &consumerWrapper{
		client:  mock,
		options: defaultConsumerOptions(),
		groupID: "test-group",
	}, mock
}

// =============================================================================
// consumerWrapper.Consumer() Tests
// =============================================================================

func TestConsumerWrapper_Consumer_ReturnsRaw(t *testing.T) {
	w := &consumerWrapper{raw: nil}
	assert.Nil(t, w.Consumer())
}

// =============================================================================
// consumerWrapper.Health() Tests
// =============================================================================

func TestConsumerWrapper_Health_WithAssignment(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	topic := "test-topic"
	mock.EXPECT().Assignment().Return([]kafka.TopicPartition{
		{Topic: &topic, Partition: 0},
	}, nil)

	err := w.Health(context.Background())
	assert.NoError(t, err)
}

func TestConsumerWrapper_Health_NoAssignment_MetadataFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	mock.EXPECT().Assignment().Return([]kafka.TopicPartition{}, nil)
	mock.EXPECT().GetMetadata((*string)(nil), true, gomock.Any()).
		Return(&kafka.Metadata{}, nil)

	err := w.Health(context.Background())
	assert.NoError(t, err)
}

func TestConsumerWrapper_Health_NoAssignment_MetadataFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	mock.EXPECT().Assignment().Return([]kafka.TopicPartition{}, nil)
	mock.EXPECT().GetMetadata((*string)(nil), true, gomock.Any()).
		Return(nil, errors.New("broker down"))

	err := w.Health(context.Background())
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrHealthCheckFailed)
	assert.Contains(t, err.Error(), "broker down")
}

func TestConsumerWrapper_Health_AssignmentError(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	mock.EXPECT().Assignment().Return(nil, errors.New("assignment error"))

	err := w.Health(context.Background())
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrHealthCheckFailed)
}

func TestConsumerWrapper_Health_AfterClose(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	// Close the consumer first
	mock.EXPECT().Commit().Return(nil, nil)
	mock.EXPECT().Close().Return(nil)
	require.NoError(t, w.Close())

	err := w.Health(context.Background())
	assert.ErrorIs(t, err, ErrClosed)
}

func TestConsumerWrapper_Health_ContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	// The goroutine inside Health will call Assignment (and maybe GetMetadata).
	// Since we cancel immediately, the goroutine may still be running after the test.
	// Use AnyTimes() to avoid "unexpected call" panics in leaked goroutines.
	mock.EXPECT().Assignment().Return([]kafka.TopicPartition{}, nil).AnyTimes()
	mock.EXPECT().GetMetadata(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ *string, _ bool, _ int) (*kafka.Metadata, error) {
			time.Sleep(2 * time.Second)
			return &kafka.Metadata{}, nil
		}).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := w.Health(ctx)
	assert.ErrorIs(t, err, context.Canceled)

	// Wait briefly for the leaked goroutine to finish so it doesn't panic after test ends
	time.Sleep(100 * time.Millisecond)
}

func TestConsumerWrapper_Health_NilContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	topic := "t"
	mock.EXPECT().Assignment().Return([]kafka.TopicPartition{
		{Topic: &topic, Partition: 0},
	}, nil)

	err := w.Health(nil) //nolint:staticcheck
	assert.NoError(t, err)
}

// =============================================================================
// consumerWrapper.Stats() Tests
// =============================================================================

func TestConsumerWrapper_Stats_WithLag(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	topic := "test-topic"
	w.messagesConsumed.Store(50)
	w.bytesConsumed.Store(2500)
	w.errorsCount.Store(2)

	// calculateLag path
	mock.EXPECT().Assignment().Return([]kafka.TopicPartition{
		{Topic: &topic, Partition: 0},
	}, nil)
	mock.EXPECT().Committed(gomock.Any(), gomock.Any()).Return([]kafka.TopicPartition{
		{Topic: &topic, Partition: 0, Offset: 80},
	}, nil)
	mock.EXPECT().QueryWatermarkOffsets("test-topic", int32(0), gomock.Any()).
		Return(int64(0), int64(100), nil)

	stats := w.Stats()
	assert.Equal(t, int64(50), stats.MessagesConsumed)
	assert.Equal(t, int64(2500), stats.BytesConsumed)
	assert.Equal(t, int64(2), stats.Errors)
	assert.Equal(t, int64(20), stats.Lag) // 100 - 80 = 20
}

func TestConsumerWrapper_Stats_AfterClose(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	mock.EXPECT().Commit().Return(nil, nil)
	mock.EXPECT().Close().Return(nil)
	require.NoError(t, w.Close())

	stats := w.Stats()
	assert.Equal(t, int64(0), stats.Lag)
}

// =============================================================================
// consumerWrapper.calculateLag() Tests
// =============================================================================

func TestConsumerWrapper_CalculateLag_MultiplePartitions(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	topic := "test-topic"
	mock.EXPECT().Assignment().Return([]kafka.TopicPartition{
		{Topic: &topic, Partition: 0},
		{Topic: &topic, Partition: 1},
	}, nil)

	// Partition 0: committed=80, high=100 => lag=20
	mock.EXPECT().Committed(gomock.Any(), gomock.Any()).
		DoAndReturn(func(partitions []kafka.TopicPartition, _ int) ([]kafka.TopicPartition, error) {
			if partitions[0].Partition == 0 {
				return []kafka.TopicPartition{{Topic: &topic, Partition: 0, Offset: 80}}, nil
			}
			return []kafka.TopicPartition{{Topic: &topic, Partition: 1, Offset: 50}}, nil
		}).Times(2)
	mock.EXPECT().QueryWatermarkOffsets("test-topic", int32(0), gomock.Any()).
		Return(int64(0), int64(100), nil)
	mock.EXPECT().QueryWatermarkOffsets("test-topic", int32(1), gomock.Any()).
		Return(int64(0), int64(70), nil)

	lag := w.calculateLag()
	assert.Equal(t, int64(40), lag) // (100-80) + (70-50) = 40
}

func TestConsumerWrapper_CalculateLag_AssignmentError(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	mock.EXPECT().Assignment().Return(nil, errors.New("no assignment"))

	lag := w.calculateLag()
	assert.Equal(t, int64(0), lag)
}

func TestConsumerWrapper_CalculateLag_NoAssignment(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	mock.EXPECT().Assignment().Return([]kafka.TopicPartition{}, nil)

	lag := w.calculateLag()
	assert.Equal(t, int64(0), lag)
}

func TestConsumerWrapper_CalculateLag_CommittedError(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	topic := "test-topic"
	mock.EXPECT().Assignment().Return([]kafka.TopicPartition{
		{Topic: &topic, Partition: 0},
	}, nil)
	mock.EXPECT().Committed(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("committed error"))

	lag := w.calculateLag()
	assert.Equal(t, int64(0), lag)
}

func TestConsumerWrapper_CalculateLag_WatermarkError(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	topic := "test-topic"
	mock.EXPECT().Assignment().Return([]kafka.TopicPartition{
		{Topic: &topic, Partition: 0},
	}, nil)
	mock.EXPECT().Committed(gomock.Any(), gomock.Any()).Return([]kafka.TopicPartition{
		{Topic: &topic, Partition: 0, Offset: 80},
	}, nil)
	mock.EXPECT().QueryWatermarkOffsets("test-topic", int32(0), gomock.Any()).
		Return(int64(0), int64(0), errors.New("watermark error"))

	lag := w.calculateLag()
	assert.Equal(t, int64(0), lag)
}

func TestConsumerWrapper_CalculateLag_NegativeOffset(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	topic := "test-topic"
	mock.EXPECT().Assignment().Return([]kafka.TopicPartition{
		{Topic: &topic, Partition: 0},
	}, nil)
	mock.EXPECT().Committed(gomock.Any(), gomock.Any()).Return([]kafka.TopicPartition{
		{Topic: &topic, Partition: 0, Offset: -1001}, // kafka.OffsetInvalid
	}, nil)
	mock.EXPECT().QueryWatermarkOffsets("test-topic", int32(0), gomock.Any()).
		Return(int64(0), int64(100), nil)

	lag := w.calculateLag()
	assert.Equal(t, int64(0), lag) // negative offset => 0
}

func TestConsumerWrapper_CalculateLag_CaughtUp(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	topic := "test-topic"
	mock.EXPECT().Assignment().Return([]kafka.TopicPartition{
		{Topic: &topic, Partition: 0},
	}, nil)
	mock.EXPECT().Committed(gomock.Any(), gomock.Any()).Return([]kafka.TopicPartition{
		{Topic: &topic, Partition: 0, Offset: 100},
	}, nil)
	mock.EXPECT().QueryWatermarkOffsets("test-topic", int32(0), gomock.Any()).
		Return(int64(0), int64(100), nil)

	lag := w.calculateLag()
	assert.Equal(t, int64(0), lag)
}

// =============================================================================
// consumerWrapper.Close() Tests
// =============================================================================

func TestConsumerWrapper_Close_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	mock.EXPECT().Commit().Return(nil, nil)
	mock.EXPECT().Close().Return(nil)

	err := w.Close()
	assert.NoError(t, err)
}

func TestConsumerWrapper_Close_CommitError(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	mock.EXPECT().Commit().Return(nil, errors.New("commit failed"))
	mock.EXPECT().Close().Return(nil)

	err := w.Close()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "commit offset on close failed")
}

func TestConsumerWrapper_Close_CloseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	mock.EXPECT().Commit().Return(nil, nil)
	mock.EXPECT().Close().Return(errors.New("close failed"))

	err := w.Close()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "close failed")
}

func TestConsumerWrapper_Close_BothErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	mock.EXPECT().Commit().Return(nil, errors.New("commit failed"))
	mock.EXPECT().Close().Return(errors.New("close failed"))

	err := w.Close()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "commit failed")
	assert.Contains(t, err.Error(), "close failed")
}

func TestConsumerWrapper_Close_NoOffsetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	// ErrNoOffset is a normal situation (no offsets to commit)
	noOffsetErr := kafka.NewError(kafka.ErrNoOffset, "no offset", false)
	mock.EXPECT().Commit().Return(nil, noOffsetErr)
	mock.EXPECT().Close().Return(nil)

	err := w.Close()
	assert.NoError(t, err)
}

func TestConsumerWrapper_Close_Idempotent(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestConsumerWrapper(ctrl)

	mock.EXPECT().Commit().Return(nil, nil)
	mock.EXPECT().Close().Return(nil)

	err1 := w.Close()
	assert.NoError(t, err1)

	err2 := w.Close()
	assert.ErrorIs(t, err2, ErrClosed)
}
