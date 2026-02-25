package xpulsar

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// =============================================================================
// mockPulsarClient — 实现 pulsar.Client 接口用于 clientWrapper 白盒测试
// =============================================================================

type mockPulsarClient struct {
	createProducerFn func(pulsar.ProducerOptions) (pulsar.Producer, error)
	subscribeFn      func(pulsar.ConsumerOptions) (pulsar.Consumer, error)
	createReaderFn   func(pulsar.ReaderOptions) (pulsar.Reader, error)
	closeCalled      bool
}

func (m *mockPulsarClient) CreateProducer(opts pulsar.ProducerOptions) (pulsar.Producer, error) {
	if m.createProducerFn != nil {
		return m.createProducerFn(opts)
	}
	return &mockProducer{}, nil
}

func (m *mockPulsarClient) Subscribe(opts pulsar.ConsumerOptions) (pulsar.Consumer, error) {
	if m.subscribeFn != nil {
		return m.subscribeFn(opts)
	}
	return &mockConsumer{}, nil
}

func (m *mockPulsarClient) CreateReader(opts pulsar.ReaderOptions) (pulsar.Reader, error) {
	if m.createReaderFn != nil {
		return m.createReaderFn(opts)
	}
	return &mockReader{}, nil
}

func (m *mockPulsarClient) CreateTableView(pulsar.TableViewOptions) (pulsar.TableView, error) {
	return nil, nil
}

func (m *mockPulsarClient) TopicPartitions(string) ([]string, error) {
	return nil, nil
}

func (m *mockPulsarClient) NewTransaction(time.Duration) (pulsar.Transaction, error) {
	return nil, nil
}

func (m *mockPulsarClient) Close() {
	m.closeCalled = true
}

// mockReader — 实现 pulsar.Reader 接口
type mockReader struct {
	closeCalled bool
}

func (r *mockReader) Topic() string { return "test" }
func (r *mockReader) Next(context.Context) (pulsar.Message, error) {
	return nil, nil
}
func (r *mockReader) HasNext() bool                               { return false }
func (r *mockReader) Close()                                      { r.closeCalled = true }
func (r *mockReader) Seek(pulsar.MessageID) error                 { return nil }
func (r *mockReader) SeekByTime(time.Time) error                  { return nil }
func (r *mockReader) GetLastMessageID() (pulsar.MessageID, error) { return nil, nil }

// =============================================================================
// clientWrapper.Client() 测试
// =============================================================================

func TestClientWrapper_Client(t *testing.T) {
	mc := &mockPulsarClient{}
	w := &clientWrapper{
		client:  mc,
		options: defaultOptions(),
	}

	assert.Equal(t, mc, w.Client())
}

// =============================================================================
// clientWrapper.Stats() 测试
// =============================================================================

func TestClientWrapper_Stats(t *testing.T) {
	w := &clientWrapper{
		options: defaultOptions(),
	}

	stats := w.Stats()
	assert.True(t, stats.Connected)
	assert.Equal(t, 0, stats.ProducersCount)
	assert.Equal(t, 0, stats.ConsumersCount)
}

func TestClientWrapper_Stats_AfterClose(t *testing.T) {
	mc := &mockPulsarClient{}
	w := &clientWrapper{
		client:  mc,
		options: defaultOptions(),
	}

	require.NoError(t, w.Close())

	stats := w.Stats()
	assert.False(t, stats.Connected)
}

func TestClientWrapper_Stats_AfterClose_CountersReset(t *testing.T) {
	mc := &mockPulsarClient{}
	w := &clientWrapper{
		client:  mc,
		options: defaultOptions(),
	}

	// 模拟创建了 producer 和 consumer
	w.producersCount.Store(3)
	w.consumersCount.Store(2)

	require.NoError(t, w.Close())

	stats := w.Stats()
	assert.False(t, stats.Connected)
	assert.Equal(t, 0, stats.ProducersCount, "Close 后计数器应重置")
	assert.Equal(t, 0, stats.ConsumersCount, "Close 后计数器应重置")
}

// =============================================================================
// clientWrapper.Close() 测试
// =============================================================================

func TestClientWrapper_Close_Idempotent(t *testing.T) {
	mc := &mockPulsarClient{}
	w := &clientWrapper{
		client:  mc,
		options: defaultOptions(),
	}

	// 首次关闭
	err := w.Close()
	assert.NoError(t, err)
	assert.True(t, mc.closeCalled)

	// 重复关闭不应 panic
	mc.closeCalled = false
	err = w.Close()
	assert.NoError(t, err)
	assert.False(t, mc.closeCalled, "底层 client.Close 不应被重复调用")
}

// =============================================================================
// clientWrapper.CreateProducer() 测试
// =============================================================================

func TestClientWrapper_CreateProducer_Success(t *testing.T) {
	mc := &mockPulsarClient{}
	w := &clientWrapper{
		client:  mc,
		options: defaultOptions(),
	}

	producer, err := w.CreateProducer(pulsar.ProducerOptions{Topic: "test"})
	require.NoError(t, err)
	assert.NotNil(t, producer)
	assert.Equal(t, int32(1), w.producersCount.Load())

	// 关闭后计数减少
	producer.Close()
	assert.Equal(t, int32(0), w.producersCount.Load())
}

func TestClientWrapper_CreateProducer_Error(t *testing.T) {
	expectedErr := errors.New("create producer failed")
	mc := &mockPulsarClient{
		createProducerFn: func(pulsar.ProducerOptions) (pulsar.Producer, error) {
			return nil, expectedErr
		},
	}
	w := &clientWrapper{
		client:  mc,
		options: defaultOptions(),
	}

	producer, err := w.CreateProducer(pulsar.ProducerOptions{Topic: "test"})
	assert.Nil(t, producer)
	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, int32(0), w.producersCount.Load())
}

func TestClientWrapper_CreateProducer_AfterClose(t *testing.T) {
	mc := &mockPulsarClient{}
	w := &clientWrapper{
		client:  mc,
		options: defaultOptions(),
	}
	require.NoError(t, w.Close())

	producer, err := w.CreateProducer(pulsar.ProducerOptions{Topic: "test"})
	assert.Nil(t, producer)
	assert.ErrorIs(t, err, ErrClosed)
}

// =============================================================================
// clientWrapper.Subscribe() 测试
// =============================================================================

func TestClientWrapper_Subscribe_Success(t *testing.T) {
	mc := &mockPulsarClient{}
	w := &clientWrapper{
		client:  mc,
		options: defaultOptions(),
	}

	consumer, err := w.Subscribe(pulsar.ConsumerOptions{
		Topic:            "test",
		SubscriptionName: "sub",
	})
	require.NoError(t, err)
	assert.NotNil(t, consumer)
	assert.Equal(t, int32(1), w.consumersCount.Load())

	// 关闭后计数减少
	consumer.Close()
	assert.Equal(t, int32(0), w.consumersCount.Load())
}

func TestClientWrapper_Subscribe_Error(t *testing.T) {
	expectedErr := errors.New("subscribe failed")
	mc := &mockPulsarClient{
		subscribeFn: func(pulsar.ConsumerOptions) (pulsar.Consumer, error) {
			return nil, expectedErr
		},
	}
	w := &clientWrapper{
		client:  mc,
		options: defaultOptions(),
	}

	consumer, err := w.Subscribe(pulsar.ConsumerOptions{Topic: "test"})
	assert.Nil(t, consumer)
	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, int32(0), w.consumersCount.Load())
}

func TestClientWrapper_Subscribe_AfterClose(t *testing.T) {
	mc := &mockPulsarClient{}
	w := &clientWrapper{
		client:  mc,
		options: defaultOptions(),
	}
	require.NoError(t, w.Close())

	consumer, err := w.Subscribe(pulsar.ConsumerOptions{Topic: "test"})
	assert.Nil(t, consumer)
	assert.ErrorIs(t, err, ErrClosed)
}

// =============================================================================
// clientWrapper.Health() 测试
// =============================================================================

func TestClientWrapper_Health_Success(t *testing.T) {
	reader := &mockReader{}
	mc := &mockPulsarClient{
		createReaderFn: func(pulsar.ReaderOptions) (pulsar.Reader, error) {
			return reader, nil
		},
	}
	w := &clientWrapper{
		client:  mc,
		options: defaultOptions(),
	}

	err := w.Health(context.Background())
	assert.NoError(t, err)
}

func TestClientWrapper_Health_TopicNotFoundTreatedAsHealthy(t *testing.T) {
	topicErrors := []string{
		"topic not found",
		"TopicNotFound",
		"topic does not exist",
	}

	for _, errMsg := range topicErrors {
		t.Run(errMsg, func(t *testing.T) {
			mc := &mockPulsarClient{
				createReaderFn: func(pulsar.ReaderOptions) (pulsar.Reader, error) {
					return nil, errors.New(errMsg)
				},
			}
			w := &clientWrapper{
				client:  mc,
				options: defaultOptions(),
			}

			err := w.Health(context.Background())
			assert.NoError(t, err, "topic 相关错误应视为连接正常")
		})
	}
}

func TestClientWrapper_Health_NonTopicNotFoundIsError(t *testing.T) {
	// "resource not found" 不再匹配（之前匹配 "not found" 过于宽泛）
	nonTopicErrors := []string{
		"connection refused",
		"authentication failed",
		"resource not found",
		"certificate not found",
	}

	for _, errMsg := range nonTopicErrors {
		t.Run(errMsg, func(t *testing.T) {
			mc := &mockPulsarClient{
				createReaderFn: func(pulsar.ReaderOptions) (pulsar.Reader, error) {
					return nil, errors.New(errMsg)
				},
			}
			w := &clientWrapper{
				client:  mc,
				options: defaultOptions(),
			}

			err := w.Health(context.Background())
			assert.Error(t, err, "非 topic 相关错误应视为健康检查失败")
		})
	}
}

func TestClientWrapper_Health_RealError(t *testing.T) {
	mc := &mockPulsarClient{
		createReaderFn: func(pulsar.ReaderOptions) (pulsar.Reader, error) {
			return nil, errors.New("connection refused")
		},
	}
	w := &clientWrapper{
		client:  mc,
		options: defaultOptions(),
	}

	err := w.Health(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "health check failed")
}

func TestClientWrapper_Health_Timeout(t *testing.T) {
	mc := &mockPulsarClient{
		createReaderFn: func(pulsar.ReaderOptions) (pulsar.Reader, error) {
			// 模拟阻塞操作
			time.Sleep(500 * time.Millisecond)
			return &mockReader{}, nil
		},
	}
	opts := defaultOptions()
	opts.HealthTimeout = 50 * time.Millisecond
	w := &clientWrapper{
		client:  mc,
		options: opts,
	}

	err := w.Health(context.Background())
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestClientWrapper_Health_AfterClose(t *testing.T) {
	mc := &mockPulsarClient{}
	w := &clientWrapper{
		client:  mc,
		options: defaultOptions(),
	}
	require.NoError(t, w.Close())

	err := w.Health(context.Background())
	assert.ErrorIs(t, err, ErrClosed)
}

func TestClientWrapper_Health_NilContext(t *testing.T) {
	reader := &mockReader{}
	mc := &mockPulsarClient{
		createReaderFn: func(pulsar.ReaderOptions) (pulsar.Reader, error) {
			return reader, nil
		},
	}
	w := &clientWrapper{
		client:  mc,
		options: defaultOptions(),
	}

	// nil context 应内部替换为 context.Background()，不 panic
	var nilCtx context.Context
	err := w.Health(nilCtx)
	assert.NoError(t, err)
}

// =============================================================================
// trackedProducer / trackedConsumer 测试
// =============================================================================

func TestTrackedProducer_Close_NilOnClose(t *testing.T) {
	mp := &mockProducer{}
	tp := &trackedProducer{
		Producer: mp,
		onClose:  nil,
	}

	// 不应 panic
	assert.NotPanics(t, func() {
		tp.Close()
	})
}

func TestTrackedProducer_Close_Idempotent(t *testing.T) {
	closeCount := 0
	mp := &mockProducer{}
	tp := &trackedProducer{
		Producer: mp,
		onClose: func() {
			closeCount++
		},
	}

	// 多次关闭只执行一次
	tp.Close()
	tp.Close()
	tp.Close()
	assert.Equal(t, 1, closeCount, "onClose 只应执行一次")
}

func TestTrackedConsumer_Close_NilOnClose(t *testing.T) {
	mc := &mockConsumer{}
	tc := &trackedConsumer{
		Consumer: mc,
		onClose:  nil,
	}

	// 不应 panic
	assert.NotPanics(t, func() {
		tc.Close()
	})
}

func TestTrackedConsumer_Close_Idempotent(t *testing.T) {
	closeCount := 0
	mc := &mockConsumer{}
	tc := &trackedConsumer{
		Consumer: mc,
		onClose: func() {
			closeCount++
		},
	}

	// 多次关闭只执行一次
	tc.Close()
	tc.Close()
	tc.Close()
	assert.Equal(t, 1, closeCount, "onClose 只应执行一次")
}

// =============================================================================
// trackedProducer/trackedConsumer Close 后 clientWrapper.Close 不导致负计数
// =============================================================================

func TestClientWrapper_Close_ThenTrackedClose_NoNegativeCount(t *testing.T) {
	mc := &mockPulsarClient{}
	w := &clientWrapper{
		client:  mc,
		options: defaultOptions(),
	}

	// 创建 producer 和 consumer
	producer, err := w.CreateProducer(pulsar.ProducerOptions{Topic: "test"})
	require.NoError(t, err)

	consumer, err := w.Subscribe(pulsar.ConsumerOptions{Topic: "test", SubscriptionName: "sub"})
	require.NoError(t, err)

	// 先关闭 clientWrapper（计数器被重置为 0）
	require.NoError(t, w.Close())
	assert.Equal(t, 0, w.Stats().ProducersCount)
	assert.Equal(t, 0, w.Stats().ConsumersCount)

	// 再关闭仍持有引用的 producer/consumer，计数器不应变为负数
	producer.Close()
	consumer.Close()
	assert.Equal(t, 0, w.Stats().ProducersCount, "计数器不应为负")
	assert.Equal(t, 0, w.Stats().ConsumersCount, "计数器不应为负")
}

func TestDecrementIfPositive(t *testing.T) {
	var counter atomic.Int32

	// 从 0 开始递减不应变为负数
	decrementIfPositive(&counter)
	assert.Equal(t, int32(0), counter.Load())

	// 正常递减
	counter.Store(2)
	decrementIfPositive(&counter)
	assert.Equal(t, int32(1), counter.Load())
	decrementIfPositive(&counter)
	assert.Equal(t, int32(0), counter.Load())

	// 再次递减不应变为负数
	decrementIfPositive(&counter)
	assert.Equal(t, int32(0), counter.Load())
}

// =============================================================================
// isTopicNotFoundErr 测试
// =============================================================================

func TestIsTopicNotFoundErr(t *testing.T) {
	tests := []struct {
		name   string
		errMsg string
		want   bool
	}{
		{"topic not found", "topic not found", true},
		{"TopicNotFound", "TopicNotFound", true},
		{"topic does not exist", "topic does not exist", true},
		{"mixed case", "Topic Not Found", true},
		{"connection refused", "connection refused", false},
		{"authentication failed", "authentication failed", false},
		{"resource not found", "resource not found", false},
		{"certificate not found", "certificate not found", false},
		{"empty error", "some error", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := errors.New(tc.errMsg)
			assert.Equal(t, tc.want, isTopicNotFoundErr(err))
		})
	}
}

// =============================================================================
// DefaultBackoffPolicy 测试
// =============================================================================

func TestDefaultBackoffPolicy(t *testing.T) {
	policy := DefaultBackoffPolicy()
	assert.NotNil(t, policy)

	// 验证返回的是非零延迟
	delay := policy.NextDelay(1)
	assert.Greater(t, delay, time.Duration(0))
}

// =============================================================================
// ConsumeLoopWithPolicy backoff 分支测试
// =============================================================================

func TestConsumeLoopWithPolicy_WithBackoff(t *testing.T) {
	mc := &mockConsumer{receiveErr: errors.New("receive error")}
	consumer, err := WrapConsumer(mc, "test-topic", NoopTracer{}, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	backoff := DefaultBackoffPolicy()
	loopErr := consumer.ConsumeLoopWithPolicy(ctx, func(ctx context.Context, msg pulsar.Message) error {
		return nil
	}, backoff)

	assert.ErrorIs(t, loopErr, context.Canceled)
}

// =============================================================================
// Health 超时 → Close 不泄漏 goroutine（goleak 验证）
// =============================================================================

func TestClientWrapper_Health_Timeout_Close_NoLeak(t *testing.T) {
	defer goleak.VerifyNone(t)

	mc := &mockPulsarClient{
		createReaderFn: func(pulsar.ReaderOptions) (pulsar.Reader, error) {
			// 模拟 broker 慢响应
			time.Sleep(300 * time.Millisecond)
			return &mockReader{}, nil
		},
	}
	opts := defaultOptions()
	opts.HealthTimeout = 50 * time.Millisecond
	w := &clientWrapper{
		client:  mc,
		options: opts,
	}

	// Health 超时
	err := w.Health(context.Background())
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	// Close 应等待后台清理 goroutine 完成
	require.NoError(t, w.Close())
}
