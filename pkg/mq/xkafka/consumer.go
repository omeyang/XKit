package xkafka

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// consumerWrapper 实现 Consumer 接口。
type consumerWrapper struct {
	consumer *kafka.Consumer
	options  *consumerOptions

	// mu 保护 Assignment、Committed、QueryWatermarkOffsets、Close 等管理操作的并发访问。
	// 注意：Consumer.ReadMessage() 本身是线程安全的，不需要加锁。
	// 锁仅用于确保管理操作（如健康检查、统计计算、关闭）的原子性。
	mu sync.Mutex

	// 统计信息
	messagesConsumed atomic.Int64
	bytesConsumed    atomic.Int64
	errorsCount      atomic.Int64
}

// Consumer 返回底层的 *kafka.Consumer。
func (w *consumerWrapper) Consumer() *kafka.Consumer {
	return w.consumer
}

// Health 执行健康检查。
// 检查消费者是否已分配分区。
func (w *consumerWrapper) Health(ctx context.Context) (err error) {
	ctx, span := xmetrics.Start(ctx, w.options.Observer, xmetrics.SpanOptions{
		Component: componentName,
		Operation: "health",
		Kind:      xmetrics.KindClient,
		Attrs:     kafkaAttrs(""),
	})
	defer func() {
		span.End(xmetrics.Result{Err: err})
	}()

	// 使用 channel 来处理 context 取消
	done := make(chan error, 1)
	go func() {
		// 加锁保护对底层 consumer 的访问
		w.mu.Lock()
		defer w.mu.Unlock()

		// 检查是否有分配的分区
		assignment, err := w.consumer.Assignment()
		if err != nil {
			done <- fmt.Errorf("kafka consumer health check failed: %w", err)
			return
		}

		// 如果没有分配分区，尝试获取元数据来验证连接
		if len(assignment) == 0 {
			timeoutMs := int(w.options.HealthTimeout.Milliseconds())
			_, err := w.consumer.GetMetadata(nil, true, timeoutMs)
			if err != nil {
				done <- fmt.Errorf("kafka consumer health check failed: %w", err)
				return
			}
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

// Stats 返回消费者统计信息。
func (w *consumerWrapper) Stats() ConsumerStats {
	return ConsumerStats{
		MessagesConsumed: w.messagesConsumed.Load(),
		BytesConsumed:    w.bytesConsumed.Load(),
		Errors:           w.errorsCount.Load(),
		Lag:              w.calculateLag(),
	}
}

// calculateLag 计算消费延迟。
func (w *consumerWrapper) calculateLag() int64 {
	// 加锁保护对底层 consumer 的访问
	w.mu.Lock()
	defer w.mu.Unlock()

	assignment, err := w.consumer.Assignment()
	if err != nil || len(assignment) == 0 {
		return 0
	}

	var totalLag int64
	for _, tp := range assignment {
		// 获取当前位置
		committed, err := w.consumer.Committed([]kafka.TopicPartition{tp}, 1000)
		if err != nil || len(committed) == 0 {
			continue
		}

		// 获取高水位
		_, high, err := w.consumer.QueryWatermarkOffsets(*tp.Topic, tp.Partition, 1000)
		if err != nil {
			continue
		}

		if committed[0].Offset >= 0 {
			lag := high - int64(committed[0].Offset)
			if lag > 0 {
				totalLag += lag
			}
		}
	}

	return totalLag
}

// Close 优雅关闭消费者。
// 会提交通过 StoreOffsets 存储的偏移量并取消订阅。
//
// 注意：只有通过 StoreOffsets 存储的 offset 才会被提交。
// 如果消息处理失败且未调用 StoreOffsets，则不会提交该 offset，
// 确保消息可以被重新消费（at-least-once 语义）。
func (w *consumerWrapper) Close() error {
	// 加锁保护对底层 consumer 的访问
	w.mu.Lock()
	defer w.mu.Unlock()

	// 提交通过 StoreOffsets 存储的偏移量
	_, commitErr := w.consumer.Commit()
	if commitErr != nil {
		var kafkaErr kafka.Error
		if errors.As(commitErr, &kafkaErr) {
			// ErrNoOffset 表示没有 offset 需要提交，是正常情况
			if kafkaErr.Code() == kafka.ErrNoOffset {
				commitErr = nil
			}
		}
	}

	// 关闭消费者
	closeErr := w.consumer.Close()

	// 合并错误返回
	if commitErr != nil && closeErr != nil {
		return errors.Join(commitErr, closeErr)
	}
	if commitErr != nil {
		return fmt.Errorf("commit offset on close failed: %w", commitErr)
	}
	return closeErr
}

// 确保实现接口
var _ Consumer = (*consumerWrapper)(nil)
