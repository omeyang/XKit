package xpulsar

import (
	"testing"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/omeyang/xkit/pkg/observability/xmetrics"
	"github.com/omeyang/xkit/pkg/resilience/xretry"
)

// =============================================================================
// DLQBuilder Benchmarks
// =============================================================================

func BenchmarkNewDLQBuilder(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = NewDLQBuilder()
	}
}

func BenchmarkDLQBuilder_Chaining(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		builder := NewDLQBuilder()
		builder.WithMaxDeliveries(5).
			WithDeadLetterTopic("dead-letter-topic").
			WithRetryLetterTopic("retry-letter-topic").
			WithInitialSubscription("my-subscription")
	}
}

func BenchmarkDLQBuilder_Build(b *testing.B) {
	builder := NewDLQBuilder().
		WithMaxDeliveries(5).
		WithDeadLetterTopic("dead-letter-topic").
		WithRetryLetterTopic("retry-letter-topic").
		WithInitialSubscription("my-subscription")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = builder.Build()
	}
}

func BenchmarkDLQBuilder_WithProducerOptions(b *testing.B) {
	opts := pulsar.ProducerOptions{
		Topic:              "test-topic",
		Name:               "test-producer",
		SendTimeout:        10 * time.Second,
		MaxPendingMessages: 100,
	}

	builder := NewDLQBuilder()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		builder.WithProducerOptions(opts)
	}
}

func BenchmarkDLQBuilder_FullWorkflow(b *testing.B) {
	opts := pulsar.ProducerOptions{
		Topic: "dlq-producer-topic",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		policy := NewDLQBuilder().
			WithMaxDeliveries(5).
			WithDeadLetterTopic("dead-letter-topic").
			WithRetryLetterTopic("retry-letter-topic").
			WithInitialSubscription("my-subscription").
			WithProducerOptions(opts).
			Build()
		_ = policy
	}
}

// =============================================================================
// ToPulsarNackBackoff Benchmarks
// =============================================================================

func BenchmarkToPulsarNackBackoff_Nil(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ToPulsarNackBackoff(nil)
	}
}

func BenchmarkToPulsarNackBackoff_WithPolicy(b *testing.B) {
	policy := xretry.NewExponentialBackoff(
		xretry.WithInitialDelay(100*time.Millisecond),
		xretry.WithMaxDelay(10*time.Second),
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ToPulsarNackBackoff(policy)
	}
}

func BenchmarkNackBackoff_Next(b *testing.B) {
	policy := xretry.NewExponentialBackoff(
		xretry.WithInitialDelay(100*time.Millisecond),
		xretry.WithMaxDelay(10*time.Second),
	)
	nackBackoff := ToPulsarNackBackoff(policy)

	b.Run("Redelivery0", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = nackBackoff.Next(0)
		}
	})

	b.Run("Redelivery5", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = nackBackoff.Next(5)
		}
	})

	b.Run("Redelivery10", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = nackBackoff.Next(10)
		}
	})
}

// =============================================================================
// ConsumerOptionsBuilder Benchmarks
// =============================================================================

func BenchmarkNewConsumerOptionsBuilder(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = NewConsumerOptionsBuilder("test-topic", "test-subscription")
	}
}

func BenchmarkConsumerOptionsBuilder_Chaining(b *testing.B) {
	dlq := NewDLQBuilder().WithMaxDeliveries(3).Build()
	backoffPolicy := xretry.NewExponentialBackoff()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		builder := NewConsumerOptionsBuilder("test-topic", "test-subscription")
		builder.WithType(pulsar.Shared).
			WithDLQ(dlq).
			WithNackBackoff(backoffPolicy).
			WithNackRedeliveryDelay(1 * time.Second).
			WithRetryEnable(true)
	}
}

func BenchmarkConsumerOptionsBuilder_Build(b *testing.B) {
	builder := NewConsumerOptionsBuilder("test-topic", "test-subscription").
		WithType(pulsar.Shared).
		WithRetryEnable(true)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = builder.Build()
	}
}

func BenchmarkConsumerOptionsBuilder_WithDLQBuilder(b *testing.B) {
	dlqBuilder := NewDLQBuilder().
		WithMaxDeliveries(5).
		WithDeadLetterTopic("dead-letter-topic")

	builder := NewConsumerOptionsBuilder("test-topic", "test-subscription")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		builder.WithDLQBuilder(dlqBuilder)
	}
}

func BenchmarkConsumerOptionsBuilder_FullWorkflow(b *testing.B) {
	backoffPolicy := xretry.NewExponentialBackoff()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		opts := NewConsumerOptionsBuilder("test-topic", "test-subscription").
			WithType(pulsar.Shared).
			WithDLQBuilder(NewDLQBuilder().
				WithMaxDeliveries(5).
				WithDeadLetterTopic("dead-letter-topic").
				WithRetryLetterTopic("retry-letter-topic")).
			WithNackBackoff(backoffPolicy).
			WithNackRedeliveryDelay(1 * time.Second).
			WithRetryEnable(true).
			Build()
		_ = opts
	}
}

func BenchmarkConsumerOptionsBuilder_Options(b *testing.B) {
	builder := NewConsumerOptionsBuilder("test-topic", "test-subscription")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = builder.Options()
	}
}

// =============================================================================
// topicFromConsumerOptions Benchmarks
// =============================================================================

func BenchmarkTopicFromConsumerOptions(b *testing.B) {
	b.Run("SingleTopic", func(b *testing.B) {
		opts := pulsar.ConsumerOptions{
			Topic: "test-topic",
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = topicFromConsumerOptions(opts)
		}
	})

	b.Run("SingleTopicFromTopics", func(b *testing.B) {
		opts := pulsar.ConsumerOptions{
			Topics: []string{"test-topic"},
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = topicFromConsumerOptions(opts)
		}
	})

	b.Run("MultipleTopics", func(b *testing.B) {
		opts := pulsar.ConsumerOptions{
			Topics: []string{"topic1", "topic2", "topic3"},
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = topicFromConsumerOptions(opts)
		}
	})

	b.Run("EmptyTopic", func(b *testing.B) {
		opts := pulsar.ConsumerOptions{}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = topicFromConsumerOptions(opts)
		}
	})
}

// =============================================================================
// pulsarAttrs Benchmarks
// =============================================================================

func BenchmarkPulsarAttrs(b *testing.B) {
	b.Run("WithTopic", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = pulsarAttrs("test-topic")
		}
	})

	b.Run("EmptyTopic", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = pulsarAttrs("")
		}
	})
}

// =============================================================================
// Client Options Benchmarks
// =============================================================================

func BenchmarkDefaultOptions(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = defaultOptions()
	}
}

func BenchmarkWithConnectionTimeout(b *testing.B) {
	opts := defaultOptions()
	optFn := WithConnectionTimeout(20 * time.Second)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		optFn(opts)
	}
}

func BenchmarkWithOperationTimeout(b *testing.B) {
	opts := defaultOptions()
	optFn := WithOperationTimeout(60 * time.Second)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		optFn(opts)
	}
}

func BenchmarkWithMaxConnectionsPerBroker(b *testing.B) {
	opts := defaultOptions()
	optFn := WithMaxConnectionsPerBroker(5)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		optFn(opts)
	}
}

func BenchmarkWithHealthTimeout(b *testing.B) {
	opts := defaultOptions()
	optFn := WithHealthTimeout(10 * time.Second)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		optFn(opts)
	}
}

func BenchmarkWithTracer(b *testing.B) {
	opts := defaultOptions()
	tracer := NoopTracer{}
	optFn := WithTracer(tracer)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		optFn(opts)
	}
}

func BenchmarkWithObserver(b *testing.B) {
	opts := defaultOptions()
	observer := xmetrics.NoopObserver{}
	optFn := WithObserver(observer)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		optFn(opts)
	}
}

func BenchmarkWithTLS(b *testing.B) {
	opts := defaultOptions()
	optFn := WithTLS("/path/to/cert.pem", true)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		optFn(opts)
	}
}

func BenchmarkWithAuthentication(b *testing.B) {
	opts := defaultOptions()
	optFn := WithAuthentication(nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		optFn(opts)
	}
}

func BenchmarkApplyAllOptions(b *testing.B) {
	tracer := NoopTracer{}
	observer := xmetrics.NoopObserver{}

	options := []Option{
		WithTracer(tracer),
		WithObserver(observer),
		WithConnectionTimeout(20 * time.Second),
		WithOperationTimeout(60 * time.Second),
		WithMaxConnectionsPerBroker(5),
		WithHealthTimeout(10 * time.Second),
		WithTLS("/path/to/cert.pem", false),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		opts := defaultOptions()
		for _, opt := range options {
			opt(opts)
		}
	}
}

// =============================================================================
// Stats Benchmarks
// =============================================================================

func BenchmarkStats_Create(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Stats{
			Connected:      true,
			ProducersCount: 5,
			ConsumersCount: 3,
		}
	}
}

func BenchmarkStats_Copy(b *testing.B) {
	stats := Stats{
		Connected:      true,
		ProducersCount: 5,
		ConsumersCount: 3,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		copyStats := stats
		_ = copyStats
	}
}

// =============================================================================
// Parallel Benchmarks
// =============================================================================

func BenchmarkNewDLQBuilder_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = NewDLQBuilder()
		}
	})
}

func BenchmarkDLQBuilder_Build_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			builder := NewDLQBuilder().
				WithMaxDeliveries(5).
				WithDeadLetterTopic("dead-letter-topic")
			_ = builder.Build()
		}
	})
}

func BenchmarkNewConsumerOptionsBuilder_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = NewConsumerOptionsBuilder("test-topic", "test-subscription")
		}
	})
}

func BenchmarkToPulsarNackBackoff_Parallel(b *testing.B) {
	policy := xretry.NewExponentialBackoff()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = ToPulsarNackBackoff(policy)
		}
	})
}

func BenchmarkDefaultOptions_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = defaultOptions()
		}
	})
}

// =============================================================================
// Subscription Type Benchmarks
// =============================================================================

func BenchmarkConsumerOptionsBuilder_WithType(b *testing.B) {
	types := []struct {
		name string
		t    pulsar.SubscriptionType
	}{
		{"Exclusive", pulsar.Exclusive},
		{"Shared", pulsar.Shared},
		{"Failover", pulsar.Failover},
		{"KeyShared", pulsar.KeyShared},
	}

	for _, tc := range types {
		b.Run(tc.name, func(b *testing.B) {
			builder := NewConsumerOptionsBuilder("test-topic", "test-subscription")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				builder.WithType(tc.t)
			}
		})
	}
}

// =============================================================================
// Edge Case Benchmarks
// =============================================================================

func BenchmarkDLQBuilder_ZeroMaxDeliveries(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		builder := NewDLQBuilder()
		builder.WithMaxDeliveries(0) // 应该不生效
		_ = builder.Build()
	}
}

func BenchmarkConsumerOptionsBuilder_NilDLQBuilder(b *testing.B) {
	builder := NewConsumerOptionsBuilder("test-topic", "test-subscription")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		builder.WithDLQBuilder(nil)
	}
}

func BenchmarkConsumerOptionsBuilder_NilNackBackoff(b *testing.B) {
	builder := NewConsumerOptionsBuilder("test-topic", "test-subscription")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		builder.WithNackBackoff(nil)
	}
}

func BenchmarkWithTracer_Nil(b *testing.B) {
	opts := defaultOptions()
	optFn := WithTracer(nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		optFn(opts)
	}
}

func BenchmarkWithObserver_Nil(b *testing.B) {
	opts := defaultOptions()
	optFn := WithObserver(nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		optFn(opts)
	}
}

func BenchmarkWithConnectionTimeout_Zero(b *testing.B) {
	opts := defaultOptions()
	optFn := WithConnectionTimeout(0)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		optFn(opts)
	}
}

func BenchmarkWithOperationTimeout_Negative(b *testing.B) {
	opts := defaultOptions()
	optFn := WithOperationTimeout(-1 * time.Second)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		optFn(opts)
	}
}

func BenchmarkWithMaxConnectionsPerBroker_Zero(b *testing.B) {
	opts := defaultOptions()
	optFn := WithMaxConnectionsPerBroker(0)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		optFn(opts)
	}
}
