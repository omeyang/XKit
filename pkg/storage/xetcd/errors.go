package xetcd

import "errors"

// 错误定义。
var (
	// ErrNilConfig 配置为空。
	ErrNilConfig = errors.New("xetcd: config is nil")

	// ErrNoEndpoints 未配置 etcd 端点。
	ErrNoEndpoints = errors.New("xetcd: no endpoints configured")

	// ErrInvalidEndpoint endpoint 格式无效。
	// 有效格式应为 "host:port"，例如 "localhost:2379" 或 "192.168.1.1:2379"。
	ErrInvalidEndpoint = errors.New("xetcd: invalid endpoint format, expected host:port")

	// ErrKeyNotFound 键不存在。
	ErrKeyNotFound = errors.New("xetcd: key not found")

	// ErrClientClosed 客户端已关闭。
	ErrClientClosed = errors.New("xetcd: client is closed")

	// ErrEmptyKey 键名为空。
	ErrEmptyKey = errors.New("xetcd: key is empty")
)

// IsKeyNotFound 检查错误是否为键不存在。
func IsKeyNotFound(err error) bool {
	return errors.Is(err, ErrKeyNotFound)
}

// IsClientClosed 检查错误是否为客户端已关闭。
func IsClientClosed(err error) bool {
	return errors.Is(err, ErrClientClosed)
}
