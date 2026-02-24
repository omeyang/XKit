package xcache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/redis/go-redis/v9"
)

// lockValueCounter 用于在 crypto/rand 失败时生成唯一的锁值后备。
var lockValueCounter atomic.Uint64

// unlockScript 是释放分布式锁的 Lua 脚本。
// 返回 1 表示成功释放，0 表示锁已不属于当前持有者（过期或被抢走）。
var unlockScript = redis.NewScript(`
	if redis.call("GET", KEYS[1]) == ARGV[1] then
		return redis.call("DEL", KEYS[1])
	else
		return 0
	end
`)

// =============================================================================
// 工厂函数
// =============================================================================

// NewRedis 创建 Redis 缓存实例。
// client 必须是已初始化的 redis.UniversalClient。
func NewRedis(client redis.UniversalClient, opts ...RedisOption) (Redis, error) {
	if client == nil {
		return nil, ErrNilClient
	}

	options := defaultRedisOptions()
	for _, opt := range opts {
		opt(options)
	}

	return &redisWrapper{
		client:  client,
		options: options,
	}, nil
}

// NewMemory 创建内存缓存实例。
func NewMemory(opts ...MemoryOption) (Memory, error) {
	options := defaultMemoryOptions()
	for _, opt := range opts {
		opt(options)
	}

	cache, err := ristretto.NewCache(&ristretto.Config[string, []byte]{
		NumCounters: options.NumCounters,
		MaxCost:     options.MaxCost,
		BufferItems: options.BufferItems,
		Metrics:     true, // 启用 Metrics 以支持 Stats() 方法
	})
	if err != nil {
		return nil, fmt.Errorf("xcache: create memory cache: %w", err)
	}

	return &memoryWrapper{
		cache: cache,
		owned: true,
	}, nil
}

// NewMemoryFromClient 从已有的 ristretto.Cache 创建内存缓存实例。
// 用于复用已有的 ristretto 实例。
func NewMemoryFromClient(client *ristretto.Cache[string, []byte]) (Memory, error) {
	if client == nil {
		return nil, ErrNilClient
	}
	if client.Metrics == nil {
		return nil, ErrMetricsDisabled
	}
	return &memoryWrapper{
		cache: client,
		owned: false,
	}, nil
}

// NewLoader 创建 Cache-Aside 加载器。
// cache 必须是已初始化的 Redis 缓存实例。
// 提供 singleflight、分布式锁、Cache-Aside 等功能。
//
// 构造期校验（fail-fast）：
//   - cache 为 nil → 返回 ErrNilClient
//   - EnableDistributedLock 为 true 但 DistributedLockTTL ≤ 0 → 返回 ErrInvalidConfig
//   - ExternalLock 非 nil 但 EnableDistributedLock 被禁用 → 返回 ErrInvalidConfig
//
// 生命周期说明：
//   - Loader 不持有需要释放的资源，无需调用 Close
//   - 内部使用的 singleflight.Group 是无状态的，会随 Loader 一起被 GC 回收
//   - 底层 Redis 缓存的生命周期由调用方管理，Loader 不会关闭传入的 cache
//
// 使用示例：
//
//	cache, _ := xcache.NewRedis(redisClient)
//	loader, _ := xcache.NewLoader(cache, xcache.WithDistributedLock(true))
//	// 使用 loader...
//	// 无需关闭 loader，只需在适当时机关闭 cache
//	cache.Close()
func NewLoader(cache Redis, opts ...LoaderOption) (Loader, error) {
	if cache == nil {
		return nil, ErrNilClient
	}

	options := defaultLoaderOptions()
	for _, opt := range opts {
		opt(options)
	}

	if err := options.validate(); err != nil {
		return nil, err
	}

	return newLoader(cache, options), nil
}

// =============================================================================
// Redis 包装器实现
// =============================================================================

// redisWrapper 实现 Redis 接口，提供分布式锁等增值功能。
type redisWrapper struct {
	client  redis.UniversalClient
	options *RedisOptions
	closed  atomic.Bool
}

func (w *redisWrapper) Lock(ctx context.Context, key string, ttl time.Duration) (Unlocker, error) {
	if w.closed.Load() {
		return nil, ErrClosed
	}
	if key == "" {
		return nil, ErrEmptyKey
	}
	if ttl <= 0 {
		return nil, ErrInvalidLockTTL
	}

	lockKey := w.options.LockKeyPrefix + key
	lockValue := generateLockValue()

	// 尝试获取锁
	acquired, err := w.tryLock(ctx, lockKey, lockValue, ttl)
	if err != nil {
		return nil, err
	}

	if !acquired {
		// 如果配置了重试，进行重试
		if w.options.LockRetryCount > 0 && w.options.LockRetryInterval > 0 {
			acquired, err = w.lockWithRetry(ctx, lockKey, lockValue, ttl)
			if err != nil {
				return nil, err
			}
		}
	}

	if !acquired {
		return nil, ErrLockFailed
	}

	// 返回解锁函数
	unlocker := func(ctx context.Context) error {
		return w.unlock(ctx, lockKey, lockValue)
	}
	return unlocker, nil
}

// tryLock 尝试获取锁（单次）。
func (w *redisWrapper) tryLock(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	result, err := w.client.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return false, err
	}
	return result, nil
}

// lockWithRetry 带重试的获取锁。
// 使用可复用的 Timer 避免 time.After 的泄漏问题。
//
// 重试行为说明：
// 此函数在 Lock() 中首次 tryLock 失败后被调用，因此：
//   - 首次尝试：在 Lock() 中立即执行（无等待）
//   - 后续重试：每次等待 LockRetryInterval 后执行
//   - 总尝试次数：1（首次）+ LockRetryCount（重试）
func (w *redisWrapper) lockWithRetry(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	timer := time.NewTimer(w.options.LockRetryInterval)
	defer timer.Stop()

	for i := 0; i < w.options.LockRetryCount; i++ {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-timer.C:
		}

		acquired, err := w.tryLock(ctx, key, value, ttl)
		if err != nil {
			return false, err
		}
		if acquired {
			return true, nil
		}

		// 重置 timer 用于下次迭代（最后一次迭代跳过，避免无消费的 timer）
		if i < w.options.LockRetryCount-1 {
			timer.Reset(w.options.LockRetryInterval)
		}
	}
	return false, nil
}

// unlock 释放锁。使用 Lua 脚本确保只释放自己持有的锁。
// 返回 ErrLockExpired 表示锁已过期或被其他持有者抢走。
func (w *redisWrapper) unlock(ctx context.Context, key, value string) error {
	result, err := unlockScript.Run(ctx, w.client, []string{key}, value).Int64()
	if err != nil {
		return err
	}
	if result == 0 {
		return ErrLockExpired
	}
	return nil
}

func (w *redisWrapper) Client() redis.UniversalClient {
	return w.client
}

func (w *redisWrapper) Close() error {
	if !w.closed.CompareAndSwap(false, true) {
		return ErrClosed
	}
	return w.client.Close()
}

// =============================================================================
// Memory 包装器实现
// =============================================================================

// memoryWrapper 实现 Memory 接口，提供统计信息等增值功能。
type memoryWrapper struct {
	cache  *ristretto.Cache[string, []byte]
	owned  bool // 标记是否由本实例创建（需要负责关闭）
	closed atomic.Bool
}

func (w *memoryWrapper) Stats() MemoryStats {
	if w.closed.Load() {
		return MemoryStats{}
	}
	metrics := w.cache.Metrics
	if metrics == nil {
		return MemoryStats{}
	}

	hits := metrics.Hits()
	misses := metrics.Misses()
	total := hits + misses

	var hitRatio float64
	if total > 0 {
		hitRatio = float64(hits) / float64(total)
	}

	return MemoryStats{
		Hits:        hits,
		Misses:      misses,
		HitRatio:    hitRatio,
		KeysAdded:   metrics.KeysAdded(),
		KeysEvicted: metrics.KeysEvicted(),
		CostAdded:   metrics.CostAdded(),
		CostEvicted: metrics.CostEvicted(),
	}
}

func (w *memoryWrapper) Client() *ristretto.Cache[string, []byte] {
	return w.cache
}

func (w *memoryWrapper) Wait() {
	w.cache.Wait()
}

func (w *memoryWrapper) Close() error {
	if !w.closed.CompareAndSwap(false, true) {
		return ErrClosed
	}
	if w.owned {
		w.cache.Close()
	}
	return nil
}

// =============================================================================
// 辅助函数
// =============================================================================

// hostIdentifier 缓存的主机标识符，用于锁值后备方案。
// 只计算一次以避免重复系统调用开销。
var hostIdentifier = getHostIdentifier()

// getHostIdentifier 获取主机标识符。
// 优先使用主机名，失败时使用固定前缀 + 随机后缀。
func getHostIdentifier() string {
	hostname, err := os.Hostname()
	if err == nil && hostname != "" {
		return hostname
	}
	// 主机名获取失败，使用固定前缀 + 启动时的纳秒时间戳
	return fmt.Sprintf("unknown-%d", time.Now().UnixNano())
}

// generateLockValue 生成唯一的锁值。
// 使用 crypto/rand 生成 16 字节随机数，确保锁值的唯一性。
// 在极少数 crypto/rand 失败的情况下，使用主机标识符 + 进程 ID + 时间戳 + 递增计数器作为后备。
//
// 后备方案的唯一性保证：
//   - hostIdentifier: 区分不同主机
//   - os.Getpid(): 区分同一主机上的不同进程
//   - time.Now().UnixNano(): 区分同一进程内的不同时间点
//   - lockValueCounter: 区分同一纳秒内的多次调用（理论上不可能，但作为额外保险）
func generateLockValue() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read 极少失败，使用多重因素确保唯一性
		counter := lockValueCounter.Add(1)
		return fmt.Sprintf("%s-%d-%d-%d", hostIdentifier, os.Getpid(), time.Now().UnixNano(), counter)
	}
	return hex.EncodeToString(b)
}
