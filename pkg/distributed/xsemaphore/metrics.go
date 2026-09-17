package xsemaphore

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// 设计决策: 指标前缀使用 "xsemaphore.*" 而非 "xkit.xsemaphore.*"。
// 项目各包自治命名，与 OTel Meter scope name 保持一致（Meter("xsemaphore")），
// 避免与 scope 名称产生冗余嵌套。如需统一命名空间，应在采集端（Prometheus relabel）处理。
const (
	// metricNameAcquireTotal 获取许可次数计数器
	metricNameAcquireTotal = "xsemaphore.acquire.total"
	// metricNameReleaseTotal 释放许可次数计数器
	metricNameReleaseTotal = "xsemaphore.release.total"
	// metricNameExtendTotal 续期次数计数器
	metricNameExtendTotal = "xsemaphore.extend.total"
	// metricNameFallbackTotal 降级次数计数器
	metricNameFallbackTotal = "xsemaphore.fallback.total"
	// metricNameAcquireDuration 获取许可耗时直方图
	metricNameAcquireDuration = "xsemaphore.acquire.duration"
	// metricNameQueryTotal 查询次数计数器
	metricNameQueryTotal = "xsemaphore.query.total"
	// metricNameQueryDuration 查询耗时直方图
	metricNameQueryDuration = "xsemaphore.query.duration"
)

// Metrics 信号量指标收集器
// 提供 Counter 和 Histogram 类型的指标收集
type Metrics struct {
	meter                metric.Meter
	acquireTotal         metric.Int64Counter
	releaseTotal         metric.Int64Counter
	extendTotal          metric.Int64Counter
	fallbackTotal        metric.Int64Counter
	acquireDuration      metric.Float64Histogram
	queryTotal           metric.Int64Counter
	queryDuration        metric.Float64Histogram
	disableResourceLabel bool // 是否禁用 resource 标签
}

// NewMetrics 创建指标收集器
// 如果 meterProvider 为 nil，返回 nil（不收集指标）
func NewMetrics(meterProvider metric.MeterProvider, opts ...MetricsOption) (*Metrics, error) {
	if meterProvider == nil {
		return nil, nil
	}

	m := &Metrics{}
	for _, opt := range opts {
		opt(m)
	}

	m.meter = meterProvider.Meter("xsemaphore",
		metric.WithInstrumentationVersion(instrumentationVersion),
	)

	if err := m.initCounters(); err != nil {
		return nil, err
	}
	if err := m.initHistograms(); err != nil {
		return nil, err
	}

	return m, nil
}

// durationBuckets 耗时直方图的桶边界
var durationBuckets = []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0}

// initCounters 初始化所有计数器指标
func (m *Metrics) initCounters() error {
	var err error
	if m.acquireTotal, err = m.meter.Int64Counter(metricNameAcquireTotal,
		metric.WithDescription("信号量获取许可次数"), metric.WithUnit("{acquire}")); err != nil {
		return err
	}
	if m.releaseTotal, err = m.meter.Int64Counter(metricNameReleaseTotal,
		metric.WithDescription("信号量释放许可次数"), metric.WithUnit("{release}")); err != nil {
		return err
	}
	if m.extendTotal, err = m.meter.Int64Counter(metricNameExtendTotal,
		metric.WithDescription("信号量续期次数"), metric.WithUnit("{extend}")); err != nil {
		return err
	}
	if m.fallbackTotal, err = m.meter.Int64Counter(metricNameFallbackTotal,
		metric.WithDescription("信号量降级次数"), metric.WithUnit("{fallback}")); err != nil {
		return err
	}
	if m.queryTotal, err = m.meter.Int64Counter(metricNameQueryTotal,
		metric.WithDescription("信号量查询次数"), metric.WithUnit("{query}")); err != nil {
		return err
	}
	return nil
}

// initHistograms 初始化所有直方图指标
func (m *Metrics) initHistograms() error {
	var err error
	if m.acquireDuration, err = m.meter.Float64Histogram(metricNameAcquireDuration,
		metric.WithDescription("信号量获取许可耗时"), metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(durationBuckets...)); err != nil {
		return err
	}
	if m.queryDuration, err = m.meter.Float64Histogram(metricNameQueryDuration,
		metric.WithDescription("信号量查询耗时"), metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(durationBuckets...)); err != nil {
		return err
	}
	return nil
}

// MetricsOption 指标收集器配置选项
type MetricsOption func(*Metrics)

// MetricsWithDisableResourceLabel 禁用 resource 标签
// 当资源名称为动态生成时（如包含用户 ID），建议启用此选项以避免高基数问题
func MetricsWithDisableResourceLabel() MetricsOption {
	return func(m *Metrics) {
		m.disableResourceLabel = true
	}
}

// RecordAcquire 记录获取许可结果
// ctx: 上下文，用于传播追踪信息
// semType: 信号量类型（"distributed" 或 "local"）
// resource: 资源名称
// acquired: 是否成功获取
// reason: 失败原因（成功时为 ReasonUnknown）
// duration: 获取耗时
func (m *Metrics) RecordAcquire(
	ctx context.Context,
	semType string,
	resource string,
	acquired bool,
	reason AcquireFailReason,
	duration time.Duration,
) {
	if m == nil {
		return
	}

	// 使用 context.WithoutCancel 确保即使 ctx 被取消，指标仍能记录
	metricsCtx := context.WithoutCancel(ctx)

	attrs := []attribute.KeyValue{
		attribute.String(attrSemType, semType),
		attribute.Bool(attrAcquired, acquired),
	}

	// 仅在未禁用时添加 resource 标签
	if !m.disableResourceLabel {
		attrs = append(attrs, attribute.String(attrResource, resource))
	}

	if !acquired {
		attrs = append(attrs, attribute.String(attrFailReason, reason.String()))
	}

	m.acquireTotal.Add(metricsCtx, 1, metric.WithAttributes(attrs...))
	m.acquireDuration.Record(metricsCtx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// RecordRelease 记录释放许可
// ctx: 上下文
// semType: 信号量类型
// resource: 资源名称
//
// 设计决策: Release 和 Extend 仅记录 counter，不记录 duration histogram。
// 这些操作是单次 Lua 脚本执行（release）或内存操作（local），耗时极短且稳定，
// 不需要分位数分布分析。网络抖动场景可通过 trace span 耗时观测。
func (m *Metrics) RecordRelease(ctx context.Context, semType, resource string) {
	if m == nil {
		return
	}

	metricsCtx := context.WithoutCancel(ctx)

	attrs := []attribute.KeyValue{
		attribute.String(attrSemType, semType),
	}

	// 仅在未禁用时添加 resource 标签
	if !m.disableResourceLabel {
		attrs = append(attrs, attribute.String(attrResource, resource))
	}

	m.releaseTotal.Add(metricsCtx, 1, metric.WithAttributes(attrs...))
}

// RecordExtend 记录续期
// ctx: 上下文
// semType: 信号量类型
// resource: 资源名称
// success: 是否成功
func (m *Metrics) RecordExtend(ctx context.Context, semType, resource string, success bool) {
	if m == nil {
		return
	}

	metricsCtx := context.WithoutCancel(ctx)

	attrs := []attribute.KeyValue{
		attribute.String(attrSemType, semType),
		attribute.Bool(attrSuccess, success),
	}

	// 仅在未禁用时添加 resource 标签
	if !m.disableResourceLabel {
		attrs = append(attrs, attribute.String(attrResource, resource))
	}

	m.extendTotal.Add(metricsCtx, 1, metric.WithAttributes(attrs...))
}

// RecordFallback 记录降级事件
// ctx: 上下文
// strategy: 降级策略（"local", "open", "close"）
// resource: 资源名称
// reason: 降级原因（错误信息）
func (m *Metrics) RecordFallback(ctx context.Context, strategy FallbackStrategy, resource, reason string) {
	if m == nil {
		return
	}

	metricsCtx := context.WithoutCancel(ctx)

	attrs := []attribute.KeyValue{
		attribute.String(attrStrategy, string(strategy)),
		attribute.String(attrFailReason, reason),
	}

	// 仅在未禁用时添加 resource 标签
	if !m.disableResourceLabel {
		attrs = append(attrs, attribute.String(attrResource, resource))
	}

	m.fallbackTotal.Add(metricsCtx, 1, metric.WithAttributes(attrs...))
}

// RecordQuery 记录查询操作
// ctx: 上下文
// semType: 信号量类型
// resource: 资源名称
// success: 是否成功
// duration: 查询耗时
func (m *Metrics) RecordQuery(ctx context.Context, semType, resource string, success bool, duration time.Duration) {
	if m == nil {
		return
	}

	metricsCtx := context.WithoutCancel(ctx)

	attrs := []attribute.KeyValue{
		attribute.String(attrSemType, semType),
		attribute.Bool(attrSuccess, success),
	}

	// 仅在未禁用时添加 resource 标签
	if !m.disableResourceLabel {
		attrs = append(attrs, attribute.String(attrResource, resource))
	}

	m.queryTotal.Add(metricsCtx, 1, metric.WithAttributes(attrs...))
	m.queryDuration.Record(metricsCtx, duration.Seconds(), metric.WithAttributes(attrs...))
}
