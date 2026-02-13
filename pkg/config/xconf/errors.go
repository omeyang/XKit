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

	// ErrNotFromFile 表示操作仅支持从文件创建的配置实例。
	// Reload 和 Watch 都需要文件路径，不支持从 bytes 创建的配置。
	ErrNotFromFile = errors.New("xconf: operation not supported for config created from bytes")

	// ErrWatchFailed 表示创建文件监视器失败。
	ErrWatchFailed = errors.New("xconf: failed to create watcher")

	// ErrInvalidDelim 表示无效的键分隔符。
	ErrInvalidDelim = errors.New("xconf: invalid delimiter")

	// ErrInvalidTag 表示无效的结构体标签名。
	ErrInvalidTag = errors.New("xconf: invalid struct tag")

	// ErrInvalidDebounce 表示无效的防抖时间。
	ErrInvalidDebounce = errors.New("xconf: invalid debounce duration")

	// ErrNilCallback 表示 Watch 回调函数为 nil。
	ErrNilCallback = errors.New("xconf: nil watch callback")
)
