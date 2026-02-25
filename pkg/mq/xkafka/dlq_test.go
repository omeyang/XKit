package xkafka

import (
	"errors"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/resilience/xretry"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// DefaultDLQTopic Tests
// =============================================================================

func TestDefaultDLQTopic(t *testing.T) {
	assert.Equal(t, "orders.dlq", DefaultDLQTopic("orders"))
	assert.Equal(t, "payments.dlq", DefaultDLQTopic("payments"))
	assert.Equal(t, ".dlq", DefaultDLQTopic(""))
}

// =============================================================================
// DLQPolicy Tests
// =============================================================================

func TestDLQPolicy_Validate_EmptyDLQTopic(t *testing.T) {
	policy := &DLQPolicy{
		DLQTopic:    "",
		RetryPolicy: xretry.NewFixedRetry(3),
	}

	err := policy.Validate()

	assert.ErrorIs(t, err, ErrDLQTopicRequired)
}

func TestDLQPolicy_Validate_NilRetryPolicy(t *testing.T) {
	policy := &DLQPolicy{
		DLQTopic:    "dlq-topic",
		RetryPolicy: nil,
	}

	err := policy.Validate()

	assert.ErrorIs(t, err, ErrRetryPolicyRequired)
}

func TestDLQPolicy_Validate_Valid(t *testing.T) {
	policy := &DLQPolicy{
		DLQTopic:    "dlq-topic",
		RetryPolicy: xretry.NewFixedRetry(3),
	}

	err := policy.Validate()

	assert.NoError(t, err)
}

// =============================================================================
// DLQStats Tests
// =============================================================================

func TestDLQStats_Clone(t *testing.T) {
	original := DLQStats{
		TotalMessages:      100,
		RetriedMessages:    50,
		DeadLetterMessages: 10,
		SuccessAfterRetry:  40,
		LastDLQTime:        time.Now(),
		ByTopic: map[string]int64{
			"topic-1": 5,
			"topic-2": 5,
		},
	}

	clone := original.Clone()

	// 验证值相等
	assert.Equal(t, original.TotalMessages, clone.TotalMessages)
	assert.Equal(t, original.RetriedMessages, clone.RetriedMessages)
	assert.Equal(t, original.DeadLetterMessages, clone.DeadLetterMessages)
	assert.Equal(t, original.SuccessAfterRetry, clone.SuccessAfterRetry)

	// 验证 ByTopic 是深拷贝
	clone.ByTopic["topic-1"] = 999
	assert.Equal(t, int64(5), original.ByTopic["topic-1"])
}

func TestDLQStats_Clone_NilByTopic(t *testing.T) {
	original := DLQStats{
		TotalMessages: 100,
		ByTopic:       nil,
	}

	clone := original.Clone()

	assert.Nil(t, clone.ByTopic)
}

// =============================================================================
// Helper Functions Tests
// =============================================================================

func TestGetRetryCount_NoHeader(t *testing.T) {
	msg := &kafka.Message{}

	count := getRetryCount(msg)

	assert.Equal(t, 0, count)
}

func TestGetRetryCount_WithHeader(t *testing.T) {
	msg := &kafka.Message{
		Headers: []kafka.Header{
			{Key: HeaderRetryCount, Value: []byte("3")},
		},
	}

	count := getRetryCount(msg)

	assert.Equal(t, 3, count)
}

func TestGetRetryCount_InvalidValue(t *testing.T) {
	msg := &kafka.Message{
		Headers: []kafka.Header{
			{Key: HeaderRetryCount, Value: []byte("invalid")},
		},
	}

	count := getRetryCount(msg)

	assert.Equal(t, 0, count)
}

func TestGetRetryCount_NegativeValue(t *testing.T) {
	msg := &kafka.Message{
		Headers: []kafka.Header{
			{Key: HeaderRetryCount, Value: []byte("-1")},
		},
	}

	count := getRetryCount(msg)

	assert.Equal(t, 0, count)
}

func TestSetHeader_NewHeader(t *testing.T) {
	msg := &kafka.Message{}

	setHeader(msg, "test-key", "test-value")

	assert.Len(t, msg.Headers, 1)
	assert.Equal(t, "test-key", msg.Headers[0].Key)
	assert.Equal(t, "test-value", string(msg.Headers[0].Value))
}

func TestSetHeader_UpdateExisting(t *testing.T) {
	msg := &kafka.Message{
		Headers: []kafka.Header{
			{Key: "test-key", Value: []byte("old-value")},
		},
	}

	setHeader(msg, "test-key", "new-value")

	assert.Len(t, msg.Headers, 1)
	assert.Equal(t, "new-value", string(msg.Headers[0].Value))
}

func TestGetHeader_Exists(t *testing.T) {
	msg := &kafka.Message{
		Headers: []kafka.Header{
			{Key: "test-key", Value: []byte("test-value")},
		},
	}

	value := getHeader(msg, "test-key")

	assert.Equal(t, "test-value", value)
}

func TestGetHeader_NotExists(t *testing.T) {
	msg := &kafka.Message{}

	value := getHeader(msg, "test-key")

	assert.Empty(t, value)
}

// =============================================================================
// buildDLQMessageFromPolicy Tests
// =============================================================================

func TestBuildDLQMessageFromPolicy_Basic(t *testing.T) {
	topic := "original-topic"
	originalMsg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: 1,
			Offset:    100,
		},
		Key:   []byte("key"),
		Value: []byte("value"),
	}

	dlqMsg := buildDLQMessageFromPolicy(originalMsg, "dlq-topic", "test error", 3)

	assert.Equal(t, "dlq-topic", *dlqMsg.TopicPartition.Topic)
	assert.Equal(t, []byte("key"), dlqMsg.Key)
	assert.Equal(t, []byte("value"), dlqMsg.Value)
	assert.Equal(t, "original-topic", getHeader(dlqMsg, HeaderOriginalTopic))
	assert.Equal(t, "1", getHeader(dlqMsg, HeaderOriginalPartition))
	assert.Equal(t, "100", getHeader(dlqMsg, HeaderOriginalOffset))
	assert.Equal(t, "3", getHeader(dlqMsg, HeaderRetryCount))
	assert.Equal(t, "test error", getHeader(dlqMsg, HeaderFailureReason))
}

func TestBuildDLQMessageFromPolicy_PreservesExistingOriginal(t *testing.T) {
	// 模拟来自重试队列的消息
	retryTopic := "retry-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &retryTopic,
			Partition: 0,
			Offset:    50,
		},
		Headers: []kafka.Header{
			{Key: HeaderOriginalTopic, Value: []byte("real-original-topic")},
			{Key: HeaderOriginalPartition, Value: []byte("2")},
			{Key: HeaderOriginalOffset, Value: []byte("200")},
			{Key: HeaderFirstFailTime, Value: []byte("2024-01-01T00:00:00Z")},
		},
		Value: []byte("value"),
	}

	dlqMsg := buildDLQMessageFromPolicy(msg, "dlq-topic", "error", 5)

	// 应保留原始信息，而非使用重试队列的 TopicPartition
	assert.Equal(t, "real-original-topic", getHeader(dlqMsg, HeaderOriginalTopic))
	assert.Equal(t, "2", getHeader(dlqMsg, HeaderOriginalPartition))
	assert.Equal(t, "200", getHeader(dlqMsg, HeaderOriginalOffset))
	assert.Equal(t, "2024-01-01T00:00:00Z", getHeader(dlqMsg, HeaderFirstFailTime))
}

func TestBuildDLQMessageFromPolicy_NilTopicPtr(t *testing.T) {
	// 测试 TopicPartition.Topic 为 nil 的场景
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     nil,
			Partition: 1,
			Offset:    100,
		},
		Value: []byte("value"),
	}

	dlqMsg := buildDLQMessageFromPolicy(msg, "dlq-topic", "error", 0)

	// originalTopic 应该为空
	assert.Empty(t, getHeader(dlqMsg, HeaderOriginalTopic))
	// 但 partition 和 offset 仍应设置
	assert.Equal(t, "1", getHeader(dlqMsg, HeaderOriginalPartition))
	assert.Equal(t, "100", getHeader(dlqMsg, HeaderOriginalOffset))
}

func TestBuildDLQMessageFromPolicy_PreservesCustomHeaders(t *testing.T) {
	topic := "original-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic: &topic,
		},
		Headers: []kafka.Header{
			{Key: "custom-header-1", Value: []byte("value1")},
			{Key: "custom-header-2", Value: []byte("value2")},
		},
		Value: []byte("value"),
	}

	dlqMsg := buildDLQMessageFromPolicy(msg, "dlq-topic", "", 0)

	// 验证自定义 headers 被保留
	assert.Equal(t, "value1", getHeader(dlqMsg, "custom-header-1"))
	assert.Equal(t, "value2", getHeader(dlqMsg, "custom-header-2"))
}

func TestBuildDLQMessageFromPolicy_NilError(t *testing.T) {
	topic := "original-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic: &topic,
		},
		Value: []byte("value"),
	}

	dlqMsg := buildDLQMessageFromPolicy(msg, "dlq-topic", "", 0)

	// nil 错误应该转为空字符串
	assert.Empty(t, getHeader(dlqMsg, HeaderFailureReason))
}

// =============================================================================
// buildDLQMetadataFromMessage Tests
// =============================================================================

func TestBuildDLQMetadataFromMessage_Basic(t *testing.T) {
	topic := "test-topic"
	now := time.Now()
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: 1,
			Offset:    100,
		},
		Timestamp: now,
	}

	metadata := buildDLQMetadataFromMessage(msg, "test error", 3)

	assert.Equal(t, "test-topic", metadata.OriginalTopic)
	assert.Equal(t, int32(1), metadata.OriginalPartition)
	assert.Equal(t, int64(100), metadata.OriginalOffset)
	assert.Equal(t, "test error", metadata.FailureReason)
	assert.Equal(t, 4, metadata.FailureCount) // retryCount + 1
}

func TestBuildDLQMetadataFromMessage_NilTopic(t *testing.T) {
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic: nil,
		},
	}

	metadata := buildDLQMetadataFromMessage(msg, "", 0)

	assert.Empty(t, metadata.OriginalTopic)
}

func TestBuildDLQMetadataFromMessage_WithValidFirstFailTime(t *testing.T) {
	topic := "test-topic"
	expectedTime := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: 1,
			Offset:    100,
		},
		Headers: []kafka.Header{
			{Key: HeaderFirstFailTime, Value: []byte(expectedTime.Format(time.RFC3339))},
		},
	}

	metadata := buildDLQMetadataFromMessage(msg, "test error", 2)

	// 验证使用了 header 中的 FirstFailTime
	assert.Equal(t, expectedTime, metadata.FirstFailureTime)
	assert.Equal(t, "test-topic", metadata.OriginalTopic)
	assert.Equal(t, 3, metadata.FailureCount) // retryCount + 1
}

func TestBuildDLQMetadataFromMessage_WithInvalidFirstFailTime(t *testing.T) {
	topic := "test-topic"
	beforeTest := time.Now()
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: 1,
			Offset:    100,
		},
		Headers: []kafka.Header{
			{Key: HeaderFirstFailTime, Value: []byte("invalid-time-format")},
		},
	}

	metadata := buildDLQMetadataFromMessage(msg, "test error", 1)

	// 解析失败时应回退到当前时间
	assert.True(t, metadata.FirstFailureTime.After(beforeTest) || metadata.FirstFailureTime.Equal(beforeTest))
	assert.True(t, metadata.FirstFailureTime.Before(time.Now().Add(time.Second)))
}

// =============================================================================
// errorString Tests
// =============================================================================

func TestErrorString_NilError(t *testing.T) {
	result := errorString(nil)

	assert.Empty(t, result)
}

func TestErrorString_WithError(t *testing.T) {
	result := errorString(errors.New("test error"))

	assert.Equal(t, "test error", result)
}

// =============================================================================
// updateRetryHeaders Tests
// =============================================================================

func TestUpdateRetryHeaders_FirstRetry(t *testing.T) {
	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: 1,
			Offset:    100,
		},
	}

	updateRetryHeaders(msg, "test error")

	assert.Equal(t, "1", getHeader(msg, HeaderRetryCount))
	assert.Equal(t, "test-topic", getHeader(msg, HeaderOriginalTopic))
	assert.Equal(t, "1", getHeader(msg, HeaderOriginalPartition))
	assert.Equal(t, "100", getHeader(msg, HeaderOriginalOffset))
	assert.NotEmpty(t, getHeader(msg, HeaderFirstFailTime))
	assert.NotEmpty(t, getHeader(msg, HeaderLastFailTime))
	assert.Equal(t, "test error", getHeader(msg, HeaderFailureReason))
}

func TestUpdateRetryHeaders_SubsequentRetry(t *testing.T) {
	topic := "test-topic"
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: 1,
			Offset:    100,
		},
		Headers: []kafka.Header{
			{Key: HeaderRetryCount, Value: []byte("2")},
			{Key: HeaderOriginalTopic, Value: []byte("original-topic")},
			{Key: HeaderFirstFailTime, Value: []byte("2024-01-01T00:00:00Z")},
		},
	}

	updateRetryHeaders(msg, "new error")

	assert.Equal(t, "3", getHeader(msg, HeaderRetryCount))
	// 原始信息应保持不变
	assert.Equal(t, "original-topic", getHeader(msg, HeaderOriginalTopic))
	assert.Equal(t, "2024-01-01T00:00:00Z", getHeader(msg, HeaderFirstFailTime))
}

// =============================================================================
// DLQ Header Constants Tests
// =============================================================================

// =============================================================================
// parseOriginalPartition / parseOriginalOffset Tests
// =============================================================================

func TestParseOriginalPartition_InvalidValue(t *testing.T) {
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Partition: 5},
		Headers: []kafka.Header{
			{Key: HeaderOriginalPartition, Value: []byte("not-a-number")},
		},
	}

	// 无效值应回退到当前消息的分区号
	assert.Equal(t, int32(5), parseOriginalPartition(msg))
}

func TestParseOriginalOffset_InvalidValue(t *testing.T) {
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Offset: 99},
		Headers: []kafka.Header{
			{Key: HeaderOriginalOffset, Value: []byte("not-a-number")},
		},
	}

	// 无效值应回退到当前消息的偏移量
	assert.Equal(t, int64(99), parseOriginalOffset(msg))
}

func TestDLQHeaderConstants(t *testing.T) {
	assert.Equal(t, "x-retry-count", HeaderRetryCount)
	assert.Equal(t, "x-original-topic", HeaderOriginalTopic)
	assert.Equal(t, "x-original-partition", HeaderOriginalPartition)
	assert.Equal(t, "x-original-offset", HeaderOriginalOffset)
	assert.Equal(t, "x-first-fail-time", HeaderFirstFailTime)
	assert.Equal(t, "x-last-fail-time", HeaderLastFailTime)
	assert.Equal(t, "x-failure-reason", HeaderFailureReason)
}

// =============================================================================
// dlqStatsCollector Tests
// =============================================================================

func TestDLQStatsCollector_New(t *testing.T) {
	collector := newDLQStatsCollector()

	assert.NotNil(t, collector)
	stats := collector.get()
	assert.Zero(t, stats.TotalMessages)
	assert.NotNil(t, stats.ByTopic)
}

func TestDLQStatsCollector_IncTotal(t *testing.T) {
	collector := newDLQStatsCollector()

	collector.incTotal()
	collector.incTotal()

	stats := collector.get()
	assert.Equal(t, int64(2), stats.TotalMessages)
}

func TestDLQStatsCollector_IncRetried(t *testing.T) {
	collector := newDLQStatsCollector()

	collector.incRetried()

	stats := collector.get()
	assert.Equal(t, int64(1), stats.RetriedMessages)
}

func TestDLQStatsCollector_IncDeadLetter(t *testing.T) {
	collector := newDLQStatsCollector()

	collector.incDeadLetter("topic-1")
	collector.incDeadLetter("topic-1")
	collector.incDeadLetter("topic-2")

	stats := collector.get()
	assert.Equal(t, int64(3), stats.DeadLetterMessages)
	assert.Equal(t, int64(2), stats.ByTopic["topic-1"])
	assert.Equal(t, int64(1), stats.ByTopic["topic-2"])
	assert.False(t, stats.LastDLQTime.IsZero())
}

func TestDLQStatsCollector_IncSuccessAfterRetry(t *testing.T) {
	collector := newDLQStatsCollector()

	collector.incSuccessAfterRetry()

	stats := collector.get()
	assert.Equal(t, int64(1), stats.SuccessAfterRetry)
}

// =============================================================================
// defaultFailureReasonFormatter Tests (FG-S1)
// =============================================================================

func TestDefaultFailureReasonFormatter_NilError(t *testing.T) {
	result := defaultFailureReasonFormatter(nil)
	assert.Empty(t, result)
}

func TestDefaultFailureReasonFormatter_ShortError(t *testing.T) {
	result := defaultFailureReasonFormatter(errors.New("short error"))
	assert.Equal(t, "short error", result)
}

func TestDefaultFailureReasonFormatter_TruncatesLongError(t *testing.T) {
	// 构造超过 maxFailureReasonLen 的错误
	longMsg := make([]byte, maxFailureReasonLen+100)
	for i := range longMsg {
		longMsg[i] = 'x'
	}
	result := defaultFailureReasonFormatter(errors.New(string(longMsg)))

	assert.Len(t, result, maxFailureReasonLen+len("...(truncated)"))
	assert.True(t, len(result) < len(string(longMsg)))
	assert.Contains(t, result, "...(truncated)")
}

func TestDefaultFailureReasonFormatter_ExactlyMaxLen(t *testing.T) {
	// 刚好等于 maxFailureReasonLen 不应截断
	exactMsg := make([]byte, maxFailureReasonLen)
	for i := range exactMsg {
		exactMsg[i] = 'y'
	}
	result := defaultFailureReasonFormatter(errors.New(string(exactMsg)))
	assert.Equal(t, string(exactMsg), result)
	assert.NotContains(t, result, "...(truncated)")
}

// =============================================================================
// parseOriginalPartition / parseOriginalOffset Success Path Tests (FG-L5)
// =============================================================================

func TestParseOriginalPartition_ValidHeader(t *testing.T) {
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Partition: 5},
		Headers: []kafka.Header{
			{Key: HeaderOriginalPartition, Value: []byte("42")},
		},
	}

	assert.Equal(t, int32(42), parseOriginalPartition(msg))
}

func TestParseOriginalOffset_ValidHeader(t *testing.T) {
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Offset: 99},
		Headers: []kafka.Header{
			{Key: HeaderOriginalOffset, Value: []byte("12345")},
		},
	}

	assert.Equal(t, int64(12345), parseOriginalOffset(msg))
}

// =============================================================================
// DefaultBackoffPolicy Tests (FG-L4)
// =============================================================================

func TestDefaultBackoffPolicy_NotNil(t *testing.T) {
	bp := DefaultBackoffPolicy()
	assert.NotNil(t, bp)

	// 验证默认延迟在合理范围内（100ms ± 10% jitter）
	delay := bp.NextDelay(1)
	assert.GreaterOrEqual(t, delay.Milliseconds(), int64(80))
	assert.LessOrEqual(t, delay.Milliseconds(), int64(120))
}
