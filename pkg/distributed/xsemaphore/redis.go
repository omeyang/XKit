package xsemaphore

import (
	"context"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// validateScriptResult 校验 Lua 脚本返回值长度
func validateScriptResult(result []int64, minLen int) error {
	if len(result) < minLen {
		return fmt.Errorf("%w: got %d elements, want >= %d", errUnexpectedScriptResult, len(result), minLen)
	}
	return nil
}

// convertScriptResult 将 Lua 脚本返回值安全转换为 []int64
// 提取为纯函数，便于直接测试各种输入类型（int64、int、float64、未知类型）
func convertScriptResult(val any) ([]int64, error) {
	// Redis Lua 脚本返回数组时，go-redis 会解析为 []any
	arr, ok := val.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: expected array, got %T", errUnexpectedScriptResult, val)
	}

	result := make([]int64, len(arr))
	for i, v := range arr {
		switch n := v.(type) {
		case int64:
			result[i] = n
		case int:
			result[i] = int64(n)
		case float64:
			if n != math.Trunc(n) {
				return nil, fmt.Errorf("%w: element %d is non-integer float64 %g", errUnexpectedScriptResult, i, n)
			}
			result[i] = int64(n)
		default:
			return nil, fmt.Errorf("%w: element %d is %T, expected number", errUnexpectedScriptResult, i, v)
		}
	}

	return result, nil
}

// evalScriptInt64Slice 执行 Lua 脚本并安全转换返回值为 []int64
// 防止 Redis 返回非预期类型时 panic（修复 #6）
func (s *redisSemaphore) evalScriptInt64Slice(ctx context.Context, script *redis.Script, keys []string, args ...any) ([]int64, error) {
	val, err := script.Run(ctx, s.client, keys, args...).Result()
	if err != nil {
		return nil, err
	}
	return convertScriptResult(val)
}

// =============================================================================
// Redis 信号量实现
// =============================================================================

// redisSemaphore 实现 Semaphore 接口
type redisSemaphore struct {
	client  redis.UniversalClient
	opts    *options
	scripts *scripts
	closed  atomic.Bool
}

// New 创建 Redis 信号量
//
// 使用 Redis 作为后端存储，支持多 Pod 共享配额。
// 如果配置了 Fallback，会自动包装为降级信号量。
func New(client redis.UniversalClient, opts ...Option) (Semaphore, error) {
	if client == nil {
		return nil, ErrNilClient
	}

	cfg := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	// 验证工厂配置
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// 初始化指标收集器
	if cfg.meterProvider != nil {
		var metricsOpts []MetricsOption
		if cfg.disableResourceLabel {
			metricsOpts = append(metricsOpts, MetricsWithDisableResourceLabel())
		}
		metrics, err := NewMetrics(cfg.meterProvider, metricsOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create metrics: %w", err)
		}
		cfg.metrics = metrics
	}

	// 初始化 tracer
	cfg.tracer = getTracer(cfg.tracerProvider)

	sem := &redisSemaphore{
		client:  client,
		opts:    cfg,
		scripts: getScripts(),
	}

	// 如果配置了降级策略，包装为降级信号量
	// localSemaphore 延迟创建，仅在 FallbackLocal 策略首次降级时初始化
	if cfg.fallback != FallbackNone {
		return newFallbackSemaphore(sem, cfg), nil
	}

	return sem, nil
}

// TryAcquire 非阻塞式获取许可
func (s *redisSemaphore) TryAcquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error) {
	// 应用默认超时
	ctx, cancel := applyDefaultTimeout(ctx, s.opts.defaultTimeout)
	defer cancel()

	cfg, tenantID, err := s.prepareAcquire(ctx, resource, opts)
	if err != nil {
		return nil, err
	}

	// 创建 span
	ctx, span := startSpan(ctx, s.opts.tracer, spanNameTryAcquire)
	defer span.End()
	span.SetAttributes(acquireSpanAttributes(SemaphoreTypeDistributed, resource, tenantID, cfg.capacity, cfg.tenantQuota)...)

	start := time.Now()
	permit, reason, err := s.doAcquire(ctx, resource, tenantID, cfg)
	duration := time.Since(start)

	// 记录 span 结果
	switch {
	case err != nil:
		setSpanError(span, err)
	case permit == nil:
		span.SetAttributes(
			attribute.Bool(attrAcquired, false),
			attribute.String(attrFailReason, reason.String()),
		)
	default:
		span.SetAttributes(
			attribute.Bool(attrAcquired, true),
			attribute.String(attrPermitID, permit.ID()),
		)
		setSpanOK(span)
	}

	// 记录指标
	if s.opts.metrics != nil {
		s.opts.metrics.RecordAcquire(ctx, SemaphoreTypeDistributed, resource, permit != nil, reason, duration)
	}

	return permit, err
}

// Acquire 阻塞式获取许可
func (s *redisSemaphore) Acquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error) {
	// 应用默认超时
	ctx, cancel := applyDefaultTimeout(ctx, s.opts.defaultTimeout)
	defer cancel()

	cfg, tenantID, err := s.prepareAcquire(ctx, resource, opts)
	if err != nil {
		return nil, err
	}

	// 校验重试参数（仅 Acquire 需要，TryAcquire 不使用重试）
	if err := cfg.validateRetryParams(); err != nil {
		return nil, err
	}

	// 创建 span
	ctx, span := startSpan(ctx, s.opts.tracer, spanNameAcquire)
	defer span.End()
	span.SetAttributes(acquireSpanAttributes(SemaphoreTypeDistributed, resource, tenantID, cfg.capacity, cfg.tenantQuota)...)

	// 记录开始时间，用于计算总耗时
	start := time.Now()

	permit, lastReason, retryCount, err := s.acquireWithRetry(ctx, resource, tenantID, cfg)

	// 计算总耗时
	totalDuration := time.Since(start)

	if err != nil {
		// 记录失败指标（只在最终失败时记录一次）
		s.recordAcquireMetrics(ctx, resource, false, lastReason, totalDuration)
		span.SetAttributes(attribute.Int(attrRetryCount, retryCount))
		setSpanError(span, err)
		return nil, err
	}

	if permit != nil {
		// 记录成功指标（只在最终成功时记录一次）
		s.recordAcquireMetrics(ctx, resource, true, ReasonUnknown, totalDuration)
		span.SetAttributes(
			attribute.Bool(attrAcquired, true),
			attribute.String(attrPermitID, permit.ID()),
			attribute.Int(attrRetryCount, retryCount),
		)
		setSpanOK(span)
		return permit, nil
	}

	// 记录失败指标（重试耗尽）
	s.recordAcquireMetrics(ctx, resource, false, lastReason, totalDuration)
	span.SetAttributes(
		attribute.Bool(attrAcquired, false),
		attribute.String(attrFailReason, lastReason.String()),
		attribute.Int(attrRetryCount, retryCount),
	)
	s.logAcquireExhausted(ctx, resource, cfg.maxRetries, lastReason)
	return nil, ErrAcquireFailed
}

// acquireWithRetry 执行带重试的获取逻辑
// 返回值：permit, lastReason, retryCount, error
// retryCount 表示实际发生的重试次数（不包括首次尝试）
//
// 计算规则：
//   - attempt=0 是首次尝试，不算重试
//   - 循环体内 tryAcquireOnce 已执行后，retryCount = attempt
//   - 循环顶部 ctx 检查时 tryAcquireOnce 尚未执行，retryCount = max(0, attempt-1)
func (s *redisSemaphore) acquireWithRetry(ctx context.Context, resource, tenantID string, cfg *acquireOptions) (Permit, AcquireFailReason, int, error) {
	var lastReason AcquireFailReason

	for attempt := range cfg.maxRetries {
		if err := ctx.Err(); err != nil {
			// 当前 attempt 尚未执行，重试次数 = 已完成的尝试数 - 1
			return nil, lastReason, max(0, attempt-1), err
		}

		permit, reason, redisErr := s.tryAcquireOnce(ctx, resource, tenantID, cfg)

		// 致命 Redis 错误（非 TRYAGAIN），立即返回（可能触发降级）
		// 当前 attempt 已执行，重试次数 = attempt（attempt=0 首次尝试不算重试）
		if redisErr != nil && !isRetryableRedisError(redisErr) {
			return nil, reason, attempt, redisErr
		}

		if redisErr == nil && permit != nil {
			return permit, reason, attempt, nil
		}

		// 更新失败原因（无论是容量已满还是可重试 Redis 错误，如 TRYAGAIN）
		lastReason = reason

		if err := s.waitIfNotLastRetry(ctx, attempt, cfg); err != nil {
			return nil, lastReason, attempt, err
		}
	}

	return nil, lastReason, max(0, cfg.maxRetries-1), nil
}

// waitIfNotLastRetry 如果不是最后一次重试，则等待
func (s *redisSemaphore) waitIfNotLastRetry(ctx context.Context, i int, cfg *acquireOptions) error {
	if i < cfg.maxRetries-1 {
		return waitForRetry(ctx, cfg.retryDelay)
	}
	return nil
}

// prepareAcquire 准备获取许可的参数
func (s *redisSemaphore) prepareAcquire(ctx context.Context, resource string, opts []AcquireOption) (*acquireOptions, string, error) {
	return prepareAcquireCommon(ctx, resource, opts, s.closed.Load())
}

// tryAcquireOnce 执行一次获取尝试，返回 (permit, reason, error)
// error 非 nil 表示 Redis 错误
// 注意：此方法不记录指标，指标由调用方统一记录（避免重试时重复记录）
func (s *redisSemaphore) tryAcquireOnce(ctx context.Context, resource, tenantID string, cfg *acquireOptions) (Permit, AcquireFailReason, error) {
	return s.doAcquire(ctx, resource, tenantID, cfg)
}

// recordAcquireMetrics 记录获取指标
func (s *redisSemaphore) recordAcquireMetrics(ctx context.Context, resource string, acquired bool, reason AcquireFailReason, duration time.Duration) {
	if s.opts.metrics != nil {
		s.opts.metrics.RecordAcquire(ctx, SemaphoreTypeDistributed, resource, acquired, reason, duration)
	}
}

// logAcquireExhausted 记录重试耗尽日志
func (s *redisSemaphore) logAcquireExhausted(ctx context.Context, resource string, maxRetries int, reason AcquireFailReason) {
	if s.opts.logger != nil {
		s.opts.logger.Warn(ctx, "acquire permit failed after retries",
			AttrResource(resource),
			AttrMaxRetries(maxRetries),
			AttrReason(reason.String()),
		)
	}
}

// doAcquire 执行获取许可的核心逻辑
func (s *redisSemaphore) doAcquire(
	ctx context.Context,
	resource string,
	tenantID string,
	cfg *acquireOptions,
) (Permit, AcquireFailReason, error) {
	now := time.Now()
	expiresAt := now.Add(cfg.ttl)

	// 生成许可 ID（通过注入的生成器，默认使用 xid.NewStringWithRetry）
	permitID, err := s.opts.effectiveIDGenerator()(ctx)
	if err != nil {
		// 设计决策: 使用 %v 而非 %w 包装内部错误，避免暴露 xid 内部错误类型给消费者。
		// 消费者只需通过 errors.Is(err, ErrIDGenerationFailed) 判断，无需区分具体原因。
		return nil, ReasonUnknown, fmt.Errorf("%w: %v", ErrIDGenerationFailed, err)
	}

	// 计算是否启用租户配额
	hasTenantQuota := tenantID != "" && cfg.tenantQuota > 0

	// 构建 Redis 键
	globalKey := s.buildGlobalKey(resource)

	// 动态构建 KEYS 数组，避免传递空字符串（Redis Cluster 兼容）
	keys := []string{globalKey}
	if hasTenantQuota {
		keys = append(keys, s.buildTenantKey(resource, tenantID))
	}

	// 执行 Lua 脚本
	args := []any{
		now.UnixMilli(),
		expiresAt.UnixMilli(),
		permitID,
		cfg.capacity,
		cfg.tenantQuota,
		keyTTLMargin.Milliseconds(),
	}

	result, err := s.evalScriptInt64Slice(ctx, s.scripts.acquire, keys, args...)
	if err != nil {
		return nil, ReasonUnknown, fmt.Errorf("acquire script failed: %w", err)
	}

	// 验证结果长度：acquire 返回 {status, globalCount, tenantCount}
	if err := validateScriptResult(result, 3); err != nil {
		return nil, ReasonUnknown, fmt.Errorf("acquire script failed: %w", err)
	}

	return s.handleAcquireResult(ctx, result, permitID, resource, tenantID, expiresAt, cfg, hasTenantQuota)
}

// handleAcquireResult 处理 acquire 脚本的返回结果
func (s *redisSemaphore) handleAcquireResult(
	ctx context.Context,
	result []int64,
	permitID, resource, tenantID string,
	expiresAt time.Time,
	cfg *acquireOptions,
	hasTenantQuota bool,
) (Permit, AcquireFailReason, error) {
	status := int(result[0])

	switch status {
	case scriptStatusOK:
		permit := newRedisPermit(s, permitID, resource, tenantID, expiresAt, cfg.ttl, hasTenantQuota, cfg.metadata)
		return permit, ReasonUnknown, nil

	case scriptStatusCapacityFull:
		return nil, ReasonCapacityFull, nil

	case scriptStatusTenantQuotaExceeded:
		return nil, ReasonTenantQuotaExceeded, nil

	default:
		// 未知状态码，记录警告日志并返回错误
		if s.opts.logger != nil {
			s.opts.logger.Warn(ctx, "acquire script returned unknown status",
				AttrResource(resource),
				AttrStatusCode(status),
			)
		}
		return nil, ReasonUnknown, fmt.Errorf("%w: acquire returned status %d", ErrUnknownScriptStatus, status)
	}
}

// releasePermit 释放许可（内部方法）
// 注意：即使信号量已关闭，也允许释放许可，确保已获取的许可能完成其生命周期。
// 这与本地信号量的行为保持一致，也符合"Close 阻止新获取，但不影响已有许可"的设计理念。
func (s *redisSemaphore) releasePermit(ctx context.Context, p *redisPermit) error {
	globalKey := s.buildGlobalKey(p.resource)

	// 动态构建 KEYS 数组（Redis Cluster 兼容）
	var keys []string
	if p.tenantID != "" && p.hasTenantQuota {
		keys = []string{globalKey, s.buildTenantKey(p.resource, p.tenantID)}
	} else {
		keys = []string{globalKey}
	}

	args := []any{p.id}

	result, err := s.evalScriptInt64Slice(ctx, s.scripts.release, keys, args...)
	if err != nil {
		return fmt.Errorf("release script failed: %w", err)
	}

	// 验证结果长度：release 返回 {status, removed}
	if err := validateScriptResult(result, 2); err != nil {
		return fmt.Errorf("release script failed: %w", err)
	}

	status := int(result[0])
	switch status {
	case scriptStatusOK:
		// 记录指标
		if s.opts.metrics != nil {
			s.opts.metrics.RecordRelease(ctx, SemaphoreTypeDistributed, p.resource)
		}
		return nil
	case scriptStatusNotHeld:
		return ErrPermitNotHeld
	default:
		return fmt.Errorf("%w: release returned status %d", ErrUnknownScriptStatus, status)
	}
}

// extendPermit 续期许可（内部方法）
// 注意：即使信号量已关闭，也允许续期许可，确保已获取的许可能完成其生命周期。
// 这与本地信号量的行为保持一致，也符合"Close 阻止新获取，但不影响已有许可"的设计理念。
func (s *redisSemaphore) extendPermit(ctx context.Context, p *redisPermit, newExpiresAt time.Time) error {
	globalKey := s.buildGlobalKey(p.resource)

	// 动态构建 KEYS 数组（Redis Cluster 兼容）
	var keys []string
	if p.tenantID != "" && p.hasTenantQuota {
		keys = []string{globalKey, s.buildTenantKey(p.resource, p.tenantID)}
	} else {
		keys = []string{globalKey}
	}

	now := time.Now()
	args := []any{
		now.UnixMilli(),
		newExpiresAt.UnixMilli(),
		p.id,
		keyTTLMargin.Milliseconds(),
	}

	result, err := s.evalScriptInt64Slice(ctx, s.scripts.extend, keys, args...)
	if err != nil {
		return fmt.Errorf("extend script failed: %w", err)
	}

	// 验证结果长度：extend 返回 {status}
	if err := validateScriptResult(result, 1); err != nil {
		return fmt.Errorf("extend script failed: %w", err)
	}

	status := int(result[0])
	switch status {
	case scriptStatusOK:
		// 记录指标
		if s.opts.metrics != nil {
			s.opts.metrics.RecordExtend(ctx, SemaphoreTypeDistributed, p.resource, true)
		}
		return nil
	case scriptStatusNotHeld:
		// 记录失败指标
		if s.opts.metrics != nil {
			s.opts.metrics.RecordExtend(ctx, SemaphoreTypeDistributed, p.resource, false)
		}
		return ErrPermitNotHeld
	default:
		return fmt.Errorf("%w: extend returned status %d", ErrUnknownScriptStatus, status)
	}
}

// Query 查询资源状态
func (s *redisSemaphore) Query(ctx context.Context, resource string, opts ...QueryOption) (*ResourceInfo, error) {
	// 应用默认超时
	ctx, cancel := applyDefaultTimeout(ctx, s.opts.defaultTimeout)
	defer cancel()

	cfg, tenantID, err := prepareQueryCommon(ctx, resource, opts, s.closed.Load())
	if err != nil {
		return nil, err
	}

	// 创建 span
	ctx, span := startSpan(ctx, s.opts.tracer, spanNameQuery)
	defer span.End()
	span.SetAttributes(
		attribute.String(attrSemType, SemaphoreTypeDistributed),
		attribute.String(attrResource, resource),
	)
	if tenantID != "" {
		span.SetAttributes(attribute.String(attrTenantID, tenantID))
	}

	start := time.Now()

	globalKey := s.buildGlobalKey(resource)

	// 动态构建 KEYS 数组（Redis Cluster 兼容）
	// 与 Acquire 保持一致：仅在 tenantID 非空且 tenantQuota > 0 时才传递租户键
	var keys []string
	hasTenantKey := tenantID != "" && cfg.tenantQuota > 0
	if hasTenantKey {
		keys = []string{globalKey, s.buildTenantKey(resource, tenantID)}
	} else {
		keys = []string{globalKey}
	}

	now := time.Now()
	args := []any{now.UnixMilli()}

	result, err := s.evalScriptInt64Slice(ctx, s.scripts.query, keys, args...)
	if err != nil {
		return nil, s.handleQueryError(ctx, span, resource, start, err)
	}

	// 验证结果长度：query 返回 {globalCount, tenantCount}
	if err := validateScriptResult(result, 2); err != nil {
		return nil, s.handleQueryError(ctx, span, resource, start, err)
	}

	globalUsed := int(result[0])
	tenantUsed := int(result[1])

	info := &ResourceInfo{
		Resource:        resource,
		GlobalCapacity:  cfg.capacity,
		GlobalUsed:      globalUsed,
		GlobalAvailable: max(0, cfg.capacity-globalUsed),
		TenantID:        tenantID,
		TenantQuota:     cfg.tenantQuota,
		TenantUsed:      tenantUsed,
		TenantAvailable: max(0, cfg.tenantQuota-tenantUsed),
	}

	// 记录查询结果到 span
	span.SetAttributes(
		attribute.Int(attrGlobalUsed, globalUsed),
		attribute.Int(attrTenantUsed, tenantUsed),
	)
	setSpanOK(span)

	// 记录查询指标
	if s.opts.metrics != nil {
		s.opts.metrics.RecordQuery(ctx, SemaphoreTypeDistributed, resource, true, time.Since(start))
	}

	return info, nil
}

// handleQueryError 处理 Query 脚本错误：记录 span 和指标
func (s *redisSemaphore) handleQueryError(ctx context.Context, span trace.Span, resource string, start time.Time, err error) error {
	setSpanError(span, err)
	if s.opts.metrics != nil {
		s.opts.metrics.RecordQuery(ctx, SemaphoreTypeDistributed, resource, false, time.Since(start))
	}
	return fmt.Errorf("query script failed: %w", err)
}

// Close 关闭信号量
func (s *redisSemaphore) Close(_ context.Context) error {
	if s.closed.Swap(true) {
		return nil // 已关闭
	}
	// Redis 客户端由调用者管理，这里不关闭
	return nil
}

// Health 健康检查
func (s *redisSemaphore) Health(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}
	if s.closed.Load() {
		return ErrSemaphoreClosed
	}

	return s.client.Ping(ctx).Err()
}

// logExtendFailed 实现 loggerForExtend 接口
func (s *redisSemaphore) logExtendFailed(ctx context.Context, permitID, resource string, err error) {
	if s.opts.logger != nil {
		s.opts.logger.Warn(ctx, "permit auto-extend failed",
			AttrPermitID(permitID),
			AttrResource(resource),
			AttrError(err),
		)
	}
}

// =============================================================================
// 键构建辅助方法
// =============================================================================

// buildGlobalKey 构建全局许可键
// 使用 {resource} 作为 hash tag，确保 Redis Cluster 下同一资源的所有键映射到同一 slot
func (s *redisSemaphore) buildGlobalKey(resource string) string {
	return s.opts.keyPrefix + "{" + resource + "}:permits"
}

// buildTenantKey 构建租户许可键
// 使用 {resource} 作为 hash tag，确保与 globalKey 在同一 slot，避免 CROSSSLOT 错误
func (s *redisSemaphore) buildTenantKey(resource, tenantID string) string {
	return s.opts.keyPrefix + "{" + resource + "}:t:" + tenantID
}

// =============================================================================
// 编译时接口检查
// =============================================================================

var (
	_ Semaphore       = (*redisSemaphore)(nil)
	_ loggerForExtend = (*redisSemaphore)(nil)
)
