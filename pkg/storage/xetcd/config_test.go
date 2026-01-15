package xetcd

import (
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
