package xpulsar

import (
	"time"

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

// BackoffConfig 退避配置。
//
// Deprecated: 请直接使用 xretry.BackoffPolicy 接口。
// 推荐使用 xretry.NewExponentialBackoff() 创建退避策略。
//
// 此类型保留用于向后兼容。
type BackoffConfig = mqcore.BackoffConfig //nolint:staticcheck // 向后兼容

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

// DefaultBackoffPolicy 返回默认退避策略。
// 使用 xretry.ExponentialBackoff，配置：
//   - InitialDelay: 100ms
//   - MaxDelay: 30s
//   - Multiplier: 2.0
//   - Jitter: 0.1 (10%)
func DefaultBackoffPolicy() BackoffPolicy {
	return xretry.NewExponentialBackoff()
}
