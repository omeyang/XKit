package xpulsar

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/apache/pulsar-client-go/pulsar"
)

// clientWrapper 实现 Client 接口。
type clientWrapper struct {
	client  pulsar.Client
	options *clientOptions

	// 统计信息
	producersCount atomic.Int32
	consumersCount atomic.Int32

	// 关闭状态
	closed atomic.Bool
}

// Client 返回底层的 pulsar.Client。
func (w *clientWrapper) Client() pulsar.Client {
	return w.client
}

// healthCheckResult 健康检查结果
type healthCheckResult struct {
	err    error
	reader pulsar.Reader
}

// Health 执行健康检查。
// 通过创建一个临时 Reader 来验证连接状态。
//
// 注意：由于 Pulsar 的 CreateReader 不接受 context，当超时发生时，
// 底层 goroutine 可能仍在执行。为避免 goroutine 泄漏，超时后会启动
// 后台清理 goroutine 来等待并清理资源。
func (w *clientWrapper) Health(ctx context.Context) (err error) {
	// 应用健康检查超时
	healthCtx, cancel := context.WithTimeout(ctx, w.options.HealthTimeout)
	defer cancel()

	healthCtx, span := xmetrics.Start(healthCtx, w.options.Observer, xmetrics.SpanOptions{
		Component: componentName,
		Operation: "health",
		Kind:      xmetrics.KindClient,
		Attrs:     pulsarAttrs(""),
	})
	defer func() {
		span.End(xmetrics.Result{Err: err})
	}()

	// 使用带缓冲的 channel，确保 goroutine 不会阻塞
	resultCh := make(chan healthCheckResult, 1)
	go func() {
		// 尝试创建一个临时 reader 来验证连接
		// Pulsar 客户端没有直接的健康检查 API，
		// 但创建 reader 会验证与 broker 的连接
		reader, err := w.client.CreateReader(pulsar.ReaderOptions{
			Topic:          "non-persistent://public/default/__health_check__",
			StartMessageID: pulsar.EarliestMessageID(),
		})
		resultCh <- healthCheckResult{err: err, reader: reader}
	}()

	select {
	case <-healthCtx.Done():
		// 超时后，goroutine 仍可能在执行（因为 CreateReader 不接受 context）
		// 启动后台清理 goroutine，避免阻塞调用方，同时确保资源被清理
		go func() {
			result := <-resultCh
			if result.reader != nil {
				result.reader.Close()
			}
		}()
		return healthCtx.Err()
	case result := <-resultCh:
		if result.err != nil {
			// 检查是否为 topic 相关的错误（连接是正常的，只是 topic 不存在）
			// 这类错误说明已经成功连接到 broker，只是 topic 操作失败
			errStr := result.err.Error()
			if strings.Contains(errStr, "topic not found") ||
				strings.Contains(errStr, "TopicNotFound") ||
				strings.Contains(errStr, "not found") ||
				strings.Contains(errStr, "does not exist") {
				return nil
			}
			// 其他错误（如连接失败、认证失败等）表示真正的健康问题
			return fmt.Errorf("pulsar client health check failed: %w", result.err)
		}
		if result.reader != nil {
			result.reader.Close()
		}
		return nil
	}
}

// CreateProducer 创建 Pulsar 生产者。
func (w *clientWrapper) CreateProducer(options pulsar.ProducerOptions) (pulsar.Producer, error) {
	producer, err := w.client.CreateProducer(options)
	if err != nil {
		return nil, err
	}
	w.producersCount.Add(1)
	return &trackedProducer{
		Producer: producer,
		onClose: func() {
			w.producersCount.Add(-1)
		},
	}, nil
}

// Subscribe 创建 Pulsar 消费者。
func (w *clientWrapper) Subscribe(options pulsar.ConsumerOptions) (pulsar.Consumer, error) {
	consumer, err := w.client.Subscribe(options)
	if err != nil {
		return nil, err
	}
	w.consumersCount.Add(1)
	return &trackedConsumer{
		Consumer: consumer,
		onClose: func() {
			w.consumersCount.Add(-1)
		},
	}, nil
}

// Stats 返回客户端统计信息。
func (w *clientWrapper) Stats() Stats {
	return Stats{
		Connected:      !w.closed.Load(),
		ProducersCount: int(w.producersCount.Load()),
		ConsumersCount: int(w.consumersCount.Load()),
	}
}

// Close 优雅关闭客户端。
func (w *clientWrapper) Close() error {
	w.closed.Store(true)
	w.client.Close()
	return nil
}

// trackedProducer 包装 Producer 以跟踪关闭。
type trackedProducer struct {
	pulsar.Producer
	onClose func()
}

// Close 关闭生产者并更新计数。
func (p *trackedProducer) Close() {
	p.Producer.Close()
	if p.onClose != nil {
		p.onClose()
	}
}

// trackedConsumer 包装 Consumer 以跟踪关闭。
type trackedConsumer struct {
	pulsar.Consumer
	onClose func()
}

// Close 关闭消费者并更新计数。
func (c *trackedConsumer) Close() {
	c.Consumer.Close()
	if c.onClose != nil {
		c.onClose()
	}
}

// 确保实现接口
var _ Client = (*clientWrapper)(nil)
