package xcache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/redis/go-redis/v9"
)

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
		return nil, err
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
func NewLoader(cache Redis, opts ...LoaderOption) Loader {
	options := defaultLoaderOptions()
	for _, opt := range opts {
		opt(options)
	}

	return newLoader(cache, options)
}

// =============================================================================
// Redis 包装器实现
// =============================================================================

// redisWrapper 实现 Redis 接口，提供分布式锁等增值功能。
type redisWrapper struct {
	client  redis.UniversalClient
	options *RedisOptions
}

func (w *redisWrapper) Lock(ctx context.Context, key string, ttl time.Duration) (Unlocker, error) {
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
func (w *redisWrapper) lockWithRetry(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	for i := 0; i < w.options.LockRetryCount; i++ {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(w.options.LockRetryInterval):
		}

		acquired, err := w.tryLock(ctx, key, value, ttl)
		if err != nil {
			return false, err
		}
		if acquired {
			return true, nil
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
	return w.client.Close()
}

// =============================================================================
// Memory 包装器实现
// =============================================================================

// memoryWrapper 实现 Memory 接口，提供统计信息等增值功能。
type memoryWrapper struct {
	cache *ristretto.Cache[string, []byte]
	owned bool // 标记是否由本实例创建（需要负责关闭）
}

func (w *memoryWrapper) Stats() MemoryStats {
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

func (w *memoryWrapper) Close() {
	if w.owned {
		w.cache.Close()
	}
}

// =============================================================================
// 辅助函数
// =============================================================================

// generateLockValue 生成唯一的锁值。
func generateLockValue() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read 极少失败，使用时间戳作为后备
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(b)
}
