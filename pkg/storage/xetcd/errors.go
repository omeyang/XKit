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

	// ErrInvalidConfig 配置值无效。
	ErrInvalidConfig = errors.New("xetcd: invalid config")

	// ErrWatchDisconnected Watch 通道意外关闭。
	// 用于 WatchWithRetry 内部标识需要重连的断开事件。
	ErrWatchDisconnected = errors.New("xetcd: watch disconnected")

	// ErrMaxRetriesExceeded 达到最大重试次数。
	// WatchWithRetry 在耗尽重试次数后，通过错误事件发送此错误。
	ErrMaxRetriesExceeded = errors.New("xetcd: max retries exceeded")

	// errNilKv 内部错误：收到 Kv 为 nil 的 etcd 事件。
	// 正常协议中不应出现，但防御性处理以避免 goroutine panic。
	errNilKv = errors.New("xetcd: received event with nil Kv")
)

// IsKeyNotFound 检查错误是否为键不存在。
func IsKeyNotFound(err error) bool {
	return errors.Is(err, ErrKeyNotFound)
}

// IsClientClosed 检查错误是否为客户端已关闭。
func IsClientClosed(err error) bool {
	return errors.Is(err, ErrClientClosed)
}
