package xkafka

import (
	"context"

	"github.com/omeyang/xkit/internal/mqcore"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

func kafkaHeadersToMap(headers []kafka.Header) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(headers))
	for _, h := range headers {
		result[h.Key] = string(h.Value)
	}
	return result
}

func injectKafkaTrace(ctx context.Context, tracer Tracer, msg *kafka.Message) {
	if tracer == nil || msg == nil {
		return
	}
	carrier := kafkaHeadersToMap(msg.Headers)
	tracer.Inject(ctx, carrier)
	for key, value := range carrier {
		setHeader(msg, key, value)
	}
}

func extractKafkaTrace(ctx context.Context, tracer Tracer, msg *kafka.Message) context.Context {
	if tracer == nil || msg == nil {
		return ctx
	}
	carrier := kafkaHeadersToMap(msg.Headers)
	extracted := tracer.Extract(carrier)
	return mqcore.MergeTraceContext(ctx, extracted)
}

func topicFromKafkaMessage(msg *kafka.Message) string {
	if msg == nil || msg.TopicPartition.Topic == nil {
		return ""
	}
	return *msg.TopicPartition.Topic
}
