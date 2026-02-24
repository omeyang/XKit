package xcache

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
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

	// unlockTimeout 解锁操作的独立超时时间。
	// 解锁只需一次 Redis DEL/Lua 操作，5 秒足够覆盖极端网络延迟。
	// 设计决策: 与 DistributedLockTTL 解耦，确保即使加载耗时接近 TTL，
	// 解锁仍有充足时间执行。
	unlockTimeout = 5 * time.Second

	// IEEE 754 双精度浮点数尾数位数及对应缩放系数，用于 randomFloat64。
	float64MantissaBits  = 53
	float64MantissaScale = 1.0 / (1 << float64MantissaBits)
)

// randomFloat64 返回 [0.0, 1.0) 范围内的随机浮点数。
// 使用 crypto/rand 确保高质量随机数。
func randomFloat64() float64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand 失败表示系统随机数源不可用，返回 0.5（中间值）
		// 确保 backoffWithJitter 中 (randomFloat64() - 0.5) 为 0，不产生偏移抖动
		return 0.5
	}
	return float64(binary.LittleEndian.Uint64(buf[:])>>11) * float64MantissaScale
}

// safeLoadFn 安全地执行回源函数，将 panic 转为 error 返回。
//
// 设计决策: 作为基础库，xcache 必须保护自身不被用户代码的 panic 拖垮。
// 在 singleflight DoChan 模式下，loadFn 的 panic 会被 singleflight 捕获后
// 在新 goroutine 中 re-panic（因为 len(c.chans) > 0），导致进程级崩溃。
// 通过 recover 将 panic 转为 ErrLoadPanic 错误，所有等待方都能收到错误而非进程崩溃。
func safeLoadFn(ctx context.Context, loadFn LoadFunc) (value []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrLoadPanic, r)
		}
	}()
	return loadFn(ctx)
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
// 调用方必须保证 ctx 不为 nil。
func contextDetached(ctx context.Context) context.Context {
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
	return strconv.Itoa(len(key)) + ":" + key + ":" + field
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
	if key == "" {
		return nil, ErrEmptyKey
	}
	if loadFn == nil {
		return nil, ErrNilLoader
	}

	// 1. 尝试从缓存获取
	value, err := l.cache.Client().Get(ctx, key).Bytes()
	if err == nil {
		return value, nil
	}

	// 设计决策: Redis 错误（非 redis.Nil）也经过 singleflight 去重路径，
	// 避免 Redis 短时异常时高并发请求全部直打后端导致回源风暴。
	if !errors.Is(err, redis.Nil) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
	}

	// 2. 缓存未命中或 Redis 错误，使用 singleflight 或直接回源
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
	if key == "" || field == "" {
		return nil, ErrEmptyKey
	}
	if loadFn == nil {
		return nil, ErrNilLoader
	}

	// 1. 尝试从 Hash 获取
	value, err := l.cache.Client().HGet(ctx, key, field).Bytes()
	if err == nil {
		return value, nil
	}

	// 设计决策: Redis 错误（非 redis.Nil）也经过 singleflight 去重路径，
	// 避免 Redis 短时异常时高并发请求全部直打后端导致回源风暴。
	if !errors.Is(err, redis.Nil) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
	}

	// 2. 缓存未命中或 Redis 错误，使用 singleflight 或直接回源
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
	// 设计决策: sfCtx 生命周期绑定到 singleflight 执行体（DoChan 闭包内），
	// 而非绑定到任一调用方的返回路径。确保首个 caller cancel 不会通过
	// defer sfCancel() 取消共享加载，避免级联取消导致缓存未命中放大。
	ch := l.group.DoChan(key, func() (any, error) {
		sfCtx, sfCancel := contextWithIndependentTimeout(ctx, l.options.LoadTimeout)
		defer sfCancel()
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
			// 防御性检查：正常不可达，仅在内部类型系统被破坏时触发
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
//
// 设计决策: 锁过期后写入缓存的行为说明
// 如果加载时间超过 DistributedLockTTL，锁会自然过期，此时另一个节点可能获取锁并加载更新的数据。
// 当前节点仍会将加载结果写入缓存（loadAndCache 不检查锁状态），可能覆盖更新的值。
// 这是有意为之的设计：
//  1. 分布式锁的目的是减轻后端压力（防击穿），而非保证缓存强一致性
//  2. 如果需要强一致性，应使用版本号或 CAS 机制在业务层处理
//  3. 合理配置 DistributedLockTTL > LoadTimeout 可以极大降低此场景的概率
//  4. logUnlockError 会记录 ErrLockExpired 事件，便于监控告警
func (l *loader) loadWithLock(ctx context.Context, key string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	lockKey := l.options.DistributedLockKeyPrefix + key
	unlock, lockErr := l.acquireLock(ctx, lockKey)
	if lockErr != nil {
		return l.handleLockError(lockErr, lockKey, func() ([]byte, error) {
			return l.waitAndRetryGet(ctx, key, loadFn, ttl)
		})
	}

	// 设计决策: 解锁使用独立短超时（在 defer 内创建），不受加载耗时消耗。
	// 之前在进入临界区前创建 unlockCtx，长任务会消耗掉超时余量导致解锁失败。
	defer func() {
		unlockCtx, unlockCancel := context.WithTimeout(contextDetached(ctx), unlockTimeout)
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
	// 包括 ErrInvalidLockTTL（TTL 无效）和 ErrInvalidConfig（如 nil unlocker）
	if errors.Is(lockErr, ErrInvalidLockTTL) {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfig, lockErr)
	}
	if errors.Is(lockErr, ErrInvalidConfig) {
		return nil, lockErr
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

// applyTTLJitter 对 TTL 应用随机抖动。
// 当 TTLJitter > 0 且 ttl > 0 时，返回 ttl * (1 + jitter * (rand - 0.5))。
func (l *loader) applyTTLJitter(ttl time.Duration) time.Duration {
	if l.options.TTLJitter <= 0 || ttl <= 0 {
		return ttl
	}
	jittered := float64(ttl) * (1 + l.options.TTLJitter*(randomFloat64()-0.5))
	if jittered <= 0 {
		return ttl // 防止抖动到负数
	}
	return time.Duration(jittered)
}

// loadAndCache 加载数据并写入缓存。
func (l *loader) loadAndCache(ctx context.Context, key string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	// 应用超时
	loadCtx, cancel := applyLoadTimeout(ctx, l.options.LoadTimeout)
	defer cancel()

	// 回源加载（panic 安全，防止 singleflight DoChan 模式下进程崩溃）
	value, err := safeLoadFn(loadCtx, loadFn)
	if err != nil {
		return nil, err
	}

	// 设计决策: 缓存写入使用脱离取消链的 context + 独立短超时，确保回源数据不会
	// 因调用方取消而丢失。singleflight 路径的 ctx 已经是 detached 的，但非
	// singleflight 路径使用调用方原始 ctx，调用方取消可能导致缓存写入失败。
	// 写入是 best-effort 操作，失败不影响业务返回。
	writeCtx, writeCancel := context.WithTimeout(contextDetached(ctx), defaultOperationTimeout)
	defer writeCancel()

	cacheTTL := l.applyTTLJitter(ttl)
	if setErr := l.cache.Client().Set(writeCtx, key, value, cacheTTL).Err(); setErr != nil {
		l.logWarn("xcache: cache set failed", "key", key, "error", setErr)
		l.onCacheSetError(writeCtx, key, setErr)
	}

	return value, nil
}

// loadHashWithSingleflight 使用 singleflight 加载 Hash。
// 使用 DoChan 支持每个调用者独立的 context 取消，同时不影响其他等待者。
func (l *loader) loadHashWithSingleflight(ctx context.Context, key, field, sfKey string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	// 设计决策: sfCtx 生命周期绑定到 singleflight 执行体（DoChan 闭包内），
	// 而非绑定到任一调用方的返回路径。确保首个 caller cancel 不会通过
	// defer sfCancel() 取消共享加载，避免级联取消导致缓存未命中放大。
	ch := l.group.DoChan(sfKey, func() (any, error) {
		sfCtx, sfCancel := contextWithIndependentTimeout(ctx, l.options.LoadTimeout)
		defer sfCancel()
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
			// 防御性检查：正常不可达，仅在内部类型系统被破坏时触发
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

	// 设计决策: 解锁使用独立短超时（在 defer 内创建），不受加载耗时消耗。
	defer func() {
		unlockCtx, unlockCancel := context.WithTimeout(contextDetached(ctx), unlockTimeout)
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

	// 回源加载（panic 安全，防止 singleflight DoChan 模式下进程崩溃）
	value, err := safeLoadFn(loadCtx, loadFn)
	if err != nil {
		return nil, err
	}

	// 设计决策: 缓存写入使用脱离取消链的 context + 独立短超时（与 loadAndCache 对称），
	// 确保回源数据不会因调用方取消而丢失。
	writeCtx, writeCancel := context.WithTimeout(contextDetached(ctx), defaultOperationTimeout)
	defer writeCancel()

	// 写入缓存（best-effort，失败不影响业务返回）
	// 设计决策: onCacheSetError 回调传递实际 Redis key（而非 hashFieldKey 合成格式），
	// 与 loadAndCache 行为一致，便于监控系统关联到实际 Redis key。
	cacheTTL := l.applyTTLJitter(ttl)
	if cacheTTL > 0 && l.options.HashTTLRefresh {
		// 设计决策: 使用 Pipeline 合并 HSet+Expire 为一次 roundtrip，
		// 减少进程崩溃时 hash key 无 TTL 的风险窗口。
		pipe := l.cache.Client().Pipeline()
		pipe.HSet(writeCtx, key, field, value)
		pipe.Expire(writeCtx, key, cacheTTL)
		if _, pipeErr := pipe.Exec(writeCtx); pipeErr != nil {
			l.logWarn("xcache: hash set/expire pipeline failed", "key", key, "field", field, "error", pipeErr)
			l.onCacheSetError(writeCtx, key, pipeErr)
		}
	} else if setErr := l.cache.Client().HSet(writeCtx, key, field, value).Err(); setErr != nil {
		l.logWarn("xcache: hash set failed", "key", key, "field", field, "error", setErr)
		l.onCacheSetError(writeCtx, key, setErr)
	} else if cacheTTL > 0 {
		// HashTTLRefresh=false: 仅首次设置 TTL
		// 设计决策: 此路径使用 HSet → TTL → Expire 三次 roundtrip（非 Pipeline），
		// 因为需要先检查 TTL 再决定是否 Expire。虽然 HSet 和 Expire 之间存在
		// TOCTOU 窗口（进程崩溃可能导致 Hash 无 TTL），但该路径仅在 HashTTLRefresh=false
		// 时使用，且仅影响首次写入，风险极低。优化为 Lua 脚本可消除窗口但增加复杂度。
		currentTTL, ttlErr := l.cache.Client().TTL(writeCtx, key).Result()
		if ttlErr != nil {
			l.logWarn("xcache: hash ttl check failed", "key", key, "error", ttlErr)
		} else if currentTTL < 0 {
			if expireErr := l.cache.Client().Expire(writeCtx, key, cacheTTL).Err(); expireErr != nil {
				l.logWarn("xcache: hash expire failed", "key", key, "ttl", cacheTTL, "error", expireErr)
			}
		}
	}

	return value, nil
}

// cacheCheckFunc 从缓存查询并返回结果。
// 返回 (value, error)：value 非 nil 表示命中；error 为 redis.Nil 表示未命中，其他表示故障。
type cacheCheckFunc func(ctx context.Context) ([]byte, error)

// fallbackLoadFunc 回源加载并写入缓存。
type fallbackLoadFunc func(ctx context.Context) ([]byte, error)

// waitAndRetry 等待后重试获取缓存（通用实现）。
// 使用指数退避 + 抖动策略，避免惊群效应。
// 等待时间受 MaxRetryAttempts 和 DistributedLockTTL 双重约束，取较小值。
func (l *loader) waitAndRetry(ctx context.Context, check cacheCheckFunc, fallback fallbackLoadFunc) ([]byte, error) {
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

		value, err := check(ctx)
		if err == nil {
			return value, nil
		}
		if !errors.Is(err, redis.Nil) {
			return fallback(ctx)
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
	return fallback(ctx)
}

// waitAndRetryGet 等待后重试获取缓存。
func (l *loader) waitAndRetryGet(ctx context.Context, key string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	return l.waitAndRetry(ctx,
		func(ctx context.Context) ([]byte, error) {
			return l.cache.Client().Get(ctx, key).Bytes()
		},
		func(ctx context.Context) ([]byte, error) {
			return l.loadAndCache(ctx, key, loadFn, ttl)
		},
	)
}

// waitAndRetryHGet 等待后重试获取 Hash field。
func (l *loader) waitAndRetryHGet(ctx context.Context, key, field string, loadFn LoadFunc, ttl time.Duration) ([]byte, error) {
	return l.waitAndRetry(ctx,
		func(ctx context.Context) ([]byte, error) {
			return l.cache.Client().HGet(ctx, key, field).Bytes()
		},
		func(ctx context.Context) ([]byte, error) {
			return l.loadHashAndCache(ctx, key, field, loadFn, ttl)
		},
	)
}

// backoffWithJitter 计算带抖动的指数退避时间。
// 返回值范围为 [backoff * (1 - jitterFraction/2), backoff * (1 + jitterFraction/2)]，
// 其中 jitterFraction=0.3，因此 wait ∈ [backoff*0.85, backoff*1.15]，始终为正。
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
//
// 防御性校验：如果锁实现返回 (nil, nil)（unlock 为 nil 但无 error），
// 转为返回 ErrInvalidConfig 错误，避免后续 defer unlock(...) 处 panic。
func (l *loader) acquireLock(ctx context.Context, key string) (Unlocker, error) {
	var unlock Unlocker
	var err error
	if l.options.ExternalLock != nil {
		unlock, err = l.options.ExternalLock(ctx, key, l.options.DistributedLockTTL)
	} else {
		unlock, err = l.cache.Lock(ctx, key, l.options.DistributedLockTTL)
	}
	if err != nil {
		return nil, err
	}
	// 设计决策: 防御性检查空 Unlocker，避免错误的锁实现导致 defer unlock(...) panic。
	// 正常锁实现不应返回 (nil, nil)，但外部锁实现可能存在 bug。
	if unlock == nil {
		return nil, fmt.Errorf("%w: lock returned nil unlocker without error", ErrInvalidConfig)
	}
	return unlock, nil
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
