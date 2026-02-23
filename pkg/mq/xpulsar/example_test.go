package xpulsar_test

import (
	"context"
	"fmt"
	"time"

	"github.com/omeyang/xkit/pkg/mq/xpulsar"
	"github.com/omeyang/xkit/pkg/resilience/xretry"

	"github.com/apache/pulsar-client-go/pulsar"
)

// ExampleDLQBuilder 演示 DLQ 配置构建器的基本用法
func ExampleDLQBuilder() {
	// 创建 DLQ 配置
	dlqPolicy := xpulsar.NewDLQBuilder().
		WithMaxDeliveries(5).                 // 最多投递 5 次
		WithDeadLetterTopic("orders.dlq").    // 自定义死信 Topic
		WithRetryLetterTopic("orders.retry"). // 自定义重试 Topic
		Build()

	fmt.Println("最大投递次数:", dlqPolicy.MaxDeliveries)
	fmt.Println("死信 Topic:", dlqPolicy.DeadLetterTopic)
	fmt.Println("重试 Topic:", dlqPolicy.RetryLetterTopic)
	// Output:
	// 最大投递次数: 5
	// 死信 Topic: orders.dlq
	// 重试 Topic: orders.retry
}

// ExampleDLQBuilder_minimal 演示最小配置的 DLQ
func ExampleDLQBuilder_minimal() {
	// 最小配置：仅设置最大投递次数
	// Topic 名称由 Pulsar 自动生成
	dlqPolicy := xpulsar.NewDLQBuilder().
		WithMaxDeliveries(3).
		Build()

	fmt.Println("最大投递次数:", dlqPolicy.MaxDeliveries)
	fmt.Println("死信 Topic 为空（自动生成）:", dlqPolicy.DeadLetterTopic == "")
	// Output:
	// 最大投递次数: 3
	// 死信 Topic 为空（自动生成）: true
}

// ExampleConsumerOptionsBuilder 演示 Consumer 配置构建器
func ExampleConsumerOptionsBuilder() {
	// 创建带 DLQ 和退避策略的 Consumer 配置
	// 推荐使用全限定 Topic 名称：persistent://tenant/namespace/topic
	opts := xpulsar.NewConsumerOptionsBuilder(
		"persistent://public/default/orders",
		"order-processor",
	).
		WithType(pulsar.Shared).
		WithDLQBuilder(xpulsar.NewDLQBuilder().WithMaxDeliveries(5)).
		WithNackBackoff(xretry.NewExponentialBackoff()).
		WithRetryEnable(true).
		Build()

	fmt.Println("Topic:", opts.Topic)
	fmt.Println("订阅名:", opts.SubscriptionName)
	fmt.Println("DLQ 已配置:", opts.DLQ != nil)
	fmt.Println("Nack 退避已配置:", opts.NackBackoffPolicy != nil)
	fmt.Println("重试启用:", opts.RetryEnable)
	// Output:
	// Topic: persistent://public/default/orders
	// 订阅名: order-processor
	// DLQ 已配置: true
	// Nack 退避已配置: true
	// 重试启用: true
}

// ExampleConsumerOptionsBuilder_simple 演示简单的 Consumer 配置
func ExampleConsumerOptionsBuilder_simple() {
	// 简单配置：无 DLQ
	opts := xpulsar.NewConsumerOptionsBuilder(
		"persistent://public/default/events",
		"event-handler",
	).
		WithType(pulsar.Exclusive).
		Build()

	fmt.Println("Topic:", opts.Topic)
	fmt.Println("订阅名:", opts.SubscriptionName)
	fmt.Println("订阅类型是 Exclusive:", opts.Type == pulsar.Exclusive)
	// Output:
	// Topic: persistent://public/default/events
	// 订阅名: event-handler
	// 订阅类型是 Exclusive: true
}

// ExampleToPulsarNackBackoff 演示将 xretry 策略转换为 Pulsar Nack 退避
func ExampleToPulsarNackBackoff() {
	// 使用 xretry 的指数退避策略
	xretryBackoff := xretry.NewExponentialBackoff(
		xretry.WithInitialDelay(100*time.Millisecond),
		xretry.WithMaxDelay(30*time.Second),
		xretry.WithMultiplier(2.0),
	)

	// 转换为 Pulsar 可用的 NackBackoffPolicy
	pulsarBackoff := xpulsar.ToPulsarNackBackoff(xretryBackoff)

	// 计算各次重投递的延迟
	fmt.Println("重投递延迟示例:")
	for i := uint32(0); i < 5; i++ {
		delay := pulsarBackoff.Next(i)
		fmt.Printf("  第 %d 次重投递: %v\n", i+1, delay)
	}
	// Note: 由于 jitter 存在，实际输出会有轻微变化
}

// ExampleDefaultBackoffPolicy 演示默认退避策略
func ExampleDefaultBackoffPolicy() {
	// 获取默认退避策略（指数退避）
	backoff := xpulsar.DefaultBackoffPolicy()

	// 计算各次重试的延迟
	fmt.Println("重试延迟示例:")
	for attempt := 1; attempt <= 3; attempt++ {
		delay := backoff.NextDelay(attempt)
		fmt.Printf("  第 %d 次: %v\n", attempt, delay)
	}
	// Note: 由于 jitter 存在，实际输出会有轻微变化
}

// ExampleNewClient 演示从 URL 创建 Client
func ExampleNewClient() {
	// 注意：实际使用需要运行中的 Pulsar 服务
	// 此示例仅展示 URL 验证

	// 空 URL 会返回错误
	_, err := xpulsar.NewClient("")
	if err == xpulsar.ErrEmptyURL {
		fmt.Println("空 URL 错误:", err)
	}

	// Output: 空 URL 错误: xpulsar: empty URL
}

// Example_errors 演示错误常量
func Example_errors() {
	// xpulsar 定义了标准错误类型
	fmt.Println("空客户端错误:", xpulsar.ErrNilClient)
	fmt.Println("空消息错误:", xpulsar.ErrNilMessage)
	fmt.Println("空处理器错误:", xpulsar.ErrNilHandler)
	fmt.Println("空 URL 错误:", xpulsar.ErrEmptyURL)
	fmt.Println("空选项错误:", xpulsar.ErrNilOption)
	fmt.Println("空生产者错误:", xpulsar.ErrNilProducer)
	fmt.Println("空消费者错误:", xpulsar.ErrNilConsumer)
	fmt.Println("客户端已关闭:", xpulsar.ErrClosed)
	// Output:
	// 空客户端错误: mq: nil client
	// 空消息错误: mq: nil message
	// 空处理器错误: mq: nil handler
	// 空 URL 错误: xpulsar: empty URL
	// 空选项错误: xpulsar: nil option
	// 空生产者错误: xpulsar: nil producer
	// 空消费者错误: xpulsar: nil consumer
	// 客户端已关闭: mq: client closed
}

// ExampleNoopTracer 演示空追踪器
func ExampleNoopTracer() {
	// NoopTracer 不执行任何操作，用于禁用追踪
	tracer := xpulsar.NoopTracer{}

	// 可以安全调用，不会panic
	headers := make(map[string]string)
	tracer.Inject(context.Background(), headers)
	_ = tracer.Extract(headers)

	fmt.Println("NoopTracer 可安全使用")
	// Output: NoopTracer 可安全使用
}

// ExampleConsumerOptionsBuilder_withNackDelay 演示配置 Nack 重投递延迟
func ExampleConsumerOptionsBuilder_withNackDelay() {
	opts := xpulsar.NewConsumerOptionsBuilder("tasks", "task-worker").
		WithType(pulsar.Failover).
		WithNackRedeliveryDelay(5 * time.Second). // 固定 5 秒延迟
		Build()

	fmt.Println("Topic:", opts.Topic)
	fmt.Println("Nack 重投递延迟:", opts.NackRedeliveryDelay)
	// Output:
	// Topic: tasks
	// Nack 重投递延迟: 5s
}
