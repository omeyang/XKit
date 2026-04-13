package xsemaphore

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Pipeline 兼容模式实现
//
// 当 Redis 代理（如 Predixy）不支持 Lua 脚本时，使用 Pipeline 基础命令替代。
// 非原子操作，但通过 add-then-check 算法保证安全性（误拒绝，不过量放行）。
// =============================================================================

// doAcquireCompat 使用 Pipeline 实现获取许可（兼容模式）
//
// 算法：乐观 add-then-check
//  1. Pipeline 1: 清理过期 + 添加许可 + 计数 [+ 租户等价操作]
//  2. 检查: globalCount > capacity || tenantCount > tenantQuota → 回滚
//  3. Pipeline 2（仅回滚时）: ZREM 回滚
//  4. Pipeline 3（仅成功时）: EXPIRE 设置键 TTL
//
// 竞态分析：两客户端同时 add 且都超容 → 都 undo → 短暂欠利用（安全，重试自愈）。
// 最坏情况是误拒绝，不会过量放行。
func (s *redisSemaphore) doAcquireCompat(
	ctx context.Context,
	resource string,
	tenantID string,
	cfg *acquireOptions,
) (Permit, AcquireFailReason, error) {
	now := time.Now()
	expiresAt := now.Add(cfg.ttl)

	permitID, err := s.opts.effectiveIDGenerator()(ctx)
	if err != nil {
		return nil, ReasonUnknown, fmt.Errorf("%w: %v", ErrIDGenerationFailed, err)
	}

	hasTenantQuota := tenantID != "" && cfg.tenantQuota > 0
	globalKey := s.buildGlobalKey(resource)
	var tenantKey string
	if hasTenantQuota {
		tenantKey = s.buildTenantKey(resource, tenantID)
	}

	nowMs := now.UnixMilli()
	expireAtMs := expiresAt.UnixMilli()

	// Pipeline 1: 清理 + 添加 + 计数
	pipe := s.client.Pipeline()
	pipe.ZRemRangeByScore(ctx, globalKey, "-inf", strconv.FormatInt(nowMs, 10))
	pipe.ZAdd(ctx, globalKey, redis.Z{Score: float64(expireAtMs), Member: permitID})
	globalCardCmd := pipe.ZCard(ctx, globalKey)

	var tenantCardCmd *redis.IntCmd
	if hasTenantQuota {
		pipe.ZRemRangeByScore(ctx, tenantKey, "-inf", strconv.FormatInt(nowMs, 10))
		pipe.ZAdd(ctx, tenantKey, redis.Z{Score: float64(expireAtMs), Member: permitID})
		tenantCardCmd = pipe.ZCard(ctx, tenantKey)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, ReasonUnknown, fmt.Errorf("acquire pipeline failed: %w", err)
	}

	globalCount := globalCardCmd.Val()
	var tenantCount int64
	if tenantCardCmd != nil {
		tenantCount = tenantCardCmd.Val()
	}

	// 检查容量
	if globalCount > int64(cfg.capacity) {
		s.undoAcquireCompat(ctx, globalKey, tenantKey, permitID, hasTenantQuota)
		return nil, ReasonCapacityFull, nil
	}
	if hasTenantQuota && tenantCount > int64(cfg.tenantQuota) {
		s.undoAcquireCompat(ctx, globalKey, tenantKey, permitID, hasTenantQuota)
		return nil, ReasonTenantQuotaExceeded, nil
	}

	// 成功: 设置键 TTL（只延长，不缩短）
	s.setKeyTTLCompat(ctx, globalKey, tenantKey, hasTenantQuota, nowMs, expireAtMs)

	permit := newRedisPermit(s, permitID, resource, tenantID, expiresAt, cfg.ttl, hasTenantQuota, cfg.metadata)
	return permit, ReasonUnknown, nil
}

// undoAcquireCompat 回滚获取操作（移除刚添加的许可）
func (s *redisSemaphore) undoAcquireCompat(ctx context.Context, globalKey, tenantKey, permitID string, hasTenant bool) {
	pipe := s.client.Pipeline()
	pipe.ZRem(ctx, globalKey, permitID)
	if hasTenant {
		pipe.ZRem(ctx, tenantKey, permitID)
	}
	// 设计决策: 回滚失败不影响正确性（TTL 自然过期清理），故忽略错误。
	if _, err := pipe.Exec(ctx); err != nil {
		return
	}
}

// setKeyTTLCompat 设置键 TTL（只延长，不缩短）
func (s *redisSemaphore) setKeyTTLCompat(ctx context.Context, globalKey, tenantKey string, hasTenant bool, nowMs, expireAtMs int64) {
	ttlMs := expireAtMs - nowMs + keyTTLMargin.Milliseconds()
	ttlSec := int64(math.Ceil(float64(ttlMs) / 1000))
	newTTL := time.Duration(ttlSec) * time.Second

	// 查询当前 TTL
	pipe := s.client.Pipeline()
	globalTTLCmd := pipe.TTL(ctx, globalKey)
	var tenantTTLCmd *redis.DurationCmd
	if hasTenant {
		tenantTTLCmd = pipe.TTL(ctx, tenantKey)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return // TTL 设置失败不影响正确性
	}

	// 仅当新 TTL 大于当前 TTL 时才设置
	expireIfLonger(ctx, s.client, globalKey, newTTL, globalTTLCmd.Val())
	if hasTenant && tenantTTLCmd != nil {
		expireIfLonger(ctx, s.client, tenantKey, newTTL, tenantTTLCmd.Val())
	}
}

// expireIfLonger 仅当新 TTL 大于当前 TTL 时才设置
func expireIfLonger(ctx context.Context, client redis.UniversalClient, key string, newTTL, currentTTL time.Duration) {
	// currentTTL: -1 = 永不过期, -2 = 键不存在 → 都需要设置 TTL
	// 正数: 只有新值更大时才更新
	if currentTTL < 0 || newTTL > currentTTL {
		//nolint:errcheck // EXPIRE 失败不影响正确性，键会在下次写入时重新设置
		client.Expire(ctx, key, newTTL)
	}
}

// releasePermitCompat 使用基础命令释放许可（兼容模式）
//
// ZREM 本身是原子的，两次 ZREM（全局 + 租户）之间崩溃时，
// 租户条目会通过 TTL 自然过期。
func (s *redisSemaphore) releasePermitCompat(ctx context.Context, p *redisPermit) error {
	globalKey := s.buildGlobalKey(p.resource)

	removed, err := s.client.ZRem(ctx, globalKey, p.id).Result()
	if err != nil {
		return fmt.Errorf("release compat failed: %w", err)
	}
	if removed == 0 {
		return ErrPermitNotHeld
	}

	// 清理租户键（崩溃时 TTL 自愈）
	if p.tenantID != "" && p.hasTenantQuota {
		tenantKey := s.buildTenantKey(p.resource, p.tenantID)
		//nolint:errcheck // 租户键清理失败 TTL 自愈
		s.client.ZRem(ctx, tenantKey, p.id)
	}

	if s.opts.metrics != nil {
		s.opts.metrics.RecordRelease(ctx, SemaphoreTypeDistributed, p.resource)
	}
	return nil
}

// extendPermitCompat 使用 ZSCORE + ZADD 续期许可（兼容模式）
//
// 先检查许可是否存在（ZSCORE），存在则更新 score（ZADD）。
// 极窄窗口内过期未清理的条目可能被"复活"，TTL 自愈。
func (s *redisSemaphore) extendPermitCompat(ctx context.Context, p *redisPermit, newExpiresAt time.Time) error {
	globalKey := s.buildGlobalKey(p.resource)
	nowMs := time.Now().UnixMilli()
	newExpireAtMs := newExpiresAt.UnixMilli()

	// 防御性检查：新的过期时间必须在当前时间之后
	if newExpireAtMs <= nowMs {
		return ErrPermitNotHeld
	}

	if err := s.checkPermitExists(ctx, globalKey, p.id, nowMs); err != nil {
		return err
	}

	if err := s.updatePermitScore(ctx, p, globalKey, newExpireAtMs, nowMs); err != nil {
		return err
	}

	if s.opts.metrics != nil {
		s.opts.metrics.RecordExtend(ctx, SemaphoreTypeDistributed, p.resource, true)
	}
	return nil
}

// checkPermitExists 检查许可是否存在且未过期
func (s *redisSemaphore) checkPermitExists(ctx context.Context, globalKey, permitID string, nowMs int64) error {
	score, err := s.client.ZScore(ctx, globalKey, permitID).Result()
	if err != nil {
		if err == redis.Nil {
			return ErrPermitNotHeld
		}
		return fmt.Errorf("extend compat failed: %w", err)
	}
	// 检查是否已过期（与 Lua 脚本 <= 语义一致）
	if int64(score) <= nowMs {
		return ErrPermitNotHeld
	}
	return nil
}

// updatePermitScore 更新许可 score 并设置键 TTL
func (s *redisSemaphore) updatePermitScore(ctx context.Context, p *redisPermit, globalKey string, newExpireAtMs, nowMs int64) error {
	hasTenant := p.tenantID != "" && p.hasTenantQuota
	var tenantKey string
	if hasTenant {
		tenantKey = s.buildTenantKey(p.resource, p.tenantID)
	}

	pipe := s.client.Pipeline()
	pipe.ZAdd(ctx, globalKey, redis.Z{Score: float64(newExpireAtMs), Member: p.id})
	if hasTenant {
		pipe.ZAdd(ctx, tenantKey, redis.Z{Score: float64(newExpireAtMs), Member: p.id})
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("extend compat failed: %w", err)
	}

	s.setKeyTTLCompat(ctx, globalKey, tenantKey, hasTenant, nowMs, newExpireAtMs)
	return nil
}

// queryCompat 使用 ZCOUNT 查询许可状态（兼容模式）
//
// 纯读取操作，完全正确，无原子性要求。
func (s *redisSemaphore) queryCompat(ctx context.Context, globalKey string, keys []string, now time.Time) (int64, int64, error) {
	nowMs := now.UnixMilli()
	// 使用 "(" 前缀表示开区间，排除恰好等于 now 的过期条目
	minScore := "(" + strconv.FormatInt(nowMs, 10)

	pipe := s.client.Pipeline()
	globalCountCmd := pipe.ZCount(ctx, globalKey, minScore, "+inf")

	var tenantCountCmd *redis.IntCmd
	if len(keys) > 1 {
		tenantCountCmd = pipe.ZCount(ctx, keys[1], minScore, "+inf")
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return 0, 0, fmt.Errorf("query compat failed: %w", err)
	}

	var tenantCount int64
	if tenantCountCmd != nil {
		tenantCount = tenantCountCmd.Val()
	}
	return globalCountCmd.Val(), tenantCount, nil
}
