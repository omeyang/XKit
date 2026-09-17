package xkafka

import (
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// kafkaProducerClient abstracts *kafka.Producer for testing.
// *kafka.Producer implements this interface naturally.
type kafkaProducerClient interface {
	Produce(msg *kafka.Message, deliveryChan chan kafka.Event) error
	GetMetadata(topic *string, allTopics bool, timeoutMs int) (*kafka.Metadata, error)
	Flush(timeoutMs int) int
	Len() int
	Close()
}

// kafkaConsumerClient abstracts *kafka.Consumer for testing.
// *kafka.Consumer implements this interface naturally.
type kafkaConsumerClient interface {
	ReadMessage(timeout time.Duration) (*kafka.Message, error)
	Assignment() ([]kafka.TopicPartition, error)
	GetMetadata(topic *string, allTopics bool, timeoutMs int) (*kafka.Metadata, error)
	Committed(partitions []kafka.TopicPartition, timeoutMs int) ([]kafka.TopicPartition, error)
	QueryWatermarkOffsets(topic string, partition int32, timeoutMs int) (low int64, high int64, err error)
	StoreMessage(msg *kafka.Message) ([]kafka.TopicPartition, error)
	Commit() ([]kafka.TopicPartition, error)
	Close() error
}

// Compile-time verification that concrete types satisfy the interfaces.
var _ kafkaProducerClient = (*kafka.Producer)(nil)
var _ kafkaConsumerClient = (*kafka.Consumer)(nil)
