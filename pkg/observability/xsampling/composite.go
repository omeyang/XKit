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

// String 返回组合模式的字符串表示
func (m CompositeMode) String() string {
	switch m {
	case ModeAND:
		return "AND"
	case ModeOR:
		return "OR"
	default:
		return "Unknown"
	}
}

// CompositeSampler 组合采样策略
//
// 将多个采样器组合在一起，支持 AND/OR 逻辑：
//   - ModeAND: 所有子采样器都返回 true 时才采样
//   - ModeOR: 任一子采样器返回 true 时即采样
//
// 组合采样器使用短路求值：AND 模式遇到 false 立即返回，OR 模式遇到 true 立即返回。
// 设计决策: 有状态子采样器（如 CountSampler）的内部状态仅在实际被求值时更新，
// 因此子采样器的排列顺序可能影响有状态采样器的行为。
type CompositeSampler struct {
	samplers []Sampler
	mode     CompositeMode
}

// NewCompositeSampler 创建组合采样器
//
// mode 指定组合逻辑（ModeAND 或 ModeOR）。
// samplers 是要组合的子采样器列表。
//
// 非法 mode 返回 ErrInvalidMode，nil 子采样器返回 ErrNilSampler。
//
// 注意：组合采样器使用短路求值——ModeAND 遇到 false 立即返回，ModeOR 遇到 true 立即返回。
// 有状态子采样器（如 CountSampler）仅在被实际求值时更新内部状态，因此子采样器的排列顺序
// 会影响有状态采样器的行为。例如 All(rateSampler, countSampler) 中，当 rateSampler
// 返回 false 时 countSampler 不会被求值，其计数器不会递增。
//
// 示例：
//
//	rateSampler, _ := NewRateSampler(0.1)
//	countSampler, _ := NewCountSampler(10)
//
//	// 同时满足比率采样和计数采样
//	sampler, err := NewCompositeSampler(ModeAND,
//	    rateSampler,
//	    countSampler,
//	)
//
//	lowRateSampler, _ := NewRateSampler(0.01)
//
//	// 满足任一条件即采样
//	sampler, err = NewCompositeSampler(ModeOR,
//	    lowRateSampler,        // 1% 随机采样
//	    debugSampler,          // 或者调试模式采样
//	)
func NewCompositeSampler(mode CompositeMode, samplers ...Sampler) (*CompositeSampler, error) {
	if mode != ModeAND && mode != ModeOR {
		return nil, ErrInvalidMode
	}

	for _, s := range samplers {
		if s == nil {
			return nil, ErrNilSampler
		}
	}

	// 复制切片以防止外部修改
	copied := make([]Sampler, len(samplers))
	copy(copied, samplers)
	return &CompositeSampler{
		samplers: copied,
		mode:     mode,
	}, nil
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
// nil 子采样器返回 ErrNilSampler。
//
// 示例：
//
//	rateSampler, _ := NewRateSampler(0.1)
//	countSampler, _ := NewCountSampler(10)
//	sampler, err := All(rateSampler, countSampler)
func All(samplers ...Sampler) (*CompositeSampler, error) {
	return NewCompositeSampler(ModeAND, samplers...)
}

// Any 创建 OR 组合采样器（便捷函数）
//
// 等同于 NewCompositeSampler(ModeOR, samplers...)
// nil 子采样器返回 ErrNilSampler。
//
// 示例：
//
//	lowRateSampler, _ := NewRateSampler(0.01)
//	sampler, err := Any(lowRateSampler, debugSampler)
func Any(samplers ...Sampler) (*CompositeSampler, error) {
	return NewCompositeSampler(ModeOR, samplers...)
}

// 确保实现了接口
var (
	_ Sampler           = (*CompositeSampler)(nil)
	_ ResettableSampler = (*CompositeSampler)(nil)
)
