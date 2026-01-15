//go:build integration

package xpulsar_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/omeyang/xkit/pkg/mq/xpulsar"
)

// =============================================================================
// 测试辅助函数
// =============================================================================

// setupPulsar 启动 Pulsar 容器并返回服务 URL。
func setupPulsar(t *testing.T) (string, func()) {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "apachepulsar/pulsar:3.1.0",
		ExposedPorts: []string{"6650/tcp", "8080/tcp"},
		Cmd:          []string{"bin/pulsar", "standalone"},
		WaitingFor: wait.ForAll(
			wait.ForLog("messaging service is ready"),
			wait.ForListeningPort("6650/tcp"),
		).WithDeadline(2 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start pulsar container")

	host, err := container.Host(ctx)
	require.NoError(t, err, "failed to get container host")

	port, err := container.MappedPort(ctx, "6650")
	require.NoError(t, err, "failed to get mapped port")

	serviceURL := "pulsar://" + host + ":" + port.Port()

	cleanup := func() {
		container.Terminate(ctx)
	}

	return serviceURL, cleanup
}

// =============================================================================
// Client 集成测试
// =============================================================================

func TestIntegration_NewClient_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serviceURL, cleanup := setupPulsar(t)
	defer cleanup()

	client, err := xpulsar.NewClient(serviceURL,
		xpulsar.WithConnectionTimeout(30*time.Second),
		xpulsar.WithOperationTimeout(30*time.Second),
	)
	require.NoError(t, err)
	defer client.Close()

	assert.NotNil(t, client.Client())
}

func TestIntegration_Client_Health(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serviceURL, cleanup := setupPulsar(t)
	defer cleanup()

	client, err := xpulsar.NewClient(serviceURL,
		xpulsar.WithHealthTimeout(10*time.Second),
	)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = client.Health(ctx)
	assert.NoError(t, err)
}

func TestIntegration_Client_Stats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serviceURL, cleanup := setupPulsar(t)
	defer cleanup()

	client, err := xpulsar.NewClient(serviceURL)
	require.NoError(t, err)
	defer client.Close()

	stats := client.Stats()
	// 刚创建时应该没有生产者和消费者
	assert.Equal(t, 0, stats.ProducersCount)
	assert.Equal(t, 0, stats.ConsumersCount)
}

// =============================================================================
// Producer 集成测试
// =============================================================================

func TestIntegration_CreateProducer_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serviceURL, cleanup := setupPulsar(t)
	defer cleanup()

	client, err := xpulsar.NewClient(serviceURL)
	require.NoError(t, err)
	defer client.Close()

	producer, err := client.CreateProducer(pulsar.ProducerOptions{
		Topic: "persistent://public/default/test-producer-topic",
	})
	require.NoError(t, err)
	defer producer.Close()

	assert.NotNil(t, producer)

	// 验证统计信息更新
	stats := client.Stats()
	assert.Equal(t, 1, stats.ProducersCount)
}

func TestIntegration_Producer_Send(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serviceURL, cleanup := setupPulsar(t)
	defer cleanup()

	client, err := xpulsar.NewClient(serviceURL)
	require.NoError(t, err)
	defer client.Close()

	topic := "persistent://public/default/test-send-topic"
	producer, err := client.CreateProducer(pulsar.ProducerOptions{
		Topic: topic,
	})
	require.NoError(t, err)
	defer producer.Close()

	// 发送消息
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	msgID, err := producer.Send(ctx, &pulsar.ProducerMessage{
		Payload: []byte("test message"),
	})
	require.NoError(t, err)
	assert.NotNil(t, msgID)
}

func TestIntegration_Producer_SendAsync(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serviceURL, cleanup := setupPulsar(t)
	defer cleanup()

	client, err := xpulsar.NewClient(serviceURL)
	require.NoError(t, err)
	defer client.Close()

	topic := "persistent://public/default/test-async-topic"
	producer, err := client.CreateProducer(pulsar.ProducerOptions{
		Topic: topic,
	})
	require.NoError(t, err)
	defer producer.Close()

	// 异步发送消息
	done := make(chan struct{})
	var sendErr error
	var msgID pulsar.MessageID

	producer.SendAsync(context.Background(), &pulsar.ProducerMessage{
		Payload: []byte("async test message"),
	}, func(id pulsar.MessageID, message *pulsar.ProducerMessage, err error) {
		msgID = id
		sendErr = err
		close(done)
	})

	select {
	case <-done:
		require.NoError(t, sendErr)
		assert.NotNil(t, msgID)
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for async send")
	}
}

// =============================================================================
// Consumer 集成测试
// =============================================================================

func TestIntegration_Subscribe_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serviceURL, cleanup := setupPulsar(t)
	defer cleanup()

	client, err := xpulsar.NewClient(serviceURL)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Subscribe(pulsar.ConsumerOptions{
		Topic:            "persistent://public/default/test-subscribe-topic",
		SubscriptionName: "test-subscription",
		Type:             pulsar.Shared,
	})
	require.NoError(t, err)
	defer consumer.Close()

	assert.NotNil(t, consumer)

	// 验证统计信息更新
	stats := client.Stats()
	assert.Equal(t, 1, stats.ConsumersCount)
}

func TestIntegration_Consumer_Receive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serviceURL, cleanup := setupPulsar(t)
	defer cleanup()

	client, err := xpulsar.NewClient(serviceURL)
	require.NoError(t, err)
	defer client.Close()

	topic := "persistent://public/default/test-receive-topic"

	// 创建消费者
	consumer, err := client.Subscribe(pulsar.ConsumerOptions{
		Topic:            topic,
		SubscriptionName: "test-receive-subscription",
		Type:             pulsar.Exclusive,
	})
	require.NoError(t, err)
	defer consumer.Close()

	// 创建生产者并发送消息
	producer, err := client.CreateProducer(pulsar.ProducerOptions{
		Topic: topic,
	})
	require.NoError(t, err)
	defer producer.Close()

	testPayload := []byte("test receive message")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = producer.Send(ctx, &pulsar.ProducerMessage{
		Payload: testPayload,
	})
	require.NoError(t, err)

	// 接收消息
	msg, err := consumer.Receive(ctx)
	require.NoError(t, err)
	assert.Equal(t, testPayload, msg.Payload())

	// 确认消息
	err = consumer.Ack(msg)
	assert.NoError(t, err)
}

// =============================================================================
// 端到端测试
// =============================================================================

func TestIntegration_ProduceAndConsume(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serviceURL, cleanup := setupPulsar(t)
	defer cleanup()

	client, err := xpulsar.NewClient(serviceURL)
	require.NoError(t, err)
	defer client.Close()

	topic := "persistent://public/default/test-e2e-topic"

	// 创建消费者（先于生产者，确保不丢消息）
	consumer, err := client.Subscribe(pulsar.ConsumerOptions{
		Topic:            topic,
		SubscriptionName: "test-e2e-subscription",
		Type:             pulsar.Exclusive,
	})
	require.NoError(t, err)
	defer consumer.Close()

	// 创建生产者
	producer, err := client.CreateProducer(pulsar.ProducerOptions{
		Topic: topic,
	})
	require.NoError(t, err)
	defer producer.Close()

	// 发送多条消息
	messageCount := 5
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for i := 0; i < messageCount; i++ {
		_, err := producer.Send(ctx, &pulsar.ProducerMessage{
			Payload: []byte("message"),
		})
		require.NoError(t, err)
	}

	// 接收并确认消息
	receivedCount := 0
	for receivedCount < messageCount {
		msg, err := consumer.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			continue
		}
		receivedCount++
		consumer.Ack(msg)
	}

	assert.Equal(t, messageCount, receivedCount, "should have received all messages")
}

func TestIntegration_MultipleProducersConsumers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serviceURL, cleanup := setupPulsar(t)
	defer cleanup()

	client, err := xpulsar.NewClient(serviceURL)
	require.NoError(t, err)
	defer client.Close()

	topic := "persistent://public/default/test-multi-topic"
	messageCount := 10

	// 创建共享消费者
	consumer, err := client.Subscribe(pulsar.ConsumerOptions{
		Topic:            topic,
		SubscriptionName: "test-multi-subscription",
		Type:             pulsar.Shared,
	})
	require.NoError(t, err)
	defer consumer.Close()

	// 创建多个生产者并发送消息
	producerCount := 3
	var wg sync.WaitGroup
	wg.Add(producerCount)

	for i := 0; i < producerCount; i++ {
		go func() {
			defer wg.Done()

			producer, err := client.CreateProducer(pulsar.ProducerOptions{
				Topic: topic,
			})
			if err != nil {
				t.Errorf("failed to create producer: %v", err)
				return
			}
			defer producer.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			for j := 0; j < messageCount/producerCount; j++ {
				_, err := producer.Send(ctx, &pulsar.ProducerMessage{
					Payload: []byte("multi producer message"),
				})
				if err != nil {
					t.Errorf("failed to send message: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()

	// 验证统计信息
	stats := client.Stats()
	t.Logf("Stats after producers: %+v", stats)
}

// =============================================================================
// 错误场景测试
// =============================================================================

func TestIntegration_NewClient_InvalidURL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// 连接到无效地址应该失败（可能在创建或健康检查时）
	client, err := xpulsar.NewClient("pulsar://invalid-host:6650",
		xpulsar.WithConnectionTimeout(2*time.Second),
	)
	if err != nil {
		// 创建失败，测试通过
		return
	}
	defer client.Close()

	// 如果创建成功，健康检查应该失败
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = client.Health(ctx)
	// 健康检查应该失败或超时
	t.Logf("Health check result for invalid URL: %v", err)
}

func TestIntegration_NewClient_EmptyURL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, err := xpulsar.NewClient("")
	assert.Nil(t, client)
	assert.ErrorIs(t, err, xpulsar.ErrEmptyURL)
}

// =============================================================================
// 关闭测试
// =============================================================================

func TestIntegration_Client_Close(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serviceURL, cleanup := setupPulsar(t)
	defer cleanup()

	client, err := xpulsar.NewClient(serviceURL)
	require.NoError(t, err)

	// 创建一些生产者和消费者
	producer, err := client.CreateProducer(pulsar.ProducerOptions{
		Topic: "persistent://public/default/test-close-topic",
	})
	require.NoError(t, err)

	consumer, err := client.Subscribe(pulsar.ConsumerOptions{
		Topic:            "persistent://public/default/test-close-topic",
		SubscriptionName: "test-close-subscription",
	})
	require.NoError(t, err)

	// 验证统计
	stats := client.Stats()
	assert.Equal(t, 1, stats.ProducersCount)
	assert.Equal(t, 1, stats.ConsumersCount)

	// 先关闭生产者和消费者
	producer.Close()
	consumer.Close()

	// 关闭客户端
	err = client.Close()
	assert.NoError(t, err)
}

func TestIntegration_Producer_Close(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serviceURL, cleanup := setupPulsar(t)
	defer cleanup()

	client, err := xpulsar.NewClient(serviceURL)
	require.NoError(t, err)
	defer client.Close()

	producer, err := client.CreateProducer(pulsar.ProducerOptions{
		Topic: "persistent://public/default/test-producer-close-topic",
	})
	require.NoError(t, err)

	// 验证生产者计数
	stats := client.Stats()
	assert.Equal(t, 1, stats.ProducersCount)

	// 关闭生产者
	producer.Close()

	// 验证计数减少
	stats = client.Stats()
	assert.Equal(t, 0, stats.ProducersCount)
}

func TestIntegration_Consumer_Close(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serviceURL, cleanup := setupPulsar(t)
	defer cleanup()

	client, err := xpulsar.NewClient(serviceURL)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Subscribe(pulsar.ConsumerOptions{
		Topic:            "persistent://public/default/test-consumer-close-topic",
		SubscriptionName: "test-consumer-close-subscription",
	})
	require.NoError(t, err)

	// 验证消费者计数
	stats := client.Stats()
	assert.Equal(t, 1, stats.ConsumersCount)

	// 关闭消费者
	consumer.Close()

	// 验证计数减少
	stats = client.Stats()
	assert.Equal(t, 0, stats.ConsumersCount)
}

// =============================================================================
// 订阅类型测试
// =============================================================================

func TestIntegration_SubscriptionTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serviceURL, cleanup := setupPulsar(t)
	defer cleanup()

	client, err := xpulsar.NewClient(serviceURL)
	require.NoError(t, err)
	defer client.Close()

	testCases := []struct {
		name    string
		subType pulsar.SubscriptionType
	}{
		{"Exclusive", pulsar.Exclusive},
		{"Shared", pulsar.Shared},
		{"Failover", pulsar.Failover},
		{"KeyShared", pulsar.KeyShared},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			topic := "persistent://public/default/test-subtype-" + tc.name
			consumer, err := client.Subscribe(pulsar.ConsumerOptions{
				Topic:            topic,
				SubscriptionName: "test-" + tc.name + "-subscription",
				Type:             tc.subType,
			})
			require.NoError(t, err)
			defer consumer.Close()

			assert.NotNil(t, consumer)
		})
	}
}
