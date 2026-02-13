package xkafka

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/resilience/xretry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// clampRetryCount 将重试次数限制在 [0, 1000] 范围内。
func clampRetryCount(n int) int {
	if n < 0 {
		return 0
	}
	if n > 1000 {
		return 1000
	}
	return n
}

// newTopicPtr 当 topic 非空时返回其指针，否则返回 nil。
func newTopicPtr(topic string) *string {
	if topic == "" {
		return nil
	}
	return &topic
}

// newFuzzMessage 构建用于 fuzz 测试的 kafka.Message。
func newFuzzMessage(topicPtr *string, partition int32, offset int64) *kafka.Message {
	return &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     topicPtr,
			Partition: partition,
			Offset:    kafka.Offset(offset),
		},
	}
}

// maybeError 当 errMsg 非空时返回对应的 error，否则返回 nil。
func maybeError(errMsg string) error {
	if errMsg == "" {
		return nil
	}
	return errors.New(errMsg)
}

// buildFuzzByTopic 构建用于 fuzz 测试的 ByTopic 映射。
func buildFuzzByTopic(topicCount int) map[string]int64 {
	if topicCount <= 0 {
		return nil
	}
	if topicCount > 100 {
		topicCount = 100
	}
	byTopic := make(map[string]int64, topicCount)
	for i := range topicCount {
		byTopic["topic-"+string(rune('a'+i%26))] = int64(i * 10)
	}
	return byTopic
}

// =============================================================================
// DLQPolicy Fuzz Tests
// =============================================================================

// FuzzDLQPolicy_Validate 模糊测试 DLQ 策略验证。
func FuzzDLQPolicy_Validate(f *testing.F) {
	// 种子语料库 - 有效值
	f.Add("dlq-topic", true)
	f.Add("my-dlq", true)
	f.Add("app.dlq.topic", true)

	// 边界值
	f.Add("", true)
	f.Add("a", true)
	f.Add("x", false)

	// 特殊字符
	f.Add("dlq_topic_123", true)
	f.Add("dlq-topic-中文", true)
	f.Add(strings.Repeat("x", 255), true)

	f.Fuzz(func(t *testing.T, dlqTopic string, hasRetryPolicy bool) {
		// 限制输入长度避免慢测试
		if len(dlqTopic) > 1000 {
			return
		}

		var retryPolicy xretry.RetryPolicy
		if hasRetryPolicy {
			retryPolicy = xretry.NewFixedRetry(3)
		}

		policy := &DLQPolicy{
			DLQTopic:    dlqTopic,
			RetryPolicy: retryPolicy,
		}

		err := policy.Validate()

		// 验证不变式
		switch {
		case dlqTopic == "":
			if !errors.Is(err, ErrDLQTopicRequired) {
				t.Errorf("expected ErrDLQTopicRequired for empty dlqTopic, got: %v", err)
			}
		case retryPolicy == nil:
			if !errors.Is(err, ErrRetryPolicyRequired) {
				t.Errorf("expected ErrRetryPolicyRequired for nil retryPolicy, got: %v", err)
			}
		default:
			if err != nil {
				t.Errorf("expected no error for valid policy, got: %v", err)
			}
		}
	})
}

// =============================================================================
// Header Helper Fuzz Tests
// =============================================================================

// FuzzGetRetryCount 模糊测试获取重试次数。
func FuzzGetRetryCount(f *testing.F) {
	// 有效数字
	f.Add("0")
	f.Add("1")
	f.Add("5")
	f.Add("100")

	// 边界值
	f.Add("")
	f.Add("-1")
	f.Add("-100")

	// 无效值
	f.Add("abc")
	f.Add("1.5")
	f.Add("1e10")
	f.Add("  3  ")

	// 极端值
	f.Add("9999999999999999999999")
	f.Add("2147483647") // int32 max
	f.Add("2147483648") // int32 max + 1

	f.Fuzz(func(t *testing.T, headerValue string) {
		msg := &kafka.Message{
			Headers: []kafka.Header{
				{Key: HeaderRetryCount, Value: []byte(headerValue)},
			},
		}

		count := getRetryCount(msg)

		// 验证不变式：返回值总是 >= 0
		if count < 0 {
			t.Errorf("getRetryCount returned negative value: %d for input: %q", count, headerValue)
		}
	})
}

// FuzzSetHeader 模糊测试设置 Header。
func FuzzSetHeader(f *testing.F) {
	// 有效键值对
	f.Add("key", "value")
	f.Add("x-custom-header", "custom-value")
	f.Add("Content-Type", "application/json")

	// 边界值
	f.Add("", "")
	f.Add("k", "v")

	// 特殊字符
	f.Add("key-with-中文", "value-with-中文")
	f.Add("key\twith\ttab", "value\nwith\nnewline")

	// 长值
	f.Add("key", strings.Repeat("x", 1000))
	f.Add(strings.Repeat("k", 100), "value")

	f.Fuzz(func(t *testing.T, key, value string) {
		// 限制输入长度
		if len(key) > 1000 || len(value) > 10000 {
			return
		}

		msg := &kafka.Message{}
		setHeader(msg, key, value)

		// 验证不变式：header 被设置
		if len(msg.Headers) == 0 {
			t.Error("setHeader should add at least one header")
			return
		}

		// 验证可以通过 getHeader 获取
		got := getHeader(msg, key)
		if got != value {
			t.Errorf("getHeader(%q) = %q, want %q", key, got, value)
		}
	})
}

// FuzzGetHeader 模糊测试获取 Header。
func FuzzGetHeader(f *testing.F) {
	f.Add("existing-key", "existing-value", "existing-key")
	f.Add("key1", "value1", "key2")
	f.Add("", "", "empty")
	f.Add("key", "value", "")

	f.Fuzz(func(t *testing.T, existingKey, existingValue, searchKey string) {
		// 限制输入长度
		if len(existingKey) > 1000 || len(existingValue) > 1000 || len(searchKey) > 1000 {
			return
		}

		msg := &kafka.Message{}
		if existingKey != "" {
			msg.Headers = []kafka.Header{
				{Key: existingKey, Value: []byte(existingValue)},
			}
		}

		result := getHeader(msg, searchKey)

		// 验证不变式
		if existingKey == searchKey {
			if result != existingValue {
				t.Errorf("getHeader should return %q for key %q, got %q", existingValue, searchKey, result)
			}
		}
	})
}

// =============================================================================
// DLQ Message Building Fuzz Tests
// =============================================================================

// FuzzBuildDLQMessageFromPolicy 模糊测试 DLQ 消息构建。
func FuzzBuildDLQMessageFromPolicy(f *testing.F) {
	// 种子语料库
	f.Add("original-topic", int32(0), int64(0), "dlq-topic", "error message", 0)
	f.Add("my-topic", int32(5), int64(12345), "my-dlq", "processing failed", 3)
	f.Add("topic", int32(100), int64(999999), "dlq", "", 10)

	// 边界值
	f.Add("", int32(0), int64(0), "", "", 0)
	f.Add("t", int32(-1), int64(-1), "d", "e", -1)

	// 特殊字符
	f.Add("topic-中文", int32(0), int64(0), "dlq-中文", "错误信息", 1)

	f.Fuzz(func(t *testing.T, originalTopic string, partition int32, offset int64, dlqTopic, errMsg string, retryCount int) {
		// 限制输入
		if len(originalTopic) > 500 || len(dlqTopic) > 500 || len(errMsg) > 1000 {
			return
		}
		retryCount = clampRetryCount(retryCount)

		msg := newFuzzMessage(newTopicPtr(originalTopic), partition, offset)
		msg.Key = []byte("test-key")
		msg.Value = []byte("test-value")

		// 不应 panic
		result := buildDLQMessageFromPolicy(msg, dlqTopic, maybeError(errMsg), retryCount)

		// 验证不变式
		require.NotNil(t, result, "buildDLQMessageFromPolicy should never return nil")
		require.NotNil(t, result.TopicPartition.Topic, "DLQ message topic should not be nil")
		assert.Equal(t, dlqTopic, *result.TopicPartition.Topic, "DLQ message should have correct topic")
		assert.Equal(t, "test-key", string(result.Key), "DLQ message should preserve original key")
		assert.Equal(t, "test-value", string(result.Value), "DLQ message should preserve original value")
	})
}

// FuzzBuildDLQMetadataFromMessage 模糊测试 DLQ 元数据构建。
func FuzzBuildDLQMetadataFromMessage(f *testing.F) {
	f.Add("topic", int32(0), int64(100), "error", 0)
	f.Add("my-topic", int32(5), int64(999), "failure reason", 5)
	f.Add("", int32(0), int64(0), "", 0)

	f.Fuzz(func(t *testing.T, topic string, partition int32, offset int64, errMsg string, retryCount int) {
		if len(topic) > 500 || len(errMsg) > 1000 {
			return
		}
		retryCount = clampRetryCount(retryCount)

		msg := newFuzzMessage(newTopicPtr(topic), partition, offset)
		msg.Timestamp = time.Now()

		// 不应 panic
		metadata := buildDLQMetadataFromMessage(msg, maybeError(errMsg), retryCount)

		// 验证不变式
		if topic != "" {
			assert.Equal(t, topic, metadata.OriginalTopic, "metadata.OriginalTopic mismatch")
		}
		assert.Equal(t, retryCount+1, metadata.FailureCount, "metadata.FailureCount mismatch")
		assert.False(t, metadata.FirstFailureTime.IsZero(), "metadata.FirstFailureTime should not be zero")
		assert.False(t, metadata.LastFailureTime.IsZero(), "metadata.LastFailureTime should not be zero")
	})
}

// FuzzUpdateRetryHeaders 模糊测试更新重试头部。
func FuzzUpdateRetryHeaders(f *testing.F) {
	f.Add("topic", int32(0), int64(100), "error", 0)
	f.Add("my-topic", int32(5), int64(999), "", 3)
	f.Add("", int32(0), int64(0), "failure", 0)

	f.Fuzz(func(t *testing.T, topic string, partition int32, offset int64, errMsg string, existingRetryCount int) {
		if len(topic) > 500 || len(errMsg) > 1000 {
			return
		}
		if existingRetryCount < 0 {
			existingRetryCount = 0
		}
		if existingRetryCount > 1000 {
			existingRetryCount = 1000
		}

		var topicPtr *string
		if topic != "" {
			topicPtr = &topic
		}

		msg := &kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     topicPtr,
				Partition: partition,
				Offset:    kafka.Offset(offset),
			},
		}

		// 如果有已存在的重试次数，添加 header
		if existingRetryCount > 0 {
			msg.Headers = []kafka.Header{
				{Key: HeaderRetryCount, Value: []byte(string(rune('0' + existingRetryCount%10)))},
			}
		}

		var err error
		if errMsg != "" {
			err = errors.New(errMsg)
		}

		// 不应 panic
		updateRetryHeaders(msg, err)

		// 验证不变式：必须有 retry count header
		retryCountHeader := getHeader(msg, HeaderRetryCount)
		if retryCountHeader == "" {
			t.Error("updateRetryHeaders should set HeaderRetryCount")
		}

		// 验证有 last fail time
		lastFailTime := getHeader(msg, HeaderLastFailTime)
		if lastFailTime == "" {
			t.Error("updateRetryHeaders should set HeaderLastFailTime")
		}
	})
}

// =============================================================================
// DLQStats Fuzz Tests
// =============================================================================

// FuzzDLQStats_Clone 模糊测试统计克隆。
func FuzzDLQStats_Clone(f *testing.F) {
	f.Add(int64(100), int64(50), int64(10), int64(40), 3)
	f.Add(int64(0), int64(0), int64(0), int64(0), 0)
	f.Add(int64(999999), int64(999999), int64(999999), int64(999999), 100)

	f.Fuzz(func(t *testing.T, total, retried, deadLetter, success int64, topicCount int) {
		byTopic := buildFuzzByTopic(topicCount)

		original := DLQStats{
			TotalMessages:      total,
			RetriedMessages:    retried,
			DeadLetterMessages: deadLetter,
			SuccessAfterRetry:  success,
			LastDLQTime:        time.Now(),
			ByTopic:            byTopic,
		}

		// 不应 panic
		clone := original.Clone()

		// 验证不变式：值相等
		assert.Equal(t, original.TotalMessages, clone.TotalMessages, "TotalMessages mismatch")
		assert.Equal(t, original.RetriedMessages, clone.RetriedMessages, "RetriedMessages mismatch")
		assert.Equal(t, original.DeadLetterMessages, clone.DeadLetterMessages, "DeadLetterMessages mismatch")

		// 验证 ByTopic 是深拷贝
		if byTopic != nil && clone.ByTopic != nil {
			clone.ByTopic["modified"] = 999
			assert.NotContains(t, original.ByTopic, "modified", "Clone should create independent copy of ByTopic")
		}
	})
}

// =============================================================================
// Producer/Consumer Options Fuzz Tests
// =============================================================================

// FuzzProducerFlushTimeout 模糊测试生产者刷新超时选项。
func FuzzProducerFlushTimeout(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(1000))
	f.Add(int64(10000000000)) // 10 seconds
	f.Add(int64(-1))
	f.Add(int64(9223372036854775807)) // max int64

	f.Fuzz(func(t *testing.T, timeoutNs int64) {
		timeout := time.Duration(timeoutNs)
		opt := WithProducerFlushTimeout(timeout)

		options := defaultProducerOptions()
		originalTimeout := options.FlushTimeout

		// 不应 panic
		opt(options)

		// 验证不变式：只有正值才会更新
		if timeout > 0 {
			if options.FlushTimeout != timeout {
				t.Errorf("FlushTimeout should be %v, got %v", timeout, options.FlushTimeout)
			}
		} else {
			if options.FlushTimeout != originalTimeout {
				t.Errorf("FlushTimeout should remain %v for non-positive input, got %v", originalTimeout, options.FlushTimeout)
			}
		}
	})
}

// FuzzProducerHealthTimeout 模糊测试生产者健康检查超时选项。
func FuzzProducerHealthTimeout(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(1000000000)) // 1 second
	f.Add(int64(5000000000)) // 5 seconds
	f.Add(int64(-1))

	f.Fuzz(func(t *testing.T, timeoutNs int64) {
		timeout := time.Duration(timeoutNs)
		opt := WithProducerHealthTimeout(timeout)

		options := defaultProducerOptions()
		originalTimeout := options.HealthTimeout

		// 不应 panic
		opt(options)

		// 验证不变式
		if timeout > 0 {
			if options.HealthTimeout != timeout {
				t.Errorf("HealthTimeout should be %v, got %v", timeout, options.HealthTimeout)
			}
		} else {
			if options.HealthTimeout != originalTimeout {
				t.Errorf("HealthTimeout should remain %v for non-positive input, got %v", originalTimeout, options.HealthTimeout)
			}
		}
	})
}

// FuzzConsumerPollTimeout 模糊测试消费者轮询超时选项。
func FuzzConsumerPollTimeout(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(100000000))  // 100ms
	f.Add(int64(1000000000)) // 1s
	f.Add(int64(-1))

	f.Fuzz(func(t *testing.T, timeoutNs int64) {
		timeout := time.Duration(timeoutNs)
		opt := WithConsumerPollTimeout(timeout)

		options := defaultConsumerOptions()
		originalTimeout := options.PollTimeout

		// 不应 panic
		opt(options)

		// 验证不变式
		if timeout > 0 {
			if options.PollTimeout != timeout {
				t.Errorf("PollTimeout should be %v, got %v", timeout, options.PollTimeout)
			}
		} else {
			if options.PollTimeout != originalTimeout {
				t.Errorf("PollTimeout should remain %v for non-positive input, got %v", originalTimeout, options.PollTimeout)
			}
		}
	})
}

// FuzzConsumerHealthTimeout 模糊测试消费者健康检查超时选项。
func FuzzConsumerHealthTimeout(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(5000000000)) // 5s
	f.Add(int64(-1))

	f.Fuzz(func(t *testing.T, timeoutNs int64) {
		timeout := time.Duration(timeoutNs)
		opt := WithConsumerHealthTimeout(timeout)

		options := defaultConsumerOptions()
		originalTimeout := options.HealthTimeout

		// 不应 panic
		opt(options)

		// 验证不变式
		if timeout > 0 {
			if options.HealthTimeout != timeout {
				t.Errorf("HealthTimeout should be %v, got %v", timeout, options.HealthTimeout)
			}
		} else {
			if options.HealthTimeout != originalTimeout {
				t.Errorf("HealthTimeout should remain %v for non-positive input, got %v", originalTimeout, options.HealthTimeout)
			}
		}
	})
}

// =============================================================================
// errorString Fuzz Test
// =============================================================================

// FuzzErrorString 模糊测试错误字符串转换。
func FuzzErrorString(f *testing.F) {
	f.Add("")
	f.Add("simple error")
	f.Add("error with special chars: \n\t\r")
	f.Add("中文错误消息")
	f.Add(strings.Repeat("x", 10000))

	f.Fuzz(func(t *testing.T, errMsg string) {
		if len(errMsg) > 100000 {
			return
		}

		var err error
		if errMsg != "" {
			err = errors.New(errMsg)
		}

		// 不应 panic
		result := errorString(err)

		// 验证不变式
		if err == nil {
			if result != "" {
				t.Errorf("errorString(nil) should return empty string, got %q", result)
			}
		} else {
			if result != errMsg {
				t.Errorf("errorString should return error message %q, got %q", errMsg, result)
			}
		}
	})
}

// =============================================================================
// dlqStatsCollector Fuzz Tests
// =============================================================================

// FuzzDLQStatsCollector_IncDeadLetter 模糊测试统计收集器死信增量。
func FuzzDLQStatsCollector_IncDeadLetter(f *testing.F) {
	f.Add("topic-1")
	f.Add("my-dlq-topic")
	f.Add("")
	f.Add("topic-中文")
	f.Add(strings.Repeat("x", 255))

	f.Fuzz(func(t *testing.T, topic string) {
		if len(topic) > 1000 {
			return
		}

		collector := newDLQStatsCollector()

		// 不应 panic
		collector.incDeadLetter(topic)

		stats := collector.get()

		// 验证不变式
		if stats.DeadLetterMessages != 1 {
			t.Errorf("DeadLetterMessages should be 1, got %d", stats.DeadLetterMessages)
		}

		if stats.ByTopic[topic] != 1 {
			t.Errorf("ByTopic[%q] should be 1, got %d", topic, stats.ByTopic[topic])
		}

		if stats.LastDLQTime.IsZero() {
			t.Error("LastDLQTime should not be zero")
		}
	})
}
