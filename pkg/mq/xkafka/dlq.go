package xkafka

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/omeyang/xkit/pkg/resilience/xretry"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// Kafka 消息 Header 键名常量
const (
	// HeaderRetryCount 重试次数
	HeaderRetryCount = "x-retry-count"
	// HeaderOriginalTopic 原始 Topic
	HeaderOriginalTopic = "x-original-topic"
	// HeaderOriginalPartition 原始分区
	HeaderOriginalPartition = "x-original-partition"
	// HeaderOriginalOffset 原始偏移量
	HeaderOriginalOffset = "x-original-offset"
	// HeaderFirstFailTime 首次失败时间
	HeaderFirstFailTime = "x-first-fail-time"
	// HeaderLastFailTime 最近失败时间
	HeaderLastFailTime = "x-last-fail-time"
	// HeaderFailureReason 失败原因
	HeaderFailureReason = "x-failure-reason"
)

// DLQPolicy Kafka 死信队列策略
type DLQPolicy struct {
	// DLQTopic 死信 Topic 名称（必须）
	DLQTopic string

	// RetryTopic 重试 Topic 名称（可选，空字符串表示原地重试）
	RetryTopic string

	// RetryPolicy 重试策略（必须）
	RetryPolicy xretry.RetryPolicy

	// BackoffPolicy 退避策略（可选，默认无延迟）
	BackoffPolicy xretry.BackoffPolicy

	// ProducerConfig DLQ Producer 配置（可选，nil 时复用消费者配置）
	ProducerConfig *kafka.ConfigMap

	// OnDLQ 消息进入 DLQ 时的回调（可选）
	OnDLQ func(msg *kafka.Message, err error, metadata DLQMetadata)

	// OnRetry 消息重试时的回调（可选）
	OnRetry func(msg *kafka.Message, attempt int, err error)
}

// Validate 验证策略配置
func (p *DLQPolicy) Validate() error {
	if p.DLQTopic == "" {
		return ErrDLQTopicRequired
	}
	if p.RetryPolicy == nil {
		return ErrRetryPolicyRequired
	}
	return nil
}

// DLQMetadata 死信消息元数据
type DLQMetadata struct {
	OriginalTopic     string    `json:"original_topic"`
	OriginalPartition int32     `json:"original_partition"`
	OriginalOffset    int64     `json:"original_offset"`
	OriginalTimestamp time.Time `json:"original_timestamp"`
	FailureReason     string    `json:"failure_reason"`
	FailureCount      int       `json:"failure_count"`
	FirstFailureTime  time.Time `json:"first_failure_time"`
	LastFailureTime   time.Time `json:"last_failure_time"`
}

// DLQStats DLQ 统计信息。
// 提示：handler 失败次数 = RetriedMessages + DeadLetterMessages - SuccessAfterRetry
type DLQStats struct {
	// TotalMessages 处理的消息总数（包括成功和失败）。
	TotalMessages int64 `json:"total_messages"`
	// RetriedMessages 触发重试的消息数。
	RetriedMessages int64 `json:"retried_messages"`
	// DeadLetterMessages 进入死信队列的消息数。
	DeadLetterMessages int64 `json:"dead_letter_messages"`
	// SuccessAfterRetry 重试后成功处理的消息数。
	SuccessAfterRetry int64 `json:"success_after_retry"`
	// LastDLQTime 最近一次消息进入 DLQ 的时间。
	LastDLQTime time.Time `json:"last_dlq_time,omitempty"`
	// ByTopic 按原始 Topic 分组的 DLQ 消息统计。
	ByTopic map[string]int64 `json:"by_topic,omitempty"`
}

// Clone 返回统计信息的副本
func (s *DLQStats) Clone() DLQStats {
	clone := *s
	if s.ByTopic != nil {
		clone.ByTopic = make(map[string]int64, len(s.ByTopic))
		for k, v := range s.ByTopic {
			clone.ByTopic[k] = v
		}
	}
	return clone
}

// MessageHandler 消息处理函数
type MessageHandler func(ctx context.Context, msg *kafka.Message) error

// ConsumerWithDLQ 支持 DLQ 的 Kafka 消费者接口
type ConsumerWithDLQ interface {
	Consumer

	// ConsumeWithRetry 消费单条消息，自动处理重试和 DLQ
	// 返回 nil 表示消息处理成功或已发送到 DLQ
	ConsumeWithRetry(ctx context.Context, handler MessageHandler) error

	// ConsumeLoop 启动消费循环
	// 会持续消费消息直到 context 取消
	ConsumeLoop(ctx context.Context, handler MessageHandler) error

	// SendToDLQ 手动发送消息到 DLQ
	SendToDLQ(ctx context.Context, msg *kafka.Message, reason error) error

	// DLQStats 返回 DLQ 统计信息
	DLQStats() DLQStats
}

// dlqStatsCollector DLQ 统计收集器
type dlqStatsCollector struct {
	stats DLQStats
	mu    sync.RWMutex
}

func newDLQStatsCollector() *dlqStatsCollector {
	return &dlqStatsCollector{
		stats: DLQStats{
			ByTopic: make(map[string]int64),
		},
	}
}

func (c *dlqStatsCollector) incTotal() {
	c.mu.Lock()
	c.stats.TotalMessages++
	c.mu.Unlock()
}

func (c *dlqStatsCollector) incRetried() {
	c.mu.Lock()
	c.stats.RetriedMessages++
	c.mu.Unlock()
}

func (c *dlqStatsCollector) incDeadLetter(topic string) {
	c.mu.Lock()
	c.stats.DeadLetterMessages++
	c.stats.LastDLQTime = time.Now()
	c.stats.ByTopic[topic]++
	c.mu.Unlock()
}

func (c *dlqStatsCollector) incSuccessAfterRetry() {
	c.mu.Lock()
	c.stats.SuccessAfterRetry++
	c.mu.Unlock()
}

func (c *dlqStatsCollector) get() DLQStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats.Clone()
}

// Helper functions for message headers

// getRetryCount 从消息 Header 中获取重试次数
func getRetryCount(msg *kafka.Message) int {
	for _, h := range msg.Headers {
		if h.Key == HeaderRetryCount {
			count, err := strconv.Atoi(string(h.Value))
			if err != nil || count < 0 {
				return 0
			}
			return count
		}
	}
	return 0
}

// setHeader 设置或更新消息 Header
func setHeader(msg *kafka.Message, key, value string) {
	for i, h := range msg.Headers {
		if h.Key == key {
			msg.Headers[i].Value = []byte(value)
			return
		}
	}
	msg.Headers = append(msg.Headers, kafka.Header{
		Key:   key,
		Value: []byte(value),
	})
}

// getHeader 获取消息 Header 值
func getHeader(msg *kafka.Message, key string) string {
	for _, h := range msg.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

// buildDLQMessageFromPolicy 根据策略构建 DLQ 消息
// 这是一个纯函数，用于测试和复用
//
// 注意：当消息来自重试队列时，会保留已有的 x-original-* 头部，
// 而不是使用当前消息的 TopicPartition（那指向的是重试队列）。
func buildDLQMessageFromPolicy(original *kafka.Message, dlqTopic string, reason error, retryCount int) *kafka.Message {
	now := time.Now()
	nowStr := now.Format(time.RFC3339)

	// 从现有头部获取原始信息（如果存在）
	// 这对于来自重试队列的消息很重要，因为 TopicPartition 指向的是重试队列
	existingOriginalTopic := getHeader(original, HeaderOriginalTopic)
	existingOriginalPartition := getHeader(original, HeaderOriginalPartition)
	existingOriginalOffset := getHeader(original, HeaderOriginalOffset)
	existingFirstFailTime := getHeader(original, HeaderFirstFailTime)

	// 确定原始主题/分区/偏移量
	// 优先使用已有头部（来自重试队列的消息），否则使用当前消息的 TopicPartition
	originalTopic := existingOriginalTopic
	if originalTopic == "" && original.TopicPartition.Topic != nil {
		originalTopic = *original.TopicPartition.Topic
	}

	originalPartition := existingOriginalPartition
	if originalPartition == "" {
		originalPartition = strconv.Itoa(int(original.TopicPartition.Partition))
	}

	originalOffset := existingOriginalOffset
	if originalOffset == "" {
		originalOffset = strconv.FormatInt(int64(original.TopicPartition.Offset), 10)
	}

	// 确定首次失败时间
	firstFailTime := existingFirstFailTime
	if firstFailTime == "" {
		firstFailTime = nowStr
	}

	// 准备 Headers（预分配容量）
	headers := make([]kafka.Header, 0, len(original.Headers)+7)

	// 添加元数据 Headers
	headers = append(headers,
		kafka.Header{Key: HeaderOriginalTopic, Value: []byte(originalTopic)},
		kafka.Header{Key: HeaderOriginalPartition, Value: []byte(originalPartition)},
		kafka.Header{Key: HeaderOriginalOffset, Value: []byte(originalOffset)},
		kafka.Header{Key: HeaderRetryCount, Value: []byte(strconv.Itoa(retryCount))},
		kafka.Header{Key: HeaderFailureReason, Value: []byte(errorString(reason))},
		kafka.Header{Key: HeaderFirstFailTime, Value: []byte(firstFailTime)},
		kafka.Header{Key: HeaderLastFailTime, Value: []byte(nowStr)},
	)

	// 保留原始 Headers（排除会被覆盖的）
	skipKeys := map[string]bool{
		HeaderRetryCount:        true,
		HeaderLastFailTime:      true,
		HeaderFirstFailTime:     true,
		HeaderFailureReason:     true,
		HeaderOriginalTopic:     true,
		HeaderOriginalPartition: true,
		HeaderOriginalOffset:    true,
	}
	for _, h := range original.Headers {
		if !skipKeys[h.Key] {
			headers = append(headers, h)
		}
	}

	return &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &dlqTopic,
			Partition: kafka.PartitionAny,
		},
		Key:     original.Key,
		Value:   original.Value,
		Headers: headers,
	}
}

// buildDLQMetadataFromMessage 从消息构建 DLQ 元数据
// 这是一个纯函数，用于测试和复用
//
// 注意：当消息来自重试队列时，会优先使用 x-original-* 头部中的值，
// 而不是使用当前消息的 TopicPartition（那指向的是重试队列）。
// 这与 buildDLQMessageFromPolicy 保持一致。
func buildDLQMetadataFromMessage(msg *kafka.Message, reason error, retryCount int) DLQMetadata {
	// 确定原始主题
	originalTopic := getHeader(msg, HeaderOriginalTopic)
	if originalTopic == "" && msg.TopicPartition.Topic != nil {
		originalTopic = *msg.TopicPartition.Topic
	}

	// 确定原始分区、偏移量和首次失败时间
	originalPartition := parseOriginalPartition(msg)
	originalOffset := parseOriginalOffset(msg)
	firstFailTime := parseFirstFailTime(msg)

	return DLQMetadata{
		OriginalTopic:     originalTopic,
		OriginalPartition: originalPartition,
		OriginalOffset:    originalOffset,
		OriginalTimestamp: msg.Timestamp,
		FailureReason:     errorString(reason),
		FailureCount:      retryCount + 1,
		FirstFailureTime:  firstFailTime,
		LastFailureTime:   time.Now(),
	}
}

// parseOriginalPartition 从消息头解析原始分区号。
func parseOriginalPartition(msg *kafka.Message) int32 {
	headerVal := getHeader(msg, HeaderOriginalPartition)
	if headerVal == "" {
		return msg.TopicPartition.Partition
	}
	// ParseInt with bitSize 32 保证返回值在 int32 范围内
	parsed, err := strconv.ParseInt(headerVal, 10, 32)
	if err != nil {
		return msg.TopicPartition.Partition
	}
	return int32(parsed)
}

// parseOriginalOffset 从消息头解析原始偏移量。
func parseOriginalOffset(msg *kafka.Message) int64 {
	headerVal := getHeader(msg, HeaderOriginalOffset)
	if headerVal == "" {
		return int64(msg.TopicPartition.Offset)
	}
	parsed, err := strconv.ParseInt(headerVal, 10, 64)
	if err != nil {
		return int64(msg.TopicPartition.Offset)
	}
	return parsed
}

// parseFirstFailTime 从消息头解析首次失败时间。
func parseFirstFailTime(msg *kafka.Message) time.Time {
	headerVal := getHeader(msg, HeaderFirstFailTime)
	if headerVal == "" {
		return time.Now()
	}
	parsed, err := time.Parse(time.RFC3339, headerVal)
	if err != nil {
		return time.Now()
	}
	return parsed
}

// errorString 安全地获取错误字符串，nil 返回空字符串
func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// updateRetryHeaders 更新重试相关的消息头
// 这是一个纯函数，用于测试和复用
func updateRetryHeaders(msg *kafka.Message, err error) {
	count := getRetryCount(msg) + 1
	now := time.Now().Format(time.RFC3339)

	setHeader(msg, HeaderRetryCount, strconv.Itoa(count))
	setHeader(msg, HeaderLastFailTime, now)
	setHeader(msg, HeaderFailureReason, errorString(err))

	// 首次失败时设置原始信息
	if count == 1 {
		setHeader(msg, HeaderFirstFailTime, now)
		if msg.TopicPartition.Topic != nil {
			setHeader(msg, HeaderOriginalTopic, *msg.TopicPartition.Topic)
		}
		setHeader(msg, HeaderOriginalPartition, strconv.Itoa(int(msg.TopicPartition.Partition)))
		setHeader(msg, HeaderOriginalOffset, strconv.FormatInt(int64(msg.TopicPartition.Offset), 10))
	}
}
