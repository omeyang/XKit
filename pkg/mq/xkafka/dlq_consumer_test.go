package xkafka

import (
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/resilience/xretry"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// filterProducerConfig Tests
// =============================================================================

func TestFilterProducerConfig_Basic(t *testing.T) {
	consumerConfig := &kafka.ConfigMap{
		"bootstrap.servers": "localhost:9092",
		"security.protocol": "PLAINTEXT",
	}

	producerConfig, err := filterProducerConfig(consumerConfig)

	require.NoError(t, err)
	assert.NotNil(t, producerConfig)

	// 验证公共配置被保留
	val, err := producerConfig.Get("bootstrap.servers", nil)
	require.NoError(t, err)
	assert.Equal(t, "localhost:9092", val)

	val, err = producerConfig.Get("security.protocol", nil)
	require.NoError(t, err)
	assert.Equal(t, "PLAINTEXT", val)
}

func TestFilterProducerConfig_FiltersConsumerOnlyKeys(t *testing.T) {
	consumerConfig := &kafka.ConfigMap{
		"bootstrap.servers":     "localhost:9092",
		"group.id":              "test-group", // consumer-only
		"auto.offset.reset":     "earliest",   // consumer-only
		"enable.auto.commit":    true,         // consumer-only
		"session.timeout.ms":    10000,        // consumer-only
		"heartbeat.interval.ms": 3000,         // consumer-only
	}

	producerConfig, err := filterProducerConfig(consumerConfig)

	require.NoError(t, err)
	assert.NotNil(t, producerConfig)

	// 验证公共配置被保留
	val, err := producerConfig.Get("bootstrap.servers", nil)
	require.NoError(t, err)
	assert.Equal(t, "localhost:9092", val)

	// 验证 consumer-only 配置被过滤（Get 返回 nil 表示不存在）
	val, _ = producerConfig.Get("group.id", nil) //nolint:errcheck // testing Get returns nil
	assert.Nil(t, val, "group.id should be filtered")

	val, _ = producerConfig.Get("auto.offset.reset", nil) //nolint:errcheck // testing Get returns nil
	assert.Nil(t, val, "auto.offset.reset should be filtered")

	val, _ = producerConfig.Get("enable.auto.commit", nil) //nolint:errcheck // testing Get returns nil
	assert.Nil(t, val, "enable.auto.commit should be filtered")
}

func TestFilterProducerConfig_AllConsumerOnlyKeys(t *testing.T) {
	// 测试所有 consumer-only 配置项
	consumerOnlyKeys := []string{
		"group.id",
		"group.instance.id",
		"auto.offset.reset",
		"enable.auto.commit",
		"auto.commit.interval.ms",
		"enable.auto.offset.store",
		"partition.assignment.strategy",
		"session.timeout.ms",
		"heartbeat.interval.ms",
		"max.poll.interval.ms",
		"fetch.min.bytes",
		"fetch.max.bytes",
		"fetch.wait.max.ms",
		"max.partition.fetch.bytes",
		"isolation.level",
		"check.crcs",
		"queued.min.messages",
		"queued.max.messages.kbytes",
		"fetch.message.max.bytes",
	}

	for _, key := range consumerOnlyKeys {
		t.Run(key, func(t *testing.T) {
			consumerConfig := &kafka.ConfigMap{
				"bootstrap.servers": "localhost:9092",
				key:                 "test-value",
			}

			producerConfig, err := filterProducerConfig(consumerConfig)
			require.NoError(t, err)

			// 验证该 key 被过滤（Get 返回 nil 表示不存在）
			val, _ := producerConfig.Get(key, nil) //nolint:errcheck // testing Get returns nil
			assert.Nil(t, val, "consumer-only key %s should be filtered", key)

			// 验证公共配置被保留
			val, err = producerConfig.Get("bootstrap.servers", nil)
			require.NoError(t, err)
			assert.Equal(t, "localhost:9092", val)
		})
	}
}

func TestFilterProducerConfig_EmptyConfig(t *testing.T) {
	consumerConfig := &kafka.ConfigMap{}

	producerConfig, err := filterProducerConfig(consumerConfig)

	require.NoError(t, err)
	assert.NotNil(t, producerConfig)
}

func TestFilterProducerConfig_OnlyConsumerOnlyKeys(t *testing.T) {
	consumerConfig := &kafka.ConfigMap{
		"group.id":          "test-group",
		"auto.offset.reset": "earliest",
	}

	producerConfig, err := filterProducerConfig(consumerConfig)

	require.NoError(t, err)
	assert.NotNil(t, producerConfig)
	// 所有配置都被过滤，结果为空
}

// =============================================================================
// NewConsumerWithDLQ Validation Tests
// =============================================================================

func TestNewConsumerWithDLQ_NilPolicy(t *testing.T) {
	config := &kafka.ConfigMap{
		"bootstrap.servers": "localhost:9092",
		"group.id":          "test-group",
	}

	consumer, err := NewConsumerWithDLQ(config, []string{"topic"}, nil)

	assert.Nil(t, consumer)
	assert.ErrorIs(t, err, ErrDLQPolicyRequired)
}

func TestNewConsumerWithDLQ_InvalidPolicy_EmptyDLQTopic(t *testing.T) {
	config := &kafka.ConfigMap{
		"bootstrap.servers": "localhost:9092",
		"group.id":          "test-group",
	}
	policy := &DLQPolicy{
		DLQTopic:    "",
		RetryPolicy: xretry.NewFixedRetry(3),
	}

	consumer, err := NewConsumerWithDLQ(config, []string{"topic"}, policy)

	assert.Nil(t, consumer)
	assert.ErrorIs(t, err, ErrDLQTopicRequired)
}

func TestNewConsumerWithDLQ_InvalidPolicy_NilRetryPolicy(t *testing.T) {
	config := &kafka.ConfigMap{
		"bootstrap.servers": "localhost:9092",
		"group.id":          "test-group",
	}
	policy := &DLQPolicy{
		DLQTopic:    "dlq-topic",
		RetryPolicy: nil,
	}

	consumer, err := NewConsumerWithDLQ(config, []string{"topic"}, policy)

	assert.Nil(t, consumer)
	assert.ErrorIs(t, err, ErrRetryPolicyRequired)
}

// =============================================================================
// DLQPolicy Validation Edge Cases
// =============================================================================

func TestDLQPolicy_Validate_WithRetryTopic(t *testing.T) {
	policy := &DLQPolicy{
		DLQTopic:    "dlq-topic",
		RetryTopic:  "retry-topic",
		RetryPolicy: xretry.NewFixedRetry(3),
	}

	err := policy.Validate()

	assert.NoError(t, err)
}

func TestDLQPolicy_Validate_WithBackoffPolicy(t *testing.T) {
	policy := &DLQPolicy{
		DLQTopic:      "dlq-topic",
		RetryPolicy:   xretry.NewFixedRetry(3),
		BackoffPolicy: xretry.NewFixedBackoff(100 * time.Millisecond),
	}

	err := policy.Validate()

	assert.NoError(t, err)
}

func TestDLQPolicy_Validate_WithProducerConfig(t *testing.T) {
	policy := &DLQPolicy{
		DLQTopic:    "dlq-topic",
		RetryPolicy: xretry.NewFixedRetry(3),
		ProducerConfig: &kafka.ConfigMap{
			"bootstrap.servers": "localhost:9092",
		},
	}

	err := policy.Validate()

	assert.NoError(t, err)
}

func TestDLQPolicy_Validate_WithCallbacks(t *testing.T) {
	policy := &DLQPolicy{
		DLQTopic:    "dlq-topic",
		RetryPolicy: xretry.NewFixedRetry(3),
		OnRetry: func(msg *kafka.Message, attempt int, err error) {
			// 重试回调
		},
		OnDLQ: func(msg *kafka.Message, reason error, metadata DLQMetadata) {
			// DLQ 回调
		},
	}

	err := policy.Validate()

	assert.NoError(t, err)
}

// =============================================================================
// Error Constants Tests
// =============================================================================

func TestErrorConstants(t *testing.T) {
	assert.NotNil(t, ErrDLQPolicyRequired)
	assert.NotNil(t, ErrDLQTopicRequired)
	assert.NotNil(t, ErrRetryPolicyRequired)

	// 验证错误消息
	assert.Contains(t, ErrDLQPolicyRequired.Error(), "DLQ")
	assert.Contains(t, ErrDLQTopicRequired.Error(), "topic")
	assert.Contains(t, ErrRetryPolicyRequired.Error(), "retry")
}

// =============================================================================
// DLQMetadata Tests
// =============================================================================

func TestDLQMetadata_ZeroValues(t *testing.T) {
	metadata := DLQMetadata{}

	assert.Empty(t, metadata.OriginalTopic)
	assert.Zero(t, metadata.OriginalPartition)
	assert.Zero(t, metadata.OriginalOffset)
	assert.Empty(t, metadata.FailureReason)
	assert.Zero(t, metadata.FailureCount)
	assert.True(t, metadata.FirstFailureTime.IsZero())
	assert.True(t, metadata.LastFailureTime.IsZero())
}

func TestDLQMetadata_AllFields(t *testing.T) {
	now := time.Now()
	metadata := DLQMetadata{
		OriginalTopic:     "original-topic",
		OriginalPartition: 1,
		OriginalOffset:    100,
		OriginalTimestamp: now,
		FailureReason:     "test error",
		FailureCount:      3,
		FirstFailureTime:  now.Add(-time.Hour),
		LastFailureTime:   now,
	}

	assert.Equal(t, "original-topic", metadata.OriginalTopic)
	assert.Equal(t, int32(1), metadata.OriginalPartition)
	assert.Equal(t, int64(100), metadata.OriginalOffset)
	assert.Equal(t, "test error", metadata.FailureReason)
	assert.Equal(t, 3, metadata.FailureCount)
	assert.False(t, metadata.FirstFailureTime.IsZero())
	assert.False(t, metadata.LastFailureTime.IsZero())
}

// =============================================================================
// ConsumerWithDLQ Interface Test
// =============================================================================

func TestConsumerWithDLQ_InterfaceDefinition(t *testing.T) {
	// 验证 dlqConsumer 实现了 ConsumerWithDLQ 接口
	// 这是一个编译时检查，在运行时验证接口定义
	var _ ConsumerWithDLQ = (*dlqConsumer)(nil)
}
