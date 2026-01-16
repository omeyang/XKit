package xlimit

import (
	"fmt"
	"time"
)

// Rule 限流规则
type Rule struct {
	// Name 规则名称，用于日志和指标
	Name string `json:"name" yaml:"name"`

	// KeyTemplate 限流键模板，支持变量替换
	// 可用变量：${tenant_id}, ${caller_id}, ${method}, ${path}, ${resource}
	KeyTemplate string `json:"key_template" yaml:"key_template"`

	// Limit 窗口内允许的最大请求数
	Limit int `json:"limit" yaml:"limit"`

	// Window 限流窗口时长
	Window time.Duration `json:"window" yaml:"window"`

	// Burst 突发容量，允许短时间内超过 Limit 的请求数
	// 如果为 0，则默认等于 Limit
	Burst int `json:"burst,omitempty" yaml:"burst,omitempty"`

	// Overrides 覆盖配置，用于特定键的定制化限流
	Overrides []Override `json:"overrides,omitempty" yaml:"overrides,omitempty"`

	// Enabled 是否启用规则，nil 或 true 表示启用
	Enabled *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}

// Override 覆盖配置，用于覆盖特定键的默认限流配置
type Override struct {
	// Match 匹配模式，支持 * 通配符
	// 例如：tenant:vip-corp, tenant:*, api:POST:*
	Match string `json:"match" yaml:"match"`

	// Limit 覆盖的配额上限
	Limit int `json:"limit" yaml:"limit"`

	// Window 覆盖的窗口时长（可选，不设置则使用规则默认值）
	Window time.Duration `json:"window,omitempty" yaml:"window,omitempty"`

	// Burst 覆盖的突发容量（可选）
	Burst int `json:"burst,omitempty" yaml:"burst,omitempty"`
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
