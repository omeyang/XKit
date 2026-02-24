package xetcd

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DialTimeout != 5*time.Second {
		t.Errorf("DialTimeout = %v, want %v", cfg.DialTimeout, 5*time.Second)
	}
	if cfg.DialKeepAliveTime != 10*time.Second {
		t.Errorf("DialKeepAliveTime = %v, want %v", cfg.DialKeepAliveTime, 10*time.Second)
	}
	if cfg.DialKeepAliveTimeout != 3*time.Second {
		t.Errorf("DialKeepAliveTimeout = %v, want %v", cfg.DialKeepAliveTimeout, 3*time.Second)
	}
	if !cfg.RejectOldCluster {
		t.Error("RejectOldCluster should be true")
	}
	if !cfg.PermitWithoutStream {
		t.Error("PermitWithoutStream should be true")
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr error
	}{
		{
			name: "valid config",
			config: &Config{
				Endpoints: []string{"localhost:2379"},
			},
			wantErr: nil,
		},
		{
			name: "multiple endpoints",
			config: &Config{
				Endpoints: []string{"host1:2379", "host2:2379", "host3:2379"},
			},
			wantErr: nil,
		},
		{
			name:    "empty endpoints",
			config:  &Config{},
			wantErr: ErrNoEndpoints,
		},
		{
			name: "nil endpoints",
			config: &Config{
				Endpoints: nil,
			},
			wantErr: ErrNoEndpoints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestConfig_Validate_EndpointFormats 测试各种 endpoint 格式的验证行为。
func TestConfig_Validate_EndpointFormats(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  string
		wantError bool
	}{
		{"valid host:port", "localhost:2379", false},
		{"valid IP:port", "192.168.1.1:2379", false},
		{"valid IPv6", "[::1]:2379", false},
		{"valid with http scheme", "http://localhost:2379", false},
		{"valid with https scheme", "https://etcd.example.com:2379", false},
		{"empty endpoint", "", true},
		{"missing port", "localhost", true},
		{"empty port", "localhost:", true},
		{"invalid port non-numeric", "localhost:abc", true},
		{"port zero", "localhost:0", true},
		{"port too large", "localhost:65536", true},
		{"IPv6 no port bracket only", "[::1]", true},
		{"port negative", "localhost:-1", true},
		{"missing host", ":2379", true},
		{"missing host with scheme", "http://:2379", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Endpoints: []string{tt.endpoint}}
			err := cfg.Validate()
			if tt.wantError && err == nil {
				t.Error("Validate() should return error")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

// TestConfig_Validate_TrimSpaceEndpoints 测试 Validate 对 endpoint 进行 TrimSpace 归一化。
func TestConfig_Validate_TrimSpaceEndpoints(t *testing.T) {
	cfg := &Config{
		Endpoints: []string{"  localhost:2379  ", "\tetcd.example.com:2379\n"},
	}
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	// 验证 TrimSpace 后的值
	if cfg.Endpoints[0] != "localhost:2379" {
		t.Errorf("Endpoints[0] = %q, want %q", cfg.Endpoints[0], "localhost:2379")
	}
	if cfg.Endpoints[1] != "etcd.example.com:2379" {
		t.Errorf("Endpoints[1] = %q, want %q", cfg.Endpoints[1], "etcd.example.com:2379")
	}
}

// TestConfig_Validate_WhitespaceOnlyEndpoint 测试纯空白 endpoint 在 TrimSpace 后被检测为空。
func TestConfig_Validate_WhitespaceOnlyEndpoint(t *testing.T) {
	cfg := &Config{
		Endpoints: []string{"  \t  "},
	}
	err := cfg.Validate()
	if !errors.Is(err, ErrInvalidEndpoint) {
		t.Errorf("Validate() error = %v, want ErrInvalidEndpoint", err)
	}
}

func TestConfig_Validate_NegativeDuration(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{"negative DialTimeout", &Config{Endpoints: []string{"localhost:2379"}, DialTimeout: -1 * time.Second}},
		{"negative DialKeepAliveTime", &Config{Endpoints: []string{"localhost:2379"}, DialKeepAliveTime: -1 * time.Second}},
		{"negative DialKeepAliveTimeout", &Config{Endpoints: []string{"localhost:2379"}, DialKeepAliveTimeout: -1 * time.Second}},
		{"negative AutoSyncInterval", &Config{Endpoints: []string{"localhost:2379"}, AutoSyncInterval: -1 * time.Second}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err == nil {
				t.Error("Validate() should return error for negative duration")
			}
			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("Validate() error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestConfig_applyDefaults(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		check  func(t *testing.T, cfg *Config)
	}{
		{
			name: "apply all defaults",
			config: &Config{
				Endpoints: []string{"localhost:2379"},
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.DialTimeout != defaultDialTimeout {
					t.Errorf("DialTimeout = %v, want %v", cfg.DialTimeout, defaultDialTimeout)
				}
				if cfg.DialKeepAliveTime != defaultDialKeepAliveTime {
					t.Errorf("DialKeepAliveTime = %v, want %v", cfg.DialKeepAliveTime, defaultDialKeepAliveTime)
				}
				if cfg.DialKeepAliveTimeout != defaultDialKeepAliveTimeout {
					t.Errorf("DialKeepAliveTimeout = %v, want %v", cfg.DialKeepAliveTimeout, defaultDialKeepAliveTimeout)
				}
			},
		},
		{
			name: "preserve custom values",
			config: &Config{
				Endpoints:            []string{"localhost:2379"},
				DialTimeout:          20 * time.Second,
				DialKeepAliveTime:    30 * time.Second,
				DialKeepAliveTimeout: 10 * time.Second,
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.DialTimeout != 20*time.Second {
					t.Errorf("DialTimeout = %v, want %v", cfg.DialTimeout, 20*time.Second)
				}
				if cfg.DialKeepAliveTime != 30*time.Second {
					t.Errorf("DialKeepAliveTime = %v, want %v", cfg.DialKeepAliveTime, 30*time.Second)
				}
				if cfg.DialKeepAliveTimeout != 10*time.Second {
					t.Errorf("DialKeepAliveTimeout = %v, want %v", cfg.DialKeepAliveTimeout, 10*time.Second)
				}
			},
		},
		{
			name: "does not modify original",
			config: &Config{
				Endpoints: []string{"localhost:2379"},
			},
			check: func(t *testing.T, cfg *Config) {
				// 原配置应该不变
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := *tt.config
			result := tt.config.applyDefaults()

			// 检查结果
			tt.check(t, result)

			// 确保原配置未被修改
			if tt.config.DialTimeout != original.DialTimeout {
				t.Error("original config was modified")
			}
		})
	}
}

func TestConfig_String(t *testing.T) {
	tests := []struct {
		name         string
		config       *Config
		wantContains string
		wantExcludes string
	}{
		{
			name: "password masked",
			config: &Config{
				Endpoints: []string{"localhost:2379"},
				Username:  "admin",
				Password:  "secret123",
			},
			wantContains: "***",
			wantExcludes: "secret123",
		},
		{
			name: "empty password not masked",
			config: &Config{
				Endpoints: []string{"localhost:2379"},
			},
			wantContains: "localhost:2379",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.config.String()
			if tt.wantContains != "" && !strings.Contains(s, tt.wantContains) {
				t.Errorf("String() = %q, should contain %q", s, tt.wantContains)
			}
			if tt.wantExcludes != "" && strings.Contains(s, tt.wantExcludes) {
				t.Errorf("String() = %q, should not contain %q", s, tt.wantExcludes)
			}
		})
	}
}

func TestConfig_String_AllFields(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Endpoints = []string{"localhost:2379"}
	cfg.Username = "admin"
	cfg.Password = "secret"
	cfg.AutoSyncInterval = 5 * time.Second

	s := cfg.String()

	requiredFields := []string{
		"DialKeepAliveTime:",
		"DialKeepAliveTimeout:",
		"AutoSyncInterval:",
		"RejectOldCluster:",
		"PermitWithoutStream:",
	}
	for _, field := range requiredFields {
		if !strings.Contains(s, field) {
			t.Errorf("String() = %q, should contain %q", s, field)
		}
	}
}
