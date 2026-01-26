package xcron

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// 包级预编译的 Lua 脚本，避免每次调用时重复编译
var (
	// unlockScript: 只有持有者才能删除锁
	unlockScript = redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`)

	// renewScript: 只有持有者才能续期
	renewScript = redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("PEXPIRE", KEYS[1], ARGV[2])
		else
			return 0
		end
	`)
)

// RedisLocker 基于 Redis 的分布式锁。
//
// 使用 Redis SETNX 命令实现互斥，支持 TTL 自动过期防止死锁。
// Unlock 和 Renew 使用 Lua 脚本确保只操作自己持有的锁。
//
// 适用于：
//   - 多副本部署
//   - 网络在线环境（可访问 Redis）
//
// 用法：
//
//	// 方式 1：直接创建
//	locker := xcron.NewRedisLocker(redisClient)
//
//	// 方式 2：复用 xcache 的客户端
//	cache := xcache.NewRedis(cfg)
//	locker := xcron.NewRedisLocker(cache.Client())
//
//	scheduler := xcron.New(xcron.WithLocker(locker))
type RedisLocker struct {
	client   redis.UniversalClient
	prefix   string // 锁 key 前缀
	identity string // 当前实例标识（用于日志，不再用于锁值）
}

// redisLockHandle 表示一次成功的 Redis 锁获取
type redisLockHandle struct {
	locker *RedisLocker
	key    string // 完整的 Redis key
	token  string // 唯一 token（每次获取独立生成）
}

// RedisLockerOption RedisLocker 配置选项
type RedisLockerOption func(*RedisLocker)

// WithRedisKeyPrefix 设置锁 key 前缀。
//
// 默认 "xcron:lock:"。用于区分不同应用的锁。
func WithRedisKeyPrefix(prefix string) RedisLockerOption {
	return func(l *RedisLocker) {
		l.prefix = prefix
	}
}

// WithRedisIdentity 设置实例标识。
//
// 默认使用 hostname:pid。用于日志记录和调试。
// 注意：实际锁值使用每次获取生成的唯一 token，而非此标识。
func WithRedisIdentity(identity string) RedisLockerOption {
	return func(l *RedisLocker) {
		l.identity = identity
	}
}

// NewRedisLocker 创建基于 Redis 的分布式锁。
//
// client 可以是 *redis.Client、*redis.ClusterClient 或 xcache.Client() 返回值。
// 如果 client 为 nil，会 panic。
func NewRedisLocker(client redis.UniversalClient, opts ...RedisLockerOption) *RedisLocker {
	if client == nil {
		panic("xcron: redis client cannot be nil")
	}

	l := &RedisLocker{
		client:   client,
		prefix:   "xcron:lock:",
		identity: defaultIdentity(),
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// TryLock 尝试获取锁（非阻塞）。
//
// 每次调用生成唯一 token，确保不同获取之间不会互相干扰。
// 使用 Redis SET key value NX PX ttl 命令原子性地获取锁。
func (l *RedisLocker) TryLock(ctx context.Context, key string, ttl time.Duration) (LockHandle, error) {
	fullKey := l.prefix + key
	// 每次获取生成唯一 token，包含实例标识便于调试
	token := fmt.Sprintf("%s:%s", l.identity, uuid.New().String())

	ok, err := l.client.SetNX(ctx, fullKey, token, ttl).Result()
	if err != nil {
		return nil, fmt.Errorf("xcron: redis setnx failed: %w", err)
	}
	if !ok {
		return nil, nil // 未获取到锁
	}

	return &redisLockHandle{
		locker: l,
		key:    fullKey,
		token:  token,
	}, nil
}

// Unlock 释放锁。
//
// 使用 Lua 脚本确保只释放自己持有的锁，防止误删其他实例的锁。
func (h *redisLockHandle) Unlock(ctx context.Context) error {
	result, err := unlockScript.Run(ctx, h.locker.client, []string{h.key}, h.token).Int()
	if err != nil {
		return fmt.Errorf("xcron: redis unlock failed: %w", err)
	}
	if result == 0 {
		return ErrLockNotHeld
	}
	return nil
}

// Renew 续期锁。
//
// 使用 Lua 脚本确保只续期自己持有的锁。
func (h *redisLockHandle) Renew(ctx context.Context, ttl time.Duration) error {
	result, err := renewScript.Run(ctx, h.locker.client, []string{h.key}, h.token, ttl.Milliseconds()).Int()
	if err != nil {
		return fmt.Errorf("xcron: redis renew failed: %w", err)
	}
	if result == 0 {
		return ErrLockNotHeld
	}
	return nil
}

// Key 返回锁的完整 key（含前缀）。
func (h *redisLockHandle) Key() string {
	return h.key
}

// Identity 返回当前实例标识。
func (l *RedisLocker) Identity() string {
	return l.identity
}

// Client 返回底层 Redis 客户端。
func (l *RedisLocker) Client() redis.UniversalClient {
	return l.client
}

// defaultIdentity 生成默认实例标识（hostname:pid）
func defaultIdentity() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	pid := os.Getpid()
	return fmt.Sprintf("%s:%d", hostname, pid)
}

// 确保 RedisLocker 实现了 Locker 接口
var _ Locker = (*RedisLocker)(nil)

// 确保 redisLockHandle 实现了 LockHandle 接口
var _ LockHandle = (*redisLockHandle)(nil)
