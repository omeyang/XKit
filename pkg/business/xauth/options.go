package xauth

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"
)

// =============================================================================
// Options 结构
// =============================================================================

// Options 定义客户端的可选配置。
type Options struct {
	// Cache 缓存存储。
	// 如果设置，用于存储 Token 和平台数据。
	Cache CacheStore

	// HTTPClient 自定义 HTTP 客户端。
	// 如果不设置，将根据配置自动创建。
	HTTPClient *http.Client

	// Logger 日志记录器。
	// 如果不设置，使用 slog.Default()。
	Logger *slog.Logger

	// Observer 可观测性接口。
	// 用于记录操作指标和追踪。
	Observer xmetrics.Observer

	// TokenRefreshThreshold Token 刷新阈值。
	// 覆盖 Config 中的设置。
	TokenRefreshThreshold time.Duration

	// PlatformDataCacheTTL 平台数据缓存 TTL。
	// 覆盖 Config 中的设置。
	PlatformDataCacheTTL time.Duration

	// EnableLocalCache 是否启用本地缓存（L1）。
	// 默认启用。
	EnableLocalCache bool

	// LocalCacheMaxSize 本地缓存最大条目数。
	// 默认 1000。
	LocalCacheMaxSize int

	// LocalCacheTTL 本地缓存 TTL。
	// 控制平台数据在本地缓存中的过期时间，默认与 PlatformDataCacheTTL 相同。
	// Token 本地缓存的 TTL 独立计算（RefreshThreshold * 2），不受此参数影响。
	LocalCacheTTL time.Duration

	// EnableSingleflight 是否启用 singleflight。
	// 防止并发请求导致的缓存击穿。
	// 默认启用。
	EnableSingleflight bool

	// EnableBackgroundRefresh 是否启用后台刷新。
	// Token 即将过期时自动在后台刷新。
	// 默认启用。
	EnableBackgroundRefresh bool

	// EnableAutoRetryOn401 是否启用 401 自动重试。
	// 启用后，Request 方法遇到 401 错误时会自动清除 Token 缓存并重试一次。
	// 这有助于处理服务端吊销 Token 的场景。
	// 默认不启用。
	EnableAutoRetryOn401 bool
}

// Option 定义配置客户端的函数类型。
type Option func(*Options)

// defaultOptions 返回默认的 Options。
func defaultOptions() *Options {
	return &Options{
		Logger:                  slog.Default(),
		Observer:                xmetrics.NoopObserver{},
		EnableLocalCache:        true,
		LocalCacheMaxSize:       1000,
		EnableSingleflight:      true,
		EnableBackgroundRefresh: true,
	}
}

// applyOptions 应用所有 Option。
func applyOptions(opts []Option) *Options {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	return options
}

// =============================================================================
// Option 函数
// =============================================================================

// WithCache 设置缓存存储。
// 支持 Redis 等分布式缓存，用于存储 Token 和平台数据。
func WithCache(cache CacheStore) Option {
	return func(o *Options) {
		o.Cache = cache
	}
}

// WithHTTPClient 设置自定义 HTTP 客户端。
// 可用于配置自定义的传输层、代理等。
//
// 设计决策: 注入自定义 Client 后，Config.TLS 和 Config.Timeout 不再生效——
// 调用方既然选择自行构造 Client，就应当自行保证其 TLS 版本和超时策略满足安全要求。
// 不对外部 Client 做事后校验，因为 http.Client 的 Transport 可以是任意实现，
// 无法可靠地提取 TLS 配置进行断言。
func WithHTTPClient(client *http.Client) Option {
	return func(o *Options) {
		o.HTTPClient = client
	}
}

// WithLogger 设置日志记录器。
// 传入 nil 时使用 slog.Default()。
// 如需禁用日志，可传入 slog.New(slog.NewTextHandler(io.Discard, nil))。
func WithLogger(logger *slog.Logger) Option {
	return func(o *Options) {
		if logger != nil {
			o.Logger = logger
		}
	}
}

// WithObserver 设置可观测性接口。
// 用于记录操作指标和追踪信息。
func WithObserver(observer xmetrics.Observer) Option {
	return func(o *Options) {
		if observer != nil {
			o.Observer = observer
		}
	}
}

// WithTokenRefreshThreshold 设置 Token 刷新阈值。
// Token 剩余有效期小于此值时触发后台刷新。
func WithTokenRefreshThreshold(d time.Duration) Option {
	return func(o *Options) {
		if d > 0 {
			o.TokenRefreshThreshold = d
		}
	}
}

// WithPlatformDataCacheTTL 设置平台数据缓存 TTL。
func WithPlatformDataCacheTTL(d time.Duration) Option {
	return func(o *Options) {
		if d > 0 {
			o.PlatformDataCacheTTL = d
		}
	}
}

// WithLocalCache 设置是否启用本地缓存（L1）。
// 同时控制 Token 缓存和平台数据缓存的本地缓存。
// 本地缓存可减少 Redis 访问，提升性能。
func WithLocalCache(enable bool) Option {
	return func(o *Options) {
		o.EnableLocalCache = enable
	}
}

// WithLocalCacheMaxSize 设置本地缓存最大条目数。
func WithLocalCacheMaxSize(size int) Option {
	return func(o *Options) {
		if size > 0 {
			o.LocalCacheMaxSize = size
		}
	}
}

// WithLocalCacheTTL 设置本地缓存 TTL。
// 控制平台数据在本地缓存中的过期时间。
func WithLocalCacheTTL(d time.Duration) Option {
	return func(o *Options) {
		if d > 0 {
			o.LocalCacheTTL = d
		}
	}
}

// WithSingleflight 设置是否启用 singleflight。
// 启用后，同一 tenantID 的并发请求只会触发一次实际请求。
func WithSingleflight(enable bool) Option {
	return func(o *Options) {
		o.EnableSingleflight = enable
	}
}

// WithBackgroundRefresh 设置是否启用后台刷新。
// 启用后，Token 即将过期时会自动在后台刷新，避免阻塞请求。
func WithBackgroundRefresh(enable bool) Option {
	return func(o *Options) {
		o.EnableBackgroundRefresh = enable
	}
}

// WithAutoRetryOn401 设置是否启用 401 自动重试。
// 启用后，Request 方法遇到 401 错误时会自动清除 Token 缓存并重试一次。
// 这有助于处理服务端吊销 Token 的场景。
func WithAutoRetryOn401(enable bool) Option {
	return func(o *Options) {
		o.EnableAutoRetryOn401 = enable
	}
}
