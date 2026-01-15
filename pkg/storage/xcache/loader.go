package xcache

import (
	"context"
	"log/slog"
	"time"
)

// =============================================================================
// Loader 接口定义
// =============================================================================

// LoadFunc 定义从后端加载数据的函数类型。
type LoadFunc func(ctx context.Context) ([]byte, error)

// LockFunc 定义获取分布式锁的函数类型。
// 签名与 Redis.Lock() 相同，便于适配各种锁实现（如 xdlock）。
type LockFunc func(ctx context.Context, key string, ttl time.Duration) (Unlocker, error)

// Loader 定义 Cache-Aside 模式的加载器接口。
// 内置 singleflight 防止缓存击穿，可选分布式锁保护。
type Loader interface {
	// Load 从缓存加载数据，未命中时调用 loader 函数回源。
	// 流程：缓存查询 → 未命中时回源 → 写入缓存 → 返回数据。
	// 内置 singleflight，同一 key 并发请求只回源一次。
	Load(ctx context.Context, key string, loader LoadFunc, ttl time.Duration) ([]byte, error)

	// LoadHash 从 Redis Hash 加载数据，未命中时调用 loader 函数回源。
	// 适用于租户隔离场景，key 为 Hash 名称，field 为具体字段。
	// ttl 用于设置整个 Hash key 的过期时间。
	// 默认行为是每次写入时刷新 TTL，可通过 WithHashTTLRefresh(false) 改为仅首次写入时设置。
	LoadHash(ctx context.Context, key, field string, loader LoadFunc, ttl time.Duration) ([]byte, error)
}

// =============================================================================
// Loader 配置选项
// =============================================================================

// RecommendedLoadTimeout 推荐的加载超时时间。
// 当使用 singleflight 时，建议设置此超时以避免 goroutine 泄漏。
const RecommendedLoadTimeout = 30 * time.Second

// LoaderOptions 定义 Loader 的配置选项。
type LoaderOptions struct {
	// EnableSingleflight 是否启用 singleflight。
	// 启用后，同一 key 的并发请求只会触发一次回源。
	// 默认为 true。
	EnableSingleflight bool

	// EnableDistributedLock 是否启用分布式锁。
	// 启用后，跨实例的并发请求也只会触发一次回源。
	// 默认为 false，需要时显式开启。
	EnableDistributedLock bool

	// DistributedLockTTL 分布式锁的超时时间。
	// 默认为 RecommendedLoadTimeout (30s)。
	DistributedLockTTL time.Duration

	// DistributedLockKeyPrefix 分布式锁 key 的前缀。
	// 此前缀用于区分 Loader 使用的锁与其他业务锁。
	// 注意：Redis.Lock() 会额外添加 "lock:" 前缀，最终 key 格式为 "lock:{DistributedLockKeyPrefix}{key}"。
	// 默认为 "loader:"，最终锁 key 为 "lock:loader:{key}"。
	DistributedLockKeyPrefix string

	// ExternalLock 外部锁函数。
	// 如果设置且 EnableDistributedLock 为 true，将使用此函数获取锁，
	// 替代 Redis.Lock() 内置实现。
	//
	// 适用场景：
	//   - Redlock 多节点（使用 xdlock.NewRedisFactory）
	//   - etcd 分布式锁（使用 xdlock.NewEtcdFactory）
	//   - 需要 Extend 续期的长任务
	//
	// 默认为 nil，使用内置简单锁。
	ExternalLock LockFunc

	// LoadTimeout 单次加载的超时时间。
	// 默认为 RecommendedLoadTimeout (30s)，防止 singleflight goroutine 泄漏。
	//
	// 行为说明：
	//   - LoadTimeout > 0: 使用指定超时时间
	//   - LoadTimeout == 0: 禁用超时（需确保 loadFn 不会无限阻塞）
	//   - LoadTimeout < 0: 使用默认超时 (30s)
	//
	// 注意：在 singleflight 场景下，即使禁用超时，context 仍会脱离原始取消链，
	// 以避免首个调用者取消影响其他等待者。
	LoadTimeout time.Duration

	// MaxRetryAttempts 等待锁释放时的最大重试次数。
	// 等待时间受 MaxRetryAttempts 和 DistributedLockTTL 双重约束，取较小值。
	// 超过任一限制后直接回源，避免无限等待或并发回源。
	// 默认为 10。
	MaxRetryAttempts int

	// HashTTLRefresh 控制 LoadHash 时是否刷新 Hash key 的 TTL。
	// 默认为 true（滑动过期）。
	HashTTLRefresh bool

	// Logger 用于记录警告和错误日志。
	// 默认使用 slog.Default()。
	Logger *slog.Logger
}

// LoaderOption 定义配置 Loader 的函数类型。
type LoaderOption func(*LoaderOptions)

// defaultLoaderOptions 返回默认的 Loader 配置。
func defaultLoaderOptions() *LoaderOptions {
	return &LoaderOptions{
		EnableSingleflight:       true,
		EnableDistributedLock:    false,
		DistributedLockTTL:       RecommendedLoadTimeout,
		DistributedLockKeyPrefix: "loader:", // Redis.Lock() 会添加 "lock:" 前缀，最终为 "lock:loader:{key}"
		LoadTimeout:              RecommendedLoadTimeout, // 默认启用超时保护，防止 goroutine 泄漏
		MaxRetryAttempts:         10,
		HashTTLRefresh:           true,
		Logger:                   slog.Default(),
	}
}

// WithSingleflight 设置是否启用 singleflight。
func WithSingleflight(enable bool) LoaderOption {
	return func(o *LoaderOptions) {
		o.EnableSingleflight = enable
	}
}

// WithDistributedLock 设置是否启用分布式锁。
func WithDistributedLock(enable bool) LoaderOption {
	return func(o *LoaderOptions) {
		o.EnableDistributedLock = enable
	}
}

// WithDistributedLockTTL 设置分布式锁的超时时间。
func WithDistributedLockTTL(ttl time.Duration) LoaderOption {
	return func(o *LoaderOptions) {
		o.DistributedLockTTL = ttl
	}
}

// WithDistributedLockKeyPrefix 设置分布式锁 key 的前缀。
func WithDistributedLockKeyPrefix(prefix string) LoaderOption {
	return func(o *LoaderOptions) {
		o.DistributedLockKeyPrefix = prefix
	}
}

// WithExternalLock 设置外部锁函数，用于替代内置简单锁。
// 适用于 Redlock 多节点、etcd 分布式锁等复杂场景。
func WithExternalLock(fn LockFunc) LoaderOption {
	return func(o *LoaderOptions) {
		o.ExternalLock = fn
	}
}

// WithLoadTimeout 设置单次加载的超时时间。
func WithLoadTimeout(timeout time.Duration) LoaderOption {
	return func(o *LoaderOptions) {
		o.LoadTimeout = timeout
	}
}

// WithMaxRetryAttempts 设置等待锁释放时的最大重试次数。
// 超过此次数后直接回源，避免无限等待。
func WithMaxRetryAttempts(n int) LoaderOption {
	return func(o *LoaderOptions) {
		if n > 0 {
			o.MaxRetryAttempts = n
		}
	}
}

// WithHashTTLRefresh 设置 LoadHash 时是否刷新 Hash key 的 TTL。
func WithHashTTLRefresh(refresh bool) LoaderOption {
	return func(o *LoaderOptions) {
		o.HashTTLRefresh = refresh
	}
}

// WithLogger 设置自定义 Logger。
// 传入 nil 将禁用日志输出。
func WithLogger(logger *slog.Logger) LoaderOption {
	return func(o *LoaderOptions) {
		o.Logger = logger
	}
}
