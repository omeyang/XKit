package xlimit

import (
	"testing"
	"time"
)

func TestRuleMatcher_FindRule(t *testing.T) {
	rules := []Rule{
		TenantRule("tenant-limit", 1000, time.Minute),
		GlobalRule("global-limit", 10000, time.Minute),
	}

	matcher := newRuleMatcher(rules)

	t.Run("finds matching rule", func(t *testing.T) {
		rule, found := matcher.findRule("tenant-limit")
		if !found {
			t.Fatal("expected to find rule")
		}
		if rule.Name != "tenant-limit" {
			t.Errorf("expected rule name tenant-limit, got %s", rule.Name)
		}
	})

	t.Run("returns not found for missing rule", func(t *testing.T) {
		_, found := matcher.findRule("non-existent")
		if found {
			t.Error("expected rule not found")
		}
	})
}

func TestRuleMatcher_GetLimit(t *testing.T) {
	rules := []Rule{
		{
			Name:        "tenant-limit",
			KeyTemplate: "tenant:${tenant_id}",
			Limit:       1000,
			Window:      time.Minute,
			Overrides: []Override{
				{Match: "tenant:vip-corp", Limit: 5000},
				{Match: "tenant:vip-*", Limit: 3000},
			},
		},
	}

	matcher := newRuleMatcher(rules)

	tests := []struct {
		name      string
		key       Key
		wantLimit int
	}{
		{
			name:      "default limit",
			key:       Key{Tenant: "normal-user"},
			wantLimit: 1000,
		},
		{
			name:      "exact override match",
			key:       Key{Tenant: "vip-corp"},
			wantLimit: 5000,
		},
		{
			name:      "wildcard override match",
			key:       Key{Tenant: "vip-enterprise"},
			wantLimit: 3000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, found := matcher.findRule("tenant-limit")
			if !found {
				t.Fatal("rule not found")
			}
			limit, _ := matcher.getEffectiveLimit(rule, tt.key)
			if limit != tt.wantLimit {
				t.Errorf("GetLimit() = %d, want %d", limit, tt.wantLimit)
			}
		})
	}
}

func TestRuleMatcher_OverridePriority(t *testing.T) {
	rules := []Rule{
		{
			Name:        "api-limit",
			KeyTemplate: "tenant:${tenant_id}:api:${method}:${path}",
			Limit:       100,
			Window:      time.Second,
			Overrides: []Override{
				{Match: "tenant:vip-corp:api:POST:/v1/orders", Limit: 500}, // 精确匹配
				{Match: "tenant:vip-corp:api:POST:*", Limit: 300},          // 部分通配
				{Match: "tenant:vip-corp:api:*:*", Limit: 200},             // 更多通配
				{Match: "tenant:*:api:POST:/v1/orders", Limit: 150},        // 租户通配
			},
		},
	}

	matcher := newRuleMatcher(rules)

	tests := []struct {
		name      string
		key       Key
		wantLimit int
	}{
		{
			name: "exact match priority",
			key: Key{
				Tenant: "vip-corp",
				Method: "POST",
				Path:   "/v1/orders",
			},
			wantLimit: 500,
		},
		{
			name: "partial wildcard priority",
			key: Key{
				Tenant: "vip-corp",
				Method: "POST",
				Path:   "/v1/users",
			},
			wantLimit: 300,
		},
		{
			name: "more wildcard priority",
			key: Key{
				Tenant: "vip-corp",
				Method: "GET",
				Path:   "/v1/items",
			},
			wantLimit: 200,
		},
		{
			name: "tenant wildcard priority",
			key: Key{
				Tenant: "normal-user",
				Method: "POST",
				Path:   "/v1/orders",
			},
			wantLimit: 150,
		},
		{
			name: "default when no override matches",
			key: Key{
				Tenant: "normal-user",
				Method: "GET",
				Path:   "/v1/items",
			},
			wantLimit: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, found := matcher.findRule("api-limit")
			if !found {
				t.Fatal("rule not found")
			}
			limit, _ := matcher.getEffectiveLimit(rule, tt.key)
			if limit != tt.wantLimit {
				t.Errorf("GetLimit() = %d, want %d", limit, tt.wantLimit)
			}
		})
	}
}

func TestRuleMatcher_DisabledRule(t *testing.T) {
	disabled := false
	rules := []Rule{
		{
			Name:        "disabled-rule",
			KeyTemplate: "tenant:${tenant_id}",
			Limit:       1000,
			Window:      time.Minute,
			Enabled:     &disabled,
		},
		TenantRule("enabled-rule", 500, time.Minute),
	}

	matcher := newRuleMatcher(rules)

	t.Run("disabled rule is not found", func(t *testing.T) {
		_, found := matcher.findRule("disabled-rule")
		if found {
			t.Error("disabled rule should not be found")
		}
	})

	t.Run("enabled rule is found", func(t *testing.T) {
		_, found := matcher.findRule("enabled-rule")
		if !found {
			t.Error("enabled rule should be found")
		}
	})
}

func TestRuleMatcher_RenderKey(t *testing.T) {
	rules := []Rule{
		TenantRule("tenant-limit", 1000, time.Minute),
	}

	matcher := newRuleMatcher(rules)

	key := Key{Tenant: "abc123"}
	rule, found := matcher.findRule("tenant-limit")
	if !found {
		t.Fatal("rule not found")
	}

	renderedKey := matcher.renderKey(rule, key, "ratelimit:")
	expected := "ratelimit:tenant:abc123"
	if renderedKey != expected {
		t.Errorf("RenderKey() = %q, want %q", renderedKey, expected)
	}
}

func TestRuleMatcher_EmptyRules(t *testing.T) {
	matcher := newRuleMatcher(nil)

	_, found := matcher.findRule("any")
	if found {
		t.Error("expected no rules found")
	}
}

func TestRuleMatcher_HasRule(t *testing.T) {
	rules := []Rule{
		TenantRule("tenant-limit", 1000, time.Minute),
		GlobalRule("global-limit", 10000, time.Minute),
	}

	matcher := newRuleMatcher(rules)

	t.Run("returns true for existing rule", func(t *testing.T) {
		if !matcher.hasRule("tenant-limit") {
			t.Error("expected HasRule to return true for existing rule")
		}
	})

	t.Run("returns true for another existing rule", func(t *testing.T) {
		if !matcher.hasRule("global-limit") {
			t.Error("expected HasRule to return true for existing rule")
		}
	})

	t.Run("returns false for non-existing rule", func(t *testing.T) {
		if matcher.hasRule("non-existent") {
			t.Error("expected HasRule to return false for non-existing rule")
		}
	})

	t.Run("returns false for empty name", func(t *testing.T) {
		if matcher.hasRule("") {
			t.Error("expected HasRule to return false for empty name")
		}
	})
}

func TestRuleMatcher_GetAllRules(t *testing.T) {
	disabled := false
	rules := []Rule{
		TenantRule("tenant-limit", 1000, time.Minute),
		GlobalRule("global-limit", 10000, time.Minute),
		{
			Name:        "disabled-rule",
			KeyTemplate: "disabled:${tenant_id}",
			Limit:       100,
			Window:      time.Minute,
			Enabled:     &disabled,
		},
	}

	matcher := newRuleMatcher(rules)
	allRules := matcher.getAllRules()

	// 应该只返回启用的规则
	if len(allRules) != 2 {
		t.Errorf("expected 2 enabled rules, got %d", len(allRules))
	}

	// 检查不包含禁用的规则
	for _, name := range allRules {
		if name == "disabled-rule" {
			t.Error("disabled rule should not be in GetAllRules result")
		}
	}
}

func TestRuleMatcher_GetEffectiveBurst(t *testing.T) {
	rules := []Rule{
		{
			Name:        "tenant-limit",
			KeyTemplate: "tenant:${tenant_id}",
			Limit:       1000,
			Burst:       1500,
			Window:      time.Minute,
			Overrides: []Override{
				{Match: "tenant:vip-corp", Limit: 5000, Burst: 7500},
				{Match: "tenant:premium-*", Limit: 3000}, // 无 Burst，应该使用 Limit
			},
		},
	}

	matcher := newRuleMatcher(rules)

	tests := []struct {
		name      string
		key       Key
		wantBurst int
	}{
		{
			name:      "default burst",
			key:       Key{Tenant: "normal-user"},
			wantBurst: 1500,
		},
		{
			name:      "override with explicit burst",
			key:       Key{Tenant: "vip-corp"},
			wantBurst: 7500,
		},
		{
			name:      "override without burst uses limit",
			key:       Key{Tenant: "premium-enterprise"},
			wantBurst: 3000, // 使用 Override 的 Limit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, found := matcher.findRule("tenant-limit")
			if !found {
				t.Fatal("rule not found")
			}
			burst := matcher.getEffectiveBurst(rule, tt.key)
			if burst != tt.wantBurst {
				t.Errorf("GetEffectiveBurst() = %d, want %d", burst, tt.wantBurst)
			}
		})
	}
}

func TestRuleMatcher_GetEffectiveLimit_WithOverrideWindow(t *testing.T) {
	rules := []Rule{
		{
			Name:        "tenant-limit",
			KeyTemplate: "tenant:${tenant_id}",
			Limit:       1000,
			Window:      time.Minute,
			Overrides: []Override{
				{Match: "tenant:vip-corp", Limit: 5000, Window: 30 * time.Second},
				{Match: "tenant:premium-*", Limit: 3000}, // 无 Window
			},
		},
	}

	matcher := newRuleMatcher(rules)

	t.Run("override with explicit window", func(t *testing.T) {
		key := Key{Tenant: "vip-corp"}
		rule, found := matcher.findRule("tenant-limit")
		if !found {
			t.Fatal("rule not found")
		}
		_, window := matcher.getEffectiveLimit(rule, key)
		if window != 30*time.Second {
			t.Errorf("GetEffectiveLimit() window = %v, want %v", window, 30*time.Second)
		}
	})

	t.Run("override without window uses rule default", func(t *testing.T) {
		key := Key{Tenant: "premium-user"}
		rule, found := matcher.findRule("tenant-limit")
		if !found {
			t.Fatal("rule not found")
		}
		_, window := matcher.getEffectiveLimit(rule, key)
		if window != time.Minute {
			t.Errorf("GetEffectiveLimit() window = %v, want %v", window, time.Minute)
		}
	})
}

func BenchmarkRuleMatcher_FindRule(b *testing.B) {
	rules := []Rule{
		GlobalRule("global", 10000, time.Minute),
		TenantRule("tenant", 1000, time.Minute),
		TenantAPIRule("tenant-api", 100, time.Second),
		CallerRule("caller", 500, time.Minute),
	}

	matcher := newRuleMatcher(rules)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = matcher.findRule("tenant")
	}
}

func BenchmarkRuleMatcher_GetEffectiveLimit(b *testing.B) {
	rules := []Rule{
		{
			Name:        "tenant-limit",
			KeyTemplate: "tenant:${tenant_id}",
			Limit:       1000,
			Window:      time.Minute,
			Overrides: []Override{
				{Match: "tenant:vip-corp", Limit: 5000},
				{Match: "tenant:vip-*", Limit: 3000},
				{Match: "tenant:premium-*", Limit: 2000},
			},
		},
	}

	matcher := newRuleMatcher(rules)
	key := Key{Tenant: "vip-enterprise"}
	rule, _ := matcher.findRule("tenant-limit")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = matcher.getEffectiveLimit(rule, key)
	}
}
