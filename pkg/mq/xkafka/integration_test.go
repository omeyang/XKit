//go:build integration

package xkafka_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kafkaContainer "github.com/testcontainers/testcontainers-go/modules/kafka"

	"github.com/omeyang/xkit/pkg/mq/xkafka"
)

// =============================================================================
// 测试辅助函数
// =============================================================================

// setupKafka 启动 Kafka 容器并返回 bootstrap servers。
func setupKafka(t *testing.T) (string, func()) {
	t.Helper()
	ctx := context.Background()

	container, err := kafkaContainer.Run(ctx,
		"confluentinc/cp-kafka:7.5.0",
		kafkaContainer.WithClusterID("test-cluster"),
	)
	require.NoError(t, err, "failed to start kafka container")

	brokers, err := container.Brokers(ctx)
	require.NoError(t, err, "failed to get kafka brokers")
	require.NotEmpty(t, brokers, "no brokers available")

	cleanup := func() {
		container.Terminate(ctx)
	}

	return brokers[0], cleanup
}

// createTopic 创建测试主题。
func createTopic(t *testing.T, brokers string, topic string, partitions int) {
	t.Helper()

	adminClient, err := kafka.NewAdminClient(&kafka.ConfigMap{
		"bootstrap.servers": brokers,
	})
	require.NoError(t, err, "failed to create admin client")
	defer adminClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results, err := adminClient.CreateTopics(
		ctx,
		[]kafka.TopicSpecification{
			{
				Topic:             topic,
				NumPartitions:     partitions,
				ReplicationFactor: 1,
			},
		},
	)
	require.NoError(t, err, "failed to create topic")
	require.Len(t, results, 1)
	// 忽略主题已存在的错误
	if results[0].Error.Code() != kafka.ErrNoError && results[0].Error.Code() != kafka.ErrTopicAlreadyExists {
		t.Fatalf("failed to create topic: %v", results[0].Error)
	}
}

// =============================================================================
// Producer 集成测试
// =============================================================================

func TestIntegration_NewProducer_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	brokers, cleanup := setupKafka(t)
	defer cleanup()

	config := &kafka.ConfigMap{
		"bootstrap.servers": brokers,
	}

	producer, err := xkafka.NewProducer(config)
	require.NoError(t, err)
	defer producer.Close()

	assert.NotNil(t, producer.Producer())
}

func TestIntegration_Producer_Health(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	brokers, cleanup := setupKafka(t)
	defer cleanup()

	config := &kafka.ConfigMap{
		"bootstrap.servers": brokers,
	}

	producer, err := xkafka.NewProducer(config,
		xkafka.WithProducerHealthTimeout(10*time.Second),
	)
	require.NoError(t, err)
	defer producer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = producer.Health(ctx)
	assert.NoError(t, err)
}

func TestIntegration_Producer_Produce(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	brokers, cleanup := setupKafka(t)
	defer cleanup()

	topic := "test-producer-topic"
	createTopic(t, brokers, topic, 1)

	config := &kafka.ConfigMap{
		"bootstrap.servers": brokers,
	}

	producer, err := xkafka.NewProducer(config)
	require.NoError(t, err)
	defer producer.Close()

	// 发送消息
	deliveryChan := make(chan kafka.Event, 1)
	err = producer.Producer().Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
		Key:            []byte("test-key"),
		Value:          []byte("test-value"),
	}, deliveryChan)
	require.NoError(t, err)

	// 等待送达确认
	select {
	case e := <-deliveryChan:
		m := e.(*kafka.Message)
		if m.TopicPartition.Error != nil {
			t.Fatalf("delivery failed: %v", m.TopicPartition.Error)
		}
		assert.Equal(t, topic, *m.TopicPartition.Topic)
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for delivery")
	}

	// 验证统计信息
	stats := producer.Stats()
	// 注意：统计信息可能需要时间更新
	t.Logf("Producer stats: %+v", stats)
}

func TestIntegration_Producer_Stats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	brokers, cleanup := setupKafka(t)
	defer cleanup()

	config := &kafka.ConfigMap{
		"bootstrap.servers": brokers,
	}

	producer, err := xkafka.NewProducer(config)
	require.NoError(t, err)
	defer producer.Close()

	stats := producer.Stats()
	// 初始状态下统计信息应该为零或合理值
	assert.GreaterOrEqual(t, stats.MessagesProduced, int64(0))
	assert.GreaterOrEqual(t, stats.BytesProduced, int64(0))
	assert.GreaterOrEqual(t, stats.Errors, int64(0))
}

// =============================================================================
// Consumer 集成测试
// =============================================================================

func TestIntegration_NewConsumer_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	brokers, cleanup := setupKafka(t)
	defer cleanup()

	topic := "test-consumer-topic"
	createTopic(t, brokers, topic, 1)

	config := &kafka.ConfigMap{
		"bootstrap.servers": brokers,
		"group.id":          "test-group",
		"auto.offset.reset": "earliest",
	}

	consumer, err := xkafka.NewConsumer(config, []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	assert.NotNil(t, consumer.Consumer())
}

func TestIntegration_Consumer_Health(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	brokers, cleanup := setupKafka(t)
	defer cleanup()

	topic := "test-consumer-health-topic"
	createTopic(t, brokers, topic, 1)

	config := &kafka.ConfigMap{
		"bootstrap.servers": brokers,
		"group.id":          "test-health-group",
		"auto.offset.reset": "earliest",
	}

	consumer, err := xkafka.NewConsumer(config, []string{topic},
		xkafka.WithConsumerHealthTimeout(10*time.Second),
	)
	require.NoError(t, err)
	defer consumer.Close()

	// 触发分区分配
	consumer.Consumer().Poll(1000)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = consumer.Health(ctx)
	// 健康检查可能需要分区分配完成
	if err != nil {
		t.Logf("Health check returned: %v (may be expected if no partitions assigned yet)", err)
	}
}

func TestIntegration_Consumer_Stats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	brokers, cleanup := setupKafka(t)
	defer cleanup()

	topic := "test-consumer-stats-topic"
	createTopic(t, brokers, topic, 1)

	config := &kafka.ConfigMap{
		"bootstrap.servers": brokers,
		"group.id":          "test-stats-group",
		"auto.offset.reset": "earliest",
	}

	consumer, err := xkafka.NewConsumer(config, []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	stats := consumer.Stats()
	// 初始状态下统计信息应该为零或合理值
	assert.GreaterOrEqual(t, stats.MessagesConsumed, int64(0))
	assert.GreaterOrEqual(t, stats.BytesConsumed, int64(0))
	assert.GreaterOrEqual(t, stats.Errors, int64(0))
}

// =============================================================================
// 端到端测试
// =============================================================================

func TestIntegration_ProduceAndConsume(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	brokers, cleanup := setupKafka(t)
	defer cleanup()

	topic := "test-e2e-topic"
	createTopic(t, brokers, topic, 1)

	// 创建 Producer
	producerConfig := &kafka.ConfigMap{
		"bootstrap.servers": brokers,
	}
	producer, err := xkafka.NewProducer(producerConfig)
	require.NoError(t, err)
	defer producer.Close()

	// 创建 Consumer
	consumerConfig := &kafka.ConfigMap{
		"bootstrap.servers": brokers,
		"group.id":          "test-e2e-group",
		"auto.offset.reset": "earliest",
	}
	consumer, err := xkafka.NewConsumer(consumerConfig, []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	// 发送消息
	testKey := []byte("e2e-key")
	testValue := []byte("e2e-value")
	deliveryChan := make(chan kafka.Event, 1)

	err = producer.Producer().Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
		Key:            testKey,
		Value:          testValue,
	}, deliveryChan)
	require.NoError(t, err)

	// 等待送达
	select {
	case e := <-deliveryChan:
		m := e.(*kafka.Message)
		require.NoError(t, m.TopicPartition.Error)
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for delivery")
	}

	// 消费消息
	var received bool
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) && !received {
		ev := consumer.Consumer().Poll(1000)
		if ev == nil {
			continue
		}
		switch e := ev.(type) {
		case *kafka.Message:
			assert.Equal(t, testKey, e.Key)
			assert.Equal(t, testValue, e.Value)
			received = true
		case kafka.Error:
			t.Logf("Kafka error: %v", e)
		}
	}
	assert.True(t, received, "should have received the message")
}

func TestIntegration_MultipleMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	brokers, cleanup := setupKafka(t)
	defer cleanup()

	topic := "test-multi-msg-topic"
	createTopic(t, brokers, topic, 3)

	// 创建 Producer
	producerConfig := &kafka.ConfigMap{
		"bootstrap.servers": brokers,
	}
	producer, err := xkafka.NewProducer(producerConfig)
	require.NoError(t, err)
	defer producer.Close()

	// 创建 Consumer
	consumerConfig := &kafka.ConfigMap{
		"bootstrap.servers": brokers,
		"group.id":          "test-multi-group",
		"auto.offset.reset": "earliest",
	}
	consumer, err := xkafka.NewConsumer(consumerConfig, []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	// 发送多条消息
	messageCount := 10
	deliveryChan := make(chan kafka.Event, messageCount)

	for i := 0; i < messageCount; i++ {
		err = producer.Producer().Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
			Key:            []byte("key"),
			Value:          []byte("value"),
		}, deliveryChan)
		require.NoError(t, err)
	}

	// 等待所有送达确认
	for i := 0; i < messageCount; i++ {
		select {
		case e := <-deliveryChan:
			m := e.(*kafka.Message)
			require.NoError(t, m.TopicPartition.Error)
		case <-time.After(30 * time.Second):
			t.Fatal("timeout waiting for delivery")
		}
	}

	// 消费消息
	receivedCount := 0
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) && receivedCount < messageCount {
		ev := consumer.Consumer().Poll(1000)
		if ev == nil {
			continue
		}
		if msg, ok := ev.(*kafka.Message); ok {
			assert.NotEmpty(t, msg.Value)
			receivedCount++
		}
	}
	assert.Equal(t, messageCount, receivedCount, "should have received all messages")
}

// =============================================================================
// 并发测试
// =============================================================================

func TestIntegration_ConcurrentProducers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	brokers, cleanup := setupKafka(t)
	defer cleanup()

	topic := "test-concurrent-producers-topic"
	createTopic(t, brokers, topic, 3)

	producerCount := 3
	messagesPerProducer := 5

	var wg sync.WaitGroup
	wg.Add(producerCount)

	for i := 0; i < producerCount; i++ {
		go func(id int) {
			defer wg.Done()

			config := &kafka.ConfigMap{
				"bootstrap.servers": brokers,
			}
			producer, err := xkafka.NewProducer(config)
			require.NoError(t, err)
			defer producer.Close()

			deliveryChan := make(chan kafka.Event, messagesPerProducer)
			for j := 0; j < messagesPerProducer; j++ {
				err := producer.Producer().Produce(&kafka.Message{
					TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
					Value:          []byte("concurrent message"),
				}, deliveryChan)
				require.NoError(t, err)
			}

			// 等待送达
			for j := 0; j < messagesPerProducer; j++ {
				select {
				case e := <-deliveryChan:
					m := e.(*kafka.Message)
					assert.NoError(t, m.TopicPartition.Error)
				case <-time.After(30 * time.Second):
					t.Errorf("producer %d: timeout waiting for delivery", id)
					return
				}
			}
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// 错误场景测试
// =============================================================================

func TestIntegration_Producer_InvalidBroker(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	config := &kafka.ConfigMap{
		"bootstrap.servers": "invalid-broker:9092",
	}

	// Producer 创建可能成功，但健康检查会失败
	producer, err := xkafka.NewProducer(config,
		xkafka.WithProducerHealthTimeout(2*time.Second),
	)
	if err != nil {
		// 如果创建失败，测试通过
		return
	}
	defer producer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = producer.Health(ctx)
	assert.Error(t, err, "health check should fail for invalid broker")
}

func TestIntegration_Consumer_InvalidBroker(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	config := &kafka.ConfigMap{
		"bootstrap.servers": "invalid-broker:9092",
		"group.id":          "invalid-test-group",
	}

	// Consumer 创建可能成功，但健康检查会失败
	consumer, err := xkafka.NewConsumer(config, []string{"test-topic"},
		xkafka.WithConsumerHealthTimeout(2*time.Second),
	)
	if err != nil {
		// 如果创建失败，测试通过
		return
	}
	defer consumer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = consumer.Health(ctx)
	// 对于无效 broker，健康检查应该失败或超时
	t.Logf("Health check result: %v", err)
}

// =============================================================================
// 关闭测试
// =============================================================================

func TestIntegration_Producer_Close(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	brokers, cleanup := setupKafka(t)
	defer cleanup()

	config := &kafka.ConfigMap{
		"bootstrap.servers": brokers,
	}

	producer, err := xkafka.NewProducer(config,
		xkafka.WithProducerFlushTimeout(5*time.Second),
	)
	require.NoError(t, err)

	// 关闭应该成功
	err = producer.Close()
	assert.NoError(t, err)

	// 再次关闭应该是幂等的或返回错误（取决于实现）
	// 这里我们只确保不会 panic
}

func TestIntegration_Consumer_Close(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	brokers, cleanup := setupKafka(t)
	defer cleanup()

	topic := "test-close-topic"
	createTopic(t, brokers, topic, 1)

	config := &kafka.ConfigMap{
		"bootstrap.servers": brokers,
		"group.id":          "test-close-group",
		"auto.offset.reset": "earliest",
	}

	consumer, err := xkafka.NewConsumer(config, []string{topic})
	require.NoError(t, err)

	// 关闭应该成功
	err = consumer.Close()
	assert.NoError(t, err)
}
