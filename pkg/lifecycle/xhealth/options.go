package xhealth

import "time"

// Option 配置 Health 的选项函数。
type Option func(*options)

// StatusListenerFunc 是状态变更回调函数。
//
// endpoint 为端点名称（"liveness"、"readiness"、"startup"），
// oldStatus 和 newStatus 为变更前后的状态。
type StatusListenerFunc func(endpoint string, oldStatus, newStatus Status)

type options struct {
	addr             string
	basePath         string
	cacheTTL         time.Duration
	statusListener   StatusListenerFunc
	detailQueryParam string
	shutdownTimeout  time.Duration
}

const (
	defaultAddr             = ":8080"
	defaultCacheTTL         = time.Second
	defaultDetailQueryParam = "full"
	defaultShutdownTimeout  = 30 * time.Second
)

func defaultOptions() options {
	return options{
		addr:             defaultAddr,
		cacheTTL:         defaultCacheTTL,
		detailQueryParam: defaultDetailQueryParam,
		shutdownTimeout:  defaultShutdownTimeout,
	}
}

// WithAddr 设置 HTTP 监听地址。
//
// 默认值为 ":8080"。传入空字符串将被忽略，保持默认值。
func WithAddr(addr string) Option {
	return func(o *options) {
		if addr != "" {
			o.addr = addr
		}
	}
}

// WithBasePath 设置 HTTP 路径前缀。
//
// 例如 WithBasePath("/api") 会将端点变为 /api/healthz、/api/readyz、/api/startupz。
// 默认无前缀。
func WithBasePath(path string) Option {
	return func(o *options) {
		o.basePath = path
	}
}

// WithCacheTTL 设置同步检查的缓存 TTL。
//
// 默认 1s。设为 0 禁用缓存（每次请求都执行检查）。
// 负值将被忽略，保持默认值。
func WithCacheTTL(ttl time.Duration) Option {
	return func(o *options) {
		if ttl >= 0 {
			o.cacheTTL = ttl
		}
	}
}

// WithStatusListener 设置状态变更回调。
//
// 当端点的聚合状态发生变化时（如 Up → Degraded），会调用此回调。
// 传入 nil 将被忽略。
func WithStatusListener(fn StatusListenerFunc) Option {
	return func(o *options) {
		if fn != nil {
			o.statusListener = fn
		}
	}
}

// WithShutdownTimeout 设置 HTTP server 优雅关闭的超时时间。
//
// 默认 30s。超时后将强制关闭连接，避免 Shutdown/Run 无界阻塞在慢 handler 上。
// 设为 0 或负值将被忽略，保持默认值。
func WithShutdownTimeout(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.shutdownTimeout = d
		}
	}
}

// WithDetailOnQueryParam 设置触发详细 JSON 响应的查询参数名。
//
// 默认为 "full"，即 /readyz?full=1 返回 JSON 详情。
// 传入空字符串将被忽略，保持默认值。
func WithDetailOnQueryParam(param string) Option {
	return func(o *options) {
		if param != "" {
			o.detailQueryParam = param
		}
	}
}
