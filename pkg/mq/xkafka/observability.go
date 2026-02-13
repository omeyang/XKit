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
	attrs := []xmetrics.Attr{xmetrics.String("messaging.system", "kafka")}
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
