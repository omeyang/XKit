package xkafka

import (
	"context"
	"sync/atomic"

	"github.com/omeyang/xkit/internal/mqcore"
	"github.com/omeyang/xkit/pkg/resilience/xretry"
)

// BackoffPolicy 退避策略接口。
// 这是 xretry.BackoffPolicy 的类型别名，用于消费循环的退避控制。
//
// 推荐使用 xretry 包中的实现：
//   - xretry.NewExponentialBackoff() - 指数退避（推荐）
//   - xretry.NewFixedBackoff(delay) - 固定延迟
//   - xretry.NewLinearBackoff(initial, increment, max) - 线性增长
type BackoffPolicy = xretry.BackoffPolicy

// DefaultBackoffPolicy 返回默认退避策略。
// 使用 xretry.ExponentialBackoff，配置：
//   - InitialDelay: 100ms
//   - MaxDelay: 30s
//   - Multiplier: 2.0
//   - Jitter: 0.1 (10%)
func DefaultBackoffPolicy() BackoffPolicy {
	return xretry.NewExponentialBackoff()
}

// runConsumeLoop 是 ConsumeLoopWithPolicy 的共享实现。
// consume 是单次消费函数，errorsCount 用于错误计数，backoff 为退避策略（nil 使用默认值）。
func runConsumeLoop(ctx context.Context, consume mqcore.ConsumeFunc, errorsCount *atomic.Int64, backoff BackoffPolicy) error {
	onError := func(_ error) {
		errorsCount.Add(1)
	}

	opts := []mqcore.ConsumeLoopOption{
		mqcore.WithOnError(onError),
	}
	if backoff != nil {
		opts = append(opts, mqcore.WithBackoff(backoff))
	}

	return mqcore.RunConsumeLoop(ctx, consume, opts...)
}
