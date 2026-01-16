package xlimit

import (
	"net/http"
)

// MiddlewareOptions HTTP 中间件配置选项
type MiddlewareOptions struct {
	// KeyExtractor 键提取器
	KeyExtractor *HTTPKeyExtractor

	// DenyHandler 自定义拒绝处理器
	// 当请求被限流时调用
	DenyHandler func(w http.ResponseWriter, r *http.Request, result *Result)

	// SkipFunc 跳过函数
	// 返回 true 时跳过限流检查
	SkipFunc func(r *http.Request) bool

	// EnableHeaders 是否在响应中添加限流头
	EnableHeaders bool
}

// MiddlewareOption 中间件选项函数
type MiddlewareOption func(*MiddlewareOptions)

// defaultMiddlewareOptions 返回默认的中间件选项
func defaultMiddlewareOptions() *MiddlewareOptions {
	return &MiddlewareOptions{
		KeyExtractor:  DefaultHTTPKeyExtractor(),
		EnableHeaders: true,
		DenyHandler:   defaultDenyHandler,
	}
}

// defaultDenyHandler 默认的拒绝处理器
func defaultDenyHandler(w http.ResponseWriter, _ *http.Request, result *Result) {
	result.SetHeaders(w)
	w.WriteHeader(http.StatusTooManyRequests)
	writeResponse(w, []byte("Too Many Requests"))
}

// writeResponse 写入 HTTP 响应体。
// 写入失败时不返回错误，因为此时连接可能已断开，无法进行补救。
func writeResponse(w http.ResponseWriter, data []byte) {
	if _, err := w.Write(data); err != nil {
		// 写入失败通常表示客户端已断开连接
		// 此时无法采取补救措施，显式处理错误以满足 errcheck
		return
	}
}

// WithMiddlewareKeyExtractor 设置键提取器
func WithMiddlewareKeyExtractor(extractor *HTTPKeyExtractor) MiddlewareOption {
	return func(opts *MiddlewareOptions) {
		opts.KeyExtractor = extractor
	}
}

// WithDenyHandler 设置自定义拒绝处理器
func WithDenyHandler(handler func(w http.ResponseWriter, r *http.Request, result *Result)) MiddlewareOption {
	return func(opts *MiddlewareOptions) {
		opts.DenyHandler = handler
	}
}

// WithSkipFunc 设置跳过函数
func WithSkipFunc(skipFunc func(r *http.Request) bool) MiddlewareOption {
	return func(opts *MiddlewareOptions) {
		opts.SkipFunc = skipFunc
	}
}

// WithMiddlewareHeaders 设置是否启用限流头
func WithMiddlewareHeaders(enable bool) MiddlewareOption {
	return func(opts *MiddlewareOptions) {
		opts.EnableHeaders = enable
	}
}
