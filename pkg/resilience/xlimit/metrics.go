package xlimit

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// 指标名称常量
const (
	// metricNameRequestsTotal 请求总数计数器
	metricNameRequestsTotal = "xlimit.requests.total"
	// metricNameDeniedTotal 被限流请求计数器
	metricNameDeniedTotal = "xlimit.denied.total"
	// metricNameFallbackTotal 降级次数计数器
	metricNameFallbackTotal = "xlimit.fallback.total"
	// metricNameCheckDuration 限流检查耗时直方图
	metricNameCheckDuration = "xlimit.check.duration"
)

// Metrics 限流指标收集器
// 提供 Counter 和 Histogram 类型的指标收集
type Metrics struct {
	meter         metric.Meter
	requestsTotal metric.Int64Counter
	deniedTotal   metric.Int64Counter
	fallbackTotal metric.Int64Counter
	checkDuration metric.Float64Histogram
}

// NewMetrics 创建指标收集器
// 如果 meterProvider 为 nil，返回 nil（不收集指标）
func NewMetrics(meterProvider metric.MeterProvider) (*Metrics, error) {
	if meterProvider == nil {
		return nil, nil
	}

	meter := meterProvider.Meter("xlimit",
		metric.WithInstrumentationVersion("1.0.0"),
	)

	requestsTotal, err := meter.Int64Counter(
		metricNameRequestsTotal,
		metric.WithDescription("限流请求总数"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	deniedTotal, err := meter.Int64Counter(
		metricNameDeniedTotal,
		metric.WithDescription("被限流拒绝的请求数"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	fallbackTotal, err := meter.Int64Counter(
		metricNameFallbackTotal,
		metric.WithDescription("降级次数"),
		metric.WithUnit("{fallback}"),
	)
	if err != nil {
		return nil, err
	}

	checkDuration, err := meter.Float64Histogram(
		metricNameCheckDuration,
		metric.WithDescription("限流检查耗时"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0,
		),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		meter:         meter,
		requestsTotal: requestsTotal,
		deniedTotal:   deniedTotal,
		fallbackTotal: fallbackTotal,
		checkDuration: checkDuration,
	}, nil
}

// RecordAllow 记录限流检查结果
// ctx: 上下文，用于传播追踪信息
// limiterType: 限流器类型（"distributed" 或 "local"）
// rule: 匹配的规则名称
// allowed: 是否允许通过
// duration: 检查耗时
func (m *Metrics) RecordAllow(ctx context.Context, limiterType, rule string, allowed bool, duration time.Duration) {
	if m == nil {
		return
	}

	// 使用 context.WithoutCancel 确保即使 ctx 被取消，指标仍能记录
	metricsCtx := context.WithoutCancel(ctx)

	attrs := []attribute.KeyValue{
		attribute.String("limiter_type", limiterType),
		attribute.String("rule", rule),
		attribute.Bool("allowed", allowed),
	}

	m.requestsTotal.Add(metricsCtx, 1, metric.WithAttributes(attrs...))
	if !allowed {
		m.deniedTotal.Add(metricsCtx, 1, metric.WithAttributes(attrs...))
	}
	m.checkDuration.Record(metricsCtx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// RecordFallback 记录降级事件
// ctx: 上下文
// strategy: 降级策略（"local", "open", "close"）
// reason: 降级原因（错误信息）
func (m *Metrics) RecordFallback(ctx context.Context, strategy FallbackStrategy, reason string) {
	if m == nil {
		return
	}

	metricsCtx := context.WithoutCancel(ctx)

	attrs := []attribute.KeyValue{
		attribute.String("strategy", string(strategy)),
		attribute.String("reason", reason),
	}

	m.fallbackTotal.Add(metricsCtx, 1, metric.WithAttributes(attrs...))
}
