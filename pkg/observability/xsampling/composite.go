package xsampling

import "context"

// CompositeMode 组合采样模式
type CompositeMode int

const (
	// ModeAND 要求所有子采样器都通过才采样
	//
	// 逻辑与：sampler1.ShouldSample() && sampler2.ShouldSample() && ...
	// 空列表时返回 true（逻辑与的恒等元）
	ModeAND CompositeMode = iota

	// ModeOR 任一子采样器通过即采样
	//
	// 逻辑或：sampler1.ShouldSample() || sampler2.ShouldSample() || ...
	// 空列表时返回 false（逻辑或的恒等元）
	ModeOR
)

// CompositeSampler 组合采样策略
//
// 将多个采样器组合在一起，支持 AND/OR 逻辑：
//   - ModeAND: 所有子采样器都返回 true 时才采样
//   - ModeOR: 任一子采样器返回 true 时即采样
//
// 组合采样器支持短路求值以提高性能。
type CompositeSampler struct {
	samplers []Sampler
	mode     CompositeMode
}

// NewCompositeSampler 创建组合采样器
//
// mode 指定组合逻辑（ModeAND 或 ModeOR）。
// samplers 是要组合的子采样器列表。
//
// 示例：
//
//	// 同时满足比率采样和计数采样
//	sampler := NewCompositeSampler(ModeAND,
//	    NewRateSampler(0.1),
//	    NewCountSampler(10),
//	)
//
//	// 满足任一条件即采样
//	sampler := NewCompositeSampler(ModeOR,
//	    NewRateSampler(0.01),  // 1% 随机采样
//	    debugSampler,          // 或者调试模式采样
//	)
func NewCompositeSampler(mode CompositeMode, samplers ...Sampler) *CompositeSampler {
	// 校验 mode，只接受 ModeAND 和 ModeOR
	// 非法值属于编程错误，立即 panic 以便快速发现
	if mode != ModeAND && mode != ModeOR {
		panic("xsampling: invalid CompositeMode, must be ModeAND or ModeOR")
	}

	// 复制切片以防止外部修改
	copied := make([]Sampler, len(samplers))
	copy(copied, samplers)
	return &CompositeSampler{
		samplers: copied,
		mode:     mode,
	}
}

func (s *CompositeSampler) ShouldSample(ctx context.Context) bool {
	if len(s.samplers) == 0 {
		// 空列表：AND 返回 true（恒等元），OR 返回 false（恒等元）
		return s.mode == ModeAND
	}

	for _, sampler := range s.samplers {
		result := sampler.ShouldSample(ctx)
		if s.mode == ModeAND && !result {
			return false // 短路求值：AND 模式遇到 false 立即返回
		}
		if s.mode == ModeOR && result {
			return true // 短路求值：OR 模式遇到 true 立即返回
		}
	}

	// AND 模式：所有都是 true，返回 true
	// OR 模式：所有都是 false，返回 false
	return s.mode == ModeAND
}

// Reset 重置所有可重置的子采样器
func (s *CompositeSampler) Reset() {
	for _, sampler := range s.samplers {
		if resettable, ok := sampler.(ResettableSampler); ok {
			resettable.Reset()
		}
	}
}

// Mode 返回组合模式
func (s *CompositeSampler) Mode() CompositeMode {
	return s.mode
}

// Samplers 返回子采样器列表（只读副本）
func (s *CompositeSampler) Samplers() []Sampler {
	copied := make([]Sampler, len(s.samplers))
	copy(copied, s.samplers)
	return copied
}

// All 创建 AND 组合采样器（便捷函数）
//
// 等同于 NewCompositeSampler(ModeAND, samplers...)
//
// 示例：
//
//	sampler := All(
//	    NewRateSampler(0.1),
//	    NewCountSampler(10),
//	)
func All(samplers ...Sampler) *CompositeSampler {
	return NewCompositeSampler(ModeAND, samplers...)
}

// Any 创建 OR 组合采样器（便捷函数）
//
// 等同于 NewCompositeSampler(ModeOR, samplers...)
//
// 示例：
//
//	sampler := Any(
//	    NewRateSampler(0.01),
//	    debugSampler,
//	)
func Any(samplers ...Sampler) *CompositeSampler {
	return NewCompositeSampler(ModeOR, samplers...)
}

// 确保实现了接口
var (
	_ Sampler           = (*CompositeSampler)(nil)
	_ ResettableSampler = (*CompositeSampler)(nil)
)
