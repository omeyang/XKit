package xlimit

import (
	"context"
)

// ConfigLoader 定义配置加载器接口
// 支持从不同数据源（文件、etcd 等）加载限流配置
type ConfigLoader interface {
	// Load 加载配置
	// 返回当前配置和可能的错误
	Load(ctx context.Context) (*Config, error)

	// Watch 监听配置变更
	// 当配置发生变化时，通过 channel 通知
	// 调用方负责关闭返回的 channel（通过调用 Stop）
	Watch(ctx context.Context) (<-chan *Config, error)

	// Stop 停止配置监听
	// 释放资源，关闭 Watch 返回的 channel
	Stop() error
}

// ConfigChangeCallback 配置变更回调函数类型
type ConfigChangeCallback func(oldConfig, newConfig *Config)

// ReloadableConfig 可热更新的配置包装器
// 封装 ConfigLoader，提供线程安全的配置访问和热更新能力
type ReloadableConfig struct {
	loader   ConfigLoader
	config   *Config
	onChange ConfigChangeCallback
	cancel   context.CancelFunc
}

// NewReloadableConfig 创建可热更新的配置
func NewReloadableConfig(loader ConfigLoader, onChange ConfigChangeCallback) *ReloadableConfig {
	return &ReloadableConfig{
		loader:   loader,
		onChange: onChange,
	}
}

// Start 启动配置加载和监听
func (r *ReloadableConfig) Start(ctx context.Context) error {
	// 首次加载配置
	config, err := r.loader.Load(ctx)
	if err != nil {
		return err
	}
	r.config = config

	// 启动监听
	watchCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel

	configCh, err := r.loader.Watch(watchCtx)
	if err != nil {
		cancel()
		return err
	}

	// 后台处理配置变更
	go r.handleConfigChanges(configCh)

	return nil
}

// handleConfigChanges 处理配置变更
func (r *ReloadableConfig) handleConfigChanges(configCh <-chan *Config) {
	for newConfig := range configCh {
		oldConfig := r.config
		r.config = newConfig

		if r.onChange != nil {
			r.onChange(oldConfig, newConfig)
		}
	}
}

// GetConfig 获取当前配置
// 线程安全
func (r *ReloadableConfig) GetConfig() *Config {
	return r.config
}

// Stop 停止配置监听
func (r *ReloadableConfig) Stop() error {
	if r.cancel != nil {
		r.cancel()
	}
	return r.loader.Stop()
}

// ConfigFormat 配置文件格式
type ConfigFormat string

const (
	// FormatYAML YAML 格式
	FormatYAML ConfigFormat = "yaml"
	// FormatJSON JSON 格式
	FormatJSON ConfigFormat = "json"
)

// FileConfig 文件配置加载器的配置结构（用于 YAML/JSON 解析）
type FileConfig struct {
	// KeyPrefix Redis 键前缀
	KeyPrefix string `yaml:"key_prefix" json:"key_prefix"`

	// LocalPodCount 本地 Pod 数量
	LocalPodCount int `yaml:"local_pod_count" json:"local_pod_count"`

	// EnableMetrics 是否启用 Prometheus 指标
	EnableMetrics bool `yaml:"enable_metrics" json:"enable_metrics"`

	// EnableHeaders 是否在响应中添加限流头
	EnableHeaders bool `yaml:"enable_headers" json:"enable_headers"`

	// FallbackStrategy 降级策略
	FallbackStrategy string `yaml:"fallback_strategy" json:"fallback_strategy"`

	// Rules 限流规则列表
	Rules []FileRule `yaml:"rules" json:"rules"`
}

// FileRule 文件中的规则定义
type FileRule struct {
	// Name 规则名称
	Name string `yaml:"name" json:"name"`

	// KeyTemplate 键模板
	KeyTemplate string `yaml:"key_template" json:"key_template"`

	// Limit 每窗口限制次数
	Limit int `yaml:"limit" json:"limit"`

	// Window 时间窗口（如 "1m", "1h"）
	Window string `yaml:"window" json:"window"`

	// Burst 突发容量
	Burst int `yaml:"burst,omitempty" json:"burst,omitempty"`

	// Enabled 是否启用
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`

	// Overrides 覆盖配置
	Overrides []FileOverride `yaml:"overrides,omitempty" json:"overrides,omitempty"`
}

// FileOverride 文件中的覆盖配置
type FileOverride struct {
	// Match 匹配模式（支持通配符）
	Match string `yaml:"match" json:"match"`

	// Limit 覆盖的限制次数
	Limit int `yaml:"limit" json:"limit"`

	// Window 覆盖的时间窗口
	Window string `yaml:"window,omitempty" json:"window,omitempty"`

	// Burst 覆盖的突发容量
	Burst int `yaml:"burst,omitempty" json:"burst,omitempty"`
}

// ToConfig 将 FileConfig 转换为 Config
func (fc *FileConfig) ToConfig() (*Config, error) {
	config := &Config{
		KeyPrefix:     fc.KeyPrefix,
		LocalPodCount: fc.LocalPodCount,
		EnableMetrics: fc.EnableMetrics,
		EnableHeaders: fc.EnableHeaders,
	}

	// 解析降级策略
	switch fc.FallbackStrategy {
	case "local":
		config.Fallback = FallbackLocal
	case "open", "fail-open":
		config.Fallback = FallbackOpen
	case "close", "fail-close":
		config.Fallback = FallbackClose
	default:
		config.Fallback = FallbackLocal
	}

	// 转换规则
	for _, fr := range fc.Rules {
		rule, err := fr.ToRule()
		if err != nil {
			return nil, err
		}
		config.Rules = append(config.Rules, rule)
	}

	return config, nil
}

// ToRule 将 FileRule 转换为 Rule
func (fr *FileRule) ToRule() (Rule, error) {
	window, err := parseDuration(fr.Window)
	if err != nil {
		return Rule{}, err
	}

	rule := Rule{
		Name:        fr.Name,
		KeyTemplate: fr.KeyTemplate,
		Limit:       fr.Limit,
		Window:      window,
		Burst:       fr.Burst,
		Enabled:     fr.Enabled, // 直接使用指针
	}

	// 转换覆盖配置
	for _, fo := range fr.Overrides {
		override, err := fo.ToOverride()
		if err != nil {
			return Rule{}, err
		}
		rule.Overrides = append(rule.Overrides, override)
	}

	return rule, nil
}

// ToOverride 将 FileOverride 转换为 Override
func (fo *FileOverride) ToOverride() (Override, error) {
	override := Override{
		Match: fo.Match,
		Limit: fo.Limit,
		Burst: fo.Burst,
	}

	if fo.Window != "" {
		window, err := parseDuration(fo.Window)
		if err != nil {
			return Override{}, err
		}
		override.Window = window
	}

	return override, nil
}
