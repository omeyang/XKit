package xsemaphore

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/omeyang/xkit/pkg/observability/xlog"
	"github.com/omeyang/xkit/pkg/util/xid"
)

// =============================================================================
// ID 生成器
// =============================================================================

// IDGeneratorFunc 许可 ID 生成函数。
// 返回唯一字符串 ID 和可能的错误。
type IDGeneratorFunc func(ctx context.Context) (string, error)

// =============================================================================
// 降级策略
// =============================================================================

// FallbackStrategy 降级策略
type FallbackStrategy string

const (
	// FallbackNone 不降级（默认）
	FallbackNone FallbackStrategy = ""

	// FallbackLocal 降级到本地信号量（推荐）
	// 本地容量 = 全局容量 / Pod 数量
	FallbackLocal FallbackStrategy = "local"

	// FallbackOpen 放行所有请求（fail-open）
	// 适用于信号量不是强需求的场景
	FallbackOpen FallbackStrategy = "open"

	// FallbackClose 拒绝所有请求（fail-close）
	// 适用于安全要求极高的场景
	FallbackClose FallbackStrategy = "close"
)

// IsValid 检查降级策略是否有效
func (s FallbackStrategy) IsValid() bool {
	switch s {
	case FallbackNone, FallbackLocal, FallbackOpen, FallbackClose:
		return true
	default:
		return false
	}
}

// =============================================================================
// 工厂配置选项
// =============================================================================

// options 工厂内部配置
type options struct {
	keyPrefix            string
	logger               xlog.Logger
	meterProvider        metric.MeterProvider
	tracerProvider       trace.TracerProvider
	tracer               trace.Tracer
	metrics              *Metrics
	fallback             FallbackStrategy
	podCount             int
	onFallback           func(resource string, strategy FallbackStrategy, err error)
	disableResourceLabel bool            // 禁用 resource 标签，避免高基数问题
	defaultTimeout       time.Duration   // 默认操作超时时间
	idGenerator          IDGeneratorFunc // 许可 ID 生成函数，nil 时使用 xid.NewStringWithRetry
}

// Option 工厂配置选项函数
type Option func(*options)

// defaultOptions 返回默认工厂配置
func defaultOptions() *options {
	return &options{
		keyPrefix: DefaultKeyPrefix,
		podCount:  DefaultPodCount,
	}
}

// WithKeyPrefix 设置 Redis 键前缀
// 默认为 "xsemaphore:"
//
// 注意：prefix 不能包含 `{` 或 `}`，否则会破坏 Redis Cluster hash tag 机制。
// 空值表示不修改默认前缀。无效前缀会在 New() 的 validate() 中返回错误。
func WithKeyPrefix(prefix string) Option {
	return func(o *options) {
		if prefix != "" {
			o.keyPrefix = prefix
		}
	}
}

// WithLogger 设置日志记录器
// 使用 xlog 进行结构化日志记录
func WithLogger(logger xlog.Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}

// WithMeterProvider 设置 OpenTelemetry MeterProvider
// 用于收集 Counter/Histogram 类型的指标
// 如果不设置，不会收集指标
func WithMeterProvider(mp metric.MeterProvider) Option {
	return func(o *options) {
		o.meterProvider = mp
	}
}

// WithTracerProvider 设置 OpenTelemetry TracerProvider
// 用于创建分布式追踪 span
// 如果不设置，会使用全局 TracerProvider（otel.GetTracerProvider()）
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(o *options) {
		o.tracerProvider = tp
	}
}

// WithFallback 设置 Redis 不可用时的降级策略
// 可选值：FallbackLocal, FallbackOpen, FallbackClose
// 无效策略会在 New() 的 validate() 中返回错误
func WithFallback(strategy FallbackStrategy) Option {
	return func(o *options) {
		o.fallback = strategy
	}
}

// WithPodCount 设置预期 Pod 数量
// 用于计算本地降级时的容量：本地容量 = max(1, 全局容量 / PodCount)
// 无效值（<= 0）会在 New() 的 validate() 中返回错误
//
// 注意：本地容量至少为 1，因此总本地容量可能超过全局配置。
// 这是可用性优先的设计，确保降级时每个 Pod 都能处理至少一个请求。
func WithPodCount(count int) Option {
	return func(o *options) {
		o.podCount = count
	}
}

// WithOnFallback 设置降级时的回调
// 当 Redis 不可用触发降级时调用
func WithOnFallback(fn func(resource string, strategy FallbackStrategy, err error)) Option {
	return func(o *options) {
		o.onFallback = fn
	}
}

// WithDisableResourceLabel 禁用指标中的 resource 标签
// 当资源名称为动态生成时（如包含用户 ID），建议启用此选项以避免高基数问题
// 高基数标签会导致指标存储和查询性能下降
func WithDisableResourceLabel() Option {
	return func(o *options) {
		o.disableResourceLabel = true
	}
}

// WithDefaultTimeout 设置操作的默认超时时间
// 当调用 TryAcquire/Acquire/Query 时，如果传入的 context 没有设置 deadline，
// 会自动添加此超时时间。
//
// 设置为 0 或负值表示不设置默认超时（保持原有行为）。
// 默认值为 0（不设置超时）。
//
// 使用示例：
//
//	sem, _ := xsemaphore.New(rdb,
//	    xsemaphore.WithDefaultTimeout(5 * time.Second),
//	)
//	// 后续调用会自动应用 5 秒超时
//	permit, err := sem.TryAcquire(ctx, "resource", ...)
func WithDefaultTimeout(timeout time.Duration) Option {
	return func(o *options) {
		if timeout > 0 {
			o.defaultTimeout = timeout
		}
	}
}

// WithIDGenerator 设置许可 ID 生成函数。
// 默认使用 xid.NewStringWithRetry。
// 通过此选项可以替换为自定义实现，便于测试和解耦。
func WithIDGenerator(fn IDGeneratorFunc) Option {
	return func(o *options) {
		if fn != nil {
			o.idGenerator = fn
		}
	}
}

// effectiveIDGenerator 返回有效的 ID 生成函数
func (o *options) effectiveIDGenerator() IDGeneratorFunc {
	if o.idGenerator != nil {
		return o.idGenerator
	}
	return xid.NewStringWithRetry
}

// validate 验证工厂配置
func (o *options) validate() error {
	if err := validateKeyPrefix(o.keyPrefix); err != nil {
		return err
	}
	if o.podCount <= 0 {
		return fmt.Errorf("%w: pod count must be positive, got %d", ErrInvalidPodCount, o.podCount)
	}
	if o.fallback != FallbackNone && !o.fallback.IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidFallbackStrategy, o.fallback)
	}
	return nil
}

// effectivePodCount 返回有效的 Pod 数量
func (o *options) effectivePodCount() int {
	if o.podCount <= 0 {
		return DefaultPodCount
	}
	return o.podCount
}

// =============================================================================
// 获取配置选项
// =============================================================================

// acquireOptions 获取许可的内部配置
type acquireOptions struct {
	capacity    int
	tenantID    string
	tenantQuota int
	ttl         time.Duration
	maxRetries  int
	retryDelay  time.Duration
	metadata    map[string]string
}

// AcquireOption 获取许可的配置选项函数
type AcquireOption func(*acquireOptions)

// defaultAcquireOptions 返回默认获取配置
func defaultAcquireOptions() *acquireOptions {
	return &acquireOptions{
		capacity:   DefaultCapacity,
		ttl:        DefaultTTL,
		maxRetries: DefaultMaxRetries,
		retryDelay: DefaultRetryDelay,
	}
}

// validate 验证获取选项（TryAcquire 和 Acquire 共用的校验）
//
// 设计决策: maxRetries 和 retryDelay 仅对 Acquire 有意义，不在此处校验。
// TryAcquire 不使用重试参数，不应因用户传入了 WithMaxRetries(0) 而报错。
// Acquire 通过 validateRetryParams 单独校验重试参数。
func (o *acquireOptions) validate() error {
	if o.capacity <= 0 {
		return fmt.Errorf("%w: capacity must be positive, got %d", ErrInvalidCapacity, o.capacity)
	}
	if o.ttl <= 0 {
		return fmt.Errorf("%w: ttl must be positive", ErrInvalidTTL)
	}
	if o.tenantQuota < 0 {
		return fmt.Errorf("%w: tenant quota cannot be negative", ErrInvalidTenantQuota)
	}
	if o.tenantQuota > 0 && o.tenantQuota > o.capacity {
		return fmt.Errorf("%w: tenant quota (%d) cannot exceed capacity (%d)", ErrInvalidTenantQuota, o.tenantQuota, o.capacity)
	}
	return nil
}

// validateRetryParams 验证重试相关参数（仅 Acquire 调用）
func (o *acquireOptions) validateRetryParams() error {
	if o.maxRetries <= 0 {
		return fmt.Errorf("%w: max retries must be positive, got %d", ErrInvalidMaxRetries, o.maxRetries)
	}
	if o.retryDelay <= 0 {
		return fmt.Errorf("%w: retry delay must be positive, got %s", ErrInvalidRetryDelay, o.retryDelay)
	}
	return nil
}

// WithCapacity 设置全局容量上限
// 这是必须配置的选项，表示该资源全局最多允许多少个并发许可
// 无效值（<= 0）会在 validate() 中返回错误
//
// 容量在每次调用时传入（而非工厂创建时），这是设计选择：
//   - 提供灵活性，允许不同场景使用不同配置
//   - 与 xdlock 的设计模式一致
func WithCapacity(capacity int) AcquireOption {
	return func(o *acquireOptions) {
		o.capacity = capacity
	}
}

// WithTenantID 设置租户 ID
// 如果不设置，会尝试从 context 中通过 xtenant 自动提取
func WithTenantID(tenantID string) AcquireOption {
	return func(o *acquireOptions) {
		o.tenantID = tenantID
	}
}

// WithTenantQuota 设置租户配额上限
// 每个租户最多允许的并发许可数
// 只有同时设置了 TenantID 时才生效
// 不能超过全局容量，负值会在 validate() 中返回错误
func WithTenantQuota(quota int) AcquireOption {
	return func(o *acquireOptions) {
		o.tenantQuota = quota
	}
}

// WithTTL 设置许可的过期时间
// 默认为 5 分钟
// 许可过期后会自动释放，防止因进程崩溃导致许可泄漏
// 无效值（<= 0）会在 validate() 中返回错误
func WithTTL(ttl time.Duration) AcquireOption {
	return func(o *acquireOptions) {
		o.ttl = ttl
	}
}

// WithMaxRetries 设置阻塞获取时的最大尝试次数（包含首次尝试）
// 例如：WithMaxRetries(10) 表示首次尝试 + 9 次重试 = 共 10 次尝试
// 默认为 10 次
// 仅对 Acquire 方法有效
// 无效值（<= 0）会在 validate() 中返回错误
func WithMaxRetries(n int) AcquireOption {
	return func(o *acquireOptions) {
		o.maxRetries = n
	}
}

// WithRetryDelay 设置重试间隔
// 默认为 100ms
// 仅对 Acquire 方法有效
// 无效值（<= 0）会在 validate() 中返回错误
func WithRetryDelay(d time.Duration) AcquireOption {
	return func(o *acquireOptions) {
		o.retryDelay = d
	}
}

// WithMetadata 设置许可的元数据
// 元数据会被复制存储在许可中，可通过 Permit.Metadata() 获取
// 用于携带业务上下文信息，如 trace_id、request_id 等
//
// 示例:
//
//	permit, _ := sem.TryAcquire(ctx, "resource",
//	    xsemaphore.WithCapacity(10),
//	    xsemaphore.WithMetadata(map[string]string{"trace_id": "xxx", "user_id": "123"}),
//	)
//	meta := permit.Metadata() // {"trace_id": "xxx", "user_id": "123"}
func WithMetadata(metadata map[string]string) AcquireOption {
	return func(o *acquireOptions) {
		if len(metadata) > 0 {
			o.metadata = metadata
		}
	}
}

// =============================================================================
// 查询配置选项
// =============================================================================

// queryOptions 查询的内部配置
type queryOptions struct {
	capacity    int
	tenantID    string
	tenantQuota int
}

// QueryOption 查询的配置选项函数
type QueryOption func(*queryOptions)

// defaultQueryOptions 返回默认查询配置
func defaultQueryOptions() *queryOptions {
	return &queryOptions{
		capacity: DefaultCapacity,
	}
}

// validate 验证查询选项
func (o *queryOptions) validate() error {
	if o.capacity <= 0 {
		return fmt.Errorf("%w: capacity must be positive, got %d", ErrInvalidCapacity, o.capacity)
	}
	if o.tenantQuota < 0 {
		return fmt.Errorf("%w: tenant quota cannot be negative, got %d", ErrInvalidTenantQuota, o.tenantQuota)
	}
	// 设计决策: o.capacity > 0 条件已由上方 capacity <= 0 校验保证，此处省略。
	if o.tenantQuota > 0 && o.tenantQuota > o.capacity {
		return fmt.Errorf("%w: tenant quota (%d) cannot exceed capacity (%d)", ErrInvalidTenantQuota, o.tenantQuota, o.capacity)
	}
	return nil
}

// QueryWithCapacity 设置查询时的全局容量
// 用于计算可用许可数
// 负值会在 validate() 中返回错误
func QueryWithCapacity(capacity int) QueryOption {
	return func(o *queryOptions) {
		o.capacity = capacity
	}
}

// QueryWithTenantID 设置查询的租户 ID
// 如果不设置，会尝试从 context 中自动提取
func QueryWithTenantID(tenantID string) QueryOption {
	return func(o *queryOptions) {
		o.tenantID = tenantID
	}
}

// QueryWithTenantQuota 设置查询时的租户配额
// 用于计算租户可用许可数
// 负值会在 validate() 中返回错误
func QueryWithTenantQuota(quota int) QueryOption {
	return func(o *queryOptions) {
		o.tenantQuota = quota
	}
}
