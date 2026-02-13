package xsemaphore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/omeyang/xkit/pkg/context/xtenant"
	"github.com/omeyang/xkit/pkg/observability/xlog"
)

// =============================================================================
// 降级信号量包装器
// =============================================================================

// fallbackSemaphore 带降级能力的信号量
// 当分布式信号量（Redis）不可用时，自动降级到备选策略
type fallbackSemaphore struct {
	distributed Semaphore
	local       *localSemaphore // 延迟初始化，仅 FallbackLocal 策略需要
	localOnce   sync.Once       // 保护 local 的延迟初始化
	localMu     sync.Mutex      // 保护 local 和 closed 的并发访问
	closed      bool            // Close 后禁止再创建 local
	strategy    FallbackStrategy
	opts        *options

	// onFallback 回调限流
	lastCallbackMu   sync.Mutex
	lastCallbackTime time.Time
}

// newFallbackSemaphore 创建带降级的信号量
// 注意：local 参数被延迟创建，仅在 FallbackLocal 策略首次需要时初始化
func newFallbackSemaphore(distributed Semaphore, opts *options) *fallbackSemaphore {
	return &fallbackSemaphore{
		distributed: distributed,
		strategy:    opts.fallback,
		opts:        opts,
	}
}

// ensureLocalSemaphore 确保本地信号量已初始化（仅 FallbackLocal 策略需要）
// Close 后不再创建新的 localSemaphore，防止 goroutine 泄漏
func (f *fallbackSemaphore) ensureLocalSemaphore() *localSemaphore {
	if f.strategy != FallbackLocal {
		return nil
	}
	f.localMu.Lock()
	defer f.localMu.Unlock()
	if f.closed {
		return nil
	}
	f.localOnce.Do(func() {
		f.local = newLocalSemaphore(f.opts)
	})
	return f.local
}

// TryAcquire 尝试获取许可，失败时降级
func (f *fallbackSemaphore) TryAcquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error) {
	permit, err := f.distributed.TryAcquire(ctx, resource, opts...)
	if err == nil {
		return permit, nil
	}

	// 检查是否是 Redis 不可用错误
	if !IsRedisError(err) {
		return nil, err
	}

	// 处理 Redis 错误：记录日志、指标和触发回调
	f.handleRedisError(ctx, resource, err)

	// 执行降级策略
	return f.fallback(ctx, resource, opts)
}

// Acquire 阻塞获取许可，失败时降级
func (f *fallbackSemaphore) Acquire(ctx context.Context, resource string, opts ...AcquireOption) (Permit, error) {
	permit, err := f.distributed.Acquire(ctx, resource, opts...)
	if err == nil {
		return permit, nil
	}

	// 检查是否是 Redis 不可用错误
	if !IsRedisError(err) {
		return nil, err
	}

	// 处理 Redis 错误：记录日志、指标和触发回调
	f.handleRedisError(ctx, resource, err)

	// 执行降级策略
	return f.fallbackAcquire(ctx, resource, opts)
}

// logFallback 记录降级日志
func (f *fallbackSemaphore) logFallback(ctx context.Context, resource string, err error) {
	if f.opts.logger != nil {
		f.opts.logger.Warn(ctx, "semaphore falling back due to Redis error",
			slog.String("strategy", string(f.strategy)),
			slog.String("resource", resource),
			slog.String("error", err.Error()),
		)
	}
}

// handleRedisError 处理 Redis 错误时的公共逻辑：记录日志、指标、trace 事件和触发回调
func (f *fallbackSemaphore) handleRedisError(ctx context.Context, resource string, err error) {
	f.logFallback(ctx, resource, err)
	if f.opts.metrics != nil {
		f.opts.metrics.RecordFallback(ctx, f.strategy, resource, ClassifyError(err))
	}

	// 记录 fallback 事件到 trace
	// 使用 AddEvent 而非创建新 span，因为 fallback 是父操作的一部分
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.AddEvent("xsemaphore.fallback", trace.WithAttributes(
			attribute.Bool(attrFallbackUsed, true),
			attribute.String("xsemaphore.fallback_strategy", string(f.strategy)),
			attribute.String("xsemaphore.fallback_reason", ClassifyError(err)),
		))
	}

	f.safeOnFallback(ctx, resource, err)
}

// safeOnFallback 安全调用 onFallback 回调，隔离 panic 并限流
// 在 Redis 故障风暴期间，限制回调频率（fallbackCallbackMinInterval），避免下游雪崩
func (f *fallbackSemaphore) safeOnFallback(ctx context.Context, resource string, err error) {
	if f.opts.onFallback == nil {
		return
	}

	// 限流：检查距离上次回调是否已过最小间隔
	f.lastCallbackMu.Lock()
	now := time.Now()
	if now.Sub(f.lastCallbackTime) < fallbackCallbackMinInterval {
		f.lastCallbackMu.Unlock()
		return
	}
	f.lastCallbackTime = now
	f.lastCallbackMu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			if f.opts.logger != nil {
				f.opts.logger.Error(ctx, "onFallback callback panicked",
					slog.String("resource", resource),
					slog.Any("panic", r),
				)
			}
		}
	}()
	f.opts.onFallback(resource, f.strategy, err)
}

// doFallback 执行降级策略的通用实现
func (f *fallbackSemaphore) doFallback(ctx context.Context, resource string, opts []AcquireOption, tryAcquire bool) (Permit, error) {
	switch f.strategy {
	case FallbackLocal:
		local := f.ensureLocalSemaphore()
		if local == nil {
			return nil, ErrSemaphoreClosed
		}
		if tryAcquire {
			return local.TryAcquire(ctx, resource, opts...)
		}
		return local.Acquire(ctx, resource, opts...)

	case FallbackOpen:
		// 创建一个虚拟的许可，不占用资源
		cfg := defaultAcquireOptions()
		for _, opt := range opts {
			if opt != nil {
				opt(cfg)
			}
		}
		// 从 context 提取租户 ID（与 Redis/Local 实现对齐）
		tenantID := cfg.tenantID
		if tenantID == "" {
			tenantID = xtenant.TenantID(ctx)
		}
		return newNoopPermit(ctx, resource, tenantID, cfg.ttl, f.opts.logger, cfg.metadata, f.opts)

	case FallbackClose:
		return nil, ErrRedisUnavailable

	default:
		// 不可达：FallbackStrategy 在工厂构造时已校验，仅 Local/Open/Close 三种
		return nil, ErrRedisUnavailable
	}
}

// fallback 执行降级策略（TryAcquire）
func (f *fallbackSemaphore) fallback(ctx context.Context, resource string, opts []AcquireOption) (Permit, error) {
	return f.doFallback(ctx, resource, opts, true)
}

// fallbackAcquire 执行降级策略（Acquire）
func (f *fallbackSemaphore) fallbackAcquire(ctx context.Context, resource string, opts []AcquireOption) (Permit, error) {
	return f.doFallback(ctx, resource, opts, false)
}

// Query 查询资源状态
func (f *fallbackSemaphore) Query(ctx context.Context, resource string, opts ...QueryOption) (*ResourceInfo, error) {
	info, err := f.distributed.Query(ctx, resource, opts...)
	if err == nil {
		return info, nil
	}

	// 如果是 Redis 错误，根据降级策略处理
	if IsRedisError(err) {
		return f.queryFallback(ctx, resource, opts, err)
	}

	return nil, err
}

// queryFallback 执行 Query 的降级策略
func (f *fallbackSemaphore) queryFallback(ctx context.Context, resource string, opts []QueryOption, err error) (*ResourceInfo, error) {
	// 处理 Redis 错误：记录日志和指标（Query 无需触发 onFallback 回调）
	f.logFallback(ctx, resource, err)
	if f.opts.metrics != nil {
		f.opts.metrics.RecordFallback(ctx, f.strategy, resource, ClassifyError(err))
	}

	// 根据策略返回不同结果
	switch f.strategy {
	case FallbackLocal:
		local := f.ensureLocalSemaphore()
		if local == nil {
			return nil, ErrSemaphoreClosed
		}
		return local.Query(ctx, resource, opts...)

	case FallbackOpen:
		return f.buildOpenQueryInfo(ctx, resource, opts), nil

	case FallbackClose:
		return nil, ErrRedisUnavailable

	default:
		// 不可达：FallbackStrategy 在工厂构造时已校验，仅 Local/Open/Close 三种
		return nil, ErrRedisUnavailable
	}
}

// buildOpenQueryInfo 构建 FallbackOpen 策略的查询信息
func (f *fallbackSemaphore) buildOpenQueryInfo(ctx context.Context, resource string, opts []QueryOption) *ResourceInfo {
	cfg := defaultQueryOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	// 从 context 提取租户 ID（与 Redis/Local 实现对齐）
	tenantID := cfg.tenantID
	if tenantID == "" {
		tenantID = xtenant.TenantID(ctx)
	}
	return &ResourceInfo{
		Resource:        resource,
		GlobalCapacity:  cfg.capacity,
		GlobalUsed:      0,
		GlobalAvailable: cfg.capacity,
		TenantID:        tenantID,
		TenantQuota:     cfg.tenantQuota,
		TenantUsed:      0,
		TenantAvailable: cfg.tenantQuota,
	}
}

// Close 关闭信号量
// 通过 localMu 与 ensureLocalSemaphore 互斥，防止 Close 后创建新 localSemaphore
func (f *fallbackSemaphore) Close(ctx context.Context) error {
	// 先标记关闭并获取 local 的快照，再释放锁
	f.localMu.Lock()
	f.closed = true
	local := f.local
	f.localMu.Unlock()

	var errs []error

	if err := f.distributed.Close(ctx); err != nil {
		errs = append(errs, err)
	}

	if local != nil {
		if err := local.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// Health 健康检查
func (f *fallbackSemaphore) Health(ctx context.Context) error {
	// 优先检查分布式信号量
	err := f.distributed.Health(ctx)
	if err == nil {
		return nil
	}

	// 如果是 Redis 错误，检查本地信号量（如果已初始化）
	if IsRedisError(err) {
		// 通过 localMu 安全读取 local，避免与 ensureLocalSemaphore/Close 竞争
		f.localMu.Lock()
		local := f.local
		f.localMu.Unlock()

		if local == nil {
			return err
		}
		localErr := local.Health(ctx)
		if localErr == nil {
			// 本地健康，返回原始 Redis 错误（降级状态）
			return err
		}
		return errors.Join(err, localErr)
	}

	return err
}

// =============================================================================
// Noop 许可（用于 FallbackOpen）
// =============================================================================

// noopPermit 空操作许可
// 用于 FallbackOpen 策略，不实际占用资源
// 内嵌 permitBase 复用 ID/Resource/TenantID/ExpiresAt/Metadata 等通用实现
type noopPermit struct {
	permitBase
	logger xlog.Logger
}

// newNoopPermit 创建空操作许可
// 使用注入的 ID 生成器生成唯一 ID，确保多个 FallbackOpen 许可可以正确区分
func newNoopPermit(ctx context.Context, resource, tenantID string, ttl time.Duration, logger xlog.Logger, metadata map[string]string, opts *options) (*noopPermit, error) {
	// 生成许可 ID（通过注入的生成器，默认使用 xid.NewStringWithRetry）
	id, err := opts.effectiveIDGenerator()(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIDGenerationFailed, err)
	}

	p := &noopPermit{logger: logger}
	expiresAt := time.Now().Add(ttl)
	initPermitBase(&p.permitBase, "noop-"+id, resource, tenantID, expiresAt, ttl, false, metadata)
	return p, nil
}

// Release 释放许可（空操作）
// 标记为已释放，确保后续 Extend 能正确返回 ErrPermitNotHeld
func (p *noopPermit) Release(_ context.Context) error {
	p.markReleased()
	return nil
}

// Extend 续期许可（空操作）
// 已释放的许可不可续期，与 redisPermit/localPermit 行为一致
// 成功续期时更新 expiresAt，确保 ExpiresAt() 返回正确值
func (p *noopPermit) Extend(_ context.Context) error {
	if p.isReleased() {
		return ErrPermitNotHeld
	}
	p.setExpiresAt(time.Now().Add(p.ttl))
	return nil
}

// StartAutoExtend 启动自动续租（空操作）
// 在 FallbackOpen 模式下，许可不实际占用资源，因此续租是空操作。
// 但仍记录日志以便于调试和问题追踪。
func (p *noopPermit) StartAutoExtend(interval time.Duration) func() {
	if p.logger != nil {
		p.logger.Info(context.Background(), "noop permit auto-extend started (fallback open mode)",
			AttrPermitID(p.ID()),
			AttrResource(p.Resource()),
			slog.Duration("interval", interval),
		)
	}
	return func() {
		if p.logger != nil {
			p.logger.Debug(context.Background(), "noop permit auto-extend stopped",
				AttrPermitID(p.ID()),
				AttrResource(p.Resource()),
			)
		}
	}
}

// 编译时接口检查
var (
	_ Semaphore = (*fallbackSemaphore)(nil)
	_ Permit    = (*noopPermit)(nil)
)
