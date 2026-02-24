package mqcore

import (
	"context"

	"github.com/omeyang/xkit/pkg/context/xctx"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type otelTracerConfig struct {
	propagator propagation.TextMapPropagator
}

// OTelTracerOption 定义 OTelTracer 的配置选项。
type OTelTracerOption func(*otelTracerConfig)

// WithOTelPropagator 设置自定义的 Propagator。
func WithOTelPropagator(propagator propagation.TextMapPropagator) OTelTracerOption {
	return func(cfg *otelTracerConfig) {
		if propagator != nil {
			cfg.propagator = propagator
		}
	}
}

// OTelTracer 基于 OpenTelemetry 的链路追踪实现。
type OTelTracer struct {
	propagator propagation.TextMapPropagator
}

// NewOTelTracer 创建 OTelTracer。
func NewOTelTracer(opts ...OTelTracerOption) OTelTracer {
	cfg := &otelTracerConfig{
		propagator: propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return OTelTracer{propagator: cfg.propagator}
}

// Inject 将追踪信息注入到消息头。
func (t OTelTracer) Inject(ctx context.Context, headers map[string]string) {
	if headers == nil {
		return
	}
	ctx = ensureSpanContext(ctx)
	t.propagator.Inject(ctx, propagation.MapCarrier(headers))
}

// Extract 从消息头提取追踪信息。
func (t OTelTracer) Extract(headers map[string]string) context.Context {
	if headers == nil {
		return context.Background()
	}
	ctx := t.propagator.Extract(context.Background(), propagation.MapCarrier(headers))
	return syncTraceToXctx(ctx)
}

func ensureSpanContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.IsValid() {
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

	// 设计决策: 优先读取 xctx.TraceFlags 映射上游采样决策；
	// 仅在缺失/非法时默认 FlagsSampled（显式注入意味着调用方期望链路被采样）。
	flags := trace.FlagsSampled
	if tf := xctx.TraceFlags(ctx); tf == "00" {
		flags = 0
	}

	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    parsedTraceID,
		SpanID:     parsedSpanID,
		TraceFlags: flags,
		Remote:     true,
	})

	return trace.ContextWithSpanContext(ctx, parent)
}

func syncTraceToXctx(ctx context.Context) context.Context {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return ctx
	}

	newCtx, err := xctx.WithTraceID(ctx, spanContext.TraceID().String())
	if err == nil {
		ctx = newCtx
	}
	newCtx, err = xctx.WithSpanID(ctx, spanContext.SpanID().String())
	if err == nil {
		ctx = newCtx
	}
	// 同步 TraceFlags 到 xctx，确保下游日志 LogAttrs 包含采样决策信息。
	newCtx, err = xctx.WithTraceFlags(ctx, spanContext.TraceFlags().String())
	if err == nil {
		ctx = newCtx
	}
	return ctx
}

var _ Tracer = OTelTracer{}
