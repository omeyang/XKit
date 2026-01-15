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
	defaultInstrumentationName = "github.com/omeyang/xkit/xmetrics"
	unknownComponent           = "unknown"
	unknownOperation           = "unknown"

	metricOperationTotal    = "xkit.operation.total"
	metricOperationDuration = "xkit.operation.duration"
)

type otelConfig struct {
	instrumentationName string
	tracerProvider      trace.TracerProvider
	meterProvider       metric.MeterProvider
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

// NewOTelObserver 创建基于 OpenTelemetry 的 Observer。
func NewOTelObserver(opts ...Option) (Observer, error) {
	cfg := &otelConfig{
		instrumentationName: defaultInstrumentationName,
		tracerProvider:      otel.GetTracerProvider(),
		meterProvider:       otel.GetMeterProvider(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	tracer := cfg.tracerProvider.Tracer(cfg.instrumentationName)
	meter := cfg.meterProvider.Meter(cfg.instrumentationName)

	total, err := meter.Int64Counter(
		metricOperationTotal,
		metric.WithDescription("total operations"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("xmetrics: create counter failed: %w", err)
	}

	duration, err := meter.Float64Histogram(
		metricOperationDuration,
		metric.WithDescription("operation duration"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("xmetrics: create histogram failed: %w", err)
	}

	return &otelObserver{
		tracer:   tracer,
		meter:    meter,
		total:    total,
		duration: duration,
	}, nil
}

type otelObserver struct {
	tracer   trace.Tracer
	meter    metric.Meter
	total    metric.Int64Counter
	duration metric.Float64Histogram
}

// Start 开始一次观测跨度。
func (o *otelObserver) Start(ctx context.Context, opts SpanOptions) (context.Context, Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = ensureParentSpan(ctx)

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
		attribute.String("component", component),
		attribute.String("operation", operation),
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

		if s.observer == nil {
			return
		}

		// 使用不可取消的 context 记录指标，确保即使请求 context 已取消/超时，
		// 指标仍能正确记录。这对于失败/超时场景的可观测性至关重要。
		// 注意：context.WithoutCancel 会保留 context 中的 values（如 baggage）。
		metricsCtx := context.WithoutCancel(s.ctx)
		elapsed := time.Since(s.start).Seconds()
		attrs := metricAttrs(s.component, s.operation, status)
		s.observer.total.Add(metricsCtx, 1, metric.WithAttributes(attrs...))
		s.observer.duration.Record(metricsCtx, elapsed, metric.WithAttributes(attrs...))
	})
}

func resolveStatus(result Result) Status {
	if result.Status != "" {
		return result.Status
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
	attrs[0] = attribute.String("component", component)
	attrs[1] = attribute.String("operation", operation)
	attrs[2] = attribute.String("status", string(status))
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
	flagsStr := fmt.Sprintf("%02x", sc.TraceFlags())
	newCtx, err = xctx.WithTraceFlags(ctx, flagsStr)
	if err == nil {
		ctx = newCtx
	}
	return ctx
}
