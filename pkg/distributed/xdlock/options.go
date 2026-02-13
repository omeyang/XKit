package xdlock

import (
	"context"
	"strings"
	"time"
)

// validateKey 验证锁 key 是否有效。
func validateKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return ErrEmptyKey
	}
	return nil
}

// =============================================================================
// etcd 工厂选项
// =============================================================================

// EtcdFactoryOption 定义 etcd 工厂的配置选项。
type EtcdFactoryOption func(*etcdFactoryOptions)

// etcdFactoryOptions etcd 工厂配置。
type etcdFactoryOptions struct {
	TTL     int             // Session TTL（秒），默认 60
	Context context.Context // Session 上下文，默认 context.Background()
}

// defaultEtcdFactoryOptions 返回默认的 etcd 工厂配置。
func defaultEtcdFactoryOptions() *etcdFactoryOptions {
	return &etcdFactoryOptions{
		TTL:     60,
		Context: context.Background(),
	}
}

// WithEtcdTTL 设置 Session TTL（秒）。
// TTL 决定了 Session 的自动续期间隔和锁的最大持有时间。
// 默认值：60 秒。
//
// 建议：
//   - 短任务（< 10s）：TTL = 15-30
//   - 普通任务（< 1min）：TTL = 60（默认）
//   - 长任务（> 1min）：TTL = 120-300
func WithEtcdTTL(ttl int) EtcdFactoryOption {
	return func(o *etcdFactoryOptions) {
		if ttl > 0 {
			o.TTL = ttl
		}
	}
}

// WithEtcdContext 设置 Session 的上下文。
// 当 context 取消时，Session 会自动关闭，所有基于该 Session 的锁都会失效。
// 默认值：context.Background()。
func WithEtcdContext(ctx context.Context) EtcdFactoryOption {
	return func(o *etcdFactoryOptions) {
		if ctx != nil {
			o.Context = ctx
		}
	}
}

// =============================================================================
// Mutex 选项（通用 + 后端专用）
// =============================================================================

// MutexOption 定义锁实例的配置选项。
type MutexOption func(*mutexOptions)

// mutexOptions 锁实例配置。
type mutexOptions struct {
	// 通用选项
	KeyPrefix string // Key 前缀，默认 "lock:"

	// Redis 专用选项
	Expiry         time.Duration // 过期时间，默认 8s
	Tries          int           // 重试次数，默认 32
	RetryDelay     time.Duration // 重试延迟，默认 200ms
	RetryDelayFunc func(tries int) time.Duration
	DriftFactor    float64 // 时钟漂移因子，默认 0.01
	TimeoutFactor  float64 // 超时因子，默认 0.05
	GenValueFunc   func() (string, error)
	FailFast       bool // 快速失败，默认 false
	ShufflePools   bool // 随机打乱 Pool 顺序，默认 false
	SetNXOnExtend  bool // Extend 时使用 SETNX，默认 false
}

// defaultMutexOptions 返回默认的锁实例配置。
func defaultMutexOptions() *mutexOptions {
	return &mutexOptions{
		KeyPrefix:     "lock:",
		Expiry:        8 * time.Second,
		Tries:         32,
		RetryDelay:    200 * time.Millisecond,
		DriftFactor:   0.01,
		TimeoutFactor: 0.05,
		FailFast:      false,
		ShufflePools:  false,
		SetNXOnExtend: false,
	}
}

// WithKeyPrefix 设置锁 key 的前缀。
// 最终 key = prefix + key。
// 默认值："lock:"。
//
// 示例：
//
//	handle, _ := factory.TryLock(ctx, "my-resource", xdlock.WithKeyPrefix("myapp:"))
//	// 实际 key: "myapp:my-resource"
func WithKeyPrefix(prefix string) MutexOption {
	return func(o *mutexOptions) {
		o.KeyPrefix = prefix
	}
}

// =============================================================================
// Redis 专用选项
// =============================================================================

// WithExpiry 设置锁的过期时间。
// 默认值：8 秒。
//
// 注意：过期时间应大于业务执行时间，否则需要调用 Extend() 续期。
func WithExpiry(d time.Duration) MutexOption {
	return func(o *mutexOptions) {
		if d > 0 {
			o.Expiry = d
		}
	}
}

// WithTries 设置获取锁的最大重试次数。
// 默认值：32。
//
// 设置为 1 表示不重试（类似 TryLock）。
func WithTries(n int) MutexOption {
	return func(o *mutexOptions) {
		if n > 0 {
			o.Tries = n
		}
	}
}

// WithRetryDelay 设置重试延迟。
// 默认值：200ms。
//
// 每次重试前会等待此时间。
func WithRetryDelay(d time.Duration) MutexOption {
	return func(o *mutexOptions) {
		if d > 0 {
			o.RetryDelay = d
		}
	}
}

// WithRetryDelayFunc 设置自定义重试延迟函数。
// tries 参数从 1 开始。
//
// 示例（指数退避）：
//
//	xdlock.WithRetryDelayFunc(func(tries int) time.Duration {
//	    return time.Duration(tries) * 100 * time.Millisecond
//	})
func WithRetryDelayFunc(fn func(tries int) time.Duration) MutexOption {
	return func(o *mutexOptions) {
		if fn != nil {
			o.RetryDelayFunc = fn
		}
	}
}

// WithDriftFactor 设置时钟漂移因子。
// 用于 Redlock 算法中补偿时钟漂移。
// 默认值：0.01。值必须 > 0，0.0 会破坏 Redlock 时钟漂移补偿。
func WithDriftFactor(f float64) MutexOption {
	return func(o *mutexOptions) {
		if f > 0 {
			o.DriftFactor = f
		}
	}
}

// WithTimeoutFactor 设置超时因子。
// 用于计算单个节点的超时时间。
// 默认值：0.05。值必须 > 0，0.0 会导致节点超时立即触发。
func WithTimeoutFactor(f float64) MutexOption {
	return func(o *mutexOptions) {
		if f > 0 {
			o.TimeoutFactor = f
		}
	}
}

// WithGenValueFunc 设置自定义锁值生成函数。
// 默认使用随机生成的唯一值。
//
// 注意：生成的值必须全局唯一，否则可能导致锁冲突。
func WithGenValueFunc(fn func() (string, error)) MutexOption {
	return func(o *mutexOptions) {
		if fn != nil {
			o.GenValueFunc = fn
		}
	}
}

// WithFailFast 设置快速失败模式。
// 开启后，如果任意节点获取锁失败，立即返回错误。
// 默认值：false。
//
// 适用场景：对延迟敏感的场景。
func WithFailFast(b bool) MutexOption {
	return func(o *mutexOptions) {
		o.FailFast = b
	}
}

// WithShufflePools 设置是否随机打乱 Pool 顺序。
// 开启后，每次获取锁时会随机打乱节点顺序。
// 默认值：false。
//
// 适用场景：负载均衡。
func WithShufflePools(b bool) MutexOption {
	return func(o *mutexOptions) {
		o.ShufflePools = b
	}
}

// WithSetNXOnExtend 设置 Extend 时使用 SETNX。
// 开启后，如果锁已过期，Extend 会尝试重新获取。
// 默认值：false。
func WithSetNXOnExtend(b bool) MutexOption {
	return func(o *mutexOptions) {
		o.SetNXOnExtend = b
	}
}
