package xpulsar

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/apache/pulsar-client-go/pulsar"
)

// 设计决策: 健康检查通过错误消息匹配判断 "topic 不存在" 这一预期错误。
// Pulsar Go SDK（pulsar-client-go v0.18）未导出结构化错误类型，仅能通过字符串匹配。
// 以下模式限定为 topic 相关关键词，避免宽泛匹配导致误判。
// SDK 版本更新时需验证这些模式是否仍然有效。
var healthCheckTopicPatterns = []string{
	"topic not found",
	"topicnotfound",
	"topic does not exist",
}

// isTopicNotFoundErr 检查错误是否为 topic 不存在的预期错误。
func isTopicNotFoundErr(err error) bool {
	errLower := strings.ToLower(err.Error())
	for _, pattern := range healthCheckTopicPatterns {
		if strings.Contains(errLower, pattern) {
			return true
		}
	}
	return false
}

// clientWrapper 实现 Client 接口。
type clientWrapper struct {
	client  pulsar.Client
	options *clientOptions

	// 统计信息
	producersCount atomic.Int32
	consumersCount atomic.Int32

	// healthWg 跟踪 Health() 超时后的后台清理 goroutine。
	// Close() 等待所有清理 goroutine 完成，防止 goroutine 泄漏。
	healthWg sync.WaitGroup

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
// 设计决策: 由于 Pulsar 的 CreateReader 不接受 context，当超时发生时，
// 底层 goroutine 可能仍在执行。超时后会启动后台清理 goroutine 来等待并
// 清理资源。清理 goroutine 的最大存活时间受限于 Pulsar 客户端的
// OperationTimeout 配置（默认 30s），不会无限堆积。
func (w *clientWrapper) Health(ctx context.Context) (err error) {
	if w.closed.Load() {
		return ErrClosed
	}

	if ctx == nil {
		ctx = context.Background()
	}

	// 应用健康检查超时
	healthCtx, cancel := context.WithTimeout(ctx, w.options.HealthTimeout)
	defer cancel()

	healthCtx, span := xmetrics.Start(healthCtx, w.options.Observer, xmetrics.SpanOptions{
		Component: componentName,
		Operation: "health",
		Kind:      xmetrics.KindClient,
		Attrs:     pulsarAttrs(w.options.HealthCheckTopic),
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
			Topic:          w.options.HealthCheckTopic,
			StartMessageID: pulsar.EarliestMessageID(),
		})
		resultCh <- healthCheckResult{err: err, reader: reader}
	}()

	select {
	case <-healthCtx.Done():
		// 超时后，goroutine 仍可能在执行（因为 CreateReader 不接受 context）
		// 启动后台清理 goroutine，避免阻塞调用方，同时确保资源被清理。
		// 清理 goroutine 最终会因 Pulsar 客户端的 OperationTimeout 而返回。
		// 通过 healthWg 跟踪，Close() 会等待所有清理 goroutine 完成。
		w.healthWg.Go(func() {
			result := <-resultCh
			if result.reader != nil {
				result.reader.Close()
			}
		})
		return healthCtx.Err()
	case result := <-resultCh:
		if result.err != nil {
			// 设计决策: 仅匹配 topic 相关的错误模式判定为"连接正常"。
			// 使用 topic 限定的匹配条件，避免 "not found" 等宽泛模式
			// 将认证/配置等错误误判为健康。
			if isTopicNotFoundErr(result.err) {
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
	if w.closed.Load() {
		return nil, ErrClosed
	}
	producer, err := w.client.CreateProducer(options)
	if err != nil {
		return nil, fmt.Errorf("xpulsar: create producer: %w", err)
	}
	w.producersCount.Add(1)
	return &trackedProducer{
		Producer: producer,
		onClose:  func() { decrementIfPositive(&w.producersCount) },
	}, nil
}

// Subscribe 创建 Pulsar 消费者。
func (w *clientWrapper) Subscribe(options pulsar.ConsumerOptions) (pulsar.Consumer, error) {
	if w.closed.Load() {
		return nil, ErrClosed
	}
	consumer, err := w.client.Subscribe(options)
	if err != nil {
		return nil, fmt.Errorf("xpulsar: subscribe: %w", err)
	}
	w.consumersCount.Add(1)
	return &trackedConsumer{
		Consumer: consumer,
		onClose:  func() { decrementIfPositive(&w.consumersCount) },
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
// 重复调用是安全的，仅首次调用会实际关闭底层连接。
// Pulsar 内部关闭所有 producer/consumer 时不经过 tracked wrapper，
// 因此手动重置计数器以确保 Stats() 返回一致状态。
// Close 会等待 Health() 超时后的后台清理 goroutine 全部完成，防止 goroutine 泄漏。
func (w *clientWrapper) Close() error {
	if !w.closed.CompareAndSwap(false, true) {
		return nil
	}
	w.client.Close()
	w.healthWg.Wait()
	w.producersCount.Store(0)
	w.consumersCount.Store(0)
	return nil
}

// trackedProducer 包装 Producer 以跟踪关闭。
type trackedProducer struct {
	pulsar.Producer
	onClose   func()
	closeOnce sync.Once
}

// Close 关闭生产者并更新计数。
// 重复调用是安全的，仅首次调用会实际关闭底层生产者并递减计数。
func (p *trackedProducer) Close() {
	p.closeOnce.Do(func() {
		p.Producer.Close()
		if p.onClose != nil {
			p.onClose()
		}
	})
}

// trackedConsumer 包装 Consumer 以跟踪关闭。
type trackedConsumer struct {
	pulsar.Consumer
	onClose   func()
	closeOnce sync.Once
}

// Close 关闭消费者并更新计数。
// 重复调用是安全的，仅首次调用会实际关闭底层消费者并递减计数。
func (c *trackedConsumer) Close() {
	c.closeOnce.Do(func() {
		c.Consumer.Close()
		if c.onClose != nil {
			c.onClose()
		}
	})
}

// decrementIfPositive 使用 CAS 循环将计数器安全递减，确保不低于 0。
//
// 设计决策: clientWrapper.Close() 调用底层 client.Close() 后手动重置计数器为 0。
// 如果用户之后再关闭仍持有引用的 trackedProducer/trackedConsumer，
// 简单的 Add(-1) 会导致计数器为负值。CAS 循环确保计数器语义不变式。
func decrementIfPositive(counter *atomic.Int32) {
	for {
		old := counter.Load()
		if old <= 0 {
			return
		}
		if counter.CompareAndSwap(old, old-1) {
			return
		}
	}
}

// 确保实现接口
var _ Client = (*clientWrapper)(nil)
