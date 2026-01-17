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
func WithOnError(onError func(err error)) ConsumeLoopOption {
	return func(o *ConsumeLoopOptions) {
		o.OnError = onError
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

// =============================================================================
// 向后兼容的 BackoffConfig 类型
// =============================================================================

// BackoffConfig 退避配置。
//
// Deprecated: 请直接使用 xretry.BackoffPolicy 接口。
// 推荐使用 xretry.NewExponentialBackoff() 创建退避策略。
//
// 此类型保留用于向后兼容。
type BackoffConfig struct {
	InitialDelay time.Duration // 初始延迟，默认 100ms
	MaxDelay     time.Duration // 最大延迟，默认 30s
	Multiplier   float64       // 退避乘数，默认 2.0
}

// DefaultBackoffConfig 返回默认退避配置。
//
// Deprecated: 请使用 xretry.NewExponentialBackoff() 替代。
func DefaultBackoffConfig() BackoffConfig {
	return BackoffConfig{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}
}

// ToBackoffPolicy 将 BackoffConfig 转换为 BackoffPolicy。
// 内部使用，将旧配置转换为新的策略接口。
func (c BackoffConfig) ToBackoffPolicy() xretry.BackoffPolicy {
	return xretry.NewExponentialBackoff(
		xretry.WithInitialDelay(c.InitialDelay),
		xretry.WithMaxDelay(c.MaxDelay),
		xretry.WithMultiplier(c.Multiplier),
		xretry.WithJitter(0), // 旧配置不支持 jitter，设为 0 保持兼容
	)
}

// RunConsumeLoop 运行消费循环，使用退避策略处理错误。
//
// 循环逻辑：
//  1. 调用 consume 函数消费消息
//  2. 如果成功（err == nil），重置退避计数器
//  3. 如果失败（err != nil），应用退避延迟后重试
//  4. 循环直到 ctx 取消
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
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
			} else {
				// 成功消费，重置退避
				attempt = 0
			}
		}
	}
}
