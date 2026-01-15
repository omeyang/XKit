package xkafka

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/resilience/xretry"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// =============================================================================
// DLQPolicy Benchmarks
// =============================================================================

// BenchmarkDLQPolicy_Validate 测试 DLQ 策略验证性能。
func BenchmarkDLQPolicy_Validate(b *testing.B) {
	b.Run("Valid", func(b *testing.B) {
		policy := &DLQPolicy{
			DLQTopic:    "dlq-topic",
			RetryPolicy: xretry.NewFixedRetry(3),
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = policy.Validate()
		}
	})

	b.Run("EmptyDLQTopic", func(b *testing.B) {
		policy := &DLQPolicy{
			DLQTopic:    "",
			RetryPolicy: xretry.NewFixedRetry(3),
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = policy.Validate()
		}
	})

	b.Run("NilRetryPolicy", func(b *testing.B) {
		policy := &DLQPolicy{
			DLQTopic:    "dlq-topic",
			RetryPolicy: nil,
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = policy.Validate()
		}
	})
}

// =============================================================================
// DLQStats Benchmarks
// =============================================================================

// BenchmarkDLQStats_Clone 测试统计信息克隆性能。
func BenchmarkDLQStats_Clone(b *testing.B) {
	b.Run("WithByTopic", func(b *testing.B) {
		stats := DLQStats{
			TotalMessages:      100000,
			RetriedMessages:    50000,
			DeadLetterMessages: 1000,
			SuccessAfterRetry:  49000,
			LastDLQTime:        time.Now(),
			ByTopic: map[string]int64{
				"topic-1": 500,
				"topic-2": 300,
				"topic-3": 200,
			},
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = stats.Clone()
		}
	})

	b.Run("NilByTopic", func(b *testing.B) {
		stats := DLQStats{
			TotalMessages:      100000,
			RetriedMessages:    50000,
			DeadLetterMessages: 1000,
			SuccessAfterRetry:  49000,
			LastDLQTime:        time.Now(),
			ByTopic:            nil,
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = stats.Clone()
		}
	})

	b.Run("LargeByTopic", func(b *testing.B) {
		byTopic := make(map[string]int64, 100)
		for i := 0; i < 100; i++ {
			byTopic["topic-"+strconv.Itoa(i)] = int64(i * 10)
		}
		stats := DLQStats{
			TotalMessages: 100000,
			ByTopic:       byTopic,
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = stats.Clone()
		}
	})
}

// =============================================================================
// Header Helper Benchmarks
// =============================================================================

// BenchmarkGetRetryCount 测试获取重试次数性能。
func BenchmarkGetRetryCount(b *testing.B) {
	b.Run("NoHeader", func(b *testing.B) {
		msg := &kafka.Message{}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = getRetryCount(msg)
		}
	})

	b.Run("WithHeader", func(b *testing.B) {
		msg := &kafka.Message{
			Headers: []kafka.Header{
				{Key: HeaderRetryCount, Value: []byte("5")},
			},
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = getRetryCount(msg)
		}
	})

	b.Run("ManyHeaders", func(b *testing.B) {
		headers := make([]kafka.Header, 10)
		for i := 0; i < 9; i++ {
			headers[i] = kafka.Header{Key: "header-" + strconv.Itoa(i), Value: []byte("value")}
		}
		headers[9] = kafka.Header{Key: HeaderRetryCount, Value: []byte("3")}
		msg := &kafka.Message{Headers: headers}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = getRetryCount(msg)
		}
	})
}

// BenchmarkSetHeader 测试设置 Header 性能。
func BenchmarkSetHeader(b *testing.B) {
	b.Run("NewHeader", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			msg := &kafka.Message{}
			setHeader(msg, "test-key", "test-value")
		}
	})

	b.Run("UpdateExisting", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			msg := &kafka.Message{
				Headers: []kafka.Header{
					{Key: "test-key", Value: []byte("old-value")},
				},
			}
			setHeader(msg, "test-key", "new-value")
		}
	})

	b.Run("ManyHeaders", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			headers := make([]kafka.Header, 10)
			for j := 0; j < 10; j++ {
				headers[j] = kafka.Header{Key: "header-" + strconv.Itoa(j), Value: []byte("value")}
			}
			msg := &kafka.Message{Headers: headers}
			setHeader(msg, "header-5", "updated-value")
		}
	})
}

// BenchmarkGetHeader 测试获取 Header 性能。
func BenchmarkGetHeader(b *testing.B) {
	b.Run("Exists", func(b *testing.B) {
		msg := &kafka.Message{
			Headers: []kafka.Header{
				{Key: "test-key", Value: []byte("test-value")},
			},
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = getHeader(msg, "test-key")
		}
	})

	b.Run("NotExists", func(b *testing.B) {
		msg := &kafka.Message{
			Headers: []kafka.Header{
				{Key: "other-key", Value: []byte("value")},
			},
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = getHeader(msg, "test-key")
		}
	})

	b.Run("EmptyHeaders", func(b *testing.B) {
		msg := &kafka.Message{}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = getHeader(msg, "test-key")
		}
	})
}

// =============================================================================
// DLQ Message Building Benchmarks
// =============================================================================

// BenchmarkBuildDLQMessageFromPolicy 测试 DLQ 消息构建性能。
func BenchmarkBuildDLQMessageFromPolicy(b *testing.B) {
	b.Run("Basic", func(b *testing.B) {
		topic := "original-topic"
		msg := &kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     &topic,
				Partition: 1,
				Offset:    100,
			},
			Key:   []byte("message-key"),
			Value: []byte("message-value"),
		}
		err := errors.New("processing failed")

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = buildDLQMessageFromPolicy(msg, "dlq-topic", err, 3)
		}
	})

	b.Run("WithCustomHeaders", func(b *testing.B) {
		topic := "original-topic"
		msg := &kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     &topic,
				Partition: 1,
				Offset:    100,
			},
			Headers: []kafka.Header{
				{Key: "custom-1", Value: []byte("value-1")},
				{Key: "custom-2", Value: []byte("value-2")},
				{Key: "custom-3", Value: []byte("value-3")},
			},
			Key:   []byte("message-key"),
			Value: []byte("message-value"),
		}
		err := errors.New("processing failed")

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = buildDLQMessageFromPolicy(msg, "dlq-topic", err, 3)
		}
	})

	b.Run("FromRetryQueue", func(b *testing.B) {
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
		err := errors.New("error")

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = buildDLQMessageFromPolicy(msg, "dlq-topic", err, 5)
		}
	})

	b.Run("NilError", func(b *testing.B) {
		topic := "original-topic"
		msg := &kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic: &topic,
			},
			Value: []byte("value"),
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = buildDLQMessageFromPolicy(msg, "dlq-topic", nil, 0)
		}
	})
}

// BenchmarkBuildDLQMetadataFromMessage 测试 DLQ 元数据构建性能。
func BenchmarkBuildDLQMetadataFromMessage(b *testing.B) {
	b.Run("Basic", func(b *testing.B) {
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
		err := errors.New("test error")

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = buildDLQMetadataFromMessage(msg, err, 3)
		}
	})

	b.Run("WithFirstFailTime", func(b *testing.B) {
		topic := "test-topic"
		msg := &kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     &topic,
				Partition: 1,
				Offset:    100,
			},
			Headers: []kafka.Header{
				{Key: HeaderFirstFailTime, Value: []byte("2024-06-15T10:30:00Z")},
			},
		}
		err := errors.New("test error")

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = buildDLQMetadataFromMessage(msg, err, 2)
		}
	})

	b.Run("NilTopic", func(b *testing.B) {
		msg := &kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic: nil,
			},
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = buildDLQMetadataFromMessage(msg, nil, 0)
		}
	})
}

// BenchmarkUpdateRetryHeaders 测试重试头部更新性能。
func BenchmarkUpdateRetryHeaders(b *testing.B) {
	b.Run("FirstRetry", func(b *testing.B) {
		err := errors.New("test error")

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			topic := "test-topic"
			msg := &kafka.Message{
				TopicPartition: kafka.TopicPartition{
					Topic:     &topic,
					Partition: 1,
					Offset:    100,
				},
			}
			updateRetryHeaders(msg, err)
		}
	})

	b.Run("SubsequentRetry", func(b *testing.B) {
		err := errors.New("new error")

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
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
			updateRetryHeaders(msg, err)
		}
	})
}

// =============================================================================
// dlqStatsCollector Benchmarks
// =============================================================================

// BenchmarkDLQStatsCollector 测试统计收集器性能。
func BenchmarkDLQStatsCollector(b *testing.B) {
	b.Run("IncTotal", func(b *testing.B) {
		collector := newDLQStatsCollector()

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			collector.incTotal()
		}
	})

	b.Run("IncRetried", func(b *testing.B) {
		collector := newDLQStatsCollector()

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			collector.incRetried()
		}
	})

	b.Run("IncDeadLetter", func(b *testing.B) {
		collector := newDLQStatsCollector()

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			collector.incDeadLetter("topic-1")
		}
	})

	b.Run("IncSuccessAfterRetry", func(b *testing.B) {
		collector := newDLQStatsCollector()

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			collector.incSuccessAfterRetry()
		}
	})

	b.Run("Get", func(b *testing.B) {
		collector := newDLQStatsCollector()
		// 预填充一些数据
		for i := 0; i < 100; i++ {
			collector.incTotal()
			collector.incRetried()
			collector.incDeadLetter("topic-" + strconv.Itoa(i%5))
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = collector.get()
		}
	})
}

// BenchmarkDLQStatsCollector_Parallel 测试统计收集器并发性能。
func BenchmarkDLQStatsCollector_Parallel(b *testing.B) {
	b.Run("IncTotal", func(b *testing.B) {
		collector := newDLQStatsCollector()

		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				collector.incTotal()
			}
		})
	})

	b.Run("IncDeadLetter", func(b *testing.B) {
		collector := newDLQStatsCollector()
		topics := []string{"topic-1", "topic-2", "topic-3", "topic-4", "topic-5"}

		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				collector.incDeadLetter(topics[i%len(topics)])
				i++
			}
		})
	})

	b.Run("MixedOperations", func(b *testing.B) {
		collector := newDLQStatsCollector()
		topics := []string{"topic-1", "topic-2", "topic-3"}

		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				switch i % 4 {
				case 0:
					collector.incTotal()
				case 1:
					collector.incRetried()
				case 2:
					collector.incDeadLetter(topics[i%len(topics)])
				case 3:
					collector.incSuccessAfterRetry()
				}
				i++
			}
		})
	})

	b.Run("ReadWrite", func(b *testing.B) {
		collector := newDLQStatsCollector()

		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				if i%10 == 0 {
					_ = collector.get()
				} else {
					collector.incTotal()
				}
				i++
			}
		})
	})
}

// =============================================================================
// Error String Benchmark
// =============================================================================

// BenchmarkErrorString 测试错误字符串转换性能。
func BenchmarkErrorString(b *testing.B) {
	b.Run("NilError", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = errorString(nil)
		}
	})

	b.Run("WithError", func(b *testing.B) {
		err := errors.New("this is an error message")

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = errorString(err)
		}
	})

	b.Run("LongError", func(b *testing.B) {
		err := errors.New("this is a very long error message that contains detailed information about what went wrong during the processing of the kafka message including stack traces and other debugging information")

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = errorString(err)
		}
	})
}

// =============================================================================
// Producer/Consumer Options Benchmarks
// =============================================================================

// BenchmarkProducerOptions 测试 Producer 选项应用性能。
func BenchmarkProducerOptions(b *testing.B) {
	b.Run("DefaultOptions", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = defaultProducerOptions()
		}
	})

	b.Run("WithFlushTimeout", func(b *testing.B) {
		opt := WithProducerFlushTimeout(30 * time.Second)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			options := defaultProducerOptions()
			opt(options)
		}
	})

	b.Run("WithHealthTimeout", func(b *testing.B) {
		opt := WithProducerHealthTimeout(10 * time.Second)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			options := defaultProducerOptions()
			opt(options)
		}
	})

	b.Run("AllOptions", func(b *testing.B) {
		opts := []ProducerOption{
			WithProducerFlushTimeout(30 * time.Second),
			WithProducerHealthTimeout(10 * time.Second),
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			options := defaultProducerOptions()
			for _, opt := range opts {
				opt(options)
			}
		}
	})
}

// BenchmarkConsumerOptions 测试 Consumer 选项应用性能。
func BenchmarkConsumerOptions(b *testing.B) {
	b.Run("DefaultOptions", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = defaultConsumerOptions()
		}
	})

	b.Run("WithPollTimeout", func(b *testing.B) {
		opt := WithConsumerPollTimeout(500 * time.Millisecond)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			options := defaultConsumerOptions()
			opt(options)
		}
	})

	b.Run("WithHealthTimeout", func(b *testing.B) {
		opt := WithConsumerHealthTimeout(10 * time.Second)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			options := defaultConsumerOptions()
			opt(options)
		}
	})

	b.Run("AllOptions", func(b *testing.B) {
		opts := []ConsumerOption{
			WithConsumerPollTimeout(500 * time.Millisecond),
			WithConsumerHealthTimeout(10 * time.Second),
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			options := defaultConsumerOptions()
			for _, opt := range opts {
				opt(options)
			}
		}
	})
}
