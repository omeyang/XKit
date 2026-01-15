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

func (p *FixedRetryPolicy) ShouldRetry(ctx context.Context, attempt int, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	if attempt >= p.maxAttempts {
		return false
	}
	return IsRetryable(err)
}

// AlwaysRetryPolicy 无限重试策略（慎用）
// 只有上下文取消或遇到永久性错误才会停止
type AlwaysRetryPolicy struct{}

// NewAlwaysRetry 创建无限重试策略
func NewAlwaysRetry() *AlwaysRetryPolicy {
	return &AlwaysRetryPolicy{}
}

func (p *AlwaysRetryPolicy) MaxAttempts() int {
	return 0 // 0 表示无限
}

func (p *AlwaysRetryPolicy) ShouldRetry(ctx context.Context, _ int, err error) bool {
	if ctx.Err() != nil {
		return false
	}
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
