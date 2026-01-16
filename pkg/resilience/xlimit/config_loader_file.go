package xlimit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// FileLoader 文件配置加载器
// 支持 YAML 和 JSON 格式的配置文件
type FileLoader struct {
	path     string
	format   ConfigFormat
	interval time.Duration

	mu       sync.RWMutex
	lastMod  time.Time
	configCh chan *Config
	stopCh   chan struct{}
	stopped  bool
}

// FileLoaderOption 文件加载器选项
type FileLoaderOption func(*FileLoader)

// WithFormat 设置配置文件格式
// 如果不设置，将根据文件扩展名自动检测
func WithFormat(format ConfigFormat) FileLoaderOption {
	return func(fl *FileLoader) {
		fl.format = format
	}
}

// WithPollInterval 设置轮询间隔
// 用于监听文件变更，默认 30 秒
func WithPollInterval(interval time.Duration) FileLoaderOption {
	return func(fl *FileLoader) {
		fl.interval = interval
	}
}

// NewFileLoader 创建文件配置加载器
func NewFileLoader(path string, opts ...FileLoaderOption) (*FileLoader, error) {
	// 检查文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", ErrConfigNotFound, path)
	}

	fl := &FileLoader{
		path:     path,
		interval: 30 * time.Second,
		stopCh:   make(chan struct{}),
	}

	// 应用选项
	for _, opt := range opts {
		opt(fl)
	}

	// 自动检测格式
	if fl.format == "" {
		fl.format = detectFormat(path)
	}

	return fl, nil
}

// detectFormat 根据文件扩展名检测配置格式
func detectFormat(path string) ConfigFormat {
	ext := filepath.Ext(path)
	switch ext {
	case ".yaml", ".yml":
		return FormatYAML
	case ".json":
		return FormatJSON
	default:
		return FormatYAML // 默认 YAML
	}
}

// Load 加载配置
func (fl *FileLoader) Load(_ context.Context) (*Config, error) {
	return fl.loadFile()
}

// loadFile 从文件加载配置
func (fl *FileLoader) loadFile() (*Config, error) {
	data, err := os.ReadFile(fl.path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var fileConfig FileConfig

	switch fl.format {
	case FormatYAML:
		if err := yaml.Unmarshal(data, &fileConfig); err != nil {
			return nil, fmt.Errorf("解析 YAML 配置失败: %w", err)
		}
	case FormatJSON:
		if err := json.Unmarshal(data, &fileConfig); err != nil {
			return nil, fmt.Errorf("解析 JSON 配置失败: %w", err)
		}
	default:
		return nil, fmt.Errorf("不支持的配置格式: %s", fl.format)
	}

	config, err := fileConfig.ToConfig()
	if err != nil {
		return nil, fmt.Errorf("转换配置失败: %w", err)
	}

	// 更新最后修改时间
	fl.mu.Lock()
	if stat, err := os.Stat(fl.path); err == nil {
		fl.lastMod = stat.ModTime()
	}
	fl.mu.Unlock()

	return config, nil
}

// Watch 监听配置变更
// 通过轮询文件修改时间实现
func (fl *FileLoader) Watch(ctx context.Context) (<-chan *Config, error) {
	fl.mu.Lock()
	if fl.stopped {
		fl.mu.Unlock()
		return nil, fmt.Errorf("loader 已停止")
	}

	fl.configCh = make(chan *Config, 1)
	fl.mu.Unlock()

	go fl.watchLoop(ctx)

	return fl.configCh, nil
}

// watchLoop 监听循环
func (fl *FileLoader) watchLoop(ctx context.Context) {
	ticker := time.NewTicker(fl.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fl.closeConfigCh()
			return
		case <-fl.stopCh:
			fl.closeConfigCh()
			return
		case <-ticker.C:
			if fl.hasFileChanged() {
				config, err := fl.loadFile()
				if err != nil {
					// 配置加载失败时忽略，保持使用旧配置
					continue
				}

				fl.mu.RLock()
				ch := fl.configCh
				fl.mu.RUnlock()

				if ch != nil {
					select {
					case ch <- config:
					default:
						// channel 已满，跳过
					}
				}
			}
		}
	}
}

// hasFileChanged 检查文件是否已修改
func (fl *FileLoader) hasFileChanged() bool {
	stat, err := os.Stat(fl.path)
	if err != nil {
		return false
	}

	fl.mu.RLock()
	lastMod := fl.lastMod
	fl.mu.RUnlock()

	return stat.ModTime().After(lastMod)
}

// closeConfigCh 关闭配置 channel
func (fl *FileLoader) closeConfigCh() {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	if fl.configCh != nil {
		close(fl.configCh)
		fl.configCh = nil
	}
}

// Stop 停止配置监听
func (fl *FileLoader) Stop() error {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	if fl.stopped {
		return nil
	}

	fl.stopped = true
	close(fl.stopCh)

	return nil
}

// parseDuration 解析时间间隔字符串
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("时间间隔不能为空")
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("解析时间间隔失败: %w", err)
	}

	return d, nil
}
