package mqcore

import (
	"context"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// =============================================================================
// OTelTracer Option Tests
// =============================================================================

func TestWithOTelPropagator(t *testing.T) {
	customPropagator := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
	)

	tracer := NewOTelTracer(WithOTelPropagator(customPropagator))

	// 应该使用自定义 propagator
	assert.NotNil(t, tracer.propagator)
}

func TestWithOTelPropagator_Nil(t *testing.T) {
	tracer := NewOTelTracer(WithOTelPropagator(nil))

	// 应该使用默认 propagator
	assert.NotNil(t, tracer.propagator)
}

func TestNewOTelTracer_Default(t *testing.T) {
	tracer := NewOTelTracer()

	assert.NotNil(t, tracer.propagator)
}

func TestNewOTelTracer_MultipleOptions(t *testing.T) {
	propagator1 := propagation.TraceContext{}
	propagator2 := propagation.Baggage{}
	composite := propagation.NewCompositeTextMapPropagator(propagator1, propagator2)

	tracer := NewOTelTracer(WithOTelPropagator(composite))

	assert.NotNil(t, tracer.propagator)
}

// =============================================================================
// OTelTracer Inject Tests
// =============================================================================

func TestOTelTracer_Inject_NilHeaders(t *testing.T) {
	tracer := NewOTelTracer()
	ctx := context.Background()

	// 不应 panic
	assert.NotPanics(t, func() {
		tracer.Inject(ctx, nil)
	})
}

func TestOTelTracer_Inject_EmptyContext(t *testing.T) {
	tracer := NewOTelTracer()
	headers := make(map[string]string)

	tracer.Inject(context.Background(), headers)

	// 空 context 不应有 traceparent
	assert.Empty(t, headers)
}

func TestOTelTracer_Inject_WithXctxIDs(t *testing.T) {
	tracer := NewOTelTracer()
	headers := make(map[string]string)

	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")

	tracer.Inject(ctx, headers)

	// 应该有 traceparent header
	traceparent, ok := headers["traceparent"]
	assert.True(t, ok)
	assert.Contains(t, traceparent, "0af7651916cd43dd8448eb211c80319c")
	assert.Contains(t, traceparent, "b7ad6b7169203331")
}

func TestOTelTracer_Inject_WithInvalidTraceID(t *testing.T) {
	tracer := NewOTelTracer()
	headers := make(map[string]string)

	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "invalid-trace-id")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")

	tracer.Inject(ctx, headers)

	// 无效的 trace ID，不应生成 traceparent
	assert.Empty(t, headers)
}

func TestOTelTracer_Inject_WithInvalidSpanID(t *testing.T) {
	tracer := NewOTelTracer()
	headers := make(map[string]string)

	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "invalid-span-id")

	tracer.Inject(ctx, headers)

	// 无效的 span ID，不应生成 traceparent
	assert.Empty(t, headers)
}

func TestOTelTracer_Inject_OnlyTraceID(t *testing.T) {
	tracer := NewOTelTracer()
	headers := make(map[string]string)

	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")

	tracer.Inject(ctx, headers)

	// 只有 trace ID，不应生成 traceparent
	assert.Empty(t, headers)
}

func TestOTelTracer_Inject_OnlySpanID(t *testing.T) {
	tracer := NewOTelTracer()
	headers := make(map[string]string)

	ctx := context.Background()
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")

	tracer.Inject(ctx, headers)

	// 只有 span ID，不应生成 traceparent
	assert.Empty(t, headers)
}

// =============================================================================
// OTelTracer Extract Tests
// =============================================================================

func TestOTelTracer_Extract_NilHeaders(t *testing.T) {
	tracer := NewOTelTracer()

	result := tracer.Extract(nil)

	assert.NotNil(t, result)
}

func TestOTelTracer_Extract_EmptyHeaders(t *testing.T) {
	tracer := NewOTelTracer()

	result := tracer.Extract(map[string]string{})

	assert.NotNil(t, result)
}

func TestOTelTracer_Extract_ValidTraceparent(t *testing.T) {
	tracer := NewOTelTracer()
	headers := map[string]string{
		"traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
	}

	result := tracer.Extract(headers)

	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", xctx.TraceID(result))
	assert.Equal(t, "b7ad6b7169203331", xctx.SpanID(result))
}

func TestOTelTracer_Extract_InvalidTraceparent(t *testing.T) {
	tracer := NewOTelTracer()
	headers := map[string]string{
		"traceparent": "invalid-traceparent",
	}

	result := tracer.Extract(headers)

	// 无效的 traceparent 应返回空 context
	assert.Empty(t, xctx.TraceID(result))
	assert.Empty(t, xctx.SpanID(result))
}

// =============================================================================
// ensureSpanContext Tests
// =============================================================================

func TestEnsureSpanContext_NilContext(t *testing.T) {
	var nilCtx context.Context
	result := ensureSpanContext(nilCtx)

	assert.NotNil(t, result)
}

func TestEnsureSpanContext_ValidSpanContext(t *testing.T) {
	// 创建一个有效的 span context
	traceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	spanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)

	result := ensureSpanContext(ctx)

	// 应返回原始 context
	sc := trace.SpanContextFromContext(result)
	assert.True(t, sc.IsValid())
	assert.Equal(t, traceID, sc.TraceID())
}

func TestEnsureSpanContext_FromXctx(t *testing.T) {
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")

	result := ensureSpanContext(ctx)

	// 应从 xctx 构建 span context
	sc := trace.SpanContextFromContext(result)
	assert.True(t, sc.IsValid())
	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", sc.TraceID().String())
	assert.Equal(t, "b7ad6b7169203331", sc.SpanID().String())
}

func TestEnsureSpanContext_EmptyXctx(t *testing.T) {
	ctx := context.Background()

	result := ensureSpanContext(ctx)

	// 没有 xctx 信息，应返回原始 context
	sc := trace.SpanContextFromContext(result)
	assert.False(t, sc.IsValid())
}

func TestEnsureSpanContext_InvalidTraceIDFormat(t *testing.T) {
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "invalid-format")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")

	result := ensureSpanContext(ctx)

	// 无效的 trace ID 格式，应返回原始 context
	sc := trace.SpanContextFromContext(result)
	assert.False(t, sc.IsValid())
}

func TestEnsureSpanContext_InvalidSpanIDFormat(t *testing.T) {
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "invalid-format")

	result := ensureSpanContext(ctx)

	// 无效的 span ID 格式，应返回原始 context
	sc := trace.SpanContextFromContext(result)
	assert.False(t, sc.IsValid())
}

func TestEnsureSpanContext_RespectsTraceFlags_NotSampled(t *testing.T) {
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")
	ctx, _ = xctx.WithTraceFlags(ctx, "00")

	result := ensureSpanContext(ctx)

	sc := trace.SpanContextFromContext(result)
	assert.True(t, sc.IsValid())
	assert.False(t, sc.IsSampled(), "TraceFlags 00 should not be sampled")
}

func TestEnsureSpanContext_RespectsTraceFlags_Sampled(t *testing.T) {
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")
	ctx, _ = xctx.WithTraceFlags(ctx, "01")

	result := ensureSpanContext(ctx)

	sc := trace.SpanContextFromContext(result)
	assert.True(t, sc.IsValid())
	assert.True(t, sc.IsSampled(), "TraceFlags 01 should be sampled")
}

func TestEnsureSpanContext_DefaultSampled_NoTraceFlags(t *testing.T) {
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")
	// 不设置 TraceFlags

	result := ensureSpanContext(ctx)

	sc := trace.SpanContextFromContext(result)
	assert.True(t, sc.IsValid())
	assert.True(t, sc.IsSampled(), "missing TraceFlags should default to sampled")
}

// =============================================================================
// syncTraceToXctx Tests
// =============================================================================

func TestSyncTraceToXctx_InvalidSpanContext(t *testing.T) {
	ctx := context.Background()

	result := syncTraceToXctx(ctx)

	// 无效的 span context，应返回原始 context
	assert.Empty(t, xctx.TraceID(result))
	assert.Empty(t, xctx.SpanID(result))
}

func TestSyncTraceToXctx_ValidSpanContext(t *testing.T) {
	traceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	spanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)

	result := syncTraceToXctx(ctx)

	// 应同步到 xctx
	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", xctx.TraceID(result))
	assert.Equal(t, "b7ad6b7169203331", xctx.SpanID(result))
	assert.Equal(t, "01", xctx.TraceFlags(result), "TraceFlags should be synced")
}

func TestSyncTraceToXctx_TraceFlags_NotSampled(t *testing.T) {
	traceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	spanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: 0, // not sampled
		Remote:     true,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)

	result := syncTraceToXctx(ctx)

	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", xctx.TraceID(result))
	assert.Equal(t, "00", xctx.TraceFlags(result), "TraceFlags 00 (not sampled) should be synced")
}

// =============================================================================
// OTelTracer Interface Compliance
// =============================================================================

func TestOTelTracer_ImplementsTracer(t *testing.T) {
	var tracer Tracer = NewOTelTracer()
	assert.NotNil(t, tracer)
}

// =============================================================================
// Round-trip Tests
// =============================================================================

func TestOTelTracer_RoundTrip(t *testing.T) {
	tracer := NewOTelTracer()

	// Inject
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")

	headers := make(map[string]string)
	tracer.Inject(ctx, headers)

	// Extract
	extracted := tracer.Extract(headers)

	// 验证 round-trip
	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", xctx.TraceID(extracted))
	assert.Equal(t, "b7ad6b7169203331", xctx.SpanID(extracted))
}

func TestOTelTracer_InjectExtract(t *testing.T) {
	tracer := NewOTelTracer()
	headers := make(map[string]string)

	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")

	tracer.Inject(ctx, headers)

	traceparent, ok := headers["traceparent"]
	assert.True(t, ok)
	assert.Contains(t, traceparent, "0af7651916cd43dd8448eb211c80319c")
	assert.Contains(t, traceparent, "b7ad6b7169203331")

	extracted := tracer.Extract(headers)
	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", xctx.TraceID(extracted))
	assert.Equal(t, "b7ad6b7169203331", xctx.SpanID(extracted))
}
