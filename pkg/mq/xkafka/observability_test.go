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

// =============================================================================
// kafkaConsumerMessageAttrs Tests
// =============================================================================

func TestKafkaConsumerMessageAttrs_WithGroup(t *testing.T) {
	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: 2,
			Offset:    10,
		},
	}

	attrs := kafkaConsumerMessageAttrs(msg, "my-group")

	assert.Len(t, attrs, 5)
	assert.Equal(t, "messaging.system", attrs[0].Key)
	assert.Equal(t, "messaging.destination.name", attrs[1].Key)
	assert.Equal(t, "test-topic", attrs[1].Value)
	assert.Equal(t, "messaging.kafka.partition", attrs[2].Key)
	assert.Equal(t, "messaging.kafka.offset", attrs[3].Key)
	assert.Equal(t, "messaging.kafka.consumer.group", attrs[4].Key)
	assert.Equal(t, "my-group", attrs[4].Value)
}

func TestKafkaConsumerMessageAttrs_EmptyGroup(t *testing.T) {
	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: 0,
			Offset:    0,
		},
	}

	attrs := kafkaConsumerMessageAttrs(msg, "")

	// 空 consumerGroup 不应追加 group 属性
	assert.Len(t, attrs, 4)
	for _, a := range attrs {
		assert.NotEqual(t, "messaging.kafka.consumer.group", a.Key)
	}
}

func TestKafkaConsumerMessageAttrs_NilMessage(t *testing.T) {
	attrs := kafkaConsumerMessageAttrs(nil, "my-group")

	// nil msg 降级到 kafkaAttrs("")，但仍追加 group
	assert.Len(t, attrs, 2)
	assert.Equal(t, "messaging.system", attrs[0].Key)
	assert.Equal(t, "messaging.kafka.consumer.group", attrs[1].Key)
}
