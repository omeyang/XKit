package xkafka

import (
	"strconv"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

const (
	componentName = "xkafka"
)

func kafkaAttrs(topic string) []xmetrics.Attr {
	attrs := make([]xmetrics.Attr, 0, 2)
	attrs = append(attrs, xmetrics.String("messaging.system", "kafka"))
	if topic != "" {
		attrs = append(attrs, xmetrics.String("messaging.destination.name", topic))
	}
	return attrs
}

// kafkaMessageAttrs 返回包含消息级别详细信息的属性列表。
// 包含 topic、partition、offset 等用于排查消息堆积和分区热点的关键维度。
func kafkaMessageAttrs(msg *kafka.Message) []xmetrics.Attr {
	if msg == nil {
		return kafkaAttrs("")
	}
	topic := topicFromKafkaMessage(msg)
	attrs := kafkaAttrs(topic)
	attrs = append(attrs,
		xmetrics.String("messaging.kafka.partition", strconv.FormatInt(int64(msg.TopicPartition.Partition), 10)),
		xmetrics.String("messaging.kafka.offset", strconv.FormatInt(int64(msg.TopicPartition.Offset), 10)),
	)
	return attrs
}

// kafkaConsumerMessageAttrs 返回消费者消息级别的属性列表。
// 在 kafkaMessageAttrs 基础上增加 messaging.kafka.consumer.group 维度，
// 便于在同一 topic 有多个 consumer group 时区分消费延迟和错误来源。
func kafkaConsumerMessageAttrs(msg *kafka.Message, consumerGroup string) []xmetrics.Attr {
	attrs := kafkaMessageAttrs(msg)
	if consumerGroup != "" {
		attrs = append(attrs, xmetrics.String("messaging.kafka.consumer.group", consumerGroup))
	}
	return attrs
}
