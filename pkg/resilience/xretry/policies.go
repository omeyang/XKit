package xretry

import "context"

// FixedRetryPolicy 固定次数重试策略
type FixedRetryPolicy struct {
	maxAttempts int
}

// NewFixedRetry 创建固定次数重试策略
// maxAttempts: 最大尝试次数（包含首次尝试），最小为 1
func NewFixedRetry(maxAttempts int) *FixedRetryPolicy {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	return &FixedRetryPolicy{maxAttempts: maxAttempts}
}

func (p *FixedRetryPolicy) MaxAttempts() int {
	return p.maxAttempts
}

// 设计决策: ShouldRetry 不检查 ctx.Err()，context 取消由 retry-go 的 Context(ctx) 选项
// 在 sleep 阶段统一处理。这确保了 Retryer.Do 和 Do wrapper 两条路径返回一致的
// context 错误（而非业务错误），便于上层通过 errors.Is 可靠分类。
func (p *FixedRetryPolicy) ShouldRetry(_ context.Context, attempt int, err error) bool {
	if attempt >= p.maxAttempts {
		return false
	}
	return IsRetryable(err)
}

// AlwaysRetryPolicy 无限重试策略（慎用）
// 只有遇到永久性错误才停止重试；context 取消由 retry-go 在 sleep 阶段检测。
type AlwaysRetryPolicy struct{}

// NewAlwaysRetry 创建无限重试策略
func NewAlwaysRetry() *AlwaysRetryPolicy {
	return &AlwaysRetryPolicy{}
}

func (p *AlwaysRetryPolicy) MaxAttempts() int {
	return 0 // 0 表示无限
}

// 设计决策: 同 FixedRetryPolicy.ShouldRetry，不检查 ctx.Err()。
func (p *AlwaysRetryPolicy) ShouldRetry(_ context.Context, _ int, err error) bool {
	return IsRetryable(err)
}

// NeverRetryPolicy 永不重试策略
type NeverRetryPolicy struct{}

// NewNeverRetry 创建永不重试策略
func NewNeverRetry() *NeverRetryPolicy {
	return &NeverRetryPolicy{}
}

func (p *NeverRetryPolicy) MaxAttempts() int {
	return 1
}

func (p *NeverRetryPolicy) ShouldRetry(_ context.Context, _ int, _ error) bool {
	return false
}

// 确保实现了 RetryPolicy 接口
var (
	_ RetryPolicy = (*FixedRetryPolicy)(nil)
	_ RetryPolicy = (*AlwaysRetryPolicy)(nil)
	_ RetryPolicy = (*NeverRetryPolicy)(nil)
)
