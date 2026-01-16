package xlimit

import (
	"testing"
	"time"
)

func TestRule_Validate(t *testing.T) {
	tests := []struct {
		name    string
		rule    Rule
		wantErr bool
	}{
		{
			name: "valid rule",
			rule: Rule{
				Name:        "tenant-limit",
				KeyTemplate: "tenant:${tenant_id}",
				Limit:       1000,
				Window:      time.Minute,
			},
			wantErr: false,
		},
		{
			name: "valid rule with burst",
			rule: Rule{
				Name:        "api-limit",
				KeyTemplate: "api:${method}:${path}",
				Limit:       100,
				Window:      time.Second,
				Burst:       150,
			},
			wantErr: false,
		},
		{
			name: "empty name",
			rule: Rule{
				Name:        "",
				KeyTemplate: "tenant:${tenant_id}",
				Limit:       100,
				Window:      time.Second,
			},
			wantErr: true,
		},
		{
			name: "empty key template",
			rule: Rule{
				Name:        "test",
				KeyTemplate: "",
				Limit:       100,
				Window:      time.Second,
			},
			wantErr: true,
		},
		{
			name: "zero limit",
			rule: Rule{
				Name:        "test",
				KeyTemplate: "tenant:${tenant_id}",
				Limit:       0,
				Window:      time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative limit",
			rule: Rule{
				Name:        "test",
				KeyTemplate: "tenant:${tenant_id}",
				Limit:       -1,
				Window:      time.Second,
			},
			wantErr: true,
		},
		{
			name: "zero window",
			rule: Rule{
				Name:        "test",
				KeyTemplate: "tenant:${tenant_id}",
				Limit:       100,
				Window:      0,
			},
			wantErr: true,
		},
		{
			name: "negative window",
			rule: Rule{
				Name:        "test",
				KeyTemplate: "tenant:${tenant_id}",
				Limit:       100,
				Window:      -time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative burst",
			rule: Rule{
				Name:        "test",
				KeyTemplate: "tenant:${tenant_id}",
				Limit:       100,
				Window:      time.Second,
				Burst:       -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Rule.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRule_IsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled *bool
		want    bool
	}{
		{
			name:    "nil means enabled",
			enabled: nil,
			want:    true,
		},
		{
			name:    "explicit true",
			enabled: boolPtr(true),
			want:    true,
		},
		{
			name:    "explicit false",
			enabled: boolPtr(false),
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := Rule{Enabled: tt.enabled}
			if got := rule.IsEnabled(); got != tt.want {
				t.Errorf("Rule.IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRule_EffectiveBurst(t *testing.T) {
	tests := []struct {
		name  string
		rule  Rule
		want  int
	}{
		{
			name: "explicit burst",
			rule: Rule{
				Limit: 100,
				Burst: 150,
			},
			want: 150,
		},
		{
			name: "zero burst defaults to limit",
			rule: Rule{
				Limit: 100,
				Burst: 0,
			},
			want: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rule.EffectiveBurst(); got != tt.want {
				t.Errorf("Rule.EffectiveBurst() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOverride_Validate(t *testing.T) {
	tests := []struct {
		name     string
		override Override
		wantErr  bool
	}{
		{
			name: "valid override",
			override: Override{
				Match: "tenant:vip-corp",
				Limit: 5000,
			},
			wantErr: false,
		},
		{
			name: "valid override with window",
			override: Override{
				Match:  "tenant:*",
				Limit:  2000,
				Window: time.Minute,
			},
			wantErr: false,
		},
		{
			name: "empty match",
			override: Override{
				Match: "",
				Limit: 100,
			},
			wantErr: true,
		},
		{
			name: "zero limit",
			override: Override{
				Match: "tenant:*",
				Limit: 0,
			},
			wantErr: true,
		},
		{
			name: "negative limit",
			override: Override{
				Match: "tenant:*",
				Limit: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.override.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Override.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewRule(t *testing.T) {
	rule := NewRule("tenant-limit", "tenant:${tenant_id}", 1000, time.Minute)

	if rule.Name != "tenant-limit" {
		t.Errorf("Name = %q, want %q", rule.Name, "tenant-limit")
	}
	if rule.KeyTemplate != "tenant:${tenant_id}" {
		t.Errorf("KeyTemplate = %q, want %q", rule.KeyTemplate, "tenant:${tenant_id}")
	}
	if rule.Limit != 1000 {
		t.Errorf("Limit = %d, want %d", rule.Limit, 1000)
	}
	if rule.Window != time.Minute {
		t.Errorf("Window = %v, want %v", rule.Window, time.Minute)
	}
}

func TestRuleBuilder(t *testing.T) {
	rule := NewRuleBuilder("api-limit").
		KeyTemplate("api:${method}:${path}").
		Limit(100).
		Window(time.Second).
		Burst(150).
		AddOverride("api:POST:*", 50).
		AddOverrideWithWindow("api:GET:/health", 1000, time.Second).
		Enabled(true).
		Build()

	if rule.Name != "api-limit" {
		t.Errorf("Name = %q, want %q", rule.Name, "api-limit")
	}
	if rule.KeyTemplate != "api:${method}:${path}" {
		t.Errorf("KeyTemplate = %q, want %q", rule.KeyTemplate, "api:${method}:${path}")
	}
	if rule.Limit != 100 {
		t.Errorf("Limit = %d, want %d", rule.Limit, 100)
	}
	if rule.Window != time.Second {
		t.Errorf("Window = %v, want %v", rule.Window, time.Second)
	}
	if rule.Burst != 150 {
		t.Errorf("Burst = %d, want %d", rule.Burst, 150)
	}
	if len(rule.Overrides) != 2 {
		t.Errorf("Overrides len = %d, want %d", len(rule.Overrides), 2)
	}
	if rule.Overrides[0].Match != "api:POST:*" {
		t.Errorf("Overrides[0].Match = %q, want %q", rule.Overrides[0].Match, "api:POST:*")
	}
	if rule.Overrides[0].Limit != 50 {
		t.Errorf("Overrides[0].Limit = %d, want %d", rule.Overrides[0].Limit, 50)
	}
	if !rule.IsEnabled() {
		t.Error("expected rule to be enabled")
	}
}

func TestTenantRule(t *testing.T) {
	rule := TenantRule("tenant-limit", 1000, time.Minute)

	if rule.Name != "tenant-limit" {
		t.Errorf("Name = %q, want %q", rule.Name, "tenant-limit")
	}
	if rule.KeyTemplate != "tenant:${tenant_id}" {
		t.Errorf("KeyTemplate = %q, want %q", rule.KeyTemplate, "tenant:${tenant_id}")
	}
	if rule.Limit != 1000 {
		t.Errorf("Limit = %d, want %d", rule.Limit, 1000)
	}
	if rule.Window != time.Minute {
		t.Errorf("Window = %v, want %v", rule.Window, time.Minute)
	}
}

func TestGlobalRule(t *testing.T) {
	rule := GlobalRule("global-limit", 10000, time.Minute)

	if rule.Name != "global-limit" {
		t.Errorf("Name = %q, want %q", rule.Name, "global-limit")
	}
	if rule.KeyTemplate != "global" {
		t.Errorf("KeyTemplate = %q, want %q", rule.KeyTemplate, "global")
	}
	if rule.Limit != 10000 {
		t.Errorf("Limit = %d, want %d", rule.Limit, 10000)
	}
}

func TestTenantAPIRule(t *testing.T) {
	rule := TenantAPIRule("tenant-api-limit", 100, time.Second)

	if rule.Name != "tenant-api-limit" {
		t.Errorf("Name = %q, want %q", rule.Name, "tenant-api-limit")
	}
	if rule.KeyTemplate != "tenant:${tenant_id}:api:${method}:${path}" {
		t.Errorf("KeyTemplate = %q, want %q", rule.KeyTemplate, "tenant:${tenant_id}:api:${method}:${path}")
	}
	if rule.Limit != 100 {
		t.Errorf("Limit = %d, want %d", rule.Limit, 100)
	}
}

func TestCallerRule(t *testing.T) {
	rule := CallerRule("caller-limit", 500, time.Minute)

	if rule.Name != "caller-limit" {
		t.Errorf("Name = %q, want %q", rule.Name, "caller-limit")
	}
	if rule.KeyTemplate != "caller:${caller_id}" {
		t.Errorf("KeyTemplate = %q, want %q", rule.KeyTemplate, "caller:${caller_id}")
	}
	if rule.Limit != 500 {
		t.Errorf("Limit = %d, want %d", rule.Limit, 500)
	}
}

func boolPtr(b bool) *bool {
	return &b
}
