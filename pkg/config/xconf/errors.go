package xconf

import "errors"

// 配置加载和解析相关错误。
var (
	// ErrEmptyPath 表示配置文件路径为空。
	ErrEmptyPath = errors.New("xconf: empty config path")

	// ErrUnsupportedFormat 表示不支持的配置格式。
	ErrUnsupportedFormat = errors.New("xconf: unsupported config format")

	// ErrLoadFailed 表示配置加载失败。
	ErrLoadFailed = errors.New("xconf: failed to load config")

	// ErrParseFailed 表示配置解析失败。
	ErrParseFailed = errors.New("xconf: failed to parse config")

	// ErrUnmarshalFailed 表示配置反序列化失败。
	ErrUnmarshalFailed = errors.New("xconf: failed to unmarshal config")
)
