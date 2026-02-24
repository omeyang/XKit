package xsampling

import (
	"context"
	"sync/atomic"
)

// alwaysSampler 全采样策略
type alwaysSampler struct{}

// alwaysSamplerInstance 全采样单例
var alwaysSamplerInstance = &alwaysSampler{}

// Always 返回全采样策略
//
// 返回的采样器总是返回 true，即所有事件都会被采样。
// 适用于调试或需要完整数据的场景。
func Always() Sampler {
	return alwaysSamplerInstance
}

func (s *alwaysSampler) ShouldSample(_ context.Context) bool {
	return true
}

// neverSampler 不采样策略
type neverSampler struct{}

// neverSamplerInstance 不采样单例
var neverSamplerInstance = &neverSampler{}

// Never 返回不采样策略
//
// 返回的采样器总是返回 false，即所有事件都会被跳过。
// 适用于禁用采样或生产环境关闭调试采样。
func Never() Sampler {
	return neverSamplerInstance
}

func (s *neverSampler) ShouldSample(_ context.Context) bool {
	return false
}

// RateSampler 固定比率采样策略
//
// 按照指定的比率进行随机采样。例如 rate=0.1 表示 10% 的事件会被采样。
//
// 设计决策: 工厂函数返回具体类型而非 Sampler 接口，因为 Rate() 方法提供了
// 有用的自省能力（如日志、调试），这些无法通过 Sampler 接口获得。
type RateSampler struct {
	rate float64
}

// NewRateSampler 创建固定比率采样器
//
// rate 表示采样比率，范围 [0.0, 1.0]：
//   - rate=0.0: 等同于 Never()，不采样任何事件
//   - rate=1.0: 等同于 Always()，采样所有事件
//   - rate=0.1: 约 10% 的事件会被采样
//
// rate 超出 [0.0, 1.0] 范围或为 NaN 时返回 ErrInvalidRate。
func NewRateSampler(rate float64) (*RateSampler, error) {
	if err := validateRate(rate); err != nil {
		return nil, err
	}
	return &RateSampler{rate: rate}, nil
}

func (s *RateSampler) ShouldSample(_ context.Context) bool {
	if s.rate <= 0 {
		return false
	}
	if s.rate >= 1 {
		return true
	}
	return randomFloat64() < s.rate
}

// Rate 返回当前采样比率
func (s *RateSampler) Rate() float64 {
	return s.rate
}

// CountSampler 计数采样策略
//
// 每 N 个事件采样 1 个。例如 n=100 表示每 100 个事件采样 1 个。
// 第 1、n+1、2n+1... 个事件会被采样。
//
// 内部使用 atomic.Uint64 计数器，自然溢出后通过无符号取模保持正确的采样周期。
//
// 设计决策: 工厂函数返回具体类型而非 Sampler 接口，因为 N() 和 Reset() 方法
// 提供了有用的自省和控制能力，这些无法通过 Sampler 接口获得。
type CountSampler struct {
	n       int
	counter atomic.Uint64
}

// NewCountSampler 创建计数采样器
//
// n 表示采样间隔，即每 n 个事件采样 1 个。
// n < 1 时返回 ErrInvalidCount。
func NewCountSampler(n int) (*CountSampler, error) {
	if n < 1 {
		return nil, ErrInvalidCount
	}
	return &CountSampler{n: n}, nil
}

func (s *CountSampler) ShouldSample(_ context.Context) bool {
	n := s.n
	if n <= 0 {
		// 零值安全：未经 NewCountSampler 构造的零值实例按全采样处理，避免除零 panic
		return true
	}
	// 使用 uint64 避免 int64 溢出后取模产生负数的问题
	count := s.counter.Add(1)
	return (count-1)%uint64(n) == 0
}

// Reset 重置计数器到初始状态
func (s *CountSampler) Reset() {
	s.counter.Store(0)
}

// N 返回采样间隔
func (s *CountSampler) N() int {
	return s.n
}

// 确保实现了接口
var (
	_ Sampler           = (*alwaysSampler)(nil)
	_ Sampler           = (*neverSampler)(nil)
	_ Sampler           = (*RateSampler)(nil)
	_ Sampler           = (*CountSampler)(nil)
	_ ResettableSampler = (*CountSampler)(nil)
)
