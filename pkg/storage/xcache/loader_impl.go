package xcache

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

const (
	// 退避策略参数
	baseBackoff    = 50 * time.Millisecond  // 基础退避时间
	maxBackoff     = 500 * time.Millisecond // 最大退避时间
	jitterFraction = 0.3                    // 抖动比例 (0-1)

	// defaultOperationTimeout 是移除调用方取消信号后的默认操作超时。
	// 用于防止 Redis 挂起时 goroutine 永久阻塞。
	defaultOperationTimeout = 30 * time.Second

	// 浮点数转换常量
	floatBits  = 53
	floatScale = 1.0 / (1 << floatBits)
)

// randomFloat64 返回 [0.0, 1.0) 范围内的随机浮点数。
// 使用 crypto/rand 确保高质量随机数。
func randomFloat64() float64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand 失败表示系统随机数源不可用，返回 0 作为安全默认值
		return 0
	}
	return float64(binary.LittleEndian.Uint64(buf[:])>>11) * floatScale
}

// detachedCtx 是一个脱离原始 context 取消链的 context。
// 它保留原始 context 的 Value，但不继承其 Done/Err/Deadline。
// 这用于 singleflight 和 unlock 场景，避免首个调用者取消影响其他等待者。
type detachedCtx struct {
	context.Context
}

func (c detachedCtx) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c detachedCtx) Done() <-chan struct{}       { return nil }
func (c detachedCtx) Err() error                  { return nil }

// contextDetached 创建一个脱离原始取消链的 context。
// 返回的 context 保留原始 context 的 Value，但不继承其取消信号。
func contextDetached(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return detachedCtx{Context: ctx}
}

// contextWithIndependentTimeout 创建一个脱离原始取消链但有独立超时的 context。
// 这解决了两个问题：
//  1. 首个调用者取消不影响其他等待者（singleflight 场景）
//  2. 锁释放不受调用方 ctx 取消影响（unlock 场景）
//  3. 添加独立超时防止 Redis 挂起时永久阻塞
//
// timeout 行为：
//   - timeout == 0: 禁用超时（仍脱离原始取消链，需确保 loadFn 不会无限阻塞）
//   - timeout < 0: 使用 defaultOperationTimeout (30s)
//   - timeout > 0: 使用指定超时时间
func contextWithIndependentTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	// 先脱离原始取消链
	detached := contextDetached(ctx)

	// timeout == 0 表示用户显式禁用超时
	if timeout == 0 {
		return context.WithCancel(detached)
	}

	// timeout < 0 表示使用默认超时
	if timeout < 0 {
		timeout = defaultOperationTimeout
	}

	return context.WithTimeout(detached, timeout)
}

// applyLoadTimeout 根据 LoadTimeout 配置创建带超时的 context。
// timeout 行为与 contextWithIndependentTimeout 一致：
//   - timeout == 0: 禁用超时，直接返回原 ctx
//   - timeout < 0: 使用 defaultOperationTimeout (30s)
//   - timeout > 0: 使用指定超时时间
func applyLoadTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout == 0 {
		return ctx, func() {} // 禁用超时
	}
	if timeout < 0 {
		timeout = defaultOperationTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

// hashFieldKey 生成 Hash field 的唯一标识 key。
// 使用长度前缀格式避免碰撞："{len(key)}:{key}:{field}"
// 例如：key="user", field="a:b" → "4:user:a:b"
//
//	key="user:a", field="b" → "6:user:a:b"
//
// 这样即使 key 或 field 中包含 ":"，也不会产生歧义。
func hashFieldKey(key, field string) string {
	return fmt.Sprintf("%d:%s:%s", len(key), key, field)
}

// =============================================================================
// Loader 实现
// =============================================================================

// loader 实现 Loader 接口。
type loader struct {
	cache   Redis
	options *LoaderOptions
	group   singleflight.Group
}

// newLoader 创建 Loader 实例。
func newLoader(cache Redis, options *LoaderOptions) *loader {
	return &loader{
		cache:   cache,
		options: options,
	}
}

// Load 从缓存加载数据，未命中时调用 loader 函数回源。
func (l *loader) Load(ctx context.Context, key string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	if l.cache == nil {
		return nil, ErrNilClient
	}
	if loadFn == nil {
		return nil, ErrNilLoader
	}

	// 1. 尝试从缓存获取
	value, err := l.cache.Client().Get(ctx, key).Bytes()
	if err == nil {
		return value, nil
	}

	// 如果不是 key 不存在的错误，回源兜底
	if !errors.Is(err, redis.Nil) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return l.loadAndCache(ctx, key, loadFn, ttl)
	}

	// 2. 缓存未命中，使用 singleflight 或直接回源
	if l.options.EnableSingleflight {
		return l.loadWithSingleflight(ctx, key, loadFn, ttl)
	}

	return l.loadWithDistLock(ctx, key, loadFn, ttl)
}

// LoadHash 从 Redis Hash 加载数据，未命中时调用 loader 函数回源。
func (l *loader) LoadHash(ctx context.Context, key, field string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	if l.cache == nil {
		return nil, ErrNilClient
	}
	if loadFn == nil {
		return nil, ErrNilLoader
	}

	// 1. 尝试从 Hash 获取
	value, err := l.cache.Client().HGet(ctx, key, field).Bytes()
	if err == nil {
		return value, nil
	}

	// 如果不是 field 不存在的错误，回源兜底
	if !errors.Is(err, redis.Nil) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return l.loadHashAndCache(ctx, key, field, loadFn, ttl)
	}

	// 2. 缓存未命中，使用 singleflight 或直接回源
	// 使用长度前缀格式生成唯一 key，避免 key/field 中包含 ":" 导致碰撞
	sfKey := hashFieldKey(key, field)
	if l.options.EnableSingleflight {
		return l.loadHashWithSingleflight(ctx, key, field, sfKey, loadFn, ttl)
	}

	return l.loadHashWithDistLock(ctx, key, field, loadFn, ttl)
}

// =============================================================================
// 内部实现
// =============================================================================

// loadWithSingleflight 使用 singleflight 加载。
// 使用 DoChan 支持每个调用者独立的 context 取消，同时不影响其他等待者。
func (l *loader) loadWithSingleflight(ctx context.Context, key string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	// 使用独立的 ctx，避免首个调用者取消或超时影响实际加载操作
	// 同时设置独立超时，防止 Redis 挂起时永久阻塞
	sfCtx, sfCancel := contextWithIndependentTimeout(ctx, l.options.LoadTimeout)
	defer sfCancel()

	ch := l.group.DoChan(key, func() (any, error) {
		return l.loadWithDistLock(sfCtx, key, loadFn, ttl)
	})

	// 每个调用者独立等待，可以各自超时
	select {
	case <-ctx.Done():
		// 原始 ctx 取消，返回错误，但后台加载继续供其他等待者使用
		return nil, ctx.Err()
	case result := <-ch:
		if result.Err != nil {
			return nil, result.Err
		}
		value, ok := result.Val.([]byte)
		if !ok {
			return nil, errors.New("xcache: unexpected result type from singleflight")
		}
		return value, nil
	}
}

// loadWithDistLock 可选使用分布式锁加载。
func (l *loader) loadWithDistLock(ctx context.Context, key string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	// 再次检查缓存（double-check）
	if value, done, err := l.checkCacheGet(ctx, key, loadFn, ttl); done {
		return value, err
	}

	// 如果启用分布式锁
	if l.options.EnableDistributedLock {
		return l.loadWithLock(ctx, key, loadFn, ttl)
	}

	return l.loadAndCache(ctx, key, loadFn, ttl)
}

// loadWithLock 使用分布式锁加载数据。
func (l *loader) loadWithLock(ctx context.Context, key string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	lockKey := l.options.DistributedLockKeyPrefix + key
	unlock, lockErr := l.acquireLock(ctx, lockKey)
	if lockErr != nil {
		return l.handleLockError(lockErr, lockKey, func() ([]byte, error) {
			return l.waitAndRetryGet(ctx, key, loadFn, ttl)
		})
	}

	// 解锁使用独立 ctx，不受调用方取消影响，但有超时保护
	unlockCtx, unlockCancel := contextWithIndependentTimeout(ctx, l.options.DistributedLockTTL)
	defer func() {
		defer unlockCancel()
		if unlockErr := unlock(unlockCtx); unlockErr != nil {
			l.logUnlockError(lockKey, unlockErr)
		}
	}()

	// 获取锁后再次检查缓存
	if value, done, err := l.checkCacheGet(ctx, key, loadFn, ttl); done {
		return value, err
	}

	return l.loadAndCache(ctx, key, loadFn, ttl)
}

// checkCacheGet 检查缓存，返回 (value, done, error)。
// 如果 done 为 true，表示已有结果（缓存命中或错误），调用方应直接返回。
func (l *loader) checkCacheGet(ctx context.Context, key string, loadFn LoadFunc, ttl time.Duration) ([]byte, bool, error) {
	value, err := l.cache.Client().Get(ctx, key).Bytes()
	if err == nil {
		return value, true, nil
	}
	if !errors.Is(err, redis.Nil) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, true, ctxErr
		}
		val, loadErr := l.loadAndCache(ctx, key, loadFn, ttl)
		return val, true, loadErr
	}
	return nil, false, nil
}

// handleLockError 处理锁获取错误，返回适当的结果或调用 fallback。
func (l *loader) handleLockError(lockErr error, lockKey string, fallback func() ([]byte, error)) ([]byte, error) {
	// context 错误直接返回
	if errors.Is(lockErr, context.Canceled) || errors.Is(lockErr, context.DeadlineExceeded) {
		return nil, lockErr
	}
	// 配置错误直接返回，不应被静默忽略
	if errors.Is(lockErr, ErrInvalidLockTTL) {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfig, lockErr)
	}
	// 区分锁竞争和运行时错误
	if !errors.Is(lockErr, ErrLockFailed) {
		// 运行时错误（Redis 异常等），记录日志后降级
		l.logWarn("xcache: acquire lock error, waiting for retry",
			"key", lockKey, "error", lockErr)
	}
	// 等待后重试获取缓存（保守策略，避免回源风暴）
	return fallback()
}

// loadAndCache 加载数据并写入缓存。
func (l *loader) loadAndCache(ctx context.Context, key string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	// 应用超时
	loadCtx, cancel := applyLoadTimeout(ctx, l.options.LoadTimeout)
	defer cancel()

	// 回源加载
	value, err := loadFn(loadCtx)
	if err != nil {
		return nil, err
	}

	// 写入缓存（best-effort，失败不影响业务返回）
	if setErr := l.cache.Client().Set(ctx, key, value, ttl).Err(); setErr != nil {
		l.logWarn("xcache: cache set failed", "key", key, "error", setErr)
		l.onCacheSetError(ctx, key, setErr)
	}

	return value, nil
}

// loadHashWithSingleflight 使用 singleflight 加载 Hash。
// 使用 DoChan 支持每个调用者独立的 context 取消，同时不影响其他等待者。
func (l *loader) loadHashWithSingleflight(ctx context.Context, key, field, sfKey string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	// 使用独立的 ctx，避免首个调用者取消或超时影响实际加载操作
	// 同时设置独立超时，防止 Redis 挂起时永久阻塞
	sfCtx, sfCancel := contextWithIndependentTimeout(ctx, l.options.LoadTimeout)
	defer sfCancel()

	ch := l.group.DoChan(sfKey, func() (any, error) {
		return l.loadHashWithDistLock(sfCtx, key, field, loadFn, ttl)
	})

	// 每个调用者独立等待，可以各自超时
	select {
	case <-ctx.Done():
		// 原始 ctx 取消，返回错误，但后台加载继续供其他等待者使用
		return nil, ctx.Err()
	case result := <-ch:
		if result.Err != nil {
			return nil, result.Err
		}
		value, ok := result.Val.([]byte)
		if !ok {
			return nil, errors.New("xcache: unexpected result type from singleflight")
		}
		return value, nil
	}
}

// loadHashWithDistLock 可选使用分布式锁加载 Hash。
func (l *loader) loadHashWithDistLock(ctx context.Context, key, field string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	// 再次检查缓存（double-check）
	if value, done, err := l.checkCacheHGet(ctx, key, field, loadFn, ttl); done {
		return value, err
	}

	// 如果启用分布式锁
	if l.options.EnableDistributedLock {
		return l.loadHashWithLock(ctx, key, field, loadFn, ttl)
	}

	return l.loadHashAndCache(ctx, key, field, loadFn, ttl)
}

// loadHashWithLock 使用分布式锁加载 Hash 数据。
func (l *loader) loadHashWithLock(ctx context.Context, key, field string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	// 使用长度前缀格式生成唯一锁 key，避免 key/field 中包含 ":" 导致碰撞
	lockKey := l.options.DistributedLockKeyPrefix + hashFieldKey(key, field)
	unlock, lockErr := l.acquireLock(ctx, lockKey)
	if lockErr != nil {
		return l.handleLockError(lockErr, lockKey, func() ([]byte, error) {
			return l.waitAndRetryHGet(ctx, key, field, loadFn, ttl)
		})
	}

	// 解锁使用独立 ctx，不受调用方取消影响，但有超时保护
	unlockCtx, unlockCancel := contextWithIndependentTimeout(ctx, l.options.DistributedLockTTL)
	defer func() {
		defer unlockCancel()
		if unlockErr := unlock(unlockCtx); unlockErr != nil {
			l.logUnlockError(lockKey, unlockErr)
		}
	}()

	// 获取锁后再次检查缓存
	if value, done, err := l.checkCacheHGet(ctx, key, field, loadFn, ttl); done {
		return value, err
	}

	return l.loadHashAndCache(ctx, key, field, loadFn, ttl)
}

// checkCacheHGet 检查 Hash 缓存，返回 (value, done, error)。
// 如果 done 为 true，表示已有结果（缓存命中或错误），调用方应直接返回。
func (l *loader) checkCacheHGet(ctx context.Context, key, field string, loadFn LoadFunc, ttl time.Duration) ([]byte, bool, error) {
	value, err := l.cache.Client().HGet(ctx, key, field).Bytes()
	if err == nil {
		return value, true, nil
	}
	if !errors.Is(err, redis.Nil) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, true, ctxErr
		}
		val, loadErr := l.loadHashAndCache(ctx, key, field, loadFn, ttl)
		return val, true, loadErr
	}
	return nil, false, nil
}

// loadHashAndCache 加载数据并写入 Hash。
func (l *loader) loadHashAndCache(ctx context.Context, key, field string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	// 应用超时
	loadCtx, cancel := applyLoadTimeout(ctx, l.options.LoadTimeout)
	defer cancel()

	// 回源加载
	value, err := loadFn(loadCtx)
	if err != nil {
		return nil, err
	}

	// 写入缓存（best-effort，失败不影响业务返回）
	hashKey := hashFieldKey(key, field)
	if setErr := l.cache.Client().HSet(ctx, key, field, value).Err(); setErr != nil {
		l.logWarn("xcache: hash set failed", "key", key, "field", field, "error", setErr)
		l.onCacheSetError(ctx, hashKey, setErr)
	} else if ttl > 0 {
		if l.options.HashTTLRefresh {
			if expireErr := l.cache.Client().Expire(ctx, key, ttl).Err(); expireErr != nil {
				l.logWarn("xcache: hash expire failed", "key", key, "ttl", ttl, "error", expireErr)
			}
		} else {
			currentTTL, ttlErr := l.cache.Client().TTL(ctx, key).Result()
			if ttlErr != nil {
				l.logWarn("xcache: hash ttl check failed", "key", key, "error", ttlErr)
			} else if currentTTL < 0 {
				if expireErr := l.cache.Client().Expire(ctx, key, ttl).Err(); expireErr != nil {
					l.logWarn("xcache: hash expire failed", "key", key, "ttl", ttl, "error", expireErr)
				}
			}
		}
	}

	return value, nil
}

// waitAndRetryGet 等待后重试获取缓存。
// 使用指数退避 + 抖动策略，避免惊群效应。
// 等待时间受 MaxRetryAttempts 和 DistributedLockTTL 双重约束，取较小值。
func (l *loader) waitAndRetryGet(ctx context.Context, key string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	maxAttempts := l.options.MaxRetryAttempts
	if maxAttempts <= 0 {
		maxAttempts = 10 // 兜底默认值
	}

	// 累计等待时间上限：取 DistributedLockTTL 作为约束（锁释放后应能获取缓存）
	maxWaitTime := l.options.DistributedLockTTL
	if maxWaitTime <= 0 {
		maxWaitTime = 10 * time.Second // 兜底默认值
	}

	timer := time.NewTimer(0)
	defer timer.Stop()
	<-timer.C // 消费初始触发

	var elapsed time.Duration
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}

		value, err := l.cache.Client().Get(ctx, key).Bytes()
		if err == nil {
			return value, nil
		}
		if !errors.Is(err, redis.Nil) {
			return l.loadAndCache(ctx, key, loadFn, ttl)
		}

		// 计算指数退避时间 + 抖动
		wait := backoffWithJitter(attempt)

		// 检查是否超过 DistributedLockTTL 约束
		if elapsed+wait > maxWaitTime {
			// 剩余时间不足以完成本次等待，直接回源
			break
		}

		timer.Reset(wait)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			elapsed += wait
		}
	}

	// 超过最大重试次数或等待时间上限，直接回源
	return l.loadAndCache(ctx, key, loadFn, ttl)
}

// waitAndRetryHGet 等待后重试获取 Hash field。
// 使用指数退避 + 抖动策略，避免惊群效应。
// 等待时间受 MaxRetryAttempts 和 DistributedLockTTL 双重约束，取较小值。
func (l *loader) waitAndRetryHGet(ctx context.Context, key, field string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	maxAttempts := l.options.MaxRetryAttempts
	if maxAttempts <= 0 {
		maxAttempts = 10 // 兜底默认值
	}

	// 累计等待时间上限：取 DistributedLockTTL 作为约束（锁释放后应能获取缓存）
	maxWaitTime := l.options.DistributedLockTTL
	if maxWaitTime <= 0 {
		maxWaitTime = 10 * time.Second // 兜底默认值
	}

	timer := time.NewTimer(0)
	defer timer.Stop()
	<-timer.C // 消费初始触发

	var elapsed time.Duration
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}

		value, err := l.cache.Client().HGet(ctx, key, field).Bytes()
		if err == nil {
			return value, nil
		}
		if !errors.Is(err, redis.Nil) {
			return l.loadHashAndCache(ctx, key, field, loadFn, ttl)
		}

		// 计算指数退避时间 + 抖动
		wait := backoffWithJitter(attempt)

		// 检查是否超过 DistributedLockTTL 约束
		if elapsed+wait > maxWaitTime {
			// 剩余时间不足以完成本次等待，直接回源
			break
		}

		timer.Reset(wait)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			elapsed += wait
		}
	}

	// 超过最大重试次数或等待时间上限，直接回源
	return l.loadHashAndCache(ctx, key, field, loadFn, ttl)
}

// backoffWithJitter 计算带抖动的指数退避时间。
func backoffWithJitter(attempt int) time.Duration {
	// 溢出保护：attempt 超过安全范围时直接使用 maxBackoff
	// time.Duration 是 int64，baseBackoff(50ms) << 30 仍在安全范围内
	const maxSafeShift = 30
	if attempt > maxSafeShift {
		attempt = maxSafeShift
	}

	// 指数退避: base * 2^attempt
	backoff := baseBackoff << attempt

	// 额外检查：如果意外溢出导致负数或异常值，使用 maxBackoff
	if backoff <= 0 || backoff > maxBackoff {
		backoff = maxBackoff
	}

	// 添加抖动: backoff * jitterFraction * (rand - 0.5)
	// 使用 crypto/rand 确保高质量随机数
	jitter := time.Duration(float64(backoff) * jitterFraction * (randomFloat64() - 0.5))
	wait := backoff + jitter

	return wait
}

// acquireLock 获取分布式锁。
//
// 锁实现优先级（互斥，只会使用其中一种）：
//  1. ExternalLock 非 nil → 使用外部锁（如 xdlock 的 Redlock 或 etcd 锁）
//  2. ExternalLock 为 nil → 使用内置简单锁（Redis SET NX）
//
// 注意：设置 WithExternalLock 会自动启用分布式锁（EnableDistributedLock = true），
// 此时内置锁不会被使用。两种锁实现不会同时生效。
func (l *loader) acquireLock(ctx context.Context, key string) (Unlocker, error) {
	if l.options.ExternalLock != nil {
		return l.options.ExternalLock(ctx, key, l.options.DistributedLockTTL)
	}
	return l.cache.Lock(ctx, key, l.options.DistributedLockTTL)
}

// logInfo 记录信息日志（如果配置了 Logger）。
func (l *loader) logInfo(msg string, args ...any) {
	if l.options.Logger != nil {
		l.options.Logger.Info(msg, args...)
	}
}

// logWarn 记录警告日志（如果配置了 Logger）。
func (l *loader) logWarn(msg string, args ...any) {
	if l.options.Logger != nil {
		l.options.Logger.Warn(msg, args...)
	}
}

// logUnlockError 记录解锁错误。
// ErrLockExpired 是预期情况（锁自然过期），使用 Info 级别；
// 其他错误使用 Warn 级别。
func (l *loader) logUnlockError(key string, err error) {
	if errors.Is(err, ErrLockExpired) {
		// 锁过期是预期情况，可能是加载时间超过锁 TTL
		l.logInfo("xcache: lock expired before unlock (consider increasing DistributedLockTTL)",
			"key", key)
	} else {
		l.logWarn("xcache: unlock failed", "key", key, "error", err)
	}
}

// onCacheSetError 触发缓存写入失败回调（如果配置了）。
func (l *loader) onCacheSetError(ctx context.Context, key string, err error) {
	if l.options.OnCacheSetError != nil {
		l.options.OnCacheSetError(ctx, key, err)
	}
}
