package xkafka

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// producerWrapper 实现 Producer 接口。
type producerWrapper struct {
	producer *kafka.Producer
	options  *producerOptions

	// mu 保护 GetMetadata、Flush、Close 等管理操作的并发访问。
	// 注意：Producer.Produce() 本身是线程安全的，不需要加锁。
	// 锁仅用于确保管理操作（如健康检查、关闭）的原子性。
	mu     sync.Mutex
	closed atomic.Bool // 防止重复关闭

	// 统计信息
	messagesProduced atomic.Int64
	bytesProduced    atomic.Int64
	errors           atomic.Int64
}

// Producer 返回底层的 *kafka.Producer。
func (w *producerWrapper) Producer() *kafka.Producer {
	return w.producer
}

// Health 执行健康检查。
// 通过获取 Broker 元数据验证连接状态。
//
// 设计决策: Health 内部启动 goroutine 获取元数据，当外部 ctx 取消时会立即返回，
// 但后台 goroutine 仍持有 mu 锁直到 GetMetadata 超时（受 HealthTimeout 限制）。
// 在此期间 Close() 会被短暂阻塞。这是可接受的权衡：HealthTimeout 默认 5s，
// 且 GetMetadata 通常在毫秒级完成。
func (w *producerWrapper) Health(ctx context.Context) (err error) {
	if w.closed.Load() {
		return ErrClosed
	}

	ctx, span := xmetrics.Start(ctx, w.options.Observer, xmetrics.SpanOptions{
		Component: componentName,
		Operation: "health",
		Kind:      xmetrics.KindClient,
		Attrs:     kafkaAttrs(""),
	})
	defer func() {
		span.End(xmetrics.Result{Err: err})
	}()

	timeoutMs := int(w.options.HealthTimeout.Milliseconds())

	// 使用 channel 来处理 context 取消
	done := make(chan error, 1)
	go func() {
		// 加锁保护对底层 producer 的访问
		w.mu.Lock()
		defer w.mu.Unlock()

		// 再次检查 closed，防止在等待锁期间 Close() 已执行
		if w.closed.Load() {
			done <- ErrClosed
			return
		}

		_, err := w.producer.GetMetadata(nil, true, timeoutMs)
		if err != nil {
			done <- fmt.Errorf("kafka producer health check failed: %w", err)
			return
		}
		done <- nil
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// Stats 返回生产者统计信息。
// 如果 producer 已关闭，QueueLength 返回 0。
func (w *producerWrapper) Stats() ProducerStats {
	var queueLen int
	if !w.closed.Load() {
		// 加锁保护对底层 producer 的访问
		w.mu.Lock()
		if !w.closed.Load() {
			queueLen = w.producer.Len()
		}
		w.mu.Unlock()
	}

	return ProducerStats{
		MessagesProduced: w.messagesProduced.Load(),
		BytesProduced:    w.bytesProduced.Load(),
		Errors:           w.errors.Load(),
		QueueLength:      queueLen,
	}
}

// Close 优雅关闭生产者。
// 会等待所有消息发送完成（受 FlushTimeout 限制）。
// 重复调用 Close 安全返回 ErrClosed。
func (w *producerWrapper) Close() error {
	if !w.closed.CompareAndSwap(false, true) {
		return ErrClosed
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	timeoutMs := int(w.options.FlushTimeout.Milliseconds())

	remaining := w.producer.Flush(timeoutMs)
	if remaining > 0 {
		w.producer.Close()
		return fmt.Errorf("%w: %d messages still in queue", ErrFlushTimeout, remaining)
	}

	w.producer.Close()
	return nil
}

// 确保实现接口
var _ Producer = (*producerWrapper)(nil)
