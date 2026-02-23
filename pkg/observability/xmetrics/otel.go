package xmetrics

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/omeyang/xkit/pkg/context/xctx"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultInstrumentationName = "github.com/omeyang/xkit/pkg/observability/xmetrics"
	unknownComponent           = "unknown"
	unknownOperation           = "unknown"

	metricOperationTotal    = "xkit.operation.total"
	metricOperationDuration = "xkit.operation.duration"

	// AttrKeyComponent 是 metrics/trace 中组件名称的属性键。
	AttrKeyComponent = "component"
	// AttrKeyOperation 是 metrics/trace 中操作名称的属性键。
	AttrKeyOperation = "operation"
	// AttrKeyStatus 是 metrics 中操作状态的属性键。
	AttrKeyStatus = "status"
)

// defaultDurationBuckets 定义了适用于典型 API 操作的 Histogram 桶边界（秒）。
// 覆盖 1ms 到 10s 范围，对热路径操作（<100ms）有足够细粒度区分 P50/P95/P99。
var defaultDurationBuckets = []float64{
	0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
}

type otelConfig struct {
	instrumentationName string
	tracerProvider      trace.TracerProvider
	meterProvider       metric.MeterProvider
	histogramBuckets    []float64
}

// Option 定义 OTel Observer 的配置选项。
type Option func(*otelConfig)

// WithInstrumentationName 设置 OTel instrumentation 名称。
func WithInstrumentationName(name string) Option {
	return func(cfg *otelConfig) {
		if name != "" {
			cfg.instrumentationName = name
		}
	}
}

// WithTracerProvider 设置 TracerProvider。
func WithTracerProvider(provider trace.TracerProvider) Option {
	return func(cfg *otelConfig) {
		if provider != nil {
			cfg.tracerProvider = provider
		}
	}
}

// WithMeterProvider 设置 MeterProvider。
func WithMeterProvider(provider metric.MeterProvider) Option {
	return func(cfg *otelConfig) {
		if provider != nil {
			cfg.meterProvider = provider
		}
	}
}

// WithHistogramBuckets 设置 duration Histogram 的桶边界（单位：秒）。
// 默认值为 [0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]，
// 适用于典型 API 操作（1ms ~ 10s）。
func WithHistogramBuckets(buckets []float64) Option {
	return func(cfg *otelConfig) {
		if len(buckets) > 0 {
			cfg.histogramBuckets = buckets
		}
	}
}

// NewOTelObserver 创建基于 OpenTelemetry 的 Observer。
func NewOTelObserver(opts ...Option) (Observer, error) {
	cfg := &otelConfig{
		instrumentationName: defaultInstrumentationName,
		tracerProvider:      otel.GetTracerProvider(),
		meterProvider:       otel.GetMeterProvider(),
		histogramBuckets:    defaultDurationBuckets,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if err := validateBuckets(cfg.histogramBuckets); err != nil {
		return nil, err
	}

	tracer := cfg.tracerProvider.Tracer(cfg.instrumentationName)
	meter := cfg.meterProvider.Meter(cfg.instrumentationName)

	total, err := meter.Int64Counter(
		metricOperationTotal,
		metric.WithDescription("total operations"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateCounter, err)
	}

	duration, err := meter.Float64Histogram(
		metricOperationDuration,
		metric.WithDescription("operation duration"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(cfg.histogramBuckets...),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateHistogram, err)
	}

	return &otelObserver{
		tracer:   tracer,
		total:    total,
		duration: duration,
	}, nil
}

type otelObserver struct {
	tracer   trace.Tracer
	total    metric.Int64Counter
	duration metric.Float64Histogram
}

// Start 开始一次观测跨度。
func (o *otelObserver) Start(ctx context.Context, opts SpanOptions) (context.Context, Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = ensureParentSpan(ctx)

	// 设计决策: component/operation 不做运行时长度或模式校验。
	// 原因：(1) 任何长度/模式检查都无法真正防止高基数（短 UUID 也会膨胀）；
	// (2) 静默截断/降级会掩盖调用方的错误使用，比不检查更难排查；
	// (3) 作为工具库，在热路径增加校验的性价比低，文档约束更适合。
	// 相关文档：doc.go "component / operation 使用约束" 段落。
	component := opts.Component
	if component == "" {
		component = unknownComponent
	}
	operation := opts.Operation
	if operation == "" {
		operation = unknownOperation
	}

	attrs := make([]attribute.KeyValue, 0, 2+len(opts.Attrs))
	attrs = append(attrs,
		attribute.String(AttrKeyComponent, component),
		attribute.String(AttrKeyOperation, operation),
	)
	attrs = append(attrs, attrsToOTel(opts.Attrs)...)

	ctx, span := o.tracer.Start(
		ctx,
		operation,
		trace.WithSpanKind(mapSpanKind(opts.Kind)),
		trace.WithAttributes(attrs...),
	)

	ctx = syncXctx(ctx, span.SpanContext())

	return ctx, &otelSpan{
		span:      span,
		observer:  o,
		ctx:       ctx,
		component: component,
		operation: operation,
		start:     time.Now(),
	}
}

type otelSpan struct {
	span      trace.Span
	observer  *otelObserver
	ctx       context.Context
	component string
	operation string
	start     time.Time
	endOnce   sync.Once // 保证 End 幂等，多次调用只记录一次 metrics
}

// End 结束观测并记录结果。
//
// End 是幂等的，多次调用只会记录一次 metrics。
// 这避免了因错误使用（如 defer 和显式调用都触发）导致的指标膨胀。
func (s *otelSpan) End(result Result) {
	if s == nil {
		return
	}

	s.endOnce.Do(func() {
		status := resolveStatus(result)

		// 统一使用 resolveStatus 结果设置 span 状态，确保与 metrics 一致
		switch status {
		case StatusError:
			if result.Err != nil {
				s.span.RecordError(result.Err)
				s.span.SetStatus(codes.Error, result.Err.Error())
			} else {
				// Status 显式设为 error 但无 Err，使用通用错误消息
				s.span.SetStatus(codes.Error, "operation failed")
			}
		default:
			// StatusOK 或其他状态
			if result.Err != nil {
				// 有 Err 但 Status 显式设为非 error，仍记录错误但不影响状态
				s.span.RecordError(result.Err)
			}
			s.span.SetStatus(codes.Ok, "")
		}

		if len(result.Attrs) > 0 {
			s.span.SetAttributes(attrsToOTel(result.Attrs)...)
		}

		s.span.End()

		// 使用不可取消的 context 记录指标，确保即使请求 context 已取消/超时，
		// 指标仍能正确记录。这对于失败/超时场景的可观测性至关重要。
		// 注意：context.WithoutCancel 会保留 context 中的 values（如 baggage）。
		// 当前 OTel SDK 的 Add/Record 调用是同步的，metricsCtx 不会被 SDK 延迟持有，
		// 因此 values 不存在语义过期风险。若未来 OTel SDK 行为变化需重新评估。
		metricsCtx := context.WithoutCancel(s.ctx)
		elapsed := time.Since(s.start).Seconds()
		attrs := metricAttrs(s.component, s.operation, status)
		s.observer.total.Add(metricsCtx, 1, metric.WithAttributes(attrs...))
		s.observer.duration.Record(metricsCtx, elapsed, metric.WithAttributes(attrs...))

		// 释放 context 引用，避免长生命周期 span 阻止 GC 回收 context 链上的值。
		s.ctx = nil
	})
}

// resolveStatus 将 Result 解析为有效的 Status。
//
// 设计决策: Status 收敛为 StatusOK / StatusError 两种值。
// 若 result.Status 为未知值，按 Err 字段推导（有 Err → error，否则 → ok）。
// 这避免了 metrics 的 status 维度出现高基数风险。
func resolveStatus(result Result) Status {
	switch result.Status {
	case StatusOK:
		return StatusOK
	case StatusError:
		return StatusError
	case "":
		// 空状态：根据 Err 推导
	default:
		// 设计决策: 未知 Status 值不透传到 metrics（防止高基数），
		// 回退到 Err 推导逻辑，与空 Status 行为一致。
	}
	if result.Err != nil {
		return StatusError
	}
	return StatusOK
}

func mapSpanKind(kind Kind) trace.SpanKind {
	switch kind {
	case KindServer:
		return trace.SpanKindServer
	case KindClient:
		return trace.SpanKindClient
	case KindProducer:
		return trace.SpanKindProducer
	case KindConsumer:
		return trace.SpanKindConsumer
	default:
		return trace.SpanKindInternal
	}
}

func metricAttrs(component, operation string, status Status) []attribute.KeyValue {
	var attrs [3]attribute.KeyValue
	attrs[0] = attribute.String(AttrKeyComponent, component)
	attrs[1] = attribute.String(AttrKeyOperation, operation)
	attrs[2] = attribute.String(AttrKeyStatus, string(status))
	return attrs[:]
}

func attrsToOTel(attrs []Attr) []attribute.KeyValue {
	if len(attrs) == 0 {
		return nil
	}
	converted := make([]attribute.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		if attr.Key == "" || attr.Value == nil {
			continue
		}
		converted = append(converted, toKeyValue(attr))
	}
	return converted
}

func toKeyValue(attr Attr) attribute.KeyValue {
	switch v := attr.Value.(type) {
	case string:
		return attribute.String(attr.Key, v)
	case bool:
		return attribute.Bool(attr.Key, v)
	case int:
		return attribute.Int(attr.Key, v)
	case int64:
		return attribute.Int64(attr.Key, v)
	case uint64:
		if v <= math.MaxInt64 {
			return attribute.Int64(attr.Key, int64(v))
		}
		return attribute.String(attr.Key, fmt.Sprint(v))
	case float64:
		return attribute.Float64(attr.Key, v)
	case float32:
		return attribute.Float64(attr.Key, float64(v))
	case time.Duration:
		return attribute.Int64(attr.Key, v.Nanoseconds())
	default:
		return attribute.String(attr.Key, fmt.Sprint(v))
	}
}

func ensureParentSpan(ctx context.Context) context.Context {
	span := trace.SpanFromContext(ctx)
	if span != nil && span.SpanContext().IsValid() {
		return ctx
	}

	traceID := xctx.TraceID(ctx)
	spanID := xctx.SpanID(ctx)
	if traceID == "" || spanID == "" {
		return ctx
	}

	parsedTraceID, err := trace.TraceIDFromHex(traceID)
	if err != nil {
		return ctx
	}
	parsedSpanID, err := trace.SpanIDFromHex(spanID)
	if err != nil {
		return ctx
	}

	// 从 xctx 解析 TraceFlags，默认为 0（未采样）
	var traceFlags trace.TraceFlags
	if flagsStr := xctx.TraceFlags(ctx); flagsStr != "" {
		if parsed, err := strconv.ParseUint(flagsStr, 16, 8); err == nil {
			traceFlags = trace.TraceFlags(parsed)
		}
	}

	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    parsedTraceID,
		SpanID:     parsedSpanID,
		TraceFlags: traceFlags,
		Remote:     true,
	})

	return trace.ContextWithSpanContext(ctx, parent)
}

// syncXctx 将 OTel SpanContext 中的 trace/span ID 同步到 xctx。
//
// 设计决策: xctx.WithXxx 的错误被安全忽略（if err == nil 模式），因为：
// 1. 这些函数仅在 ctx 为 nil 时返回 ErrNilContext
// 2. 调用方 Start 已保证 ctx 非 nil（nil 已被归一化为 context.Background()）
// 3. 即使未来 xctx 增加新校验，跳过同步不影响核心功能（仅影响 xctx 链路信息）
func syncXctx(ctx context.Context, sc trace.SpanContext) context.Context {
	if !sc.IsValid() {
		return ctx
	}
	newCtx, err := xctx.WithTraceID(ctx, sc.TraceID().String())
	if err == nil {
		ctx = newCtx
	}
	newCtx, err = xctx.WithSpanID(ctx, sc.SpanID().String())
	if err == nil {
		ctx = newCtx
	}
	// 同步 TraceFlags 到 xctx（格式：2位十六进制，如 "01"）
	flagsStr := traceFlagsToHex(sc.TraceFlags())
	newCtx, err = xctx.WithTraceFlags(ctx, flagsStr)
	if err == nil {
		ctx = newCtx
	}
	return ctx
}

// validateBuckets 校验 Histogram 桶边界的合法性。
// 要求：所有值必须是非负有限数（非 NaN/Inf、≥ 0），且严格递增。
// 桶边界用于 duration Histogram（记录 time.Since 秒数，始终 ≥ 0），负值无实际意义。
func validateBuckets(buckets []float64) error {
	for i, b := range buckets {
		if math.IsNaN(b) || math.IsInf(b, 0) {
			return fmt.Errorf("%w: bucket[%d] is NaN or Inf", ErrInvalidBuckets, i)
		}
		if b < 0 {
			return fmt.Errorf("%w: bucket[%d] (%g) must be non-negative", ErrInvalidBuckets, i, b)
		}
		if i > 0 && b <= buckets[i-1] {
			return fmt.Errorf("%w: bucket[%d] (%g) must be greater than bucket[%d] (%g)",
				ErrInvalidBuckets, i, b, i-1, buckets[i-1])
		}
	}
	return nil
}

const hexDigits = "0123456789abcdef"

// hexLookup 预计算所有 256 种 TraceFlags 值对应的 2 位十六进制字符串。
// 查表实现在调用时零分配。
var hexLookup = func() [256]string {
	var t [256]string
	for i := range t {
		t[i] = string([]byte{hexDigits[i>>4], hexDigits[i&0x0f]})
	}
	return t
}()

// traceFlagsToHex 将 TraceFlags 转换为 2 位十六进制字符串。
// 使用预计算查表实现，调用时零分配。
func traceFlagsToHex(flags trace.TraceFlags) string {
	return hexLookup[flags]
}
