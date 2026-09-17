package xsampling

import (
	"context"
	"math"

	"github.com/cespare/xxhash/v2"
)

// KeyFunc 从上下文中提取采样 key 的函数
//
// 返回的 key 用于一致性哈希采样，相同的 key 总是产生相同的采样决策。
// 如果返回空字符串，KeyBasedSampler 会回退到随机采样，此时仍保持近似的采样率语义，
// 但失去跨进程一致性保证。
type KeyFunc func(ctx context.Context) string

// KeyBasedOption 配置 KeyBasedSampler 的可选参数
type KeyBasedOption func(*KeyBasedSampler)

// WithOnEmptyKey 设置空 key 回调函数
//
// 当 KeyFunc 返回空字符串时，在执行随机采样回退前调用此回调。
// 用于指标计数或日志记录，帮助发现上下文传播链路断裂问题。
//
// 设计决策: 回调未做 recover 隔离——回调 panic 会直接传播到 ShouldSample 调用方。
// 这是有意为之：(1) 采样热路径不引入 defer 开销；(2) panic 是编程错误，应快速暴露而非静默吞没；
// (3) 回调由调用方注入，调用方有责任保证其安全性。
// 回调应当轻量（如原子计数器递增），避免阻塞采样热路径。
// nil 回调会被忽略。
func WithOnEmptyKey(fn func()) KeyBasedOption {
	return func(s *KeyBasedSampler) {
		if fn != nil {
			s.onEmptyKey = fn
		}
	}
}

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
	rate       float64
	keyFunc    KeyFunc
	onEmptyKey func() // 空 key 回调，用于可观测性（指标/日志）
}

// NewKeyBasedSampler 创建基于 key 的一致性采样器
//
// rate 表示采样比率，范围 [0.0, 1.0]，超出范围或为 NaN 时返回 ErrInvalidRate。
// keyFunc 用于从 context 中提取采样 key，不能为 nil（为 nil 时返回 ErrNilKeyFunc）。
// nil option 返回 ErrNilOption。
//
// 当 keyFunc 返回空字符串时，采样器回退到随机采样（保持采样率语义但失去一致性）。
// 可通过 WithOnEmptyKey 注册回调来监控空 key 事件，帮助排查上下文传播问题。
//
// 示例：
//
//	// 按 trace_id 采样
//	sampler, err := NewKeyBasedSampler(0.1, func(ctx context.Context) string {
//	    return xctx.TraceID(ctx)
//	})
//
//	// 按 tenant_id 采样，并监控空 key
//	var emptyKeyCount atomic.Int64
//	sampler, err := NewKeyBasedSampler(0.05, func(ctx context.Context) string {
//	    return getTenantID(ctx)
//	}, WithOnEmptyKey(func() {
//	    emptyKeyCount.Add(1)
//	}))
func NewKeyBasedSampler(rate float64, keyFunc KeyFunc, opts ...KeyBasedOption) (*KeyBasedSampler, error) {
	if err := validateRate(rate); err != nil {
		return nil, err
	}
	if keyFunc == nil {
		return nil, ErrNilKeyFunc
	}
	s := &KeyBasedSampler{
		rate:    rate,
		keyFunc: keyFunc,
	}
	for _, opt := range opts {
		if opt == nil {
			return nil, ErrNilOption
		}
		opt(s)
	}
	return s, nil
}

func (s *KeyBasedSampler) ShouldSample(ctx context.Context) bool {
	if s.rate <= 0 {
		return false
	}
	if s.rate >= 1 {
		return true
	}

	// 设计决策: nil ctx 与空 key 同等处理——保持弹性，不因上下文缺失而 panic。
	// 与 XKit 其他包（xkeylock ErrNilContext、xsemaphore applyDefaultTimeout）防御风格一致。
	if ctx == nil {
		if s.onEmptyKey != nil {
			s.onEmptyKey()
		}
		return randomFloat64() < s.rate
	}

	// 获取 key
	key := s.keyFunc(ctx)

	// 设计决策: 空 key 回退到随机采样而非 fail-fast，因为采样器应保持弹性——
	// key 提取失败（如缺少 trace ID）不应导致采样功能完全失效。
	// 随机采样保持了近似的采样率语义，只是失去了跨进程一致性。
	if key == "" {
		if s.onEmptyKey != nil {
			s.onEmptyKey()
		}
		return randomFloat64() < s.rate
	}

	// 使用 xxhash 零分配确定性哈希
	// xxhash 是确定性的，同一 key 在所有进程中产生相同哈希值
	// 这对分布式追踪采样至关重要：同一 trace_id 在所有服务中被一致采样
	hashValue := xxhash.Sum64String(key)

	// 将 hash 值归一化到 [0, 1] 区间
	// 设计决策: 此处使用 uint64/MaxUint64 归一化（与 randomFloat64 的 >>11 * floatScale 不同），
	// 因为确定性哈希需要完整 uint64 值域的均匀映射，而 randomFloat64 优化 IEEE 754 精度。
	// 注意：float64 精度有限，极大 uint64 值（约 2^53 以上）的归一化结果可能不精确，
	// 且当 hashValue == MaxUint64 时 normalized 可能等于 1.0。但 rate < 1 时（rate=1.0 有
	// 提前返回保护）normalized == 1.0 不会通过 normalized < rate，因此行为正确。
	normalized := float64(hashValue) / float64(math.MaxUint64)

	return normalized < s.rate
}

// Rate 返回当前采样比率
func (s *KeyBasedSampler) Rate() float64 {
	return s.rate
}

// 确保实现了接口
var _ Sampler = (*KeyBasedSampler)(nil)
