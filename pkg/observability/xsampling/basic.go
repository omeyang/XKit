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
// rate 超出范围时会被夹紧到 [0.0, 1.0]。
func NewRateSampler(rate float64) *RateSampler {
	if rate < 0 {
		rate = 0
	} else if rate > 1 {
		rate = 1
	}
	return &RateSampler{rate: rate}
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
type CountSampler struct {
	n       int64
	counter atomic.Int64
}

// NewCountSampler 创建计数采样器
//
// n 表示采样间隔，即每 n 个事件采样 1 个。
// n < 1 时会被设为 1，等同于 Always()。
func NewCountSampler(n int) *CountSampler {
	if n < 1 {
		n = 1
	}
	return &CountSampler{n: int64(n)}
}

func (s *CountSampler) ShouldSample(_ context.Context) bool {
	count := s.counter.Add(1)
	// 采样第 1、(n+1)、(2n+1)... 个事件
	// 即 count-1 能被 n 整除时采样
	return (count-1)%s.n == 0
}

// Reset 重置计数器到初始状态
func (s *CountSampler) Reset() {
	s.counter.Store(0)
}

// N 返回采样间隔
func (s *CountSampler) N() int {
	return int(s.n)
}

// ProbabilitySampler 概率采样策略
//
// 按照指定的概率进行随机采样，与 RateSampler 实现相同但语义不同。
// ProbabilitySampler 强调"概率"语义，适用于统计采样场景。
type ProbabilitySampler struct {
	probability float64
}

// NewProbabilitySampler 创建概率采样器
//
// probability 表示采样概率，范围 [0.0, 1.0]：
//   - probability=0.0: 不采样任何事件
//   - probability=1.0: 采样所有事件
//   - probability=0.5: 约 50% 的事件会被采样
//
// probability 超出范围时会被夹紧到 [0.0, 1.0]。
func NewProbabilitySampler(probability float64) *ProbabilitySampler {
	if probability < 0 {
		probability = 0
	} else if probability > 1 {
		probability = 1
	}
	return &ProbabilitySampler{probability: probability}
}

func (s *ProbabilitySampler) ShouldSample(_ context.Context) bool {
	if s.probability <= 0 {
		return false
	}
	if s.probability >= 1 {
		return true
	}
	return randomFloat64() < s.probability
}

// Probability 返回当前采样概率
func (s *ProbabilitySampler) Probability() float64 {
	return s.probability
}

// 确保实现了接口
var (
	_ Sampler           = (*alwaysSampler)(nil)
	_ Sampler           = (*neverSampler)(nil)
	_ Sampler           = (*RateSampler)(nil)
	_ Sampler           = (*CountSampler)(nil)
	_ Sampler           = (*ProbabilitySampler)(nil)
	_ ResettableSampler = (*CountSampler)(nil)
)
