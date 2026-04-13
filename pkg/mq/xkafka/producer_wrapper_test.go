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

// newTestProducerWrapper creates a producerWrapper with a mock client for testing.
func newTestProducerWrapper(ctrl *gomock.Controller) (*producerWrapper, *MockkafkaProducerClient) {
	mock := NewMockkafkaProducerClient(ctrl)
	return &producerWrapper{
		client:  mock,
		options: defaultProducerOptions(),
	}, mock
}

// =============================================================================
// producerWrapper.Producer() Tests
// =============================================================================

func TestProducerWrapper_Producer_ReturnsRaw(t *testing.T) {
	w := &producerWrapper{raw: nil}
	assert.Nil(t, w.Producer())
}

// =============================================================================
// producerWrapper.Health() Tests
// =============================================================================

func TestProducerWrapper_Health_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestProducerWrapper(ctrl)

	mock.EXPECT().GetMetadata((*string)(nil), true, gomock.Any()).
		Return(&kafka.Metadata{}, nil)

	err := w.Health(context.Background())
	assert.NoError(t, err)
}

func TestProducerWrapper_Health_MetadataFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestProducerWrapper(ctrl)

	mock.EXPECT().GetMetadata((*string)(nil), true, gomock.Any()).
		Return(nil, errors.New("broker unavailable"))

	err := w.Health(context.Background())
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrHealthCheckFailed)
	assert.Contains(t, err.Error(), "broker unavailable")
}

func TestProducerWrapper_Health_ContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestProducerWrapper(ctrl)

	// GetMetadata blocks forever; context cancel should return first
	mock.EXPECT().GetMetadata((*string)(nil), true, gomock.Any()).
		DoAndReturn(func(_ *string, _ bool, _ int) (*kafka.Metadata, error) {
			time.Sleep(5 * time.Second)
			return &kafka.Metadata{}, nil
		}).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := w.Health(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestProducerWrapper_Health_AfterClose(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestProducerWrapper(ctrl)

	// Close the producer first
	mock.EXPECT().Flush(gomock.Any()).Return(0)
	mock.EXPECT().Close()
	require.NoError(t, w.Close())

	err := w.Health(context.Background())
	assert.ErrorIs(t, err, ErrClosed)
}

func TestProducerWrapper_Health_NilContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestProducerWrapper(ctrl)

	mock.EXPECT().GetMetadata((*string)(nil), true, gomock.Any()).
		Return(&kafka.Metadata{}, nil)

	err := w.Health(nil) //nolint:staticcheck
	assert.NoError(t, err)
}

func TestProducerWrapper_Health_ClosedDuringWait(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestProducerWrapper(ctrl)

	// Simulate: Health goroutine acquires lock, but closed was set while waiting
	w.closed.Store(true)

	// GetMetadata should not be called because closed check happens inside goroutine
	_ = mock // not expecting any calls

	err := w.Health(context.Background())
	assert.ErrorIs(t, err, ErrClosed)
}

// =============================================================================
// producerWrapper.Stats() Tests
// =============================================================================

func TestProducerWrapper_Stats_ReturnsQueueLength(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestProducerWrapper(ctrl)

	mock.EXPECT().Len().Return(42)

	stats := w.Stats()
	assert.Equal(t, 42, stats.QueueLength)
}

func TestProducerWrapper_Stats_AfterClose(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestProducerWrapper(ctrl)

	mock.EXPECT().Flush(gomock.Any()).Return(0)
	mock.EXPECT().Close()
	require.NoError(t, w.Close())

	stats := w.Stats()
	assert.Equal(t, 0, stats.QueueLength)
}

func TestProducerWrapper_Stats_WithProducedMessages(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestProducerWrapper(ctrl)

	w.messagesProduced.Store(100)
	w.bytesProduced.Store(5000)
	w.errors.Store(3)

	mock.EXPECT().Len().Return(5)

	stats := w.Stats()
	assert.Equal(t, int64(100), stats.MessagesProduced)
	assert.Equal(t, int64(5000), stats.BytesProduced)
	assert.Equal(t, int64(3), stats.Errors)
	assert.Equal(t, 5, stats.QueueLength)
}

// =============================================================================
// producerWrapper.Close() Tests
// =============================================================================

func TestProducerWrapper_Close_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestProducerWrapper(ctrl)

	mock.EXPECT().Flush(gomock.Any()).Return(0)
	mock.EXPECT().Close()

	err := w.Close()
	assert.NoError(t, err)
}

func TestProducerWrapper_Close_FlushTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestProducerWrapper(ctrl)

	mock.EXPECT().Flush(gomock.Any()).Return(5) // 5 messages remaining
	mock.EXPECT().Close()

	err := w.Close()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrFlushTimeout)
	assert.Contains(t, err.Error(), "5 messages still in queue")
}

func TestProducerWrapper_Close_Idempotent(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestProducerWrapper(ctrl)

	mock.EXPECT().Flush(gomock.Any()).Return(0)
	mock.EXPECT().Close()

	err1 := w.Close()
	assert.NoError(t, err1)

	err2 := w.Close()
	assert.ErrorIs(t, err2, ErrClosed)
}

func TestProducerWrapper_Close_UsesFlushTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	w, mock := newTestProducerWrapper(ctrl)
	w.options.FlushTimeout = 20 * time.Second

	mock.EXPECT().Flush(20000).Return(0) // 20s = 20000ms
	mock.EXPECT().Close()

	err := w.Close()
	assert.NoError(t, err)
}
