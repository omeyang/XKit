package xlimit

import (
	"testing"
	"time"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				KeyPrefix: "ratelimit:",
				Rules: []Rule{
					TenantRule("tenant-limit", 1000, time.Minute),
				},
				Fallback:      FallbackLocal,
				LocalPodCount: 3,
			},
			wantErr: false,
		},
		{
			name: "empty rules is valid",
			config: Config{
				KeyPrefix: "ratelimit:",
				Rules:     []Rule{},
			},
			wantErr: false,
		},
		{
			name: "default fallback is valid",
			config: Config{
				KeyPrefix: "ratelimit:",
				Fallback:  "",
			},
			wantErr: false,
		},
		{
			name: "invalid rule",
			config: Config{
				Rules: []Rule{
					{Name: ""},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid fallback strategy",
			config: Config{
				Fallback: FallbackStrategy("invalid"),
			},
			wantErr: true,
		},
		{
			name: "negative pod count",
			config: Config{
				Fallback:      FallbackLocal,
				LocalPodCount: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.KeyPrefix != "ratelimit:" {
		t.Errorf("KeyPrefix = %q, want %q", config.KeyPrefix, "ratelimit:")
	}

	if config.Fallback != FallbackLocal {
		t.Errorf("Fallback = %q, want %q", config.Fallback, FallbackLocal)
	}

	if config.LocalPodCount != 1 {
		t.Errorf("LocalPodCount = %d, want %d", config.LocalPodCount, 1)
	}

	if !config.EnableMetrics {
		t.Error("EnableMetrics should be true by default")
	}

	if !config.EnableHeaders {
		t.Error("EnableHeaders should be true by default")
	}
}

func TestFallbackStrategy_IsValid(t *testing.T) {
	tests := []struct {
		strategy FallbackStrategy
		want     bool
	}{
		{FallbackLocal, true},
		{FallbackOpen, true},
		{FallbackClose, true},
		{FallbackStrategy(""), true}, // 空表示禁用降级
		{FallbackStrategy("invalid"), false},
		{FallbackStrategy("LOCAL"), false}, // 区分大小写
	}

	for _, tt := range tests {
		t.Run(string(tt.strategy), func(t *testing.T) {
			if got := tt.strategy.IsValid(); got != tt.want {
				t.Errorf("FallbackStrategy(%q).IsValid() = %v, want %v", tt.strategy, got, tt.want)
			}
		})
	}
}

func TestConfig_EffectivePodCount(t *testing.T) {
	tests := []struct {
		name  string
		count int
		want  int
	}{
		{
			name:  "explicit count",
			count: 3,
			want:  3,
		},
		{
			name:  "zero defaults to 1",
			count: 0,
			want:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{LocalPodCount: tt.count}
			if got := config.EffectivePodCount(); got != tt.want {
				t.Errorf("Config.EffectivePodCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestConfig_Clone(t *testing.T) {
	original := Config{
		KeyPrefix: "ratelimit:",
		Rules: []Rule{
			TenantRule("tenant-limit", 1000, time.Minute),
		},
		Fallback:      FallbackLocal,
		LocalPodCount: 3,
		EnableMetrics: true,
		EnableHeaders: true,
	}

	clone := original.Clone()

	// 验证值相等
	if clone.KeyPrefix != original.KeyPrefix {
		t.Errorf("KeyPrefix mismatch")
	}
	if clone.Fallback != original.Fallback {
		t.Errorf("Fallback mismatch")
	}
	if clone.LocalPodCount != original.LocalPodCount {
		t.Errorf("LocalPodCount mismatch")
	}

	// 验证规则列表是深拷贝
	if len(clone.Rules) != len(original.Rules) {
		t.Errorf("Rules length mismatch")
	}

	// 修改克隆不影响原始
	clone.KeyPrefix = "modified:"
	clone.Rules = nil
	if original.KeyPrefix == "modified:" {
		t.Error("Clone should not affect original KeyPrefix")
	}
	if len(original.Rules) == 0 {
		t.Error("Clone should not affect original Rules")
	}
}
