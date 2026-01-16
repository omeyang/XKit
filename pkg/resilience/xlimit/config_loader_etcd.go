package xlimit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	clientv3 "go.etcd.io/etcd/client/v3"
	"gopkg.in/yaml.v3"
)

// EtcdLoader etcd 配置加载器
// 支持配置热更新，当 etcd 中的配置发生变化时自动通知
type EtcdLoader struct {
	client *clientv3.Client
	key    string
	format ConfigFormat

	mu       sync.RWMutex
	configCh chan *Config
	cancel   context.CancelFunc
	stopped  bool
}

// EtcdLoaderOption etcd 加载器选项
type EtcdLoaderOption func(*EtcdLoader)

// WithEtcdFormat 设置配置格式
// 支持 YAML 和 JSON，默认 YAML
func WithEtcdFormat(format ConfigFormat) EtcdLoaderOption {
	return func(el *EtcdLoader) {
		el.format = format
	}
}

// NewEtcdLoader 创建 etcd 配置加载器
// client: etcd 客户端
// key: 配置存储的 key（如 "/config/xlimit"）
func NewEtcdLoader(client *clientv3.Client, key string, opts ...EtcdLoaderOption) *EtcdLoader {
	el := &EtcdLoader{
		client: client,
		key:    key,
		format: FormatYAML, // 默认 YAML
	}

	for _, opt := range opts {
		opt(el)
	}

	return el
}

// Load 加载配置
func (el *EtcdLoader) Load(ctx context.Context) (*Config, error) {
	resp, err := el.client.Get(ctx, el.key)
	if err != nil {
		return nil, fmt.Errorf("从 etcd 获取配置失败: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrConfigNotFound, el.key)
	}

	data := resp.Kvs[0].Value
	return el.parseConfig(data)
}

// parseConfig 解析配置数据
func (el *EtcdLoader) parseConfig(data []byte) (*Config, error) {
	var fileConfig FileConfig

	switch el.format {
	case FormatYAML:
		if err := yaml.Unmarshal(data, &fileConfig); err != nil {
			return nil, fmt.Errorf("解析 YAML 配置失败: %w", err)
		}
	case FormatJSON:
		if err := json.Unmarshal(data, &fileConfig); err != nil {
			return nil, fmt.Errorf("解析 JSON 配置失败: %w", err)
		}
	default:
		return nil, fmt.Errorf("不支持的配置格式: %s", el.format)
	}

	config, err := fileConfig.ToConfig()
	if err != nil {
		return nil, fmt.Errorf("转换配置失败: %w", err)
	}

	return config, nil
}

// Watch 监听配置变更
// 使用 etcd 的 Watch 机制实时监听配置变化
func (el *EtcdLoader) Watch(ctx context.Context) (<-chan *Config, error) {
	el.mu.Lock()
	if el.stopped {
		el.mu.Unlock()
		return nil, fmt.Errorf("loader 已停止")
	}

	el.configCh = make(chan *Config, 1)
	watchCtx, cancel := context.WithCancel(ctx)
	el.cancel = cancel
	el.mu.Unlock()

	go el.watchLoop(watchCtx)

	return el.configCh, nil
}

// watchLoop 监听循环
func (el *EtcdLoader) watchLoop(ctx context.Context) {
	watchCh := el.client.Watch(ctx, el.key)

	for {
		select {
		case <-ctx.Done():
			el.closeConfigCh()
			return
		case wresp, ok := <-watchCh:
			if !ok {
				// watch channel 关闭，尝试重新建立
				watchCh = el.client.Watch(ctx, el.key)
				continue
			}

			if wresp.Err() != nil {
				continue
			}

			el.handleWatchEvents(wresp.Events)
		}
	}
}

// handleWatchEvents 处理 watch 事件
func (el *EtcdLoader) handleWatchEvents(events []*clientv3.Event) {
	for _, ev := range events {
		if ev.Type != clientv3.EventTypePut {
			continue
		}

		config, err := el.parseConfig(ev.Kv.Value)
		if err != nil {
			continue
		}

		el.sendConfig(config)
	}
}

// sendConfig 发送配置到 channel
func (el *EtcdLoader) sendConfig(config *Config) {
	el.mu.RLock()
	ch := el.configCh
	el.mu.RUnlock()

	if ch == nil {
		return
	}

	select {
	case ch <- config:
	default:
		// channel 已满，跳过
	}
}

// closeConfigCh 关闭配置 channel
func (el *EtcdLoader) closeConfigCh() {
	el.mu.Lock()
	defer el.mu.Unlock()

	if el.configCh != nil {
		close(el.configCh)
		el.configCh = nil
	}
}

// Stop 停止配置监听
func (el *EtcdLoader) Stop() error {
	el.mu.Lock()
	defer el.mu.Unlock()

	if el.stopped {
		return nil
	}

	el.stopped = true
	if el.cancel != nil {
		el.cancel()
	}

	return nil
}

// SaveConfig 保存配置到 etcd
// 这是一个便利方法，用于初始化或更新 etcd 中的配置
func (el *EtcdLoader) SaveConfig(ctx context.Context, config *Config) error {
	fileConfig := configToFileConfig(config)

	var data []byte
	var err error

	switch el.format {
	case FormatYAML:
		data, err = yaml.Marshal(fileConfig)
	case FormatJSON:
		data, err = json.Marshal(fileConfig)
	default:
		return fmt.Errorf("不支持的配置格式: %s", el.format)
	}

	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	_, err = el.client.Put(ctx, el.key, string(data))
	if err != nil {
		return fmt.Errorf("保存配置到 etcd 失败: %w", err)
	}

	return nil
}

// configToFileConfig 将 Config 转换为 FileConfig
func configToFileConfig(config *Config) *FileConfig {
	fc := &FileConfig{
		KeyPrefix:     config.KeyPrefix,
		LocalPodCount: config.LocalPodCount,
		EnableMetrics: config.EnableMetrics,
		EnableHeaders: config.EnableHeaders,
	}

	// 转换降级策略
	switch config.Fallback {
	case FallbackLocal:
		fc.FallbackStrategy = "local"
	case FallbackOpen:
		fc.FallbackStrategy = "fail-open"
	case FallbackClose:
		fc.FallbackStrategy = "fail-close"
	}

	// 转换规则
	for _, rule := range config.Rules {
		fr := FileRule{
			Name:        rule.Name,
			KeyTemplate: rule.KeyTemplate,
			Limit:       rule.Limit,
			Window:      rule.Window.String(),
			Burst:       rule.Burst,
			Enabled:     rule.Enabled,
		}

		// 转换覆盖配置
		for _, override := range rule.Overrides {
			fo := FileOverride{
				Match: override.Match,
				Limit: override.Limit,
				Burst: override.Burst,
			}
			if override.Window > 0 {
				fo.Window = override.Window.String()
			}
			fr.Overrides = append(fr.Overrides, fo)
		}

		fc.Rules = append(fc.Rules, fr)
	}

	return fc
}
