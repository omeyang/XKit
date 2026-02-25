package xkafka_test

import (
	"fmt"

	"github.com/omeyang/xkit/pkg/mq/xkafka"
	"github.com/omeyang/xkit/pkg/resilience/xretry"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// ExampleDLQPolicy_Validate 演示 DLQ 策略验证
func ExampleDLQPolicy_Validate() {
	// 创建有效的 DLQ 策略
	policy := &xkafka.DLQPolicy{
		DLQTopic:    "my-topic.dlq",
		RetryPolicy: xretry.NewFixedRetry(3),
	}

	if err := policy.Validate(); err != nil {
		fmt.Println("策略无效:", err)
		return
	}

	fmt.Println("策略有效")
	// Output: 策略有效
}

// ExampleDLQPolicy_Validate_withBackoff 演示带退避策略的 DLQ 配置
func ExampleDLQPolicy_Validate_withBackoff() {
	policy := &xkafka.DLQPolicy{
		DLQTopic:      "orders.dlq",
		RetryTopic:    "orders.retry", // 可选：单独的重试队列
		RetryPolicy:   xretry.NewFixedRetry(5),
		BackoffPolicy: xretry.NewExponentialBackoff(),
	}

	if err := policy.Validate(); err != nil {
		fmt.Println("策略无效:", err)
		return
	}

	fmt.Println("策略有效，包含退避配置")
	// Output: 策略有效，包含退避配置
}

// ExampleDLQPolicy_Validate_invalid 演示无效策略的错误
func ExampleDLQPolicy_Validate_invalid() {
	// DLQTopic 为空会导致验证失败
	policy := &xkafka.DLQPolicy{
		DLQTopic:    "",
		RetryPolicy: xretry.NewFixedRetry(3),
	}

	err := policy.Validate()
	if err != nil {
		fmt.Println("验证错误:", err)
	}
	// Output: 验证错误: xkafka: DLQ topic is required
}

// ExampleDLQStats_Clone 演示统计信息的克隆
func ExampleDLQStats_Clone() {
	stats := xkafka.DLQStats{
		TotalMessages:      100,
		RetriedMessages:    20,
		DeadLetterMessages: 5,
		SuccessAfterRetry:  15,
		ByTopic: map[string]int64{
			"orders":   3,
			"payments": 2,
		},
	}

	// 克隆统计信息（用于安全导出）
	cloned := stats.Clone()

	// 修改原始数据不影响克隆
	stats.ByTopic["orders"] = 100

	fmt.Println("克隆后的 orders 统计:", cloned.ByTopic["orders"])
	fmt.Println("重试后成功数:", cloned.SuccessAfterRetry)
	// Output:
	// 克隆后的 orders 统计: 3
	// 重试后成功数: 15
}

// ExampleDLQMetadata 演示 DLQ 元数据结构
func ExampleDLQMetadata() {
	// DLQMetadata 包含消息进入 DLQ 时的完整上下文
	metadata := xkafka.DLQMetadata{
		OriginalTopic:     "orders",
		OriginalPartition: 0,
		OriginalOffset:    12345,
		FailureReason:     "invalid order format",
		FailureCount:      3,
	}

	fmt.Printf("原始 Topic: %s, 分区: %d, 偏移量: %d\n",
		metadata.OriginalTopic,
		metadata.OriginalPartition,
		metadata.OriginalOffset)
	fmt.Printf("失败原因: %s, 重试次数: %d\n",
		metadata.FailureReason,
		metadata.FailureCount)
	// Output:
	// 原始 Topic: orders, 分区: 0, 偏移量: 12345
	// 失败原因: invalid order format, 重试次数: 3
}

// ExampleDefaultBackoffPolicy 演示默认退避策略
func ExampleDefaultBackoffPolicy() {
	// 获取默认退避策略（指数退避）
	backoff := xkafka.DefaultBackoffPolicy()

	// 计算各次重试的延迟
	fmt.Println("重试延迟示例:")
	for attempt := 1; attempt <= 5; attempt++ {
		delay := backoff.NextDelay(attempt)
		fmt.Printf("  第 %d 次: %v\n", attempt, delay)
	}
	// Note: 由于 jitter 存在，实际输出会有轻微变化
}

// ExampleNewConsumerWithDLQ_validation 演示创建 DLQ 消费者时的参数验证
func ExampleNewConsumerWithDLQ_validation() {
	config := &kafka.ConfigMap{
		"bootstrap.servers": "localhost:9092",
		"group.id":          "test-group",
	}

	// nil 策略会返回错误
	_, err := xkafka.NewConsumerWithDLQ(config, []string{"topic"}, nil)
	if err == xkafka.ErrDLQPolicyRequired {
		fmt.Println("需要提供 DLQ 策略")
	}

	// 无效策略也会返回错误
	policy := &xkafka.DLQPolicy{
		DLQTopic:    "",
		RetryPolicy: xretry.NewFixedRetry(3),
	}
	_, err = xkafka.NewConsumerWithDLQ(config, []string{"topic"}, policy)
	if err == xkafka.ErrDLQTopicRequired {
		fmt.Println("需要提供 DLQ Topic")
	}

	// Output:
	// 需要提供 DLQ 策略
	// 需要提供 DLQ Topic
}

// ExampleProducerStats 演示生产者统计
func ExampleProducerStats() {
	stats := xkafka.ProducerStats{
		MessagesProduced: 1000,
		BytesProduced:    1024000,
		Errors:           5,
	}

	fmt.Printf("已发送消息: %d, 字节: %d, 错误: %d\n",
		stats.MessagesProduced,
		stats.BytesProduced,
		stats.Errors)
	// Output: 已发送消息: 1000, 字节: 1024000, 错误: 5
}

// ExampleConsumerStats 演示消费者统计
func ExampleConsumerStats() {
	stats := xkafka.ConsumerStats{
		MessagesConsumed: 5000,
		BytesConsumed:    5120000,
		Errors:           10,
		Lag:              100,
	}

	fmt.Printf("已消费消息: %d, 字节: %d\n",
		stats.MessagesConsumed,
		stats.BytesConsumed)
	fmt.Printf("错误数: %d, 消费延迟: %d\n",
		stats.Errors,
		stats.Lag)
	// Output:
	// 已消费消息: 5000, 字节: 5120000
	// 错误数: 10, 消费延迟: 100
}

// Example_errors 演示错误常量
func Example_errors() {
	// xkafka 定义了标准错误类型
	fmt.Println("DLQ 策略必需:", xkafka.ErrDLQPolicyRequired)
	fmt.Println("DLQ Topic 必需:", xkafka.ErrDLQTopicRequired)
	fmt.Println("重试策略必需:", xkafka.ErrRetryPolicyRequired)
	fmt.Println("空消息错误:", xkafka.ErrNilMessage)
	fmt.Println("空处理器错误:", xkafka.ErrNilHandler)
	// Output:
	// DLQ 策略必需: xkafka: DLQ policy is required
	// DLQ Topic 必需: xkafka: DLQ topic is required
	// 重试策略必需: xkafka: retry policy is required
	// 空消息错误: mq: nil message
	// 空处理器错误: mq: nil handler
}

// ExampleDLQPolicy_callbacks 演示 DLQ 策略的回调配置
func ExampleDLQPolicy_callbacks() {
	var retryCount, dlqCount int

	policy := &xkafka.DLQPolicy{
		DLQTopic:    "orders.dlq",
		RetryPolicy: xretry.NewFixedRetry(3),
		OnRetry: func(msg *kafka.Message, attempt int, err error) {
			retryCount++
			fmt.Printf("重试第 %d 次: %v\n", attempt, err)
		},
		OnDLQ: func(msg *kafka.Message, err error, metadata xkafka.DLQMetadata) {
			dlqCount++
			fmt.Printf("消息进入 DLQ: %v\n", err)
		},
	}

	// 验证策略
	if err := policy.Validate(); err != nil {
		fmt.Println("策略无效:", err)
		return
	}

	fmt.Println("策略有效，已配置回调")
	// Output: 策略有效，已配置回调
}
