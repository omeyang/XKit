package xkafka

import (
	"context"

	"github.com/omeyang/xkit/internal/mqcore"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// kafkaHeadersToMap 将 Kafka 消息 Header 转换为 map。
// 注意：Kafka 允许重复 Header Key，转换为 map 时后者覆盖前者。
// 这对 trace propagation 是正确的行为（carrier 每个 key 只保留一个值）。
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
