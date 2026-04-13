package xconf

import "github.com/knadh/koanf/v2"

// Format 定义配置文件格式。
type Format string

// 支持的配置格式。
const (
	// FormatYAML YAML 格式（推荐用于 K8s ConfigMap）。
	FormatYAML Format = "yaml"

	// FormatJSON JSON 格式。
	FormatJSON Format = "json"
)

// Config 定义配置接口。
// 只提供增值功能，基础操作请直接使用 Client() 返回的 koanf 实例。
//
// 设计决策: MustUnmarshal 是包级函数而非接口方法。
// 接口中定义 panic 方法会强制所有实现者 panic，限制灵活性（如 mock 实现）。
// 使用包级函数 MustUnmarshal(cfg, path, target) 替代。
type Config interface {
	// Client 返回底层的 koanf 实例。
	// 用于执行所有 koanf 支持的操作。
	Client() *koanf.Koanf

	// Unmarshal 将指定路径的配置反序列化到目标结构体。
	// path 为空字符串时反序列化整个配置。
	// 使用 mapstructure 进行反序列化。
	Unmarshal(path string, target any) error

	// Reload 重新加载配置文件。
	// 此方法是并发安全的。
	// 仅对从文件创建的 Config 有效，从字节数据创建的 Config 调用会返回错误。
	Reload() error

	// Path 返回配置文件路径。
	// 从字节数据创建的 Config 返回空字符串。
	Path() string

	// Format 返回配置格式。
	Format() Format
}
