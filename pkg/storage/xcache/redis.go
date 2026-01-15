package xcache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Redis 接口定义
// =============================================================================

// Unlocker 释放分布式锁的函数类型。
// 调用后释放锁，允许其他持有者获取。
type Unlocker func(ctx context.Context) error

// Redis 定义 Redis 缓存接口。
// 只提供 go-redis 原生不具备的增值功能，基础操作请直接使用 Client()。
type Redis interface {
	// Lock 获取分布式锁。
	// 使用 SET NX EX 实现轻量级锁，返回解锁函数。
	// ttl 为锁的最大持有时间，超时自动释放。
	// 如果获取失败，返回 ErrLockFailed。
	Lock(ctx context.Context, key string, ttl time.Duration) (Unlocker, error)

	// Client 返回底层的 redis.UniversalClient。
	// 用于执行所有 Redis 操作。
	Client() redis.UniversalClient

	// Close 关闭缓存连接。
	Close() error
}

// =============================================================================
// Redis 配置选项
// =============================================================================

// RedisOptions 定义 Redis 缓存的配置选项。
type RedisOptions struct {
	// LockKeyPrefix 分布式锁 key 的前缀。
	// 默认为 "lock:"。
	LockKeyPrefix string

	// LockRetryInterval 获取锁失败后的重试间隔。
	// 默认为 0，表示不重试。
	LockRetryInterval time.Duration

	// LockRetryCount 获取锁失败后的最大重试次数。
	// 默认为 0，表示不重试。
	LockRetryCount int
}

// RedisOption 定义配置 Redis 缓存的函数类型。
type RedisOption func(*RedisOptions)

// defaultRedisOptions 返回默认的 Redis 配置。
func defaultRedisOptions() *RedisOptions {
	return &RedisOptions{
		LockKeyPrefix:     "lock:",
		LockRetryInterval: 0,
		LockRetryCount:    0,
	}
}

// WithLockKeyPrefix 设置分布式锁 key 前缀。
func WithLockKeyPrefix(prefix string) RedisOption {
	return func(o *RedisOptions) {
		o.LockKeyPrefix = prefix
	}
}

// WithLockRetry 设置锁重试策略。
//
// 重试行为说明：
//   - Lock() 首先会立即尝试获取锁（无等待）
//   - 若失败且配置了重试，则每隔 interval 重试一次，最多重试 count 次
//   - 因此总尝试次数为 1 + count，总等待时间最多为 interval * count
//
// 示例：WithLockRetry(100*time.Millisecond, 3) 表示：
//   - 首次立即尝试
//   - 失败后等待 100ms 重试（第 1 次重试）
//   - 再失败等待 100ms 重试（第 2 次重试）
//   - 再失败等待 100ms 重试（第 3 次重试）
//   - 若仍失败，返回 ErrLockFailed
func WithLockRetry(interval time.Duration, count int) RedisOption {
	return func(o *RedisOptions) {
		o.LockRetryInterval = interval
		o.LockRetryCount = count
	}
}
