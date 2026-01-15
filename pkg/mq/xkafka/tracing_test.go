package xkafka

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// NewTracingProducer Tests
// =============================================================================

func TestNewTracingProducer_NilConfig(t *testing.T) {
	producer, err := NewTracingProducer(nil)

	assert.Nil(t, producer)
	assert.ErrorIs(t, err, ErrNilConfig)
}

// =============================================================================
// NewTracingConsumer Tests
// =============================================================================

func TestNewTracingConsumer_NilConfig(t *testing.T) {
	consumer, err := NewTracingConsumer(nil, []string{"topic"})

	assert.Nil(t, consumer)
	assert.ErrorIs(t, err, ErrNilConfig)
}

// =============================================================================
// Type Tests
// =============================================================================

func TestTracingTypes(t *testing.T) {
	// 验证类型结构
	producer := &TracingProducer{}
	assert.Nil(t, producer.producerWrapper)

	consumer := &TracingConsumer{}
	assert.Nil(t, consumer.consumerWrapper)
}
