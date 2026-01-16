// Package wildcard 提供通配符模式匹配功能
package wildcard

// Match 检查 text 是否匹配 pattern
// 支持的通配符：
//   - *: 匹配任意字符序列（包括空字符串）
//
// 示例：
//   - Match("tenant:*", "tenant:abc") -> true
//   - Match("tenant:vip-*", "tenant:vip-corp") -> true
//   - Match("*:api:*", "tenant:api:POST") -> true
func Match(pattern, text string) bool {
	// 空模式只匹配空文本
	if pattern == "" {
		return text == ""
	}

	// 使用动态规划进行匹配
	// dp[i][j] 表示 pattern[0:i] 是否匹配 text[0:j]
	pLen, tLen := len(pattern), len(text)

	// 优化：使用两行 DP 代替完整矩阵
	prev := make([]bool, tLen+1)
	curr := make([]bool, tLen+1)

	// 空模式匹配空文本
	prev[0] = true

	// 初始化：检查 pattern 前缀中的 * 能否匹配空文本
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
				// 字符匹配
				curr[j] = prev[j-1]
			default:
				curr[j] = false
			}
		}

		// 交换行
		prev, curr = curr, prev
	}

	return prev[tLen]
}

// Matcher 预编译的模式匹配器，用于多模式匹配
type Matcher struct {
	patterns []string
}

// NewMatcher 创建新的模式匹配器
// 模式按优先级排序：精确匹配 > 部分通配 > 全通配
func NewMatcher(patterns []string) *Matcher {
	return &Matcher{patterns: patterns}
}

// Match 返回第一个匹配的模式索引，如果无匹配返回 -1
func (m *Matcher) Match(text string) int {
	for i, pattern := range m.patterns {
		if Match(pattern, text) {
			return i
		}
	}
	return -1
}

// MatchAll 返回所有匹配的模式索引
func (m *Matcher) MatchAll(text string) []int {
	var matches []int
	for i, pattern := range m.patterns {
		if Match(pattern, text) {
			matches = append(matches, i)
		}
	}
	return matches
}
