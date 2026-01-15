package xsemaphore

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/omeyang/xkit/pkg/observability/xlog"
)

// =============================================================================
// Permit 基础实现（内嵌到具体实现中）
// =============================================================================

// permitBase 许可的公共字段和方法
type permitBase struct {
	id       string
	resource string
	tenantID string
	ttl      time.Duration

	// hasTenantQuota 记录获取许可时是否启用了租户配额
	// 用于确保 release/extend 操作与 acquire 时的语义一致
	hasTenantQuota bool

	// metadata 存储用户自定义的元数据
	metadata map[string]string

	// expiresAt 使用原子指针保护，避免读写竞争
	expiresAt atomic.Pointer[time.Time]

	// 自动续租状态
	autoExtendMu sync.Mutex
	stopCh       chan struct{}
	autoRunning  bool

	// 释放状态
	released atomic.Bool
}

// initPermitBase 初始化许可基础字段
func initPermitBase(base *permitBase, id, resource, tenantID string, expiresAt time.Time, ttl time.Duration, hasTenantQuota bool, metadata map[string]string) {
	base.id = id
	base.resource = resource
	base.tenantID = tenantID
	base.ttl = ttl
	base.hasTenantQuota = hasTenantQuota
	base.expiresAt.Store(&expiresAt)
	// 复制 metadata，防止外部修改影响内部状态
	if len(metadata) > 0 {
		base.metadata = make(map[string]string, len(metadata))
		for k, v := range metadata {
			base.metadata[k] = v
		}
	}
}

// ID 返回许可 ID
func (b *permitBase) ID() string {
	return b.id
}

// Resource 返回资源名称
func (b *permitBase) Resource() string {
	return b.resource
}

// TenantID 返回租户 ID
func (b *permitBase) TenantID() string {
	return b.tenantID
}

// ExpiresAt 返回过期时间（线程安全）
func (b *permitBase) ExpiresAt() time.Time {
	if ptr := b.expiresAt.Load(); ptr != nil {
		return *ptr
	}
	return time.Time{}
}

// Metadata 返回元数据的副本
func (b *permitBase) Metadata() map[string]string {
	if b.metadata == nil {
		return nil
	}
	// 返回副本，防止外部修改
	result := make(map[string]string, len(b.metadata))
	for k, v := range b.metadata {
		result[k] = v
	}
	return result
}

// setExpiresAt 设置过期时间（线程安全）
func (b *permitBase) setExpiresAt(t time.Time) {
	b.expiresAt.Store(&t)
}

// isReleased 检查是否已释放
func (b *permitBase) isReleased() bool {
	return b.released.Load()
}

// markReleased 标记为已释放，返回之前的状态
func (b *permitBase) markReleased() bool {
	return b.released.Swap(true)
}

// startAutoExtendLoop 启动自动续租循环
// extendFunc 是实际执行续租的函数
// 如果已经在运行，直接返回现有的 stop 函数（单次启动策略，避免竞态）
func (b *permitBase) startAutoExtendLoop(interval time.Duration, extendFunc func(context.Context) error, logger loggerForExtend) func() {
	// 校验 interval，防止 time.NewTicker panic
	if interval <= 0 {
		return func() {} // 返回空操作
	}

	b.autoExtendMu.Lock()
	defer b.autoExtendMu.Unlock()

	// 如果已经在运行，直接返回现有的 stop 函数（单次启动策略）
	// 避免重复启动导致的 goroutine 竞态
	if b.autoRunning && b.stopCh != nil {
		return b.stopAutoExtend
	}

	b.stopCh = make(chan struct{})
	b.autoRunning = true

	go b.runAutoExtendLoop(interval, b.stopCh, extendFunc, logger)

	return b.stopAutoExtend
}

// runAutoExtendLoop 自动续租循环
func (b *permitBase) runAutoExtendLoop(interval time.Duration, stopCh <-chan struct{}, extendFunc func(context.Context) error, logger loggerForExtend) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			if b.isReleased() {
				return
			}

			timeout := min(autoExtendTimeout, b.ttl/3)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			err := extendFunc(ctx)
			cancel()

			if err != nil {
				if logger != nil {
					logger.logExtendFailed(ctx, b.id, b.resource, err)
				}

				// 如果许可已不存在，停止续租
				if IsPermitNotHeld(err) {
					return
				}
			}
		}
	}
}

// stopAutoExtend 停止自动续租
func (b *permitBase) stopAutoExtend() {
	b.autoExtendMu.Lock()
	defer b.autoExtendMu.Unlock()

	if b.autoRunning && b.stopCh != nil {
		close(b.stopCh)
		b.stopCh = nil
		b.autoRunning = false
	}
}

// loggerForExtend 用于自动续租日志的接口
type loggerForExtend interface {
	logExtendFailed(ctx context.Context, permitID, resource string, err error)
}

// =============================================================================
// 模板方法 - 消除 Release/Extend 的代码重复
// =============================================================================

// releaseCommon 释放许可的模板方法
// 参数：
//   - ctx: 上下文
//   - tracer: 用于创建 span 的 tracer
//   - semType: 信号量类型（distributed/local）
//   - logger: 日志记录器（可为 nil）
//   - doRelease: 实际执行释放的函数
func (b *permitBase) releaseCommon(ctx context.Context, tracer trace.Tracer, semType string, logger xlog.Logger, doRelease func(context.Context) error) error {
	// 检查是否已释放，防止重复释放
	if b.isReleased() {
		return nil // 已释放，静默返回
	}

	// 创建 span
	ctx, span := startSpan(ctx, tracer, spanNameRelease)
	defer span.End()
	span.SetAttributes(releaseSpanAttributes(semType, b.resource, b.tenantID, b.id)...)

	// 停止自动续租
	b.stopAutoExtend()

	// 执行释放
	err := doRelease(ctx)
	if err != nil {
		// 如果是 ErrPermitNotHeld，说明许可已过期或已被外部释放。
		// 标记为已释放并返回 nil（保持幂等语义），但记录警告以便可观测。
		if IsPermitNotHeld(err) {
			b.markReleased()
			if logger != nil {
				logger.Warn(ctx, "permit already expired or released externally",
					AttrPermitID(b.id),
					AttrResource(b.resource),
					slog.String("sem_type", semType),
				)
			}
			setSpanOK(span)
			return nil
		}
		// 其他错误（如网络错误），不标记，允许重试
		setSpanError(span, err)
		return err
	}

	// 成功释放后标记
	b.markReleased()
	setSpanOK(span)
	return nil
}

// extendCommon 续期许可的模板方法
// 参数：
//   - ctx: 上下文
//   - tracer: 用于创建 span 的 tracer
//   - semType: 信号量类型（distributed/local）
//   - doExtend: 实际执行续期的函数，接收新的过期时间
func (b *permitBase) extendCommon(ctx context.Context, tracer trace.Tracer, semType string, doExtend func(context.Context, time.Time) error) error {
	if b.isReleased() {
		return ErrPermitNotHeld
	}

	// 创建 span
	ctx, span := startSpan(ctx, tracer, spanNameExtend)
	defer span.End()
	span.SetAttributes(extendSpanAttributes(semType, b.resource, b.tenantID, b.id)...)

	newExpiresAt := time.Now().Add(b.ttl)
	if err := doExtend(ctx, newExpiresAt); err != nil {
		setSpanError(span, err)
		return err
	}

	b.setExpiresAt(newExpiresAt)
	setSpanOK(span)
	return nil
}

// =============================================================================
// Redis 许可实现
// =============================================================================

// redisPermit 实现 Permit 接口
type redisPermit struct {
	permitBase
	sem *redisSemaphore
}

// newRedisPermit 创建新的 Redis 许可
func newRedisPermit(sem *redisSemaphore, id, resource, tenantID string, expiresAt time.Time, ttl time.Duration, hasTenantQuota bool, metadata map[string]string) *redisPermit {
	p := &redisPermit{sem: sem}
	initPermitBase(&p.permitBase, id, resource, tenantID, expiresAt, ttl, hasTenantQuota, metadata)
	return p
}

// Release 释放许可
func (p *redisPermit) Release(ctx context.Context) error {
	return p.releaseCommon(ctx, p.sem.opts.tracer, SemaphoreTypeDistributed, p.sem.opts.logger,
		func(ctx context.Context) error {
			return p.sem.releasePermit(ctx, p)
		})
}

// Extend 续期许可
func (p *redisPermit) Extend(ctx context.Context) error {
	return p.extendCommon(ctx, p.sem.opts.tracer, SemaphoreTypeDistributed,
		func(ctx context.Context, newExpiresAt time.Time) error {
			return p.sem.extendPermit(ctx, p, newExpiresAt)
		})
}

// StartAutoExtend 启动自动续租
func (p *redisPermit) StartAutoExtend(interval time.Duration) (stop func()) {
	return p.startAutoExtendLoop(interval, p.Extend, p.sem)
}

// =============================================================================
// 本地许可实现
// =============================================================================

// localPermit 本地信号量的许可实现
type localPermit struct {
	permitBase
	sem *localSemaphore
}

// newLocalPermit 创建新的本地许可
func newLocalPermit(sem *localSemaphore, id, resource, tenantID string, expiresAt time.Time, ttl time.Duration, hasTenantQuota bool, metadata map[string]string) *localPermit {
	p := &localPermit{sem: sem}
	initPermitBase(&p.permitBase, id, resource, tenantID, expiresAt, ttl, hasTenantQuota, metadata)
	return p
}

// Release 释放本地许可
func (p *localPermit) Release(ctx context.Context) error {
	return p.releaseCommon(ctx, p.sem.opts.tracer, SemaphoreTypeLocal, p.sem.opts.logger,
		func(ctx context.Context) error {
			return p.sem.releasePermit(ctx, p)
		})
}

// Extend 续期本地许可
func (p *localPermit) Extend(ctx context.Context) error {
	return p.extendCommon(ctx, p.sem.opts.tracer, SemaphoreTypeLocal,
		func(ctx context.Context, newExpiresAt time.Time) error {
			return p.sem.extendPermit(ctx, p, newExpiresAt)
		})
}

// StartAutoExtend 启动自动续租
func (p *localPermit) StartAutoExtend(interval time.Duration) (stop func()) {
	return p.startAutoExtendLoop(interval, p.Extend, p.sem)
}

// =============================================================================
// 编译时接口检查
// =============================================================================

var (
	_ Permit = (*redisPermit)(nil)
	_ Permit = (*localPermit)(nil)
)
