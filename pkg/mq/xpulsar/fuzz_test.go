package xpulsar

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/omeyang/xkit/pkg/resilience/xretry"
)

// =============================================================================
// DLQBuilder Fuzz Tests
// =============================================================================

func FuzzDLQBuilder_WithMaxDeliveries(f *testing.F) {
	// 种子语料库
	f.Add(uint32(0))
	f.Add(uint32(1))
	f.Add(uint32(3))
	f.Add(uint32(10))
	f.Add(uint32(100))
	f.Add(uint32(1000))
	f.Add(^uint32(0)) // max uint32

	f.Fuzz(func(t *testing.T, n uint32) {
		builder := NewDLQBuilder()
		result := builder.WithMaxDeliveries(n)

		// 验证不变式：必须返回 builder 自身（链式调用）
		if result != builder {
			t.Errorf("WithMaxDeliveries should return the same builder")
		}

		// 验证行为
		policy := builder.Build()
		if n > 0 {
			if policy.MaxDeliveries != n {
				t.Errorf("expected MaxDeliveries=%d, got %d", n, policy.MaxDeliveries)
			}
		} else {
			// n == 0 时应保持默认值 3
			if policy.MaxDeliveries != 3 {
				t.Errorf("expected default MaxDeliveries=3 for input 0, got %d", policy.MaxDeliveries)
			}
		}
	})
}

func FuzzDLQBuilder_WithDeadLetterTopic(f *testing.F) {
	// 种子语料库：有效值
	f.Add("dead-letter-topic")
	f.Add("my-app-dlq")
	// 边界值
	f.Add("")
	f.Add("a")
	// 特殊值
	f.Add("中文主题")
	f.Add("topic/with/slashes")
	f.Add("topic:with:colons")
	f.Add(strings.Repeat("x", 1000))
	f.Add("topic-with-special-chars-!@#$%")

	f.Fuzz(func(t *testing.T, topic string) {
		if len(topic) > 10000 {
			return // 避免过长输入
		}

		builder := NewDLQBuilder()
		result := builder.WithDeadLetterTopic(topic)

		if result != builder {
			t.Errorf("WithDeadLetterTopic should return the same builder")
		}

		policy := builder.Build()
		if policy.DeadLetterTopic != topic {
			t.Errorf("expected DeadLetterTopic=%q, got %q", topic, policy.DeadLetterTopic)
		}
	})
}

func FuzzDLQBuilder_WithRetryLetterTopic(f *testing.F) {
	// 种子语料库
	f.Add("retry-letter-topic")
	f.Add("my-app-retry")
	f.Add("")
	f.Add("a")
	f.Add("中文重试主题")
	f.Add(strings.Repeat("r", 500))

	f.Fuzz(func(t *testing.T, topic string) {
		if len(topic) > 10000 {
			return
		}

		builder := NewDLQBuilder()
		result := builder.WithRetryLetterTopic(topic)

		if result != builder {
			t.Errorf("WithRetryLetterTopic should return the same builder")
		}

		policy := builder.Build()
		if policy.RetryLetterTopic != topic {
			t.Errorf("expected RetryLetterTopic=%q, got %q", topic, policy.RetryLetterTopic)
		}
	})
}

func FuzzDLQBuilder_WithInitialSubscription(f *testing.F) {
	// 种子语料库
	f.Add("my-subscription")
	f.Add("sub-001")
	f.Add("")
	f.Add("中文订阅")
	f.Add(strings.Repeat("s", 256))

	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 10000 {
			return
		}

		builder := NewDLQBuilder()
		result := builder.WithInitialSubscription(name)

		if result != builder {
			t.Errorf("WithInitialSubscription should return the same builder")
		}

		policy := builder.Build()
		if policy.InitialSubscriptionName != name {
			t.Errorf("expected InitialSubscriptionName=%q, got %q", name, policy.InitialSubscriptionName)
		}
	})
}

func FuzzDLQBuilder_FullChain(f *testing.F) {
	// 种子语料库：(maxDeliveries, deadLetter, retry, subscription)
	f.Add(uint32(3), "dlq", "retry", "sub")
	f.Add(uint32(5), "", "", "")
	f.Add(uint32(0), "topic", "retry", "subscription")
	f.Add(uint32(100), "a", "b", "c")

	f.Fuzz(func(t *testing.T, maxDeliveries uint32, deadLetter, retry, subscription string) {
		if len(deadLetter) > 1000 || len(retry) > 1000 || len(subscription) > 1000 {
			return
		}

		policy := NewDLQBuilder().
			WithMaxDeliveries(maxDeliveries).
			WithDeadLetterTopic(deadLetter).
			WithRetryLetterTopic(retry).
			WithInitialSubscription(subscription).
			Build()

		// 验证结果
		if policy == nil {
			t.Error("Build() should not return nil")
			return
		}

		// 验证各字段设置正确
		if policy.DeadLetterTopic != deadLetter {
			t.Errorf("DeadLetterTopic mismatch")
		}
		if policy.RetryLetterTopic != retry {
			t.Errorf("RetryLetterTopic mismatch")
		}
		if policy.InitialSubscriptionName != subscription {
			t.Errorf("InitialSubscriptionName mismatch")
		}
	})
}

// =============================================================================
// ConsumerOptionsBuilder Fuzz Tests
// =============================================================================

func FuzzNewConsumerOptionsBuilder(f *testing.F) {
	// 种子语料库
	f.Add("test-topic", "test-subscription")
	f.Add("", "")
	f.Add("a", "b")
	f.Add("中文主题", "中文订阅")
	f.Add(strings.Repeat("t", 500), strings.Repeat("s", 500))
	f.Add("topic/with/path", "subscription-name")

	f.Fuzz(func(t *testing.T, topic, subscription string) {
		if len(topic) > 5000 || len(subscription) > 5000 {
			return
		}

		builder := NewConsumerOptionsBuilder(topic, subscription)

		if builder == nil {
			t.Error("NewConsumerOptionsBuilder should not return nil")
			return
		}

		opts := builder.Build()
		if opts.Topic != topic {
			t.Errorf("expected Topic=%q, got %q", topic, opts.Topic)
		}
		if opts.SubscriptionName != subscription {
			t.Errorf("expected SubscriptionName=%q, got %q", subscription, opts.SubscriptionName)
		}
		// 默认类型应该是 Shared
		if opts.Type != pulsar.Shared {
			t.Errorf("expected default Type=Shared, got %v", opts.Type)
		}
	})
}

func FuzzConsumerOptionsBuilder_WithNackRedeliveryDelay(f *testing.F) {
	// 种子语料库（纳秒）
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(time.Second))
	f.Add(int64(time.Minute))
	f.Add(int64(time.Hour))
	f.Add(int64(-1))
	f.Add(int64(-time.Second))

	f.Fuzz(func(t *testing.T, delayNs int64) {
		delay := time.Duration(delayNs)

		builder := NewConsumerOptionsBuilder("topic", "sub")
		result := builder.WithNackRedeliveryDelay(delay)

		if result != builder {
			t.Errorf("WithNackRedeliveryDelay should return the same builder")
		}

		opts := builder.Build()
		if delay > 0 {
			if opts.NackRedeliveryDelay != delay {
				t.Errorf("expected NackRedeliveryDelay=%v, got %v", delay, opts.NackRedeliveryDelay)
			}
		} else {
			// 非正值应保持默认值（0）
			if opts.NackRedeliveryDelay != 0 {
				t.Errorf("non-positive delay should keep default, got %v", opts.NackRedeliveryDelay)
			}
		}
	})
}

func FuzzConsumerOptionsBuilder_WithRetryEnable(f *testing.F) {
	f.Add(true)
	f.Add(false)

	f.Fuzz(func(t *testing.T, enable bool) {
		builder := NewConsumerOptionsBuilder("topic", "sub")
		result := builder.WithRetryEnable(enable)

		if result != builder {
			t.Errorf("WithRetryEnable should return the same builder")
		}

		opts := builder.Build()
		if opts.RetryEnable != enable {
			t.Errorf("expected RetryEnable=%v, got %v", enable, opts.RetryEnable)
		}
	})
}

// =============================================================================
// topicFromConsumerOptions Fuzz Tests
// =============================================================================

func FuzzTopicFromConsumerOptions_SingleTopic(f *testing.F) {
	// 种子语料库
	f.Add("test-topic")
	f.Add("")
	f.Add("a")
	f.Add("中文主题")
	f.Add("topic/with/slashes")
	f.Add(strings.Repeat("x", 500))

	f.Fuzz(func(t *testing.T, topic string) {
		if len(topic) > 5000 {
			return
		}

		opts := pulsar.ConsumerOptions{
			Topic: topic,
		}

		result := topicFromConsumerOptions(opts)
		if result != topic {
			t.Errorf("expected %q, got %q", topic, result)
		}
	})
}

func FuzzTopicFromConsumerOptions_TopicsPriority(f *testing.F) {
	// 测试 Topic 优先于 Topics
	f.Add("single-topic", "topics-topic")

	f.Fuzz(func(t *testing.T, topic, topicsFirst string) {
		if len(topic) > 1000 || len(topicsFirst) > 1000 {
			return
		}

		// 当 Topic 有值时，应该优先使用 Topic
		opts := pulsar.ConsumerOptions{
			Topic:  topic,
			Topics: []string{topicsFirst},
		}

		result := topicFromConsumerOptions(opts)
		if topic != "" && result != topic {
			t.Errorf("Topic should take priority: expected %q, got %q", topic, result)
		}
	})
}

// =============================================================================
// ToPulsarNackBackoff Fuzz Tests
// =============================================================================

func FuzzNackBackoff_Next(f *testing.F) {
	// 种子语料库
	f.Add(uint32(0))
	f.Add(uint32(1))
	f.Add(uint32(5))
	f.Add(uint32(10))
	f.Add(uint32(100))
	f.Add(^uint32(0)) // max uint32

	policy := xretry.NewExponentialBackoff(
		xretry.WithInitialDelay(100*time.Millisecond),
		xretry.WithMaxDelay(10*time.Second),
	)
	nackBackoff := ToPulsarNackBackoff(policy)

	f.Fuzz(func(t *testing.T, redeliveryCount uint32) {
		// 应该不会 panic
		delay := nackBackoff.Next(redeliveryCount)

		// 验证返回值是非负的
		if delay < 0 {
			t.Errorf("Next() should return non-negative delay, got %v", delay)
		}
	})
}

// =============================================================================
// Client Options Fuzz Tests
// =============================================================================

func FuzzWithConnectionTimeout(f *testing.F) {
	// 种子语料库（纳秒）
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(time.Second))
	f.Add(int64(10 * time.Second))
	f.Add(int64(time.Minute))
	f.Add(int64(-1))
	f.Add(int64(-time.Second))

	f.Fuzz(func(t *testing.T, timeoutNs int64) {
		timeout := time.Duration(timeoutNs)
		opts := defaultOptions()
		original := opts.ConnectionTimeout

		WithConnectionTimeout(timeout)(opts)

		// 验证行为
		if timeout > 0 {
			if opts.ConnectionTimeout != timeout {
				t.Errorf("expected ConnectionTimeout=%v, got %v", timeout, opts.ConnectionTimeout)
			}
		} else {
			// 无效值应该保持原值
			if opts.ConnectionTimeout != original {
				t.Errorf("invalid timeout should keep original value, got %v", opts.ConnectionTimeout)
			}
		}
	})
}

func FuzzWithOperationTimeout(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(30 * time.Second))
	f.Add(int64(time.Minute))
	f.Add(int64(-1))

	f.Fuzz(func(t *testing.T, timeoutNs int64) {
		timeout := time.Duration(timeoutNs)
		opts := defaultOptions()
		original := opts.OperationTimeout

		WithOperationTimeout(timeout)(opts)

		if timeout > 0 {
			if opts.OperationTimeout != timeout {
				t.Errorf("expected OperationTimeout=%v, got %v", timeout, opts.OperationTimeout)
			}
		} else {
			if opts.OperationTimeout != original {
				t.Errorf("invalid timeout should keep original value")
			}
		}
	})
}

func FuzzWithMaxConnectionsPerBroker(f *testing.F) {
	f.Add(0)
	f.Add(1)
	f.Add(5)
	f.Add(100)
	f.Add(-1)
	f.Add(-100)

	f.Fuzz(func(t *testing.T, maxConn int) {
		opts := defaultOptions()
		original := opts.MaxConnectionsPerBroker

		WithMaxConnectionsPerBroker(maxConn)(opts)

		if maxConn > 0 {
			if opts.MaxConnectionsPerBroker != maxConn {
				t.Errorf("expected MaxConnectionsPerBroker=%d, got %d", maxConn, opts.MaxConnectionsPerBroker)
			}
		} else {
			if opts.MaxConnectionsPerBroker != original {
				t.Errorf("invalid maxConn should keep original value")
			}
		}
	})
}

func FuzzWithHealthTimeout(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(5 * time.Second))
	f.Add(int64(time.Minute))
	f.Add(int64(-1))

	f.Fuzz(func(t *testing.T, timeoutNs int64) {
		timeout := time.Duration(timeoutNs)
		opts := defaultOptions()
		original := opts.HealthTimeout

		WithHealthTimeout(timeout)(opts)

		if timeout > 0 {
			if opts.HealthTimeout != timeout {
				t.Errorf("expected HealthTimeout=%v, got %v", timeout, opts.HealthTimeout)
			}
		} else {
			if opts.HealthTimeout != original {
				t.Errorf("invalid timeout should keep original value")
			}
		}
	})
}

func FuzzWithTLS(f *testing.F) {
	// 种子语料库
	f.Add("/path/to/cert.pem", true)
	f.Add("/path/to/cert.pem", false)
	f.Add("", true)
	f.Add("", false)
	f.Add("a", true)
	f.Add(strings.Repeat("/path", 100), false)
	f.Add("中文路径/证书.pem", true)

	f.Fuzz(func(t *testing.T, certPath string, allowInsecure bool) {
		if len(certPath) > 5000 {
			return
		}

		opts := defaultOptions()
		WithTLS(certPath, allowInsecure)(opts)

		if opts.TLSTrustCertsFilePath != certPath {
			t.Errorf("expected TLSTrustCertsFilePath=%q, got %q", certPath, opts.TLSTrustCertsFilePath)
		}
		if opts.TLSAllowInsecureConnection != allowInsecure {
			t.Errorf("expected TLSAllowInsecureConnection=%v, got %v", allowInsecure, opts.TLSAllowInsecureConnection)
		}
	})
}

// =============================================================================
// pulsarAttrs Fuzz Tests
// =============================================================================

func FuzzPulsarAttrs(f *testing.F) {
	f.Add("test-topic")
	f.Add("")
	f.Add("a")
	f.Add("中文主题")
	f.Add(strings.Repeat("x", 500))
	f.Add("topic/with/slashes")

	f.Fuzz(func(t *testing.T, topic string) {
		if len(topic) > 5000 {
			return
		}

		attrs := pulsarAttrs(topic)

		// 验证不变式
		if len(attrs) < 1 {
			t.Error("pulsarAttrs should return at least 1 attribute")
		}

		// 第一个属性应该是 messaging.system
		// （这里我们不能直接检查 Attr 内容，因为它是不透明的）

		if topic != "" {
			// 有 topic 时应该有 2 个属性
			if len(attrs) != 2 {
				t.Errorf("expected 2 attrs for non-empty topic, got %d", len(attrs))
			}
		} else {
			// 空 topic 时应该只有 1 个属性
			if len(attrs) != 1 {
				t.Errorf("expected 1 attr for empty topic, got %d", len(attrs))
			}
		}
	})
}

// =============================================================================
// NewClient Fuzz Tests
// =============================================================================

func FuzzNewClient_URL(f *testing.F) {
	// 种子语料库
	f.Add("")
	f.Add("pulsar://localhost:6650")
	f.Add("pulsar+ssl://localhost:6651")
	f.Add("a")
	f.Add("invalid-url")
	f.Add("http://not-pulsar:8080")
	f.Add(strings.Repeat("x", 1000))

	f.Fuzz(func(t *testing.T, url string) {
		if len(url) > 5000 {
			return
		}

		client, err := NewClient(url)

		// 空 URL 应该返回错误
		if url == "" {
			if err == nil {
				t.Error("empty URL should return error")
			}
			if client != nil {
				t.Error("empty URL should return nil client")
			}
			return
		}

		// 非空 URL 的行为取决于 Pulsar 客户端库
		// 我们只确保不会 panic
		if client != nil {
			// 如果成功创建，需要关闭
			_ = client.Close(context.Background())
		}
	})
}

// =============================================================================
// Stats Fuzz Tests
// =============================================================================

func FuzzStats_Fields(f *testing.F) {
	f.Add(true, 0, 0)
	f.Add(false, 0, 0)
	f.Add(true, 1, 1)
	f.Add(true, 100, 100)
	f.Add(false, -1, -1)
	f.Add(true, 1000000, 1000000)

	f.Fuzz(func(t *testing.T, connected bool, producers, consumers int) {
		stats := Stats{
			Connected:      connected,
			ProducersCount: producers,
			ConsumersCount: consumers,
		}

		// 验证字段设置正确
		if stats.Connected != connected {
			t.Errorf("Connected mismatch")
		}
		if stats.ProducersCount != producers {
			t.Errorf("ProducersCount mismatch")
		}
		if stats.ConsumersCount != consumers {
			t.Errorf("ConsumersCount mismatch")
		}
	})
}

// =============================================================================
// SubscriptionType Fuzz Tests
// =============================================================================

func FuzzConsumerOptionsBuilder_WithType(f *testing.F) {
	// pulsar.SubscriptionType 是 int 类型
	f.Add(int(pulsar.Exclusive))
	f.Add(int(pulsar.Shared))
	f.Add(int(pulsar.Failover))
	f.Add(int(pulsar.KeyShared))
	f.Add(0)
	f.Add(-1)
	f.Add(100)

	f.Fuzz(func(t *testing.T, subType int) {
		builder := NewConsumerOptionsBuilder("topic", "sub")
		result := builder.WithType(pulsar.SubscriptionType(subType))

		if result != builder {
			t.Errorf("WithType should return the same builder")
		}

		opts := builder.Build()
		if opts.Type != pulsar.SubscriptionType(subType) {
			t.Errorf("expected Type=%v, got %v", subType, opts.Type)
		}
	})
}

// =============================================================================
// Combined Builder Fuzz Tests
// =============================================================================

func FuzzConsumerOptionsBuilder_FullChain(f *testing.F) {
	f.Add("topic", "sub", uint32(5), int64(time.Second), true)
	f.Add("", "", uint32(0), int64(0), false)
	f.Add("中文", "订阅", uint32(10), int64(time.Minute), true)

	f.Fuzz(func(t *testing.T, topic, sub string, maxDeliveries uint32, delayNs int64, retryEnable bool) {
		if len(topic) > 1000 || len(sub) > 1000 {
			return
		}

		delay := time.Duration(delayNs)
		// 限制 delay 范围避免极端值
		if delay < -time.Hour || delay > time.Hour {
			return
		}

		opts := NewConsumerOptionsBuilder(topic, sub).
			WithType(pulsar.Shared).
			WithDLQBuilder(NewDLQBuilder().WithMaxDeliveries(maxDeliveries)).
			WithNackRedeliveryDelay(delay).
			WithRetryEnable(retryEnable).
			Build()

		// 验证基本字段
		if opts.Topic != topic {
			t.Errorf("Topic mismatch")
		}
		if opts.SubscriptionName != sub {
			t.Errorf("SubscriptionName mismatch")
		}
		if opts.RetryEnable != retryEnable {
			t.Errorf("RetryEnable mismatch")
		}
	})
}
