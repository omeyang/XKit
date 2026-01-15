package xconf

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
)

// koanfConfig 是 Config 接口的 koanf 实现。
type koanfConfig struct {
	k       *koanf.Koanf
	path    string
	format  Format
	opts    *Options
	mu      sync.RWMutex
	isBytes bool // 标记是否从字节数据创建
}

// New 从文件路径创建配置实例。
// 根据文件扩展名自动检测格式（.yaml/.yml 或 .json）。
func New(path string, opts ...Option) (Config, error) {
	if path == "" {
		return nil, ErrEmptyPath
	}

	format, err := detectFormat(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLoadFailed, err)
	}

	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	k := koanf.New(options.Delim)
	if err := loadData(k, data, format); err != nil {
		return nil, err
	}

	return &koanfConfig{
		k:       k,
		path:    path,
		format:  format,
		opts:    options,
		isBytes: false,
	}, nil
}

// NewFromBytes 从字节数据创建配置实例。
// 需要显式指定格式，适用于 K8s ConfigMap 等场景。
//
// 空数据处理：
//   - 空数据（len(data) == 0）会创建一个空配置实例
//   - 与 New 行为一致：New 允许读取空文件，NewFromBytes 也允许空数据
//   - 空配置可以正常使用，Unmarshal 会返回目标结构体的零值
func NewFromBytes(data []byte, format Format, opts ...Option) (Config, error) {
	if !isValidFormat(format) {
		return nil, ErrUnsupportedFormat
	}

	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	k := koanf.New(options.Delim)

	// 空数据时创建空配置，与 New 行为一致
	if len(data) > 0 {
		if err := loadData(k, data, format); err != nil {
			return nil, err
		}
	}

	return &koanfConfig{
		k:       k,
		path:    "",
		format:  format,
		opts:    options,
		isBytes: true,
	}, nil
}

// Client 返回底层的 koanf 实例。
func (c *koanfConfig) Client() *koanf.Koanf {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.k
}

// Unmarshal 将指定路径的配置反序列化到目标结构体。
func (c *koanfConfig) Unmarshal(path string, target any) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if err := c.k.UnmarshalWithConf(path, target, koanf.UnmarshalConf{
		Tag: c.opts.Tag,
	}); err != nil {
		return fmt.Errorf("%w: %w", ErrUnmarshalFailed, err)
	}
	return nil
}

// MustUnmarshal 与 Unmarshal 相同，但失败时 panic。
func (c *koanfConfig) MustUnmarshal(path string, target any) {
	if err := c.Unmarshal(path, target); err != nil {
		panic(err)
	}
}

// Reload 重新加载配置文件。
func (c *koanfConfig) Reload() error {
	if c.isBytes {
		return errors.New("xconf: cannot reload config created from bytes")
	}

	data, err := os.ReadFile(c.path)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrLoadFailed, err)
	}

	newK := koanf.New(c.opts.Delim)
	if err := loadData(newK, data, c.format); err != nil {
		return err
	}

	c.mu.Lock()
	c.k = newK
	c.mu.Unlock()

	return nil
}

// Path 返回配置文件路径。
func (c *koanfConfig) Path() string {
	return c.path
}

// Format 返回配置格式。
func (c *koanfConfig) Format() Format {
	return c.format
}

// =============================================================================
// 内部辅助函数
// =============================================================================

// detectFormat 根据文件扩展名检测配置格式。
func detectFormat(path string) (Format, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		return FormatYAML, nil
	case ".json":
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("%w: unknown extension %s", ErrUnsupportedFormat, ext)
	}
}

// isValidFormat 检查格式是否有效。
func isValidFormat(format Format) bool {
	switch format {
	case FormatYAML, FormatJSON:
		return true
	default:
		return false
	}
}

// loadData 加载数据到 koanf 实例。
func loadData(k *koanf.Koanf, data []byte, format Format) error {
	var parser koanf.Parser
	switch format {
	case FormatYAML:
		parser = yaml.Parser()
	case FormatJSON:
		parser = json.Parser()
	default:
		return ErrUnsupportedFormat
	}

	if err := k.Load(rawbytes.Provider(data), parser); err != nil {
		return fmt.Errorf("%w: %w", ErrParseFailed, err)
	}
	return nil
}
