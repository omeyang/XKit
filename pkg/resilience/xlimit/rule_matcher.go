package xlimit

import (
	"time"

	"github.com/omeyang/xkit/pkg/resilience/xlimit/internal/wildcard"
)

// RuleMatcher 规则匹配器，用于查找适用的限流规则和覆盖配置
type RuleMatcher struct {
	rules    map[string]Rule
	matchers map[string]*wildcard.Matcher
}

// NewRuleMatcher 创建规则匹配器
func NewRuleMatcher(rules []Rule) *RuleMatcher {
	rm := &RuleMatcher{
		rules:    make(map[string]Rule),
		matchers: make(map[string]*wildcard.Matcher),
	}

	for _, rule := range rules {
		rm.rules[rule.Name] = rule

		// 为每个规则的覆盖配置创建匹配器
		if len(rule.Overrides) > 0 {
			patterns := make([]string, len(rule.Overrides))
			for i, override := range rule.Overrides {
				patterns[i] = override.Match
			}
			rm.matchers[rule.Name] = wildcard.NewMatcher(patterns)
		}
	}

	return rm
}

// FindRule 根据规则名称查找规则
// 返回规则和是否找到的标志
func (rm *RuleMatcher) FindRule(key Key, ruleName string) (Rule, bool) {
	rule, ok := rm.rules[ruleName]
	if !ok {
		return Rule{}, false
	}

	// 检查规则是否启用
	if !rule.IsEnabled() {
		return Rule{}, false
	}

	return rule, true
}

// GetEffectiveLimit 获取适用于给定键的有效限流配置
// 返回限流值和窗口时长
func (rm *RuleMatcher) GetEffectiveLimit(rule Rule, key Key) (int, time.Duration) {
	// 渲染键值用于匹配
	renderedKey := key.Render(rule.KeyTemplate)

	// 查找匹配的覆盖配置
	if matcher, ok := rm.matchers[rule.Name]; ok {
		idx := matcher.Match(renderedKey)
		if idx >= 0 && idx < len(rule.Overrides) {
			override := rule.Overrides[idx]
			window := override.Window
			if window == 0 {
				window = rule.Window
			}
			return override.Limit, window
		}
	}

	// 使用默认配置
	return rule.Limit, rule.Window
}

// GetEffectiveBurst 获取适用于给定键的有效突发容量
func (rm *RuleMatcher) GetEffectiveBurst(rule Rule, key Key) int {
	// 渲染键值用于匹配
	renderedKey := key.Render(rule.KeyTemplate)

	// 查找匹配的覆盖配置
	if matcher, ok := rm.matchers[rule.Name]; ok {
		idx := matcher.Match(renderedKey)
		if idx >= 0 && idx < len(rule.Overrides) {
			override := rule.Overrides[idx]
			if override.Burst > 0 {
				return override.Burst
			}
			return override.Limit // 如果未指定 burst，默认等于 limit
		}
	}

	// 使用默认配置
	return rule.EffectiveBurst()
}

// RenderKey 渲染完整的 Redis 键
// 格式：prefix + template 渲染后的值
func (rm *RuleMatcher) RenderKey(rule Rule, key Key, prefix string) string {
	return prefix + key.Render(rule.KeyTemplate)
}

// GetAllRules 返回所有启用的规则名称
func (rm *RuleMatcher) GetAllRules() []string {
	var names []string
	for name, rule := range rm.rules {
		if rule.IsEnabled() {
			names = append(names, name)
		}
	}
	return names
}

// HasRule 检查是否存在指定名称的规则
func (rm *RuleMatcher) HasRule(name string) bool {
	_, ok := rm.rules[name]
	return ok
}
