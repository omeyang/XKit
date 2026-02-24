package mqcore

import (
	"context"
	"time"

	"github.com/omeyang/xkit/pkg/resilience/xretry"
)

// ConsumeFunc 消费函数签名。
// 返回 error 时会触发退避重试，返回 nil 时重置退避。
type ConsumeFunc func(ctx context.Context) error

// ConsumeLoopOptions 消费循环配置选项。
type ConsumeLoopOptions struct {
	// Backoff 退避策略，默认使用 xretry.NewExponentialBackoff()。
	Backoff xretry.BackoffPolicy

	// OnError 错误回调，可选。
	// 在每次消费错误时调用，用于记录日志或指标。
	OnError func(err error)
}

// ConsumeLoopOption 配置函数类型。
type ConsumeLoopOption func(*ConsumeLoopOptions)

// WithBackoff 设置退避策略。
func WithBackoff(backoff xretry.BackoffPolicy) ConsumeLoopOption {
	return func(o *ConsumeLoopOptions) {
		if backoff != nil {
			o.Backoff = backoff
		}
	}
}

// WithOnError 设置错误回调。
// nil 值会被忽略，与 WithBackoff 的 nil 处理保持一致。
func WithOnError(onError func(err error)) ConsumeLoopOption {
	return func(o *ConsumeLoopOptions) {
		if onError != nil {
			o.OnError = onError
		}
	}
}

// DefaultBackoff 返回默认退避策略。
// 使用 xretry.ExponentialBackoff，默认配置：
//   - InitialDelay: 100ms
//   - MaxDelay: 30s
//   - Multiplier: 2.0
//   - Jitter: 0.1 (10%)
func DefaultBackoff() xretry.BackoffPolicy {
	return xretry.NewExponentialBackoff()
}

// RunConsumeLoop 运行消费循环，使用退避策略处理错误。
//
// 循环逻辑：
//  1. 调用 consume 函数消费消息
//  2. 如果成功（err == nil），重置退避计数器
//  3. 如果失败（err != nil），应用退避延迟后重试
//  4. 循环直到 ctx 取消
//
// 契约: consume 函数在无消息时应自行阻塞（如 poll with timeout），而非立即返回 nil。
// 成功路径不插入延迟，依赖 consume 自身的阻塞语义避免忙等。
//
// 参数：
//   - ctx: 上下文，取消时退出循环
//   - consume: 消费函数
//   - opts: 可选配置
//
// 返回：
//   - ctx 取消时返回 ctx.Err()
func RunConsumeLoop(ctx context.Context, consume ConsumeFunc, opts ...ConsumeLoopOption) error {
	// 应用配置
	options := &ConsumeLoopOptions{
		Backoff: DefaultBackoff(),
	}
	for _, opt := range opts {
		opt(options)
	}

	attempt := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := consume(ctx); err != nil {
				attempt++

				// 触发错误回调
				if options.OnError != nil {
					options.OnError(err)
				}

				// 应用退避延迟
				delay := options.Backoff.NextDelay(attempt)
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			} else {
				// 成功消费，重置退避
				attempt = 0
				// 如果退避策略支持重置，调用 Reset()
				if resettable, ok := options.Backoff.(xretry.ResettableBackoff); ok {
					resettable.Reset()
				}
			}
		}
	}
}
