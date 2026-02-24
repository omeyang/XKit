package xconf

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
)

// koanfConfig 是 Config 接口的 koanf 实现。
//
// 设计决策: 使用 atomic.Pointer 替代 sync.RWMutex 保护 koanf 实例指针。
// koanf 内部已有 sync.RWMutex，外层再加一层会导致双重加锁（cache line bouncing）。
// atomic.Pointer 实现无锁读取，Reload 使用 Store 原子替换，性能更优。
type koanfConfig struct {
	k        atomic.Pointer[koanf.Koanf]
	reloadMu sync.Mutex // 序列化 Reload 调用，防止并发重载导致配置回退
	path     string
	format   Format
	opts     *options
	isBytes  bool // 标记是否从字节数据创建
}

// New 从文件路径创建配置实例。
// 根据文件扩展名自动检测格式（.yaml/.yml 或 .json）。
//
// 设计决策: 路径使用 filepath.Abs 转为绝对路径，防止 Reload 因进程 cwd 变化而读取漂移。
// 路径安全（防止路径穿越等）由调用方负责，xconf 是工具库不做沙箱限制。
func New(path string, opts ...Option) (Config, error) {
	if path == "" {
		return nil, ErrEmptyPath
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLoadFailed, err)
	}
	path = absPath

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
		if opt == nil {
			return nil, ErrNilOption
		}
		opt(options)
	}
	if err := options.validate(); err != nil {
		return nil, err
	}

	k := koanf.New(options.delim)

	// 空数据时创建空配置，与 NewFromBytes 行为一致
	if len(data) > 0 {
		if err := loadData(k, data, format); err != nil {
			return nil, err
		}
	}

	cfg := &koanfConfig{
		path:    path,
		format:  format,
		opts:    options,
		isBytes: false,
	}
	cfg.k.Store(k)
	return cfg, nil
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
		if opt == nil {
			return nil, ErrNilOption
		}
		opt(options)
	}
	if err := options.validate(); err != nil {
		return nil, err
	}

	k := koanf.New(options.delim)

	// 空数据时创建空配置，与 New 行为一致
	if len(data) > 0 {
		if err := loadData(k, data, format); err != nil {
			return nil, err
		}
	}

	cfg := &koanfConfig{
		path:    "",
		format:  format,
		opts:    options,
		isBytes: true,
	}
	cfg.k.Store(k)
	return cfg, nil
}

// Client 返回底层的 koanf 实例（当前时刻的快照）。
//
// 设计决策: 返回的指针在 Reload() 后仍然有效，但指向旧配置。
// 推荐用法：每次需要时调用 Client()，不要长期缓存返回的指针。
func (c *koanfConfig) Client() *koanf.Koanf {
	return c.k.Load()
}

// Unmarshal 将指定路径的配置反序列化到目标结构体。
func (c *koanfConfig) Unmarshal(path string, target any) error {
	if target == nil {
		return fmt.Errorf("%w: target must be a non-nil pointer", ErrUnmarshalFailed)
	}
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("%w: target must be a non-nil pointer, got %T", ErrUnmarshalFailed, target)
	}

	k := c.k.Load()

	if err := k.UnmarshalWithConf(path, target, koanf.UnmarshalConf{
		Tag: c.opts.tag,
	}); err != nil {
		return fmt.Errorf("%w: %w", ErrUnmarshalFailed, err)
	}
	return nil
}

// Reload 重新加载配置文件。
//
// 设计决策: reloadMu 序列化并发 Reload 调用，防止配置回退。
// 场景：Goroutine A 读取 v2 → Goroutine B 读取 v3 并 Store → Goroutine A Store v2，v3 被回退。
// Mutex 仅保护 Reload 路径，不影响 Client/Unmarshal 的无锁读取性能。
func (c *koanfConfig) Reload() error {
	if c.isBytes {
		return ErrNotFromFile
	}

	c.reloadMu.Lock()
	defer c.reloadMu.Unlock()

	data, err := os.ReadFile(c.path)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrLoadFailed, err)
	}

	newK := koanf.New(c.opts.delim)

	// 空数据时创建空配置，与 New/NewFromBytes 行为一致
	if len(data) > 0 {
		if err := loadData(newK, data, c.format); err != nil {
			return err
		}
	}

	c.k.Store(newK)

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

// MustUnmarshal 与 Unmarshal 相同，但失败时 panic。
// 适用于程序启动时的必要配置加载。
func MustUnmarshal(cfg Config, path string, target any) {
	if cfg == nil {
		panic("xconf: MustUnmarshal called with nil Config")
	}
	if err := cfg.Unmarshal(path, target); err != nil {
		panic(err)
	}
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
//
// 设计决策: default 分支使用 panic 而非返回错误。
// 所有调用方（New/NewFromBytes/Reload）在调用前已验证 format 有效性，
// 到达 default 分支表示内部逻辑错误，应立即暴露而非静默返回。
func loadData(k *koanf.Koanf, data []byte, format Format) error {
	var parser koanf.Parser
	switch format {
	case FormatYAML:
		parser = yaml.Parser()
	case FormatJSON:
		parser = json.Parser()
	default:
		panic("xconf: loadData called with invalid format: " + string(format))
	}

	if err := k.Load(rawbytes.Provider(data), parser); err != nil {
		return fmt.Errorf("%w: %w", ErrParseFailed, err)
	}
	return nil
}
