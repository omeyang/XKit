package xlimit

import (
	"fmt"
)

// FallbackStrategy 降级策略
type FallbackStrategy string

const (
	// FallbackLocal 降级到本地限流（推荐）
	// 本地配额 = 分布式配额 / Pod 数量
	FallbackLocal FallbackStrategy = "local"

	// FallbackOpen 放行所有请求（fail-open）
	// 适用于限流不是强需求的场景
	FallbackOpen FallbackStrategy = "fail-open"

	// FallbackClose 拒绝所有请求（fail-close）
	// 适用于安全要求极高的场景
	FallbackClose FallbackStrategy = "fail-close"
)

// IsValid 检查降级策略是否有效
func (s FallbackStrategy) IsValid() bool {
	switch s {
	case FallbackLocal, FallbackOpen, FallbackClose, "":
		return true
	default:
		return false
	}
}

// Config 限流器配置
type Config struct {
	// KeyPrefix Redis 键前缀，默认为 "ratelimit:"
	KeyPrefix string `json:"key_prefix" yaml:"key_prefix"`

	// Rules 限流规则列表
	Rules []Rule `json:"rules" yaml:"rules"`

	// Fallback Redis 不可用时的降级策略
	Fallback FallbackStrategy `json:"fallback" yaml:"fallback"`

	// LocalPodCount 预期 Pod 数量，用于计算本地降级配额
	// 本地配额 = 分布式配额 / LocalPodCount
	LocalPodCount int `json:"local_pod_count" yaml:"local_pod_count"`

	// EnableMetrics 是否启用 Prometheus 指标
	EnableMetrics bool `json:"enable_metrics" yaml:"enable_metrics"`

	// EnableHeaders 是否在响应中添加限流头
	EnableHeaders bool `json:"enable_headers" yaml:"enable_headers"`
}

// Validate 验证配置是否有效
func (c Config) Validate() error {
	// 验证降级策略
	if !c.Fallback.IsValid() {
		return fmt.Errorf("%w: invalid fallback strategy %q", ErrInvalidRule, c.Fallback)
	}

	// 验证 Pod 数量
	if c.LocalPodCount < 0 {
		return fmt.Errorf("%w: local_pod_count cannot be negative", ErrInvalidRule)
	}

	// 验证规则
	for i, rule := range c.Rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
	}

	return nil
}

// EffectivePodCount 返回有效的 Pod 数量
// 如果 LocalPodCount 为 0，返回 1
func (c Config) EffectivePodCount() int {
	if c.LocalPodCount == 0 {
		return 1
	}
	return c.LocalPodCount
}

// Clone 创建配置的深拷贝
func (c Config) Clone() Config {
	clone := Config{
		KeyPrefix:     c.KeyPrefix,
		Fallback:      c.Fallback,
		LocalPodCount: c.LocalPodCount,
		EnableMetrics: c.EnableMetrics,
		EnableHeaders: c.EnableHeaders,
	}

	if c.Rules != nil {
		clone.Rules = make([]Rule, len(c.Rules))
		for i, rule := range c.Rules {
			clone.Rules[i] = rule
			if rule.Overrides != nil {
				clone.Rules[i].Overrides = make([]Override, len(rule.Overrides))
				copy(clone.Rules[i].Overrides, rule.Overrides)
			}
		}
	}

	return clone
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		KeyPrefix:     "ratelimit:",
		Fallback:      FallbackLocal,
		LocalPodCount: 1,
		EnableMetrics: true,
		EnableHeaders: true,
	}
}
