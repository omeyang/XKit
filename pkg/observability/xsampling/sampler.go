package xsampling

import "context"

// Sampler 采样策略接口
//
// 采样器用于决定是否对某个事件进行采样。
// 返回 true 表示应该采样，false 表示跳过。
type Sampler interface {
	// ShouldSample 判断是否应该采样
	//
	// ctx 可以携带采样决策所需的上下文信息，
	// 如 trace_id、tenant_id 等，供 KeyBasedSampler 等策略使用。
	// ctx 不得为 nil；如需占位请使用 context.TODO()。
	ShouldSample(ctx context.Context) bool
}

// ResettableSampler 可重置的采样器
//
// 某些有状态的采样器（如 CountSampler）可以被重置到初始状态。
type ResettableSampler interface {
	Sampler
	// Reset 重置采样器状态
	Reset()
}
