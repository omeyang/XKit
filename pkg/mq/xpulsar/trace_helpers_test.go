package xpulsar

import (
	"context"
	"testing"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// injectPulsarTrace Tests
// =============================================================================

func TestInjectPulsarTrace_NilTracer(t *testing.T) {
	msg := &pulsar.ProducerMessage{}

	// 不应 panic
	assert.NotPanics(t, func() {
		injectPulsarTrace(context.Background(), nil, msg)
	})
}

func TestInjectPulsarTrace_NilMessage(t *testing.T) {
	tracer := NoopTracer{}

	assert.NotPanics(t, func() {
		injectPulsarTrace(context.Background(), tracer, nil)
	})
}

func TestInjectPulsarTrace_NilProperties(t *testing.T) {
	tracer := NoopTracer{}
	msg := &pulsar.ProducerMessage{}

	injectPulsarTrace(context.Background(), tracer, msg)

	// Properties 应被初始化
	assert.NotNil(t, msg.Properties)
}

func TestInjectPulsarTrace_ExistingProperties(t *testing.T) {
	tracer := NoopTracer{}
	msg := &pulsar.ProducerMessage{
		Properties: map[string]string{
			"existing-key": "existing-value",
		},
	}

	injectPulsarTrace(context.Background(), tracer, msg)

	// 现有属性应保留
	assert.Equal(t, "existing-value", msg.Properties["existing-key"])
}

// =============================================================================
// extractPulsarTrace Tests
// =============================================================================

func TestExtractPulsarTrace_NilTracer(t *testing.T) {
	ctx := context.Background()

	result := extractPulsarTrace(ctx, nil, nil)

	assert.Equal(t, ctx, result)
}

func TestExtractPulsarTrace_NilMessage(t *testing.T) {
	ctx := context.Background()
	tracer := NoopTracer{}

	result := extractPulsarTrace(ctx, tracer, nil)

	assert.Equal(t, ctx, result)
}

// =============================================================================
// topicFromConsumerOptions Tests
// =============================================================================

func TestTopicFromConsumerOptions_SingleTopic(t *testing.T) {
	opts := pulsar.ConsumerOptions{
		Topic: "my-topic",
	}

	topic := topicFromConsumerOptions(opts)

	assert.Equal(t, "my-topic", topic)
}

func TestTopicFromConsumerOptions_SingleTopicInArray(t *testing.T) {
	opts := pulsar.ConsumerOptions{
		Topics: []string{"my-topic"},
	}

	topic := topicFromConsumerOptions(opts)

	assert.Equal(t, "my-topic", topic)
}

func TestTopicFromConsumerOptions_MultipleTopics(t *testing.T) {
	opts := pulsar.ConsumerOptions{
		Topics: []string{"topic-1", "topic-2", "topic-3"},
	}

	topic := topicFromConsumerOptions(opts)

	assert.Equal(t, "multi", topic)
}

func TestTopicFromConsumerOptions_TopicsPattern(t *testing.T) {
	opts := pulsar.ConsumerOptions{
		TopicsPattern: "persistent://public/default/order-.*",
	}

	topic := topicFromConsumerOptions(opts)

	assert.Equal(t, "pattern", topic)
}

func TestTopicFromConsumerOptions_Empty(t *testing.T) {
	opts := pulsar.ConsumerOptions{}

	topic := topicFromConsumerOptions(opts)

	assert.Equal(t, "", topic)
}

func TestTopicFromConsumerOptions_TopicTakesPrecedence(t *testing.T) {
	opts := pulsar.ConsumerOptions{
		Topic:  "primary-topic",
		Topics: []string{"secondary-topic"},
	}

	topic := topicFromConsumerOptions(opts)

	assert.Equal(t, "primary-topic", topic)
}

// =============================================================================
// mockMessage - 实现 pulsar.Message 接口用于测试（共享）
// =============================================================================

type mockMessage struct {
	properties map[string]string
}

func (m *mockMessage) Topic() string                                   { return "test-topic" }
func (m *mockMessage) Properties() map[string]string                   { return m.properties }
func (m *mockMessage) Payload() []byte                                 { return nil }
func (m *mockMessage) ID() pulsar.MessageID                            { return nil }
func (m *mockMessage) PublishTime() time.Time                          { return time.Time{} }
func (m *mockMessage) EventTime() time.Time                            { return time.Time{} }
func (m *mockMessage) Key() string                                     { return "" }
func (m *mockMessage) OrderingKey() string                             { return "" }
func (m *mockMessage) RedeliveryCount() uint32                         { return 0 }
func (m *mockMessage) IsReplicated() bool                              { return false }
func (m *mockMessage) GetReplicatedFrom() string                       { return "" }
func (m *mockMessage) GetSchemaValue(v interface{}) error              { return nil }
func (m *mockMessage) ProducerName() string                            { return "" }
func (m *mockMessage) SchemaVersion() []byte                           { return nil }
func (m *mockMessage) GetEncryptionContext() *pulsar.EncryptionContext { return nil }
func (m *mockMessage) Index() *uint64                                  { return nil }
func (m *mockMessage) BrokerPublishTime() *time.Time                   { return nil }

// =============================================================================
// extractPulsarTrace Additional Tests
// =============================================================================

func TestExtractPulsarTrace_WithValidMessage(t *testing.T) {
	ctx := context.Background()
	tracer := NoopTracer{}
	msg := &mockMessage{
		properties: map[string]string{
			"traceparent": "00-1234567890abcdef-fedcba0987654321-01",
		},
	}

	result := extractPulsarTrace(ctx, tracer, msg)

	// NoopTracer.Extract 返回空 context，所以 MergeTraceContext 返回原始 context
	assert.NotNil(t, result)
}

func TestExtractPulsarTrace_WithEmptyProperties(t *testing.T) {
	ctx := context.Background()
	tracer := NoopTracer{}
	msg := &mockMessage{
		properties: map[string]string{},
	}

	result := extractPulsarTrace(ctx, tracer, msg)

	assert.NotNil(t, result)
}
