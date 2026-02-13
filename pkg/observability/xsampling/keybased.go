package xsampling

import (
	"context"
	"math"

	"github.com/cespare/xxhash/v2"
)

// KeyFunc 从上下文中提取采样 key 的函数
//
// 返回的 key 用于一致性哈希采样，相同的 key 总是产生相同的采样决策。
// 如果返回空字符串，KeyBasedSampler 会回退到随机采样。
type KeyFunc func(ctx context.Context) string

// KeyBasedSampler 基于 key 的一致性采样策略
//
// 设计决策: 工厂函数返回具体类型而非 Sampler 接口，因为 Rate() 方法提供了
// 有用的自省能力（如日志、调试），这些无法通过 Sampler 接口获得。
//
// 对于相同的 key，在相同的 rate 下总是产生相同的采样决策。
// 这对于需要采样一致性的场景非常有用，例如：
//   - 按 trace_id 采样，确保同一条链路的所有 span 都被采样或都不被采样
//   - 按 tenant_id 采样，确保同一租户的请求采样行为一致
//   - 按 user_id 采样，确保同一用户的请求采样行为一致
type KeyBasedSampler struct {
	rate    float64
	keyFunc KeyFunc
}

// NewKeyBasedSampler 创建基于 key 的一致性采样器
//
// rate 表示采样比率，范围 [0.0, 1.0]，超出范围或为 NaN 时返回 ErrInvalidRate。
// keyFunc 用于从 context 中提取采样 key，不能为 nil（为 nil 时返回 ErrNilKeyFunc）。
//
// 示例：
//
//	// 按 trace_id 采样
//	sampler, err := NewKeyBasedSampler(0.1, func(ctx context.Context) string {
//	    return xctx.TraceID(ctx)
//	})
//
//	// 按 tenant_id 采样
//	sampler, err := NewKeyBasedSampler(0.05, func(ctx context.Context) string {
//	    return getTenantID(ctx)
//	})
func NewKeyBasedSampler(rate float64, keyFunc KeyFunc) (*KeyBasedSampler, error) {
	if math.IsNaN(rate) || rate < 0 || rate > 1 {
		return nil, ErrInvalidRate
	}
	if keyFunc == nil {
		return nil, ErrNilKeyFunc
	}
	return &KeyBasedSampler{
		rate:    rate,
		keyFunc: keyFunc,
	}, nil
}

func (s *KeyBasedSampler) ShouldSample(ctx context.Context) bool {
	if s.rate <= 0 {
		return false
	}
	if s.rate >= 1 {
		return true
	}

	// 获取 key
	key := s.keyFunc(ctx)

	// 设计决策: 空 key 回退到随机采样而非 fail-fast，因为采样器应保持弹性——
	// key 提取失败（如缺少 trace ID）不应导致采样功能完全失效。
	// 随机采样保持了近似的采样率语义，只是失去了跨进程一致性。
	if key == "" {
		return randomFloat64() < s.rate
	}

	// 使用 xxhash 零分配确定性哈希
	// xxhash 是确定性的，同一 key 在所有进程中产生相同哈希值
	// 这对分布式追踪采样至关重要：同一 trace_id 在所有服务中被一致采样
	hashValue := xxhash.Sum64String(key)

	// 将 hash 值映射到 [0, 1) 区间
	// 设计决策: 此处使用 uint64/MaxUint64 归一化（与 randomFloat64 的 >>11 * floatScale 不同），
	// 因为确定性哈希需要完整 uint64 值域的均匀映射，而 randomFloat64 优化 IEEE 754 精度。
	normalized := float64(hashValue) / (float64(math.MaxUint64) + 1)

	return normalized < s.rate
}

// Rate 返回当前采样比率
func (s *KeyBasedSampler) Rate() float64 {
	return s.rate
}

// 确保实现了接口
var _ Sampler = (*KeyBasedSampler)(nil)
