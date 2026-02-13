package xkafka

import (
	"testing"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// NewProducer Tests
// =============================================================================

func TestNewProducer_NilConfig(t *testing.T) {
	producer, err := NewProducer(nil)

	assert.Nil(t, producer)
	assert.ErrorIs(t, err, ErrNilConfig)
}

// =============================================================================
// NewConsumer Tests
// =============================================================================

func TestNewConsumer_NilConfig(t *testing.T) {
	consumer, err := NewConsumer(nil, []string{"topic"})

	assert.Nil(t, consumer)
	assert.ErrorIs(t, err, ErrNilConfig)
}

func TestNewConsumer_EmptyTopics(t *testing.T) {
	// 使用有效 config 测试空 topics 验证
	config := &kafka.ConfigMap{
		"bootstrap.servers": "localhost:9092",
		"group.id":          "test-group",
	}

	consumer, err := NewConsumer(config, nil)
	assert.Nil(t, consumer)
	assert.ErrorIs(t, err, ErrEmptyTopics)

	consumer, err = NewConsumer(config, []string{})
	assert.Nil(t, consumer)
	assert.ErrorIs(t, err, ErrEmptyTopics)
}

// =============================================================================
// Producer Options Tests
// =============================================================================

func TestProducerOptions_Default(t *testing.T) {
	opts := defaultProducerOptions()

	assert.NotNil(t, opts.Tracer)
	assert.NotNil(t, opts.Observer)
	assert.Equal(t, 10*time.Second, opts.FlushTimeout)
	assert.Equal(t, 5*time.Second, opts.HealthTimeout)
}

func TestWithProducerTracer(t *testing.T) {
	opts := defaultProducerOptions()
	tracer := NoopTracer{}

	WithProducerTracer(tracer)(opts)

	assert.Equal(t, tracer, opts.Tracer)
}

func TestWithProducerTracer_Nil(t *testing.T) {
	opts := defaultProducerOptions()
	original := opts.Tracer

	WithProducerTracer(nil)(opts)

	assert.Equal(t, original, opts.Tracer)
}

func TestWithProducerObserver(t *testing.T) {
	opts := defaultProducerOptions()
	observer := xmetrics.NoopObserver{}

	WithProducerObserver(observer)(opts)

	assert.Equal(t, observer, opts.Observer)
}

func TestWithProducerObserver_Nil(t *testing.T) {
	opts := defaultProducerOptions()
	original := opts.Observer

	WithProducerObserver(nil)(opts)

	assert.Equal(t, original, opts.Observer)
}

func TestWithProducerFlushTimeout(t *testing.T) {
	opts := defaultProducerOptions()

	WithProducerFlushTimeout(20 * time.Second)(opts)

	assert.Equal(t, 20*time.Second, opts.FlushTimeout)
}

func TestWithProducerFlushTimeout_Zero(t *testing.T) {
	opts := defaultProducerOptions()
	original := opts.FlushTimeout

	WithProducerFlushTimeout(0)(opts)

	assert.Equal(t, original, opts.FlushTimeout)
}

func TestWithProducerHealthTimeout(t *testing.T) {
	opts := defaultProducerOptions()

	WithProducerHealthTimeout(10 * time.Second)(opts)

	assert.Equal(t, 10*time.Second, opts.HealthTimeout)
}

func TestWithProducerHealthTimeout_Zero(t *testing.T) {
	opts := defaultProducerOptions()
	original := opts.HealthTimeout

	WithProducerHealthTimeout(0)(opts)

	assert.Equal(t, original, opts.HealthTimeout)
}

// =============================================================================
// Consumer Options Tests
// =============================================================================

func TestConsumerOptions_Default(t *testing.T) {
	opts := defaultConsumerOptions()

	assert.NotNil(t, opts.Tracer)
	assert.NotNil(t, opts.Observer)
	assert.Equal(t, 100*time.Millisecond, opts.PollTimeout)
	assert.Equal(t, 5*time.Second, opts.HealthTimeout)
}

func TestWithConsumerTracer(t *testing.T) {
	opts := defaultConsumerOptions()
	tracer := NoopTracer{}

	WithConsumerTracer(tracer)(opts)

	assert.Equal(t, tracer, opts.Tracer)
}

func TestWithConsumerTracer_Nil(t *testing.T) {
	opts := defaultConsumerOptions()
	original := opts.Tracer

	WithConsumerTracer(nil)(opts)

	assert.Equal(t, original, opts.Tracer)
}

func TestWithConsumerObserver(t *testing.T) {
	opts := defaultConsumerOptions()
	observer := xmetrics.NoopObserver{}

	WithConsumerObserver(observer)(opts)

	assert.Equal(t, observer, opts.Observer)
}

func TestWithConsumerObserver_Nil(t *testing.T) {
	opts := defaultConsumerOptions()
	original := opts.Observer

	WithConsumerObserver(nil)(opts)

	assert.Equal(t, original, opts.Observer)
}

func TestWithConsumerPollTimeout(t *testing.T) {
	opts := defaultConsumerOptions()

	WithConsumerPollTimeout(200 * time.Millisecond)(opts)

	assert.Equal(t, 200*time.Millisecond, opts.PollTimeout)
}

func TestWithConsumerPollTimeout_Zero(t *testing.T) {
	opts := defaultConsumerOptions()
	original := opts.PollTimeout

	WithConsumerPollTimeout(0)(opts)

	assert.Equal(t, original, opts.PollTimeout)
}

func TestWithConsumerHealthTimeout(t *testing.T) {
	opts := defaultConsumerOptions()

	WithConsumerHealthTimeout(10 * time.Second)(opts)

	assert.Equal(t, 10*time.Second, opts.HealthTimeout)
}

func TestWithConsumerHealthTimeout_Zero(t *testing.T) {
	opts := defaultConsumerOptions()
	original := opts.HealthTimeout

	WithConsumerHealthTimeout(0)(opts)

	assert.Equal(t, original, opts.HealthTimeout)
}

// =============================================================================
// Stats Tests
// =============================================================================

func TestProducerStats_ZeroValues(t *testing.T) {
	stats := ProducerStats{}

	assert.Zero(t, stats.MessagesProduced)
	assert.Zero(t, stats.BytesProduced)
	assert.Zero(t, stats.Errors)
	assert.Zero(t, stats.QueueLength)
}

func TestConsumerStats_ZeroValues(t *testing.T) {
	stats := ConsumerStats{}

	assert.Zero(t, stats.MessagesConsumed)
	assert.Zero(t, stats.BytesConsumed)
	assert.Zero(t, stats.Errors)
	assert.Zero(t, stats.Lag)
}
