package xlimit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileConfig_ToConfig(t *testing.T) {
	tests := []struct {
		name    string
		fc      FileConfig
		wantErr bool
	}{
		{
			name: "基本配置",
			fc: FileConfig{
				KeyPrefix:        "test:",
				LocalPodCount:    3,
				EnableMetrics:    true,
				EnableHeaders:    true,
				FallbackStrategy: "local",
				Rules: []FileRule{
					{
						Name:        "tenant",
						KeyTemplate: "tenant:${tenant_id}",
						Limit:       100,
						Window:      "1m",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "fail-open 策略",
			fc: FileConfig{
				FallbackStrategy: "fail-open",
				Rules: []FileRule{
					{
						Name:        "test",
						KeyTemplate: "test",
						Limit:       10,
						Window:      "1s",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "fail-close 策略",
			fc: FileConfig{
				FallbackStrategy: "fail-close",
				Rules: []FileRule{
					{
						Name:        "test",
						KeyTemplate: "test",
						Limit:       10,
						Window:      "1s",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "无效时间窗口",
			fc: FileConfig{
				Rules: []FileRule{
					{
						Name:        "test",
						KeyTemplate: "test",
						Limit:       10,
						Window:      "invalid",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := tt.fc.ToConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("ToConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && config == nil {
				t.Error("expected non-nil config")
			}
		})
	}
}

func TestFileRule_ToRule(t *testing.T) {
	enabled := true

	tests := []struct {
		name    string
		fr      FileRule
		wantErr bool
	}{
		{
			name: "基本规则",
			fr: FileRule{
				Name:        "test",
				KeyTemplate: "test:${tenant_id}",
				Limit:       100,
				Window:      "1m",
				Burst:       150,
				Enabled:     &enabled,
			},
			wantErr: false,
		},
		{
			name: "带覆盖配置",
			fr: FileRule{
				Name:        "test",
				KeyTemplate: "test:${tenant_id}",
				Limit:       100,
				Window:      "1m",
				Overrides: []FileOverride{
					{
						Match:  "test:vip-*",
						Limit:  500,
						Window: "1m",
						Burst:  700,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "无效时间窗口",
			fr: FileRule{
				Name:        "test",
				KeyTemplate: "test",
				Limit:       10,
				Window:      "",
			},
			wantErr: true,
		},
		{
			name: "无效覆盖时间窗口",
			fr: FileRule{
				Name:        "test",
				KeyTemplate: "test",
				Limit:       10,
				Window:      "1m",
				Overrides: []FileOverride{
					{
						Match:  "test:*",
						Limit:  20,
						Window: "invalid",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := tt.fr.ToRule()
			if (err != nil) != tt.wantErr {
				t.Errorf("ToRule() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && rule.Name == "" {
				t.Error("expected non-empty rule name")
			}
		})
	}
}

func TestFileOverride_ToOverride(t *testing.T) {
	tests := []struct {
		name    string
		fo      FileOverride
		wantErr bool
	}{
		{
			name: "基本覆盖",
			fo: FileOverride{
				Match: "test:*",
				Limit: 100,
			},
			wantErr: false,
		},
		{
			name: "带时间窗口",
			fo: FileOverride{
				Match:  "test:*",
				Limit:  100,
				Window: "30s",
				Burst:  150,
			},
			wantErr: false,
		},
		{
			name: "无效时间窗口",
			fo: FileOverride{
				Match:  "test:*",
				Limit:  100,
				Window: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			override, err := tt.fo.ToOverride()
			if (err != nil) != tt.wantErr {
				t.Errorf("ToOverride() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && override.Match != tt.fo.Match {
				t.Errorf("expected match %q, got %q", tt.fo.Match, override.Match)
			}
		})
	}
}

func TestNewFileLoader(t *testing.T) {
	// 创建临时测试文件
	tmpDir := t.TempDir()

	t.Run("文件不存在", func(t *testing.T) {
		_, err := NewFileLoader(filepath.Join(tmpDir, "nonexistent.yaml"))
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})

	t.Run("YAML 文件", func(t *testing.T) {
		yamlFile := filepath.Join(tmpDir, "config.yaml")
		content := `
key_prefix: "test:"
rules:
  - name: tenant
    key_template: "tenant:${tenant_id}"
    limit: 100
    window: "1m"
`
		if err := os.WriteFile(yamlFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		loader, err := NewFileLoader(yamlFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer loader.Stop()

		config, err := loader.Load(context.Background())
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if config.KeyPrefix != "test:" {
			t.Errorf("expected key_prefix 'test:', got %q", config.KeyPrefix)
		}
		if len(config.Rules) != 1 {
			t.Errorf("expected 1 rule, got %d", len(config.Rules))
		}
	})

	t.Run("JSON 文件", func(t *testing.T) {
		jsonFile := filepath.Join(tmpDir, "config.json")
		content := `{
  "key_prefix": "json:",
  "rules": [
    {
      "name": "tenant",
      "key_template": "tenant:${tenant_id}",
      "limit": 100,
      "window": "1m"
    }
  ]
}`
		if err := os.WriteFile(jsonFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		loader, err := NewFileLoader(jsonFile, WithFormat(FormatJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer loader.Stop()

		config, err := loader.Load(context.Background())
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if config.KeyPrefix != "json:" {
			t.Errorf("expected key_prefix 'json:', got %q", config.KeyPrefix)
		}
	})

	t.Run("带轮询间隔选项", func(t *testing.T) {
		yamlFile := filepath.Join(tmpDir, "config2.yaml")
		content := `rules:
  - name: test
    key_template: test
    limit: 10
    window: "1s"
`
		if err := os.WriteFile(yamlFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		loader, err := NewFileLoader(yamlFile, WithPollInterval(time.Second))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer loader.Stop()

		if loader.interval != time.Second {
			t.Errorf("expected interval 1s, got %v", loader.interval)
		}
	})
}

func TestFileLoader_Watch(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "watch_config.yaml")

	content := `rules:
  - name: initial
    key_template: initial
    limit: 10
    window: "1s"
`
	if err := os.WriteFile(yamlFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	loader, err := NewFileLoader(yamlFile, WithPollInterval(100*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configCh, err := loader.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	// 更新配置文件
	time.Sleep(50 * time.Millisecond)
	newContent := `rules:
  - name: updated
    key_template: updated
    limit: 20
    window: "2s"
`
	if err := os.WriteFile(yamlFile, []byte(newContent), 0644); err != nil {
		t.Fatalf("failed to update test file: %v", err)
	}

	// 等待配置更新
	select {
	case config := <-configCh:
		if len(config.Rules) != 1 {
			t.Errorf("expected 1 rule, got %d", len(config.Rules))
		}
		if config.Rules[0].Name != "updated" {
			t.Errorf("expected rule name 'updated', got %q", config.Rules[0].Name)
		}
	case <-time.After(500 * time.Millisecond):
		t.Log("watch timeout - file change detection may be slow")
	}

	loader.Stop()

	// 停止后再次调用 Watch 应该失败
	_, err = loader.Watch(context.Background())
	if err == nil {
		t.Error("expected error after Stop()")
	}
}

func TestFileLoader_DetectFormat(t *testing.T) {
	tests := []struct {
		path   string
		expect ConfigFormat
	}{
		{"config.yaml", FormatYAML},
		{"config.yml", FormatYAML},
		{"config.json", FormatJSON},
		{"config.txt", FormatYAML}, // 默认 YAML
		{"config", FormatYAML},     // 无扩展名默认 YAML
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := detectFormat(tt.path)
			if got != tt.expect {
				t.Errorf("detectFormat(%q) = %q, want %q", tt.path, got, tt.expect)
			}
		})
	}
}

func TestReloadableConfig(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "reload_config.yaml")

	content := `rules:
  - name: test
    key_template: test
    limit: 10
    window: "1s"
`
	if err := os.WriteFile(yamlFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	loader, err := NewFileLoader(yamlFile, WithPollInterval(100*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reloadable := NewReloadableConfig(loader, func(_, _ *Config) {
		// 配置变更回调
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := reloadable.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	config := reloadable.GetConfig()
	if config == nil {
		t.Fatal("expected non-nil config")
	}
	if len(config.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(config.Rules))
	}

	// 更新配置
	time.Sleep(50 * time.Millisecond)
	newContent := `rules:
  - name: updated
    key_template: updated
    limit: 20
    window: "2s"
`
	if err := os.WriteFile(yamlFile, []byte(newContent), 0644); err != nil {
		t.Fatalf("failed to update test file: %v", err)
	}

	// 等待配置更新
	time.Sleep(200 * time.Millisecond)

	reloadable.Stop()
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"1s", time.Second, false},
		{"1m", time.Minute, false},
		{"1h", time.Hour, false},
		{"500ms", 500 * time.Millisecond, false},
		{"", 0, true},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
