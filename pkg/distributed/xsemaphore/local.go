package xsemaphore

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

// =============================================================================
// 本地信号量实现
// =============================================================================

// permitEntry 许可条目
type permitEntry struct {
	id        string
	resource  string
	tenantID  string
	expiresAt time.Time
}

// localSemaphore 本地信号量实现
// 用于降级场景，使用内存存储许可
type localSemaphore struct {
	opts    *options
	permits sync.Map // resource -> *resourcePermits
	closed  atomic.Bool

	// 后台清理
	cleanupOnce   sync.Once
	cleanupDone   chan struct{}
	cleanupTicker *time.Ticker
	cleanupWg     sync.WaitGroup // 等待 backgroundCleanupLoop 退出
}

// resourcePermits 资源的许可集合
type resourcePermits struct {
	mu      sync.RWMutex
	global  map[string]*permitEntry            // permitID -> entry
	tenants map[string]map[string]*permitEntry // tenantID -> permitID -> entry
}

// newResourcePermits 创建新的资源许可集合
func newResourcePermits() *resourcePermits {
	return &resourcePermits{
		global:  make(map[string]*permitEntry),
		tenants: make(map[string]map[string]*permitEntry),
	}
}

// newLocalSemaphore 创建本地信号量
func newLocalSemaphore(opts *options) *localSemaphore {
	s := &localSemaphore{
		opts:        opts,
		cleanupDone: make(chan struct{}),
	}

	// 启动后台清理
	s.startBackgroundCleanup()

	return s
}

// startBackgroundCleanup 启动后台清理任务
func (s *localSemaphore) startBackgroundCleanup() {
	s.cleanupOnce.Do(func() {
		s.cleanupTicker = time.NewTicker(localCleanupInterval)
		s.cleanupWg.Add(1)
		go s.backgroundCleanupLoop()
	})
}

// backgroundCleanupLoop 后台清理循环
func (s *localSemaphore) backgroundCleanupLoop() {
	defer s.cleanupWg.Done()
	defer s.cleanupTicker.Stop()

	for {
		select {
		case <-s.cleanupDone:
			return
		case <-s.cleanupTicker.C:
			s.cleanupAllExpired()
		}
	}
}

// cleanupAllExpired 清理所有资源的过期许可
// 设计决策：不删除空 bucket（resourcePermits）。空 map 的内存开销极小（~80 bytes），
// 而删除操作可能引发竞态：另一个 goroutine 通过 getResourcePermits 持有旧 rp 引用，
// 写入的许可会进入已从 sync.Map 脱链的 bucket，导致许可丢失。
func (s *localSemaphore) cleanupAllExpired() {
	now := time.Now()
	s.permits.Range(func(_, value any) bool {
		rp, ok := value.(*resourcePermits)
		if !ok {
			return true // 设计决策: sync.Map 中仅存储 *resourcePermits，此分支不可达
		}
		rp.mu.Lock()
		s.cleanupExpiredLocked(rp, now)
		rp.mu.Unlock()
		return true
	})
}

// getResourcePermits 获取或创建资源的许可集合
func (s *localSemaphore) getResourcePermits(resource string) *resourcePermits {
	if v, ok := s.permits.Load(resource); ok {
		if rp, ok := v.(*resourcePermits); ok {
			return rp
		}
	}

	rp := newResourcePermits()
	actual, _ := s.permits.LoadOrStore(resource, rp)
	if typed, ok := actual.(*resourcePermits); ok {
		return typed
	}
	// 设计决策: sync.Map 中仅存储 *resourcePermits，此路径不可达
	return rp
}

// tryGetResourcePermits 尝试获取资源许可集合，不存在时返回 nil
// 用于 release/extend/count 等不应创建空 bucket 的操作
func (s *localSemaphore) tryGetResourcePermits(resource string) *resourcePermits {
	if v, ok := s.permits.Load(resource); ok {
		if rp, ok := v.(*resourcePermits); ok {
			return rp
		}
	}
	return nil
}

// TryAcquire 非阻塞式获取本地许可
func (s *localSemaphore) TryAcquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error) {
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
	span.SetAttributes(acquireSpanAttributes(SemaphoreTypeLocal, resource, tenantID, cfg.capacity, cfg.tenantQuota)...)

	// 检查 context 是否已取消（本地操作是同步的，需要提前检查）
	if err := ctx.Err(); err != nil {
		setSpanError(span, err)
		return nil, err
	}

	localCapacity, localTenantQuota := s.calculateLocalCapacity(cfg)

	start := time.Now()
	permit, reason, err := s.doAcquire(ctx, resource, tenantID, localCapacity, localTenantQuota, cfg.ttl, cfg.metadata)
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
	s.recordAcquireMetrics(ctx, resource, permit != nil, reason, duration)

	return permit, err
}

// Acquire 阻塞式获取本地许可
func (s *localSemaphore) Acquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error) {
	// 应用默认超时
	ctx, cancel := applyDefaultTimeout(ctx, s.opts.defaultTimeout)
	defer cancel()

	cfg, tenantID, err := s.prepareAcquire(ctx, resource, opts)
	if err != nil {
		return nil, err
	}

	// 创建 span
	ctx, span := startSpan(ctx, s.opts.tracer, spanNameAcquire)
	defer span.End()
	span.SetAttributes(acquireSpanAttributes(SemaphoreTypeLocal, resource, tenantID, cfg.capacity, cfg.tenantQuota)...)

	localCapacity, localTenantQuota := s.calculateLocalCapacity(cfg)

	// 记录开始时间，用于计算总耗时
	start := time.Now()
	var lastReason AcquireFailReason
	var retryCount int

	for attempt := range cfg.maxRetries {
		if err := ctx.Err(); err != nil {
			setSpanError(span, err)
			return nil, err
		}

		permit, reason, err := s.tryAcquireOnce(ctx, resource, tenantID, localCapacity, localTenantQuota, cfg.ttl, cfg.metadata)
		if err != nil {
			s.recordAcquireMetrics(ctx, resource, false, ReasonUnknown, time.Since(start))
			setSpanError(span, err)
			return nil, err
		}
		retryCount = attempt
		if permit != nil {
			// 记录成功指标（只在最终成功时记录一次）
			s.recordAcquireMetrics(ctx, resource, true, ReasonUnknown, time.Since(start))
			span.SetAttributes(
				attribute.Bool(attrAcquired, true),
				attribute.String(attrPermitID, permit.ID()),
				attribute.Int(attrRetryCount, retryCount),
			)
			setSpanOK(span)
			return permit, nil
		}
		lastReason = reason

		// 最后一次重试不等待
		if attempt < cfg.maxRetries-1 {
			if err := waitForRetry(ctx, cfg.retryDelay); err != nil {
				setSpanError(span, err)
				return nil, err
			}
		}
	}

	// 记录失败指标（重试耗尽，只记录一次）
	s.recordAcquireMetrics(ctx, resource, false, lastReason, time.Since(start))
	span.SetAttributes(
		attribute.Bool(attrAcquired, false),
		attribute.String(attrFailReason, lastReason.String()),
		attribute.Int(attrRetryCount, retryCount),
	)
	return nil, ErrAcquireFailed
}

// prepareAcquire 准备获取许可的参数
func (s *localSemaphore) prepareAcquire(ctx context.Context, resource string, opts []AcquireOption) (*acquireOptions, string, error) {
	return prepareAcquireCommon(ctx, resource, opts, s.closed.Load())
}

// divideByPodCount 将值按 Pod 数量等分，保底为 1
// 如果 value <= 0，返回 0（表示未设置）
func divideByPodCount(value, podCount int) int {
	if value <= 0 {
		return 0
	}
	return max(1, value/podCount)
}

// calculateLocalCapacity 计算本地容量
func (s *localSemaphore) calculateLocalCapacity(cfg *acquireOptions) (localCapacity, localTenantQuota int) {
	podCount := s.opts.effectivePodCount()
	localCapacity = divideByPodCount(cfg.capacity, podCount)
	localTenantQuota = divideByPodCount(cfg.tenantQuota, podCount)
	return
}

// tryAcquireOnce 执行一次获取尝试
// 注意：此方法不记录指标，指标由调用方统一记录（避免重试时重复记录）
func (s *localSemaphore) tryAcquireOnce(ctx context.Context, resource, tenantID string, localCapacity, localTenantQuota int, ttl time.Duration, metadata map[string]string) (Permit, AcquireFailReason, error) {
	return s.doAcquire(ctx, resource, tenantID, localCapacity, localTenantQuota, ttl, metadata)
}

// doAcquire 执行获取许可的核心逻辑
func (s *localSemaphore) doAcquire(
	ctx context.Context,
	resource string,
	tenantID string,
	capacity int,
	tenantQuota int,
	ttl time.Duration,
	metadata map[string]string,
) (Permit, AcquireFailReason, error) {
	// 在锁外生成许可 ID，避免时钟回拨等待期间（最多 500ms）阻塞其他 goroutine
	permitID, err := s.opts.effectiveIDGenerator()(ctx)
	if err != nil {
		// 设计决策: 使用 %v 而非 %w 包装内部错误，避免暴露 xid 内部错误类型给消费者。
		return nil, ReasonUnknown, fmt.Errorf("%w: %v", ErrIDGenerationFailed, err)
	}

	rp := s.getResourcePermits(resource)
	rp.mu.Lock()
	defer rp.mu.Unlock()

	now := time.Now()

	// 清理过期许可
	s.cleanupExpiredLocked(rp, now)

	// 检查全局容量
	if len(rp.global) >= capacity {
		return nil, ReasonCapacityFull, nil
	}

	// 检查租户配额
	if tenantID != "" && tenantQuota > 0 {
		tenantPermits := rp.tenants[tenantID]
		if tenantPermits != nil && len(tenantPermits) >= tenantQuota {
			return nil, ReasonTenantQuotaExceeded, nil
		}
	}

	expiresAt := now.Add(ttl)
	entry := &permitEntry{
		id:        permitID,
		resource:  resource,
		tenantID:  tenantID,
		expiresAt: expiresAt,
	}

	// 添加到全局集合
	rp.global[permitID] = entry

	// 计算是否启用租户配额（与租户检查条件一致）
	hasTenantQuota := tenantID != "" && tenantQuota > 0

	// 添加到租户集合（仅在启用租户配额时）
	if hasTenantQuota {
		if rp.tenants[tenantID] == nil {
			rp.tenants[tenantID] = make(map[string]*permitEntry)
		}
		rp.tenants[tenantID][permitID] = entry
	}

	return newLocalPermit(s, permitID, resource, tenantID, expiresAt, ttl, hasTenantQuota, metadata), ReasonUnknown, nil
}

// cleanupExpiredLocked 清理过期许可（调用者必须持有 rp.mu 锁）
// 使用 !After(now) 而非 Before(now)，以匹配 Redis 的 <= 语义：
// 当 expiresAt == now 时，许可应被视为已过期
func (s *localSemaphore) cleanupExpiredLocked(rp *resourcePermits, now time.Time) {
	// 清理全局许可
	for id, entry := range rp.global {
		if !entry.expiresAt.After(now) {
			delete(rp.global, id)
			// 同时从租户集合删除
			if entry.tenantID != "" {
				if tenantPermits := rp.tenants[entry.tenantID]; tenantPermits != nil {
					delete(tenantPermits, id)
					// 如果租户集合为空，删除整个 key 以回收内存
					if len(tenantPermits) == 0 {
						delete(rp.tenants, entry.tenantID)
					}
				}
			}
		}
	}
}

// releasePermit 释放许可
func (s *localSemaphore) releasePermit(ctx context.Context, p *localPermit) error {
	rp := s.tryGetResourcePermits(p.resource)
	if rp == nil {
		return ErrPermitNotHeld
	}
	rp.mu.Lock()
	defer rp.mu.Unlock()

	// 从全局集合删除
	if _, ok := rp.global[p.id]; !ok {
		return ErrPermitNotHeld
	}
	delete(rp.global, p.id)

	// 从租户集合删除（使用 hasTenantQuota 判断，与 acquire 时保持一致）
	if p.tenantID != "" && p.hasTenantQuota {
		if tenantPermits := rp.tenants[p.tenantID]; tenantPermits != nil {
			delete(tenantPermits, p.id)
			// 如果租户集合为空，删除整个 key 以回收内存
			if len(tenantPermits) == 0 {
				delete(rp.tenants, p.tenantID)
			}
		}
	}

	// 记录指标（保留 trace context）
	if s.opts.metrics != nil {
		s.opts.metrics.RecordRelease(ctx, SemaphoreTypeLocal, p.resource)
	}

	return nil
}

// recordAcquireMetrics 记录获取指标
func (s *localSemaphore) recordAcquireMetrics(ctx context.Context, resource string, acquired bool, reason AcquireFailReason, duration time.Duration) {
	if s.opts.metrics != nil {
		s.opts.metrics.RecordAcquire(ctx, SemaphoreTypeLocal, resource, acquired, reason, duration)
	}
}

// recordExtendMetrics 记录续期指标
func (s *localSemaphore) recordExtendMetrics(ctx context.Context, resource string, success bool) {
	if s.opts.metrics != nil {
		s.opts.metrics.RecordExtend(ctx, SemaphoreTypeLocal, resource, success)
	}
}

// removeExpiredPermitLocked 删除过期许可（调用者必须持有 rp.mu 锁）
func (s *localSemaphore) removeExpiredPermitLocked(rp *resourcePermits, p *localPermit) {
	delete(rp.global, p.id)
	// 从租户集合删除（使用 hasTenantQuota 判断，与 acquire 时保持一致）
	if p.tenantID != "" && p.hasTenantQuota {
		if tenantPermits := rp.tenants[p.tenantID]; tenantPermits != nil {
			delete(tenantPermits, p.id)
			// 如果租户集合为空，删除整个 key 以回收内存
			if len(tenantPermits) == 0 {
				delete(rp.tenants, p.tenantID)
			}
		}
	}
}

// extendPermit 续期许可
func (s *localSemaphore) extendPermit(ctx context.Context, p *localPermit, newExpiresAt time.Time) error {
	rp := s.tryGetResourcePermits(p.resource)
	if rp == nil {
		s.recordExtendMetrics(ctx, p.resource, false)
		return ErrPermitNotHeld
	}
	rp.mu.Lock()
	defer rp.mu.Unlock()

	entry, ok := rp.global[p.id]
	if !ok {
		s.recordExtendMetrics(ctx, p.resource, false)
		return ErrPermitNotHeld
	}

	// 检查是否已过期（使用 !After 语义，与 cleanupExpiredLocked 保持一致：expiresAt <= now 视为过期）
	if !entry.expiresAt.After(time.Now()) {
		s.removeExpiredPermitLocked(rp, p)
		s.recordExtendMetrics(ctx, p.resource, false)
		return ErrPermitNotHeld
	}

	// 更新过期时间
	entry.expiresAt = newExpiresAt
	s.recordExtendMetrics(ctx, p.resource, true)
	return nil
}

// Query 查询资源状态
func (s *localSemaphore) Query(ctx context.Context, resource string, opts ...QueryOption) (*ResourceInfo, error) {
	// 应用默认超时
	ctx, cancel := applyDefaultTimeout(ctx, s.opts.defaultTimeout)
	defer cancel()

	start := time.Now()

	cfg, tenantID, err := prepareQueryCommon(ctx, resource, opts, s.closed.Load())
	if err != nil {
		if s.opts.metrics != nil {
			s.opts.metrics.RecordQuery(ctx, SemaphoreTypeLocal, resource, false, time.Since(start))
		}
		return nil, err
	}

	// 创建 span
	ctx, span := startSpan(ctx, s.opts.tracer, spanNameQuery)
	defer span.End()
	span.SetAttributes(
		attribute.String(attrSemType, SemaphoreTypeLocal),
		attribute.String(attrResource, resource),
	)
	if tenantID != "" {
		span.SetAttributes(attribute.String(attrTenantID, tenantID))
	}

	globalUsed, tenantUsed := s.countActivePermits(resource, tenantID)
	// 与 Redis 保持一致：仅当 tenantQuota > 0 时才统计租户
	if cfg.tenantQuota <= 0 {
		tenantUsed = 0
	}
	localCapacity, localTenantQuota := s.calculateLocalQueryCapacity(cfg)

	if s.opts.metrics != nil {
		s.opts.metrics.RecordQuery(ctx, SemaphoreTypeLocal, resource, true, time.Since(start))
	}

	span.SetAttributes(
		attribute.Int(attrGlobalUsed, globalUsed),
		attribute.Int(attrTenantUsed, tenantUsed),
	)
	setSpanOK(span)

	return &ResourceInfo{
		Resource:        resource,
		GlobalCapacity:  localCapacity,
		GlobalUsed:      globalUsed,
		GlobalAvailable: max(0, localCapacity-globalUsed),
		TenantID:        tenantID,
		TenantQuota:     localTenantQuota,
		TenantUsed:      tenantUsed,
		TenantAvailable: max(0, localTenantQuota-tenantUsed),
	}, nil
}

// countActivePermits 计算活跃许可数（全局和租户）
// 纯只读操作，与 query.lua 一致，不执行清理。
// 过期许可通过 expiresAt.After(now) 语义自动排除。
func (s *localSemaphore) countActivePermits(resource, tenantID string) (globalUsed, tenantUsed int) {
	rp := s.tryGetResourcePermits(resource)
	if rp == nil {
		return 0, 0
	}
	rp.mu.RLock()
	defer rp.mu.RUnlock()

	now := time.Now()

	// 统计未过期的全局许可
	for _, entry := range rp.global {
		if entry.expiresAt.After(now) {
			globalUsed++
		}
	}

	// 统计未过期的租户许可
	if tenantID != "" {
		if tenantPermits := rp.tenants[tenantID]; tenantPermits != nil {
			for _, entry := range tenantPermits {
				if entry.expiresAt.After(now) {
					tenantUsed++
				}
			}
		}
	}
	return
}

// calculateLocalQueryCapacity 计算查询用的本地容量
// 与 Acquire 保持一致，使用 divideByPodCount（保底为 1）
// 这确保 Query 返回的 GlobalCapacity/TenantQuota 与 Acquire 实际使用的容量一致
// 避免出现"Query 显示容量为 0，但 Acquire 能成功"的困惑场景
func (s *localSemaphore) calculateLocalQueryCapacity(cfg *queryOptions) (localCapacity, localTenantQuota int) {
	podCount := s.opts.effectivePodCount()
	localCapacity = divideByPodCount(cfg.capacity, podCount)
	localTenantQuota = divideByPodCount(cfg.tenantQuota, podCount)
	return
}

// Close 关闭本地信号量
func (s *localSemaphore) Close(_ context.Context) error {
	if s.closed.Swap(true) {
		return nil
	}

	// 停止后台清理并等待 goroutine 退出
	close(s.cleanupDone)
	s.cleanupWg.Wait()

	return nil
}

// Health 健康检查
func (s *localSemaphore) Health(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}
	if s.closed.Load() {
		return ErrSemaphoreClosed
	}
	return nil
}

// logExtendFailed 实现 loggerForExtend 接口
func (s *localSemaphore) logExtendFailed(ctx context.Context, permitID, resource string, err error) {
	if s.opts.logger != nil {
		s.opts.logger.Warn(ctx, "permit auto-extend failed",
			AttrPermitID(permitID),
			AttrResource(resource),
			AttrError(err),
		)
	}
}

// =============================================================================
// 编译时接口检查
// =============================================================================

var (
	_ Semaphore       = (*localSemaphore)(nil)
	_ loggerForExtend = (*localSemaphore)(nil)
)
