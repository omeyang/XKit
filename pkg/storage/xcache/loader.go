package xcache

import (
	"context"
	"fmt"
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
//
// # Singleflight 去重说明
//
// singleflight 去重仅基于 key（对于 LoadHash 是 key+field），不包含 ttl。
// 这意味着同一 key 的并发请求（即使 ttl 不同）只会触发一次回源，
// 最终缓存的 TTL 取决于首个请求的配置。
//
// 这是设计决策而非 bug：同一数据应使用一致的 TTL 配置。
// 如果业务确实需要为同一 key 使用不同的 TTL，应使用不同的 key 或禁用 singleflight。
type Loader interface {
	// Load 从缓存加载数据，未命中时调用 loader 函数回源。
	// 流程：缓存查询 → 未命中时回源 → 写入缓存 → 返回数据。
	// 内置 singleflight，同一 key 并发请求只回源一次。
	//
	// 空 key 会返回 ErrEmptyKey（fail-fast），空字符串在 Redis 中合法但几乎总是使用错误。
	//
	// loader 返回值说明：
	//   - (nil, nil): nil 值会被写入 Redis（存储为空字符串），后续 Get 返回 []byte("")，
	//     视为缓存命中。这等效于空值缓存，可防止缓存穿透。
	//   - (data, nil): 正常写入缓存。
	//   - (_, err): 不写入缓存，直接返回错误。
	//
	// ttl 说明（与 LoadHash 语义一致）：
	//   - ttl > 0: 设置指定过期时间。
	//   - ttl == 0: 不设置过期（key 永不过期）。
	//   - ttl < 0: 不写入缓存（仅回源，等同于不缓存）。
	//
	// 注意：singleflight 去重仅基于 key，不包含 ttl。
	// 同一 key 的并发请求（即使 ttl 不同）只会触发一次回源，
	// 最终缓存的 TTL 取决于首个请求的配置。
	Load(ctx context.Context, key string, loader LoadFunc, ttl time.Duration) ([]byte, error)

	// LoadHash 从 Redis Hash 加载数据，未命中时调用 loader 函数回源。
	// 适用于租户隔离场景，key 为 Hash 名称，field 为具体字段。
	// ttl 用于设置整个 Hash key 的过期时间。
	// 默认行为是每次写入时刷新 TTL，可通过 WithHashTTLRefresh(false) 改为仅首次写入时设置。
	//
	// 空 key 或空 field 会返回 ErrEmptyKey（fail-fast）。
	//
	// loader 返回值说明与 Load 相同：nil 值存储为空字符串，等效于空值缓存。
	//
	// ttl 说明（与 Load 语义一致）：
	//   - ttl > 0: 设置指定过期时间。
	//   - ttl == 0: 不设置过期（Hash key 永不过期）。
	//   - ttl < 0: 不写入缓存（仅回源，等同于不缓存）。
	//
	// 注意：singleflight 去重基于 key+field 组合，不包含 ttl。
	// 同一 key+field 的并发请求（即使 ttl 不同）只会触发一次回源。
	LoadHash(ctx context.Context, key, field string, loader LoadFunc, ttl time.Duration) ([]byte, error)
}

// =============================================================================
// Loader 配置选项
// =============================================================================

// RecommendedLoadTimeout 推荐的加载超时时间。
// 当使用 singleflight 时，建议设置此超时以避免 goroutine 泄漏。
const RecommendedLoadTimeout = 30 * time.Second

// RecommendedDistributedLockTTL 推荐的分布式锁 TTL。
// 设置为 LoadTimeout 的 1.5 倍，确保锁在加载完成前不会过期。
// 如果锁 TTL 等于或小于 LoadTimeout，当加载接近超时时锁可能刚好过期，
// 导致其他节点并发回源，降低防击穿效果。
const RecommendedDistributedLockTTL = 45 * time.Second

// CacheSetErrorHook 缓存写入失败回调钩子。
// 当缓存写入失败时调用，用于监控告警或自定义处理。
// 注意：此钩子在请求路径上同步执行，应避免耗时操作。
type CacheSetErrorHook func(ctx context.Context, key string, err error)

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
	// 默认为 RecommendedDistributedLockTTL (45s)，即 LoadTimeout 的 1.5 倍。
	//
	// 重要：DistributedLockTTL 必须大于 LoadTimeout，以确保锁在加载完成前不会过期。
	// 如果锁在加载完成前过期，其他节点可能并发回源，降低防击穿效果。
	// 当自定义 LoadTimeout 时，建议同时调整此值为 LoadTimeout * 1.5 或更大。
	DistributedLockTTL time.Duration

	// DistributedLockKeyPrefix 分布式锁 key 的前缀。
	// 此前缀用于区分 Loader 使用的锁与其他业务锁。
	// 注意：Redis.Lock() 会额外添加 "lock:" 前缀，最终 key 格式为 "lock:{DistributedLockKeyPrefix}{key}"。
	// 默认为 "loader:"，最终锁 key 为 "lock:loader:{key}"。
	//
	// 对于 LoadHash 操作，锁 key 使用长度前缀格式避免碰撞：
	// "lock:{DistributedLockKeyPrefix}{len(key)}:{key}:{field}"
	// 例如：key="user", field="profile" → "lock:loader:4:user:profile"
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

	// TTLJitter 控制写入缓存时 TTL 的随机抖动比例 (0.0-1.0)。
	// 启用后，实际 TTL 为 ttl * (1 + jitter * (rand - 0.5))，避免大量 key 同时过期（缓存雪崩）。
	// 例如 TTLJitter=0.1 时，1 小时 TTL 会被随机化到约 57-63 分钟。
	// 默认为 0（不抖动）。
	TTLJitter float64

	// HashTTLRefresh 控制 LoadHash 时是否刷新 Hash key 的 TTL。
	// 默认为 true（滑动过期）。
	HashTTLRefresh bool

	// OnCacheSetError 缓存写入失败回调钩子。
	// 当缓存写入失败时调用，用于监控告警或自定义处理。
	// 默认为 nil，仅记录日志。
	OnCacheSetError CacheSetErrorHook

	// Logger 用于记录警告和错误日志。
	// 默认使用 slog.Default()。
	Logger *slog.Logger
}

// LoaderOption 定义配置 Loader 的函数类型。
type LoaderOption func(*LoaderOptions)

// validate 校验配置一致性，在 NewLoader 构造期调用实现 fail-fast。
func (o *LoaderOptions) validate() error {
	if o.ExternalLock != nil && !o.EnableDistributedLock {
		return fmt.Errorf("%w: ExternalLock is set but EnableDistributedLock is false", ErrInvalidConfig)
	}
	if o.EnableDistributedLock && o.DistributedLockTTL <= 0 {
		return fmt.Errorf("%w: %w", ErrInvalidConfig, ErrInvalidLockTTL)
	}
	// 设计决策: 当 LoadTimeout > 0 且启用分布式锁时，强制 DistributedLockTTL > LoadTimeout。
	// 如果锁 TTL ≤ LoadTimeout，慢回源可能导致锁过期并引发并发回源，降低防击穿效果。
	// 当 LoadTimeout == 0（禁用超时）时跳过此检查，由用户自行保证 loadFn 不会无限阻塞。
	if o.EnableDistributedLock && o.LoadTimeout > 0 && o.DistributedLockTTL <= o.LoadTimeout {
		return fmt.Errorf("%w: DistributedLockTTL (%v) must be greater than LoadTimeout (%v)",
			ErrInvalidConfig, o.DistributedLockTTL, o.LoadTimeout)
	}
	return nil
}

// defaultLoaderOptions 返回默认的 Loader 配置。
func defaultLoaderOptions() *LoaderOptions {
	return &LoaderOptions{
		EnableSingleflight:       true,
		EnableDistributedLock:    false,
		DistributedLockTTL:       RecommendedDistributedLockTTL, // 45s，为 LoadTimeout 的 1.5 倍
		DistributedLockKeyPrefix: "loader:",                     // Redis.Lock() 会添加 "lock:" 前缀，最终为 "lock:loader:{key}"
		LoadTimeout:              RecommendedLoadTimeout,        // 默认启用超时保护，防止 goroutine 泄漏
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
//
// 重要：设置此选项后会自动启用分布式锁（无需额外调用 WithDistributedLock(true)）。
// 当 ExternalLock 非 nil 时，将优先使用外部锁，忽略内置的 Redis.Lock() 实现。
//
// 传入 nil 仅清除外部锁函数，不修改 EnableDistributedLock 标志。
// 这确保 WithDistributedLock(true) + WithExternalLock(nil) 仍使用内置锁，
// 而非意外禁用分布式锁。
//
// 锁函数签名与 Redis.Lock() 相同，便于适配各种锁实现：
//
//	func(ctx context.Context, key string, ttl time.Duration) (Unlocker, error)
func WithExternalLock(fn LockFunc) LoaderOption {
	return func(o *LoaderOptions) {
		o.ExternalLock = fn
		if fn != nil {
			// 设计决策: 设置外部锁时自动启用分布式锁，减少配置负担。
			// 传入 nil 时仅清除外部锁函数，不修改 EnableDistributedLock 标志，
			// 避免 WithDistributedLock(true) + WithExternalLock(nil) 意外禁用分布式锁。
			o.EnableDistributedLock = true
		}
	}
}

// WithLoadTimeout 设置单次加载的超时时间。
func WithLoadTimeout(timeout time.Duration) LoaderOption {
	return func(o *LoaderOptions) {
		o.LoadTimeout = timeout
	}
}

// MaxMaxRetryAttempts 是 MaxRetryAttempts 的上界，防止误配导致密集重试。
// 实际等待时间还受 DistributedLockTTL 约束，此上界仅作额外防御。
const MaxMaxRetryAttempts = 1000

// WithMaxRetryAttempts 设置等待锁释放时的最大重试次数。
// 超过此次数后直接回源，避免无限等待。
// n 会被钳位到 [1, MaxMaxRetryAttempts] 范围内；非正值被忽略（保持默认值）。
func WithMaxRetryAttempts(n int) LoaderOption {
	return func(o *LoaderOptions) {
		if n > 0 {
			if n > MaxMaxRetryAttempts {
				n = MaxMaxRetryAttempts
			}
			o.MaxRetryAttempts = n
		}
	}
}

// WithTTLJitter 设置缓存 TTL 的随机抖动比例，用于防止缓存雪崩。
// factor 范围为 [0.0, 1.0]，超出范围会被钳位。
//
// 实际 TTL 范围为 [ttl * (1 - factor/2), ttl * (1 + factor/2)]：
//   - factor=0.1 → TTL ±5%（例如 1h → 57~63min）
//   - factor=0.3 → TTL ±15%（例如 1h → 51~69min）
//   - factor=1.0 → TTL ±50%（例如 1h → 30~90min，极端值慎用）
//
// 默认为 0（不抖动）。
func WithTTLJitter(factor float64) LoaderOption {
	return func(o *LoaderOptions) {
		if factor < 0 {
			factor = 0
		}
		if factor > 1 {
			factor = 1
		}
		o.TTLJitter = factor
	}
}

// WithHashTTLRefresh 设置 LoadHash 时是否刷新 Hash key 的 TTL。
func WithHashTTLRefresh(refresh bool) LoaderOption {
	return func(o *LoaderOptions) {
		o.HashTTLRefresh = refresh
	}
}

// WithOnCacheSetError 设置缓存写入失败回调钩子。
// 当缓存写入失败时调用，用于监控告警或自定义处理。
// 注意：此钩子在请求路径上同步执行，应避免耗时操作。
func WithOnCacheSetError(hook CacheSetErrorHook) LoaderOption {
	return func(o *LoaderOptions) {
		o.OnCacheSetError = hook
	}
}

// WithLogger 设置自定义 Logger。
// 传入 nil 将禁用日志输出。
func WithLogger(logger *slog.Logger) LoaderOption {
	return func(o *LoaderOptions) {
		o.Logger = logger
	}
}
