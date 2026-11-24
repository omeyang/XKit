package xmetrics

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// ============================================================================
// otelSpan.End 错误路径测试
// ============================================================================

func TestOTelSpan_End_StatusErrorWithoutErr(t *testing.T) {
	tp, exporter := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(WithTracerProvider(tp))
	require.NoError(t, err)

	_, span := obs.Start(context.Background(), SpanOptions{
		Component: "test",
		Operation: "error-no-err",
	})

	// 显式设置 StatusError 但不提供 Err
	span.End(Result{Status: StatusError})

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	// 应该设置了 error 状态
	assert.Equal(t, codes.Error, spans[0].Status.Code)
	assert.Equal(t, "operation failed", spans[0].Status.Description)
}

func TestOTelSpan_End_StatusOKWithErr(t *testing.T) {
	tp, exporter := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(WithTracerProvider(tp))
	require.NoError(t, err)

	_, span := obs.Start(context.Background(), SpanOptions{
		Component: "test",
		Operation: "ok-with-err",
	})

	// 显式设置 StatusOK 但仍提供 Err（错误被记录但不影响状态）
	span.End(Result{Status: StatusOK, Err: errors.New("logged but ok")})

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Ok, spans[0].Status.Code)
	assert.NotEmpty(t, spans[0].Events) // 错误事件仍被记录
}

// ============================================================================
// NewOTelObserver 错误路径测试
// ============================================================================

// failingMeterProvider 用于测试 meter 创建失败场景。
type failingMeterProvider struct {
	metric.MeterProvider
}

func (failingMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	return &failingMeter{}
}

type failingMeter struct {
	metric.Meter
}

func (failingMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, errors.New("counter creation failed")
}

// failingHistogramMeter 仅 histogram 创建失败。
type failingHistogramMeter struct {
	metric.Meter
}

func (failingHistogramMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	mp, _ := newTestMeterProvider()
	m := mp.Meter("test")
	return m.Int64Counter("test")
}

func (failingHistogramMeter) Float64Histogram(string, ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	return nil, errors.New("histogram creation failed")
}

type failingHistogramMeterProvider struct {
	metric.MeterProvider
}

func (failingHistogramMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	return &failingHistogramMeter{}
}

func TestNewOTelObserver_NilOption(t *testing.T) {
	t.Parallel()

	obs, err := NewOTelObserver(nil)
	assert.Nil(t, obs)
	assert.ErrorIs(t, err, ErrNilOption)
}

func TestNewOTelObserver_NilOptionAmongValid(t *testing.T) {
	t.Parallel()

	obs, err := NewOTelObserver(
		WithInstrumentationName("test"),
		nil,
		WithHistogramBuckets([]float64{0.1, 1, 10}),
	)
	assert.Nil(t, obs)
	assert.ErrorIs(t, err, ErrNilOption)
}

func TestNewOTelObserver_CounterCreationFails(t *testing.T) {
	obs, err := NewOTelObserver(WithMeterProvider(failingMeterProvider{}))
	assert.Nil(t, obs)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrCreateCounter)
}

func TestNewOTelObserver_HistogramCreationFails(t *testing.T) {
	obs, err := NewOTelObserver(WithMeterProvider(failingHistogramMeterProvider{}))
	assert.Nil(t, obs)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrCreateHistogram)
}

// ============================================================================
// FG-S1: nil span 防御测试
// ============================================================================

// nilSpanTracerProvider 返回 nil span 的 TracerProvider，用于测试 nil span 防御。
type nilSpanTracerProvider struct {
	trace.TracerProvider
}

func (nilSpanTracerProvider) Tracer(string, ...trace.TracerOption) trace.Tracer {
	return &nilSpanTracer{}
}

type nilSpanTracer struct {
	trace.Tracer
}

func (nilSpanTracer) Start(ctx context.Context, _ string, _ ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx, nil // 故意返回 nil span
}

func TestOTelObserver_Start_NilSpanFromTracer(t *testing.T) {
	// 自定义 TracerProvider 返回 nil span 时不应 panic
	obs, err := NewOTelObserver(WithTracerProvider(nilSpanTracerProvider{}))
	require.NoError(t, err)

	ctx, span := obs.Start(context.Background(), SpanOptions{
		Component: "test",
		Operation: "nil-tracer-span",
	})

	assert.NotNil(t, ctx)
	assert.NotNil(t, span)

	// End 不应 panic
	assert.NotPanics(t, func() {
		span.End(Result{})
	})
}

// typedNilSpanTracerProvider 返回 typed-nil span 的 TracerProvider，
// 用于测试 typed-nil span 防御（FG-S1 回归）。
type typedNilSpanTracerProvider struct {
	trace.TracerProvider
}

func (typedNilSpanTracerProvider) Tracer(string, ...trace.TracerOption) trace.Tracer {
	return &typedNilSpanTracer{}
}

type typedNilSpanTracer struct {
	trace.Tracer
}

// customTraceSpan 用于构造 typed-nil trace.Span。
type customTraceSpan struct {
	trace.Span
}

func (typedNilSpanTracer) Start(ctx context.Context, _ string, _ ...trace.SpanStartOption) (context.Context, trace.Span) {
	var s *customTraceSpan // typed-nil：接口 type=*customTraceSpan, value=nil
	return ctx, s
}

func TestOTelObserver_Start_TypedNilSpanFromTracer(t *testing.T) {
	// 自定义 TracerProvider 返回 typed-nil span 时不应 panic
	obs, err := NewOTelObserver(WithTracerProvider(typedNilSpanTracerProvider{}))
	require.NoError(t, err)

	ctx, span := obs.Start(context.Background(), SpanOptions{
		Component: "test",
		Operation: "typed-nil-tracer-span",
	})

	assert.NotNil(t, ctx)
	assert.NotNil(t, span)

	// End 不应 panic
	assert.NotPanics(t, func() {
		span.End(Result{})
	})
}

// ============================================================================
// FG-S2: nil instrument 防御测试
// ============================================================================

// nilCounterMeterProvider 返回 nil counter（但 err==nil）的 MeterProvider。
type nilCounterMeterProvider struct {
	metric.MeterProvider
}

func (nilCounterMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	return &nilCounterMeter{}
}

type nilCounterMeter struct {
	metric.Meter
}

func (nilCounterMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, nil // nil counter, no error
}

func TestNewOTelObserver_NilCounterFromMeter(t *testing.T) {
	obs, err := NewOTelObserver(WithMeterProvider(nilCounterMeterProvider{}))
	assert.Nil(t, obs)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrCreateCounter)
}

// nilHistogramMeterProvider 返回 nil histogram（但 err==nil）的 MeterProvider。
type nilHistogramMeterProvider struct {
	metric.MeterProvider
}

func (nilHistogramMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	return &nilHistogramMeter{}
}

type nilHistogramMeter struct {
	metric.Meter
}

func (m nilHistogramMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	mp, _ := newTestMeterProvider()
	meter := mp.Meter("test")
	return meter.Int64Counter("test")
}

func (nilHistogramMeter) Float64Histogram(string, ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	return nil, nil // nil histogram, no error
}

func TestNewOTelObserver_NilHistogramFromMeter(t *testing.T) {
	obs, err := NewOTelObserver(WithMeterProvider(nilHistogramMeterProvider{}))
	assert.Nil(t, obs)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrCreateHistogram)
}

// ============================================================================
// typed-nil Tracer/Meter/Instrument 防御测试（OTel API 契约缺口）
// ============================================================================

// typedNilTracer 是实现 trace.Tracer 的具体类型，用于构造 typed-nil 返回值。
type typedNilTracer struct{ trace.Tracer }

type typedNilTracerProvider struct{ trace.TracerProvider }

func (typedNilTracerProvider) Tracer(string, ...trace.TracerOption) trace.Tracer {
	var t *typedNilTracer
	return t // typed-nil
}

func TestNewOTelObserver_TypedNilTracer(t *testing.T) {
	obs, err := NewOTelObserver(WithTracerProvider(typedNilTracerProvider{}))
	assert.Nil(t, obs)
	assert.ErrorIs(t, err, ErrNilTracer)
}

// typedNilMeterProvider 返回 typed-nil meter。
type typedNilMeter struct{ metric.Meter }

type typedNilMeterProvider struct{ metric.MeterProvider }

func (typedNilMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	var m *typedNilMeter
	return m // typed-nil
}

func TestNewOTelObserver_TypedNilMeter(t *testing.T) {
	obs, err := NewOTelObserver(WithMeterProvider(typedNilMeterProvider{}))
	assert.Nil(t, obs)
	assert.ErrorIs(t, err, ErrNilMeter)
}

// typedNilCounterMeterProvider 返回 typed-nil counter。
type typedNilCounter struct{ metric.Int64Counter }

type typedNilCounterMeter struct{ metric.Meter }

func (typedNilCounterMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	var c *typedNilCounter
	return c, nil // typed-nil, no error
}

type typedNilCounterMeterProvider struct{ metric.MeterProvider }

func (typedNilCounterMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	return typedNilCounterMeter{}
}

func TestNewOTelObserver_TypedNilCounter(t *testing.T) {
	obs, err := NewOTelObserver(WithMeterProvider(typedNilCounterMeterProvider{}))
	assert.Nil(t, obs)
	assert.ErrorIs(t, err, ErrCreateCounter)
}

// typedNilHistogramMeter 返回有效 counter 但 typed-nil histogram。
type typedNilHistogram struct{ metric.Float64Histogram }

type typedNilHistogramMeter struct{ metric.Meter }

func (typedNilHistogramMeter) Int64Counter(name string, _ ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	mp, _ := newTestMeterProvider()
	return mp.Meter("test").Int64Counter(name)
}

func (typedNilHistogramMeter) Float64Histogram(string, ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	var h *typedNilHistogram
	return h, nil // typed-nil, no error
}

type typedNilHistogramMeterProvider struct{ metric.MeterProvider }

func (typedNilHistogramMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	return typedNilHistogramMeter{}
}

func TestNewOTelObserver_TypedNilHistogram(t *testing.T) {
	obs, err := NewOTelObserver(WithMeterProvider(typedNilHistogramMeterProvider{}))
	assert.Nil(t, obs)
	assert.ErrorIs(t, err, ErrCreateHistogram)
}

// ============================================================================
// typed-nil context 从 tracer.Start 返回的防御测试
// ============================================================================

type typedNilCtxTracerType struct{ context.Context }

type typedNilCtxTracer struct{ trace.Tracer }

func (typedNilCtxTracer) Start(_ context.Context, _ string, _ ...trace.SpanStartOption) (context.Context, trace.Span) {
	var c *typedNilCtxTracerType
	return c, trace.SpanFromContext(context.Background())
}

type typedNilCtxTracerProvider struct{ trace.TracerProvider }

func (typedNilCtxTracerProvider) Tracer(string, ...trace.TracerOption) trace.Tracer {
	return typedNilCtxTracer{}
}

func TestOTelObserver_Start_TypedNilCtxFromTracer(t *testing.T) {
	obs, err := NewOTelObserver(WithTracerProvider(typedNilCtxTracerProvider{}))
	require.NoError(t, err)

	ctx, span := obs.Start(context.Background(), SpanOptions{
		Component: "test",
		Operation: "typed-nil-ctx-tracer",
	})

	assert.NotNil(t, ctx)
	assert.NotNil(t, span)

	assert.NotPanics(t, func() {
		span.End(Result{})
	})
}

// ============================================================================
// nil ctx + valid span 时 span 传播测试
// ============================================================================

type nilCtxRealSpanTracerProvider struct {
	trace.TracerProvider
	wrapped *sdktrace.TracerProvider
}

func (p *nilCtxRealSpanTracerProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return &nilCtxRealSpanTracer{real: p.wrapped.Tracer(name, opts...)}
}

type nilCtxRealSpanTracer struct {
	trace.Tracer
	real trace.Tracer
}

func (t *nilCtxRealSpanTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	_, span := t.real.Start(ctx, name, opts...)
	return nil, span
}

func TestOTelObserver_Start_NilCtxPreservesSpanPropagation(t *testing.T) {
	tp, exporter := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(WithTracerProvider(&nilCtxRealSpanTracerProvider{wrapped: tp}))
	require.NoError(t, err)

	ctx, span := obs.Start(context.Background(), SpanOptions{
		Component: "test",
		Operation: "nil-ctx-propagation",
	})

	require.NotNil(t, ctx)
	require.NotNil(t, span)

	spanFromCtx := trace.SpanFromContext(ctx)
	assert.True(t, spanFromCtx.SpanContext().IsValid(),
		"span should be embedded in returned ctx for child propagation")

	span.End(Result{})

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "nil-ctx-propagation", spans[0].Name)
}

// ============================================================================
// typed-nil error 防御测试
// ============================================================================

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestOTelSpan_End_TypedNilError(t *testing.T) {
	tp, _ := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	mp, reader := newTestMeterProvider()
	defer func() { _ = mp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(
		WithTracerProvider(tp),
		WithMeterProvider(mp),
	)
	require.NoError(t, err)

	_, span := obs.Start(context.Background(), SpanOptions{
		Component: "test",
		Operation: "typed-nil-err",
	})

	var typedNilErr *testError // typed-nil：Err != nil 为 true，但 .Error() 会 panic

	assert.NotPanics(t, func() {
		span.End(Result{Err: typedNilErr})
	})

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "xkit.operation.total" {
				found = true
			}
		}
	}
	assert.True(t, found, "metrics should still be recorded after typed-nil error normalization")
}

func TestOTelSpan_End_TypedNilErrorWithStatusError(t *testing.T) {
	tp, exporter := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(WithTracerProvider(tp))
	require.NoError(t, err)

	_, span := obs.Start(context.Background(), SpanOptions{
		Component: "test",
		Operation: "typed-nil-err-status",
	})

	var typedNilErr *testError
	assert.NotPanics(t, func() {
		span.End(Result{Status: StatusError, Err: typedNilErr})
	})

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status.Code)
	assert.Equal(t, "operation failed", spans[0].Status.Description)
}
