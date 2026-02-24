package xlimit

import (
	"strings"
	"time"
)

// =============================================================================
// ruleMatcher 实现 RuleProvider 接口
// =============================================================================

// ruleMatcher 规则匹配器，用于查找适用的限流规则和覆盖配置
type ruleMatcher struct {
	rules     map[string]Rule
	ruleNames []string // 保持规则顺序
	matchers  map[string][]string
}

// newRuleMatcher 创建规则匹配器
func newRuleMatcher(rules []Rule) *ruleMatcher {
	rm := &ruleMatcher{
		rules:    make(map[string]Rule),
		matchers: make(map[string][]string),
	}

	for _, rule := range rules {
		if !rule.IsEnabled() {
			continue
		}
		rm.rules[rule.Name] = rule
		rm.ruleNames = append(rm.ruleNames, rule.Name)

		if len(rule.Overrides) > 0 {
			patterns := make([]string, len(rule.Overrides))
			for i, override := range rule.Overrides {
				patterns[i] = override.Match
			}
			rm.matchers[rule.Name] = patterns
		}
	}

	return rm
}

// FindRule 实现 RuleProvider 接口
// 根据 Key 查找第一个适用的规则
//
// 设计决策: 无匹配时返回 (Rule{}, false)，不静默回退到第一条规则。
// 静默回退会掩盖配置错误（如 Key 缺少必要字段），调用方无法区分
// "匹配了默认规则"和"没有规则适用"两种情况。
func (rm *ruleMatcher) FindRule(key Key) (Rule, bool) {
	for _, name := range rm.ruleNames {
		rule, ok := rm.rules[name]
		if !ok {
			continue
		}

		// 静态模板（无变量占位符）始终匹配——如 GlobalRule("global")
		if !strings.Contains(rule.KeyTemplate, "${") {
			return rule, true
		}

		// 动态模板：检查 Key 是否能渲染模板（即包含必要的字段）
		rendered := key.Render(rule.KeyTemplate)
		if rendered != "" && rendered != rule.KeyTemplate {
			return rule, true
		}
	}

	return Rule{}, false
}

// findRule 根据规则名称查找规则（内部使用）
func (rm *ruleMatcher) findRule(ruleName string) (Rule, bool) {
	rule, ok := rm.rules[ruleName]
	if !ok {
		return Rule{}, false
	}
	return rule, true
}

// getEffectiveLimit 获取适用于给定键的有效限流配置
func (rm *ruleMatcher) getEffectiveLimit(rule Rule, key Key) (int, time.Duration) {
	renderedKey := key.Render(rule.KeyTemplate)

	if patterns, ok := rm.matchers[rule.Name]; ok {
		idx := matchFirst(patterns, renderedKey)
		if idx >= 0 && idx < len(rule.Overrides) {
			override := rule.Overrides[idx]
			window := override.Window
			if window == 0 {
				window = rule.Window
			}
			return override.Limit, window
		}
	}

	return rule.Limit, rule.Window
}

// getEffectiveBurst 获取适用于给定键的有效突发容量
func (rm *ruleMatcher) getEffectiveBurst(rule Rule, key Key) int {
	renderedKey := key.Render(rule.KeyTemplate)

	if patterns, ok := rm.matchers[rule.Name]; ok {
		idx := matchFirst(patterns, renderedKey)
		if idx >= 0 && idx < len(rule.Overrides) {
			override := rule.Overrides[idx]
			if override.Burst > 0 {
				return override.Burst
			}
			return override.Limit
		}
	}

	return rule.EffectiveBurst()
}

// renderKey 渲染完整的 Redis 键
func (rm *ruleMatcher) renderKey(rule Rule, key Key, prefix string) string {
	return prefix + key.Render(rule.KeyTemplate)
}

// getAllRules 返回所有启用的规则名称
func (rm *ruleMatcher) getAllRules() []string {
	return rm.ruleNames
}

// hasRule 检查是否存在指定名称的规则
func (rm *ruleMatcher) hasRule(name string) bool {
	_, ok := rm.rules[name]
	return ok
}

// =============================================================================
// 通配符匹配
// =============================================================================

// matchFirst 返回第一个匹配的模式索引，如果无匹配返回 -1
func matchFirst(patterns []string, text string) int {
	for i, pattern := range patterns {
		if wildcardMatch(pattern, text) {
			return i
		}
	}
	return -1
}

// wildcardMatch 检查 text 是否匹配 pattern
// 支持的通配符：
//   - *: 匹配任意字符序列（包括空字符串）
//
// 使用动态规划进行匹配，O(m*n) 时间复杂度，O(n) 空间复杂度
func wildcardMatch(pattern, text string) bool {
	if pattern == "" {
		return text == ""
	}

	pLen, tLen := len(pattern), len(text)

	// 使用两行 DP 代替完整矩阵
	prev := make([]bool, tLen+1)
	curr := make([]bool, tLen+1)

	prev[0] = true

	for i := 1; i <= pLen; i++ {
		switch pattern[i-1] {
		case '*':
			curr[0] = prev[0]
		default:
			curr[0] = false
		}

		for j := 1; j <= tLen; j++ {
			switch pattern[i-1] {
			case '*':
				// * 可以匹配空字符串（prev[j]）或匹配一个或多个字符（curr[j-1]）
				curr[j] = prev[j] || curr[j-1]
			case text[j-1]:
				curr[j] = prev[j-1]
			default:
				curr[j] = false
			}
		}

		prev, curr = curr, prev
	}

	return prev[tLen]
}

// =============================================================================
// 接口实现验证
// =============================================================================

var _ RuleProvider = (*ruleMatcher)(nil)
