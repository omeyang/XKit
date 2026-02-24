package xpulsar

import (
	"testing"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/omeyang/xkit/pkg/resilience/xretry"
	"github.com/stretchr/testify/assert"
)

func TestDLQBuilder(t *testing.T) {
	t.Run("DefaultValues", func(t *testing.T) {
		b := NewDLQBuilder()
		p := b.Build()

		assert.Equal(t, uint32(3), p.MaxDeliveries)
		assert.Equal(t, "", p.DeadLetterTopic)
		assert.Equal(t, "", p.RetryLetterTopic)
		assert.Equal(t, "", p.InitialSubscriptionName)
	})

	t.Run("WithMaxDeliveries", func(t *testing.T) {
		b := NewDLQBuilder().WithMaxDeliveries(5)
		p := b.Build()

		assert.Equal(t, uint32(5), p.MaxDeliveries)
	})

	t.Run("WithMaxDeliveries_Zero", func(t *testing.T) {
		// 0 不应该改变默认值
		b := NewDLQBuilder().WithMaxDeliveries(0)
		p := b.Build()

		assert.Equal(t, uint32(3), p.MaxDeliveries)
	})

	t.Run("WithDeadLetterTopic", func(t *testing.T) {
		b := NewDLQBuilder().WithDeadLetterTopic("my-dlq")
		p := b.Build()

		assert.Equal(t, "my-dlq", p.DeadLetterTopic)
	})

	t.Run("WithRetryLetterTopic", func(t *testing.T) {
		b := NewDLQBuilder().WithRetryLetterTopic("my-retry")
		p := b.Build()

		assert.Equal(t, "my-retry", p.RetryLetterTopic)
	})

	t.Run("WithInitialSubscription", func(t *testing.T) {
		b := NewDLQBuilder().WithInitialSubscription("my-sub")
		p := b.Build()

		assert.Equal(t, "my-sub", p.InitialSubscriptionName)
	})

	t.Run("WithProducerOptions", func(t *testing.T) {
		opts := pulsar.ProducerOptions{
			Name: "dlq-producer",
		}
		b := NewDLQBuilder().WithProducerOptions(opts)
		p := b.Build()

		assert.Equal(t, "dlq-producer", p.ProducerOptions.Name)
	})

	t.Run("Chaining", func(t *testing.T) {
		p := NewDLQBuilder().
			WithMaxDeliveries(5).
			WithDeadLetterTopic("my-dlq").
			WithRetryLetterTopic("my-retry").
			WithInitialSubscription("my-sub").
			Build()

		assert.Equal(t, uint32(5), p.MaxDeliveries)
		assert.Equal(t, "my-dlq", p.DeadLetterTopic)
		assert.Equal(t, "my-retry", p.RetryLetterTopic)
		assert.Equal(t, "my-sub", p.InitialSubscriptionName)
	})
}

func TestToPulsarNackBackoff(t *testing.T) {
	t.Run("Nil", func(t *testing.T) {
		result := ToPulsarNackBackoff(nil)
		assert.Nil(t, result)
	})

	t.Run("FixedBackoff", func(t *testing.T) {
		backoff := xretry.NewFixedBackoff(100 * time.Millisecond)
		pulsarBackoff := ToPulsarNackBackoff(backoff)

		// Pulsar 的 redeliveryCount 从 0 开始
		assert.Equal(t, 100*time.Millisecond, pulsarBackoff.Next(0))
		assert.Equal(t, 100*time.Millisecond, pulsarBackoff.Next(1))
		assert.Equal(t, 100*time.Millisecond, pulsarBackoff.Next(10))
	})

	t.Run("ExponentialBackoff", func(t *testing.T) {
		backoff := xretry.NewExponentialBackoff(
			xretry.WithInitialDelay(100*time.Millisecond),
			xretry.WithMultiplier(2.0),
			xretry.WithMaxDelay(1*time.Second),
			xretry.WithJitter(0), // 无抖动便于测试
		)
		pulsarBackoff := ToPulsarNackBackoff(backoff)

		// redeliveryCount=0 对应 attempt=1
		assert.Equal(t, 100*time.Millisecond, pulsarBackoff.Next(0))
		// redeliveryCount=1 对应 attempt=2
		assert.Equal(t, 200*time.Millisecond, pulsarBackoff.Next(1))
		// redeliveryCount=2 对应 attempt=3
		assert.Equal(t, 400*time.Millisecond, pulsarBackoff.Next(2))
	})

	t.Run("LinearBackoff", func(t *testing.T) {
		backoff := xretry.NewLinearBackoff(100*time.Millisecond, 50*time.Millisecond, 500*time.Millisecond)
		pulsarBackoff := ToPulsarNackBackoff(backoff)

		assert.Equal(t, 100*time.Millisecond, pulsarBackoff.Next(0))
		assert.Equal(t, 150*time.Millisecond, pulsarBackoff.Next(1))
		assert.Equal(t, 200*time.Millisecond, pulsarBackoff.Next(2))
	})
}

func TestConsumerOptionsBuilder(t *testing.T) {
	t.Run("Basic", func(t *testing.T) {
		b := NewConsumerOptionsBuilder("my-topic", "my-sub")
		opts := b.Build()

		assert.Equal(t, "my-topic", opts.Topic)
		assert.Equal(t, "my-sub", opts.SubscriptionName)
		assert.Equal(t, pulsar.Shared, opts.Type)
	})

	t.Run("WithType", func(t *testing.T) {
		opts := NewConsumerOptionsBuilder("my-topic", "my-sub").
			WithType(pulsar.Exclusive).
			Build()

		assert.Equal(t, pulsar.Exclusive, opts.Type)
	})

	t.Run("WithDLQ", func(t *testing.T) {
		dlq := &pulsar.DLQPolicy{
			MaxDeliveries: 5,
		}
		opts := NewConsumerOptionsBuilder("my-topic", "my-sub").
			WithDLQ(dlq).
			Build()

		assert.Equal(t, uint32(5), opts.DLQ.MaxDeliveries)
	})

	t.Run("WithDLQBuilder", func(t *testing.T) {
		builder := NewDLQBuilder().
			WithMaxDeliveries(10).
			WithDeadLetterTopic("custom-dlq")

		opts := NewConsumerOptionsBuilder("my-topic", "my-sub").
			WithDLQBuilder(builder).
			Build()

		assert.Equal(t, uint32(10), opts.DLQ.MaxDeliveries)
		assert.Equal(t, "custom-dlq", opts.DLQ.DeadLetterTopic)
	})

	t.Run("WithDLQBuilder_Nil", func(t *testing.T) {
		opts := NewConsumerOptionsBuilder("my-topic", "my-sub").
			WithDLQBuilder(nil).
			Build()

		assert.Nil(t, opts.DLQ)
	})

	t.Run("WithNackBackoff", func(t *testing.T) {
		backoff := xretry.NewFixedBackoff(100 * time.Millisecond)
		opts := NewConsumerOptionsBuilder("my-topic", "my-sub").
			WithNackBackoff(backoff).
			Build()

		assert.NotNil(t, opts.NackBackoffPolicy)
		assert.Equal(t, 100*time.Millisecond, opts.NackBackoffPolicy.Next(0))
	})

	t.Run("WithNackBackoff_Nil", func(t *testing.T) {
		opts := NewConsumerOptionsBuilder("my-topic", "my-sub").
			WithNackBackoff(nil).
			Build()

		assert.Nil(t, opts.NackBackoffPolicy)
	})

	t.Run("WithNackRedeliveryDelay", func(t *testing.T) {
		opts := NewConsumerOptionsBuilder("my-topic", "my-sub").
			WithNackRedeliveryDelay(500 * time.Millisecond).
			Build()

		assert.Equal(t, 500*time.Millisecond, opts.NackRedeliveryDelay)
	})

	t.Run("WithNackRedeliveryDelay_Negative", func(t *testing.T) {
		opts := NewConsumerOptionsBuilder("my-topic", "my-sub").
			WithNackRedeliveryDelay(-1 * time.Second).
			Build()

		assert.Equal(t, time.Duration(0), opts.NackRedeliveryDelay, "负值应被忽略")
	})

	t.Run("WithNackRedeliveryDelay_Zero", func(t *testing.T) {
		opts := NewConsumerOptionsBuilder("my-topic", "my-sub").
			WithNackRedeliveryDelay(0).
			Build()

		assert.Equal(t, time.Duration(0), opts.NackRedeliveryDelay, "零值应被忽略")
	})

	t.Run("WithRetryEnable", func(t *testing.T) {
		opts := NewConsumerOptionsBuilder("my-topic", "my-sub").
			WithRetryEnable(true).
			Build()

		assert.True(t, opts.RetryEnable)
	})

	t.Run("Options", func(t *testing.T) {
		b := NewConsumerOptionsBuilder("my-topic", "my-sub")
		opts := b.Options()

		// 直接修改
		opts.ReceiverQueueSize = 1000

		// 验证修改生效
		built := b.Build()
		assert.Equal(t, 1000, built.ReceiverQueueSize)
	})

	t.Run("FullChaining", func(t *testing.T) {
		backoff := xretry.NewExponentialBackoff()
		dlqBuilder := NewDLQBuilder().
			WithMaxDeliveries(5).
			WithDeadLetterTopic("dlq-topic")

		opts := NewConsumerOptionsBuilder("my-topic", "my-sub").
			WithType(pulsar.KeyShared).
			WithDLQBuilder(dlqBuilder).
			WithNackBackoff(backoff).
			WithNackRedeliveryDelay(1 * time.Second).
			WithRetryEnable(true).
			Build()

		assert.Equal(t, "my-topic", opts.Topic)
		assert.Equal(t, "my-sub", opts.SubscriptionName)
		assert.Equal(t, pulsar.KeyShared, opts.Type)
		assert.Equal(t, uint32(5), opts.DLQ.MaxDeliveries)
		assert.Equal(t, "dlq-topic", opts.DLQ.DeadLetterTopic)
		assert.NotNil(t, opts.NackBackoffPolicy)
		assert.Equal(t, 1*time.Second, opts.NackRedeliveryDelay)
		assert.True(t, opts.RetryEnable)
	})
}
