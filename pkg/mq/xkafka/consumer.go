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
	groupID  string // consumer group 标识，用于可观测性 span 属性

	// mu 保护 Assignment、Committed、QueryWatermarkOffsets、Close 等管理操作的并发访问。
	// 设计决策: confluent-kafka-go 底层基于 librdkafka，其 API 是线程安全的。
	// 本包设计为单 goroutine 消费模型（不应从多个 goroutine 并发调用 ReadMessage），
	// mu 仅串行化管理操作（Health/Stats/Close）之间的并发，不覆盖消费路径。
	mu     sync.Mutex
	closed atomic.Bool // 防止重复关闭，atomic 确保消费路径无锁读取安全

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
//
// 设计决策: Health 内部启动 goroutine 检查分区分配，当外部 ctx 取消时会立即返回，
// 但后台 goroutine 仍持有 mu 锁直到操作完成（受 HealthTimeout 限制）。
// 在此期间 Close() 会被短暂阻塞。这是可接受的权衡：HealthTimeout 默认 5s，
// 且 Assignment/GetMetadata 通常在毫秒级完成。
func (w *consumerWrapper) Health(ctx context.Context) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
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

	// 使用 channel 来处理 context 取消
	done := make(chan error, 1)
	go func() {
		// 加锁保护对底层 consumer 的访问
		w.mu.Lock()
		defer w.mu.Unlock()

		// 再次检查 closed，防止在等待锁期间 Close() 已执行
		if w.closed.Load() {
			done <- ErrClosed
			return
		}

		// 检查是否有分配的分区
		assignment, err := w.consumer.Assignment()
		if err != nil {
			done <- fmt.Errorf("%w: consumer get assignment: %w", ErrHealthCheckFailed, err)
			return
		}

		// 如果没有分配分区，尝试获取元数据来验证连接
		if len(assignment) == 0 {
			timeoutMs := int(w.options.HealthTimeout.Milliseconds())
			_, err := w.consumer.GetMetadata(nil, true, timeoutMs)
			if err != nil {
				done <- fmt.Errorf("%w: consumer get metadata: %w", ErrHealthCheckFailed, err)
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
// 设计决策: calculateLag 持有 mu 锁并对每个分区执行 Committed + QueryWatermarkOffsets RPC。
// 在分区数较多时（如 50 个分区），可能持有锁数秒。这是简单性和正确性的权衡：
// 如果在 RPC 期间释放锁，consumer 可能被 Close() 关闭导致后续 RPC 崩溃。
func (w *consumerWrapper) calculateLag() int64 {
	if w.closed.Load() {
		return 0
	}

	// 加锁保护对底层 consumer 的访问
	w.mu.Lock()
	defer w.mu.Unlock()

	// 再次检查 closed，防止在等待锁期间 Close() 已执行
	if w.closed.Load() {
		return 0
	}

	assignment, err := w.consumer.Assignment()
	if err != nil || len(assignment) == 0 {
		return 0
	}

	// 设计决策: 复用 HealthTimeout 作为 Committed/QueryWatermarkOffsets 的 RPC 超时。
	// 避免增加独立的 LagTimeout 选项（增加配置复杂度），且两者的超时语义相近。
	// 注意：设置较短的 HealthTimeout（如 1s）可能导致分区多时 lag 计算因 RPC 超时返回 0。
	timeoutMs := int(w.options.HealthTimeout.Milliseconds())

	var totalLag int64
	for _, tp := range assignment {
		totalLag += w.partitionLag(tp, timeoutMs)
	}

	return totalLag
}

// partitionLag 计算单个分区的消费延迟。
// 调用方必须持有 mu 锁。
func (w *consumerWrapper) partitionLag(tp kafka.TopicPartition, timeoutMs int) int64 {
	committed, err := w.consumer.Committed([]kafka.TopicPartition{tp}, timeoutMs)
	if err != nil || len(committed) == 0 {
		return 0
	}

	_, high, err := w.consumer.QueryWatermarkOffsets(*tp.Topic, tp.Partition, timeoutMs)
	if err != nil {
		return 0
	}

	if committed[0].Offset >= 0 {
		if lag := high - int64(committed[0].Offset); lag > 0 {
			return lag
		}
	}
	return 0
}

// Close 优雅关闭消费者。
// 会提交通过 StoreOffsets 存储的偏移量并取消订阅。
// 重复调用 Close 安全返回 ErrClosed。
//
// 注意：只有通过 StoreOffsets 存储的 offset 才会被提交。
// 如果消息处理失败且未调用 StoreOffsets，则不会提交该 offset，
// 确保消息可以被重新消费（at-least-once 语义）。
func (w *consumerWrapper) Close() error {
	if !w.closed.CompareAndSwap(false, true) {
		return ErrClosed
	}

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
