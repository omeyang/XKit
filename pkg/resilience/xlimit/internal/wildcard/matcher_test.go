package wildcard

import (
	"testing"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		text    string
		want    bool
	}{
		// 精确匹配
		{
			name:    "exact match",
			pattern: "tenant:abc123",
			text:    "tenant:abc123",
			want:    true,
		},
		{
			name:    "exact match failure",
			pattern: "tenant:abc123",
			text:    "tenant:xyz789",
			want:    false,
		},

		// 单个通配符
		{
			name:    "single wildcard at end",
			pattern: "tenant:*",
			text:    "tenant:abc123",
			want:    true,
		},
		{
			name:    "single wildcard in middle",
			pattern: "tenant:*:api",
			text:    "tenant:abc123:api",
			want:    true,
		},
		{
			name:    "single wildcard at start",
			pattern: "*:api:POST",
			text:    "tenant:api:POST",
			want:    true,
		},

		// 多个通配符
		{
			name:    "multiple wildcards",
			pattern: "tenant:*:api:*",
			text:    "tenant:abc:api:POST",
			want:    true,
		},
		{
			name:    "multiple wildcards failure",
			pattern: "tenant:*:api:*",
			text:    "tenant:abc:other:POST",
			want:    false,
		},

		// 全通配符
		{
			name:    "all wildcard",
			pattern: "*",
			text:    "anything",
			want:    true,
		},
		{
			name:    "all wildcard empty text",
			pattern: "*",
			text:    "",
			want:    true,
		},

		// 边界情况
		{
			name:    "empty pattern matches empty text",
			pattern: "",
			text:    "",
			want:    true,
		},
		{
			name:    "empty pattern does not match non-empty text",
			pattern: "",
			text:    "abc",
			want:    false,
		},
		{
			name:    "non-empty pattern does not match empty text",
			pattern: "abc",
			text:    "",
			want:    false,
		},
		{
			name:    "wildcard only matches empty text",
			pattern: "*",
			text:    "",
			want:    true,
		},

		// 复杂模式
		{
			name:    "complex pattern match",
			pattern: "tenant:vip-*:api:POST:/v1/*",
			text:    "tenant:vip-corp:api:POST:/v1/users",
			want:    true,
		},
		{
			name:    "complex pattern no match",
			pattern: "tenant:vip-*:api:POST:/v1/*",
			text:    "tenant:normal:api:POST:/v1/users",
			want:    false,
		},

		// 连续通配符
		{
			name:    "double wildcard",
			pattern: "a**b",
			text:    "axyzb",
			want:    true,
		},

		// 特殊字符
		{
			name:    "colon in text",
			pattern: "tenant:*",
			text:    "tenant:abc:def",
			want:    true,
		},
		{
			name:    "slash in text",
			pattern: "api:*",
			text:    "api:/v1/users",
			want:    true,
		},

		// 部分匹配失败
		{
			name:    "prefix only pattern",
			pattern: "tenant:",
			text:    "tenant:abc",
			want:    false,
		},
		{
			name:    "suffix only pattern",
			pattern: ":abc",
			text:    "tenant:abc",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Match(tt.pattern, tt.text)
			if got != tt.want {
				t.Errorf("Match(%q, %q) = %v, want %v", tt.pattern, tt.text, got, tt.want)
			}
		})
	}
}

func TestMatcher_Match(t *testing.T) {
	// 测试编译后的匹配器
	patterns := []string{
		"tenant:vip-corp",          // 精确匹配
		"tenant:vip-*",             // 前缀匹配
		"tenant:*:api:POST:*",      // 复杂模式
		"tenant:*",                 // 租户通配
	}

	m := NewMatcher(patterns)

	tests := []struct {
		text  string
		index int // 期望匹配的模式索引，-1 表示无匹配
	}{
		{"tenant:vip-corp", 0},           // 精确匹配优先
		{"tenant:vip-enterprise", 1},     // 前缀匹配
		{"tenant:normal:api:POST:/v1", 2},// 复杂模式
		{"tenant:other", 3},              // 租户通配
		{"other:abc", -1},                // 无匹配
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := m.Match(tt.text)
			if got != tt.index {
				t.Errorf("Matcher.Match(%q) = %d, want %d", tt.text, got, tt.index)
			}
		})
	}
}

func TestMatcher_Empty(t *testing.T) {
	m := NewMatcher(nil)
	if m.Match("anything") != -1 {
		t.Error("empty matcher should return -1")
	}
}

func BenchmarkMatch_Exact(b *testing.B) {
	pattern := "tenant:abc123:api:POST:/v1/users"
	text := "tenant:abc123:api:POST:/v1/users"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		Match(pattern, text)
	}
}

func BenchmarkMatch_Wildcard(b *testing.B) {
	pattern := "tenant:*:api:*:*"
	text := "tenant:abc123:api:POST:/v1/users"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		Match(pattern, text)
	}
}

func BenchmarkMatcher_Match(b *testing.B) {
	patterns := []string{
		"tenant:vip-corp",
		"tenant:vip-*",
		"tenant:*:api:POST:*",
		"tenant:*",
	}

	m := NewMatcher(patterns)
	text := "tenant:normal:api:POST:/v1/users"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.Match(text)
	}
}

func FuzzMatch(f *testing.F) {
	f.Add("tenant:*", "tenant:abc")
	f.Add("*:api:*", "tenant:api:POST")
	f.Add("a*b*c", "axbxc")
	f.Add("", "")
	f.Add("*", "anything")

	f.Fuzz(func(t *testing.T, pattern, text string) {
		// 验证不会 panic
		_ = Match(pattern, text)
	})
}
