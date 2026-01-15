package xlimit

import (
	"fmt"
	"time"
)

// FallbackStrategy 降级策略
type FallbackStrategy string

const (
	// FallbackLocal 降级到本地限流（推荐）
	// 本地配额 = 分布式配额 / Pod 数量
	FallbackLocal FallbackStrategy = "local"

	// FallbackOpen 放行所有请求（fail-open）
	// 适用于限流不是强需求的场景
	FallbackOpen FallbackStrategy = "open"

	// FallbackClose 拒绝所有请求（fail-close）
	// 适用于安全要求极高的场景
	FallbackClose FallbackStrategy = "close"
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
	KeyPrefix string `json:"key_prefix" yaml:"key_prefix" koanf:"key_prefix"`

	// Rules 限流规则列表
	Rules []Rule `json:"rules" yaml:"rules" koanf:"rules"`

	// Fallback Redis 不可用时的降级策略
	Fallback FallbackStrategy `json:"fallback" yaml:"fallback" koanf:"fallback"`

	// LocalPodCount 预期 Pod 数量，用于计算本地降级配额
	// 本地配额 = 分布式配额 / LocalPodCount
	LocalPodCount int `json:"local_pod_count" yaml:"local_pod_count" koanf:"local_pod_count"`

	// EnableMetrics 是否启用指标收集
	EnableMetrics bool `json:"enable_metrics" yaml:"enable_metrics" koanf:"enable_metrics"`

	// EnableHeaders 是否在响应中添加限流头
	EnableHeaders bool `json:"enable_headers" yaml:"enable_headers" koanf:"enable_headers"`
}

// Validate 验证配置是否有效
func (c Config) Validate() error {
	if !c.Fallback.IsValid() {
		return fmt.Errorf("%w: invalid fallback strategy %q", ErrInvalidRule, c.Fallback)
	}

	if c.LocalPodCount < 0 {
		return fmt.Errorf("%w: local_pod_count cannot be negative", ErrInvalidRule)
	}

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
	if c.LocalPodCount <= 0 {
		return 1
	}
	return c.LocalPodCount
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
			clone.Rules[i] = rule.Clone()
		}
	}

	return clone
}

// Clone 创建规则的深拷贝
func (r Rule) Clone() Rule {
	clone := r
	if r.Overrides != nil {
		clone.Overrides = make([]Override, len(r.Overrides))
		copy(clone.Overrides, r.Overrides)
	}
	if r.Enabled != nil {
		enabled := *r.Enabled
		clone.Enabled = &enabled
	}
	return clone
}

// Rule 限流规则
type Rule struct {
	// Name 规则名称，用于日志和指标
	Name string `json:"name" yaml:"name" koanf:"name"`

	// KeyTemplate 限流键模板，支持变量替换
	// 可用变量：${tenant_id}, ${caller_id}, ${method}, ${path}, ${resource}
	KeyTemplate string `json:"key_template" yaml:"key_template" koanf:"key_template"`

	// Limit 窗口内允许的最大请求数
	Limit int `json:"limit" yaml:"limit" koanf:"limit"`

	// Window 限流窗口时长
	Window time.Duration `json:"window" yaml:"window" koanf:"window"`

	// Burst 突发容量，允许短时间内超过 Limit 的请求数
	// 如果为 0，则默认等于 Limit
	Burst int `json:"burst,omitempty" yaml:"burst,omitempty" koanf:"burst"`

	// Overrides 覆盖配置，用于特定键的定制化限流
	Overrides []Override `json:"overrides,omitempty" yaml:"overrides,omitempty" koanf:"overrides"`

	// Enabled 是否启用规则，nil 或 true 表示启用
	Enabled *bool `json:"enabled,omitempty" yaml:"enabled,omitempty" koanf:"enabled"`
}

// Validate 验证规则配置是否有效
func (r Rule) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidRule)
	}
	if r.KeyTemplate == "" {
		return fmt.Errorf("%w: key_template is required", ErrInvalidRule)
	}
	if r.Limit <= 0 {
		return fmt.Errorf("%w: limit must be positive", ErrInvalidRule)
	}
	if r.Window <= 0 {
		return fmt.Errorf("%w: window must be positive", ErrInvalidRule)
	}
	if r.Burst < 0 {
		return fmt.Errorf("%w: burst cannot be negative", ErrInvalidRule)
	}

	for i, override := range r.Overrides {
		if err := override.Validate(); err != nil {
			return fmt.Errorf("%w: override[%d]: %v", ErrInvalidRule, i, err)
		}
	}

	return nil
}

// IsEnabled 检查规则是否启用
func (r Rule) IsEnabled() bool {
	if r.Enabled == nil {
		return true
	}
	return *r.Enabled
}

// EffectiveBurst 返回有效的突发容量
// 如果 Burst 为 0，返回 Limit
func (r Rule) EffectiveBurst() int {
	if r.Burst == 0 {
		return r.Limit
	}
	return r.Burst
}

// Override 覆盖配置，用于覆盖特定键的默认限流配置
type Override struct {
	// Match 匹配模式，支持 * 通配符
	// 例如：tenant:vip-corp, tenant:*, api:POST:*
	Match string `json:"match" yaml:"match" koanf:"match"`

	// Limit 覆盖的配额上限
	Limit int `json:"limit" yaml:"limit" koanf:"limit"`

	// Window 覆盖的窗口时长（可选，不设置则使用规则默认值）
	Window time.Duration `json:"window,omitempty" yaml:"window,omitempty" koanf:"window"`

	// Burst 覆盖的突发容量（可选）
	Burst int `json:"burst,omitempty" yaml:"burst,omitempty" koanf:"burst"`
}

// Validate 验证覆盖配置是否有效
func (o Override) Validate() error {
	if o.Match == "" {
		return fmt.Errorf("match pattern is required")
	}
	if o.Limit <= 0 {
		return fmt.Errorf("limit must be positive")
	}
	return nil
}

// NewRule 创建一个新规则
func NewRule(name, keyTemplate string, limit int, window time.Duration) Rule {
	return Rule{
		Name:        name,
		KeyTemplate: keyTemplate,
		Limit:       limit,
		Window:      window,
	}
}

// TenantRule 创建租户级限流规则
// 键模板：tenant:${tenant_id}
func TenantRule(name string, limit int, window time.Duration) Rule {
	return NewRule(name, "tenant:${tenant_id}", limit, window)
}

// GlobalRule 创建全局限流规则
// 键模板：global（所有请求共享同一配额）
func GlobalRule(name string, limit int, window time.Duration) Rule {
	return NewRule(name, "global", limit, window)
}

// TenantAPIRule 创建租户+API级限流规则
// 键模板：tenant:${tenant_id}:api:${method}:${path}
func TenantAPIRule(name string, limit int, window time.Duration) Rule {
	return NewRule(name, "tenant:${tenant_id}:api:${method}:${path}", limit, window)
}

// CallerRule 创建调用方级限流规则
// 键模板：caller:${caller_id}
func CallerRule(name string, limit int, window time.Duration) Rule {
	return NewRule(name, "caller:${caller_id}", limit, window)
}

// RuleBuilder 规则构建器，支持链式调用
type RuleBuilder struct {
	rule Rule
}

// NewRuleBuilder 创建规则构建器
func NewRuleBuilder(name string) *RuleBuilder {
	return &RuleBuilder{
		rule: Rule{Name: name},
	}
}

// KeyTemplate 设置键模板
func (b *RuleBuilder) KeyTemplate(template string) *RuleBuilder {
	b.rule.KeyTemplate = template
	return b
}

// Limit 设置配额上限
func (b *RuleBuilder) Limit(limit int) *RuleBuilder {
	b.rule.Limit = limit
	return b
}

// Window 设置窗口时长
func (b *RuleBuilder) Window(window time.Duration) *RuleBuilder {
	b.rule.Window = window
	return b
}

// Burst 设置突发容量
func (b *RuleBuilder) Burst(burst int) *RuleBuilder {
	b.rule.Burst = burst
	return b
}

// Enabled 设置是否启用
func (b *RuleBuilder) Enabled(enabled bool) *RuleBuilder {
	b.rule.Enabled = &enabled
	return b
}

// AddOverride 添加覆盖配置
func (b *RuleBuilder) AddOverride(match string, limit int) *RuleBuilder {
	b.rule.Overrides = append(b.rule.Overrides, Override{
		Match: match,
		Limit: limit,
	})
	return b
}

// AddOverrideWithWindow 添加带窗口的覆盖配置
func (b *RuleBuilder) AddOverrideWithWindow(match string, limit int, window time.Duration) *RuleBuilder {
	b.rule.Overrides = append(b.rule.Overrides, Override{
		Match:  match,
		Limit:  limit,
		Window: window,
	})
	return b
}

// Build 构建规则
func (b *RuleBuilder) Build() Rule {
	return b.rule
}
