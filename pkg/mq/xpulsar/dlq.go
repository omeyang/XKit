package xpulsar

import (
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/omeyang/xkit/pkg/resilience/xretry"
)

// DLQBuilder Pulsar DLQ 配置构建器，提供流畅的 API 来构建 pulsar.DLQPolicy。
type DLQBuilder struct {
	maxDeliveries           uint32
	deadLetterTopic         string
	retryLetterTopic        string
	initialSubscriptionName string
	producerOptions         pulsar.ProducerOptions
}

// NewDLQBuilder 创建 DLQ 配置构建器
func NewDLQBuilder() *DLQBuilder {
	return &DLQBuilder{
		maxDeliveries: 3, // 默认 3 次
	}
}

// WithMaxDeliveries 设置最大投递次数
// 超过此次数的消息将被发送到死信 Topic
func (b *DLQBuilder) WithMaxDeliveries(n uint32) *DLQBuilder {
	if n > 0 {
		b.maxDeliveries = n
	}
	return b
}

// WithDeadLetterTopic 设置死信 Topic 名称
// 如果不设置，Pulsar 会自动生成：{topic}-{subscription}-DLQ
func (b *DLQBuilder) WithDeadLetterTopic(topic string) *DLQBuilder {
	b.deadLetterTopic = topic
	return b
}

// WithRetryLetterTopic 设置重试 Topic 名称
// 如果不设置，Pulsar 会自动生成：{topic}-{subscription}-RETRY
func (b *DLQBuilder) WithRetryLetterTopic(topic string) *DLQBuilder {
	b.retryLetterTopic = topic
	return b
}

// WithInitialSubscription 设置初始订阅名
// 用于在第一次订阅死信 Topic 时使用
func (b *DLQBuilder) WithInitialSubscription(name string) *DLQBuilder {
	b.initialSubscriptionName = name
	return b
}

// WithProducerOptions 设置 DLQ Producer 选项
func (b *DLQBuilder) WithProducerOptions(opts pulsar.ProducerOptions) *DLQBuilder {
	b.producerOptions = opts
	return b
}

// Build 构建 pulsar.DLQPolicy
func (b *DLQBuilder) Build() *pulsar.DLQPolicy {
	return &pulsar.DLQPolicy{
		MaxDeliveries:           b.maxDeliveries,
		DeadLetterTopic:         b.deadLetterTopic,
		RetryLetterTopic:        b.retryLetterTopic,
		InitialSubscriptionName: b.initialSubscriptionName,
		ProducerOptions:         b.producerOptions,
	}
}

// xretryNackBackoff 将 xretry.BackoffPolicy 适配为 Pulsar NackBackoffPolicy
type xretryNackBackoff struct {
	policy xretry.BackoffPolicy
}

// ToPulsarNackBackoff 将 xretry.BackoffPolicy 转换为 Pulsar NackBackoffPolicy。
func ToPulsarNackBackoff(policy xretry.BackoffPolicy) pulsar.NackBackoffPolicy {
	if policy == nil {
		return nil
	}
	return &xretryNackBackoff{policy: policy}
}

// Next 返回下次重试的延迟时间
// 实现 pulsar.NackBackoffPolicy 接口
func (b *xretryNackBackoff) Next(redeliveryCount uint32) time.Duration {
	// Pulsar 的 redeliveryCount 从 0 开始，xretry 的 attempt 从 1 开始。
	// 在 32 位架构上 int 为 32 位，直接 int(uint32_max)+1 会溢出为负数或零。
	// 使用 maxNackAttempt 上界保护，超过上界的 redeliveryCount 统一使用最大退避延迟。
	attempt := int(redeliveryCount) + 1
	if attempt <= 0 {
		attempt = maxNackAttempt
	}
	return b.policy.NextDelay(attempt)
}

// maxNackAttempt Nack 退避的最大 attempt 上界。
// 在正常的退避策略中，此值已远超 MaxDelay 收敛点，不影响实际行为。
const maxNackAttempt = 1 << 30

// 确保实现接口
var _ pulsar.NackBackoffPolicy = (*xretryNackBackoff)(nil)

// ConsumerOptionsBuilder Pulsar Consumer 配置构建器
// 提供便捷的方法来配置 DLQ 和重试策略
type ConsumerOptionsBuilder struct {
	opts pulsar.ConsumerOptions
}

// NewConsumerOptionsBuilder 创建 Consumer 配置构建器。
//
// 设计决策: 默认订阅类型为 Shared（而非 Pulsar 原生默认的 Exclusive），
// 因为 Shared 模式更适合多实例部署的微服务场景。如需 Exclusive 模式，
// 请显式调用 WithType(pulsar.Exclusive)。
func NewConsumerOptionsBuilder(topic, subscription string) *ConsumerOptionsBuilder {
	return &ConsumerOptionsBuilder{
		opts: pulsar.ConsumerOptions{
			Topic:            topic,
			SubscriptionName: subscription,
			Type:             pulsar.Shared,
		},
	}
}

// WithType 设置订阅类型
func (b *ConsumerOptionsBuilder) WithType(t pulsar.SubscriptionType) *ConsumerOptionsBuilder {
	b.opts.Type = t
	return b
}

// WithDLQ 设置 DLQ 策略
func (b *ConsumerOptionsBuilder) WithDLQ(dlq *pulsar.DLQPolicy) *ConsumerOptionsBuilder {
	b.opts.DLQ = dlq
	return b
}

// WithDLQBuilder 使用 DLQ Builder 设置 DLQ 策略
func (b *ConsumerOptionsBuilder) WithDLQBuilder(builder *DLQBuilder) *ConsumerOptionsBuilder {
	if builder != nil {
		b.opts.DLQ = builder.Build()
	}
	return b
}

// WithNackBackoff 设置 Nack 退避策略
func (b *ConsumerOptionsBuilder) WithNackBackoff(policy xretry.BackoffPolicy) *ConsumerOptionsBuilder {
	if policy != nil {
		b.opts.NackBackoffPolicy = ToPulsarNackBackoff(policy)
	}
	return b
}

// WithNackRedeliveryDelay 设置 Nack 重投递延迟。
// delay 必须大于 0，否则保持默认值（与 WithConnectionTimeout 等 Duration 选项行为一致）。
func (b *ConsumerOptionsBuilder) WithNackRedeliveryDelay(delay time.Duration) *ConsumerOptionsBuilder {
	if delay > 0 {
		b.opts.NackRedeliveryDelay = delay
	}
	return b
}

// WithRetryEnable 启用重试 Topic
func (b *ConsumerOptionsBuilder) WithRetryEnable(enable bool) *ConsumerOptionsBuilder {
	b.opts.RetryEnable = enable
	return b
}

// Build 构建 pulsar.ConsumerOptions
func (b *ConsumerOptionsBuilder) Build() pulsar.ConsumerOptions {
	return b.opts
}

// Options 返回配置指针，用于进一步自定义
func (b *ConsumerOptionsBuilder) Options() *pulsar.ConsumerOptions {
	return &b.opts
}
