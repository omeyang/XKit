package xkafka

import (
	"context"
	"testing"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// kafkaHeadersToMap Tests
// =============================================================================

func TestKafkaHeadersToMap_Empty(t *testing.T) {
	result := kafkaHeadersToMap(nil)

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestKafkaHeadersToMap_WithHeaders(t *testing.T) {
	headers := []kafka.Header{
		{Key: "key1", Value: []byte("value1")},
		{Key: "key2", Value: []byte("value2")},
	}

	result := kafkaHeadersToMap(headers)

	assert.Len(t, result, 2)
	assert.Equal(t, "value1", result["key1"])
	assert.Equal(t, "value2", result["key2"])
}

// =============================================================================
// injectKafkaTrace Tests
// =============================================================================

func TestInjectKafkaTrace_NilTracer(t *testing.T) {
	msg := &kafka.Message{}

	assert.NotPanics(t, func() {
		injectKafkaTrace(context.Background(), nil, msg)
	})
}

func TestInjectKafkaTrace_NilMessage(t *testing.T) {
	tracer := NoopTracer{}

	assert.NotPanics(t, func() {
		injectKafkaTrace(context.Background(), tracer, nil)
	})
}

func TestInjectKafkaTrace_ExistingHeaders(t *testing.T) {
	tracer := NoopTracer{}
	msg := &kafka.Message{
		Headers: []kafka.Header{
			{Key: "existing-key", Value: []byte("existing-value")},
		},
	}

	injectKafkaTrace(context.Background(), tracer, msg)

	// 现有 Headers 应保留
	found := false
	for _, h := range msg.Headers {
		if h.Key == "existing-key" && string(h.Value) == "existing-value" {
			found = true
			break
		}
	}
	assert.True(t, found, "existing header should be preserved")
}

// =============================================================================
// extractKafkaTrace Tests
// =============================================================================

func TestExtractKafkaTrace_NilTracer(t *testing.T) {
	ctx := context.Background()

	result := extractKafkaTrace(ctx, nil, nil)

	assert.Equal(t, ctx, result)
}

func TestExtractKafkaTrace_NilMessage(t *testing.T) {
	ctx := context.Background()
	tracer := NoopTracer{}

	result := extractKafkaTrace(ctx, tracer, nil)

	assert.Equal(t, ctx, result)
}

// =============================================================================
// topicFromKafkaMessage Tests
// =============================================================================

func TestTopicFromKafkaMessage_Nil(t *testing.T) {
	topic := topicFromKafkaMessage(nil)

	assert.Empty(t, topic)
}

func TestTopicFromKafkaMessage_NilTopic(t *testing.T) {
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic: nil,
		},
	}

	topic := topicFromKafkaMessage(msg)

	assert.Empty(t, topic)
}

func TestTopicFromKafkaMessage_WithTopic(t *testing.T) {
	topicName := "my-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic: &topicName,
		},
	}

	topic := topicFromKafkaMessage(msg)

	assert.Equal(t, "my-topic", topic)
}

func TestTopicFromKafkaMessage_EmptyTopic(t *testing.T) {
	topicName := ""
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic: &topicName,
		},
	}

	topic := topicFromKafkaMessage(msg)

	assert.Equal(t, "", topic)
}

// =============================================================================
// Additional kafkaHeadersToMap Tests
// =============================================================================

func TestKafkaHeadersToMap_DuplicateKeys(t *testing.T) {
	// 如果有重复 key，后面的会覆盖前面的
	headers := []kafka.Header{
		{Key: "key1", Value: []byte("value1")},
		{Key: "key1", Value: []byte("value2")},
	}

	result := kafkaHeadersToMap(headers)

	assert.Len(t, result, 1)
	assert.Equal(t, "value2", result["key1"])
}

func TestKafkaHeadersToMap_EmptyValue(t *testing.T) {
	headers := []kafka.Header{
		{Key: "key1", Value: []byte("")},
	}

	result := kafkaHeadersToMap(headers)

	assert.Equal(t, "", result["key1"])
}

func TestKafkaHeadersToMap_NilValue(t *testing.T) {
	headers := []kafka.Header{
		{Key: "key1", Value: nil},
	}

	result := kafkaHeadersToMap(headers)

	assert.Equal(t, "", result["key1"])
}

func TestKafkaHeadersToMap_SingleHeader(t *testing.T) {
	headers := []kafka.Header{
		{Key: "traceparent", Value: []byte("00-1234-5678-01")},
	}

	result := kafkaHeadersToMap(headers)

	assert.Len(t, result, 1)
	assert.Equal(t, "00-1234-5678-01", result["traceparent"])
}

// =============================================================================
// Additional extractKafkaTrace Tests
// =============================================================================

func TestExtractKafkaTrace_WithHeaders(t *testing.T) {
	ctx := context.Background()
	tracer := NoopTracer{}
	topicName := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic: &topicName,
		},
		Headers: []kafka.Header{
			{Key: "traceparent", Value: []byte("00-1234-5678-01")},
		},
	}

	result := extractKafkaTrace(ctx, tracer, msg)

	assert.NotNil(t, result)
}

// =============================================================================
// Additional injectKafkaTrace Tests
// =============================================================================

func TestInjectKafkaTrace_EmptyMessage(t *testing.T) {
	tracer := NoopTracer{}
	msg := &kafka.Message{}

	assert.NotPanics(t, func() {
		injectKafkaTrace(context.Background(), tracer, msg)
	})
}
