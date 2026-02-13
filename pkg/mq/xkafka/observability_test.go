package xkafka

import (
	"testing"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// kafkaAttrs Tests
// =============================================================================

func TestKafkaAttrs_WithTopic(t *testing.T) {
	attrs := kafkaAttrs("test-topic")

	assert.Len(t, attrs, 2)
	assert.Equal(t, "messaging.system", attrs[0].Key)
	assert.Equal(t, "kafka", attrs[0].Value)
	assert.Equal(t, "messaging.destination.name", attrs[1].Key)
	assert.Equal(t, "test-topic", attrs[1].Value)
}

func TestKafkaAttrs_EmptyTopic(t *testing.T) {
	attrs := kafkaAttrs("")

	assert.Len(t, attrs, 1)
	assert.Equal(t, "messaging.system", attrs[0].Key)
	assert.Equal(t, "kafka", attrs[0].Value)
}

func TestComponentName(t *testing.T) {
	assert.Equal(t, "xkafka", componentName)
}

// =============================================================================
// kafkaMessageAttrs Tests
// =============================================================================

func TestKafkaMessageAttrs_WithMessage(t *testing.T) {
	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: 3,
			Offset:    42,
		},
	}

	attrs := kafkaMessageAttrs(msg)

	assert.Len(t, attrs, 4)
	assert.Equal(t, "messaging.system", attrs[0].Key)
	assert.Equal(t, "kafka", attrs[0].Value)
	assert.Equal(t, "messaging.destination.name", attrs[1].Key)
	assert.Equal(t, "test-topic", attrs[1].Value)
	assert.Equal(t, "messaging.kafka.partition", attrs[2].Key)
	assert.Equal(t, "3", attrs[2].Value)
	assert.Equal(t, "messaging.kafka.offset", attrs[3].Key)
	assert.Equal(t, "42", attrs[3].Value)
}

func TestKafkaMessageAttrs_NilMessage(t *testing.T) {
	attrs := kafkaMessageAttrs(nil)

	assert.Len(t, attrs, 1)
	assert.Equal(t, "messaging.system", attrs[0].Key)
}
