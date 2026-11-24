package xmetrics

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
)

// ============================================================================
// toKeyValue 测试
// ============================================================================

func TestToKeyValue(t *testing.T) {
	tests := []struct {
		name     string
		attr     Attr
		expected attribute.KeyValue
	}{
		{
			name:     "string",
			attr:     String("key", "value"),
			expected: attribute.String("key", "value"),
		},
		{
			name:     "bool_true",
			attr:     Bool("key", true),
			expected: attribute.Bool("key", true),
		},
		{
			name:     "bool_false",
			attr:     Bool("key", false),
			expected: attribute.Bool("key", false),
		},
		{
			name:     "int",
			attr:     Int("key", 42),
			expected: attribute.Int("key", 42),
		},
		{
			name:     "int64",
			attr:     Int64("key", 100),
			expected: attribute.Int64("key", 100),
		},
		{
			name:     "uint64_within_int64",
			attr:     Uint64("key", 100),
			expected: attribute.Int64("key", 100),
		},
		{
			name:     "uint64_exceeds_int64",
			attr:     Uint64("key", math.MaxInt64+1),
			expected: attribute.String("key", "9223372036854775808"),
		},
		{
			name:     "float64",
			attr:     Float64("key", 3.14),
			expected: attribute.Float64("key", 3.14),
		},
		{
			name:     "float32",
			attr:     Attr{Key: "key", Value: float32(2.5)},
			expected: attribute.Float64("key", 2.5),
		},
		{
			name:     "duration",
			attr:     Duration("key", time.Second),
			expected: attribute.Int64("key", time.Second.Nanoseconds()),
		},
		{
			name:     "unknown_type",
			attr:     Any("key", struct{ Name string }{"test"}),
			expected: attribute.String("key", "{test}"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toKeyValue(tt.attr)
			assert.Equal(t, tt.expected.Key, got.Key)
			// 值类型和内容验证
			assert.Equal(t, tt.expected.Value.Type(), got.Value.Type())
		})
	}
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestOTelObserver_ConcurrentStartEnd(t *testing.T) {
	tp, _ := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	mp, _ := newTestMeterProvider()
	defer func() { _ = mp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(
		WithTracerProvider(tp),
		WithMeterProvider(mp),
	)
	require.NoError(t, err)

	const goroutines = 100
	var wg sync.WaitGroup

	for i := range goroutines {
		id := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 10 {
				_, span := obs.Start(context.Background(), SpanOptions{
					Component: "concurrent",
					Operation: "test",
					Attrs:     []Attr{Int("goroutine", id), Int("iteration", j)},
				})

				time.Sleep(time.Microsecond)

				span.End(Result{
					Status: StatusOK,
					Attrs:  []Attr{String("result", "done")},
				})
			}
		}()
	}

	wg.Wait()
}

// ============================================================================
// Context 传播测试
// ============================================================================

func TestOTelObserver_ContextPropagation(t *testing.T) {
	tp, exporter := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(WithTracerProvider(tp))
	require.NoError(t, err)

	// 创建父 span
	ctx1, span1 := obs.Start(context.Background(), SpanOptions{
		Component: "parent",
		Operation: "parent-op",
	})

	// 创建子 span（使用父 context）
	_, span2 := obs.Start(ctx1, SpanOptions{
		Component: "child",
		Operation: "child-op",
	})

	span2.End(Result{})
	span1.End(Result{})

	spans := exporter.GetSpans()
	require.Len(t, spans, 2)

	// 验证父子关系
	childSpan := spans[0]
	parentSpan := spans[1]

	assert.Equal(t, parentSpan.SpanContext.TraceID(), childSpan.SpanContext.TraceID())
	assert.Equal(t, parentSpan.SpanContext.SpanID(), childSpan.Parent.SpanID())
}

// ============================================================================
// Metrics 测试
// ============================================================================

func TestOTelObserver_Metrics(t *testing.T) {
	tp, _ := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	mp, reader := newTestMeterProvider()
	defer func() { _ = mp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(
		WithTracerProvider(tp),
		WithMeterProvider(mp),
	)
	require.NoError(t, err)

	// 执行几次操作
	for range 5 {
		_, span := obs.Start(context.Background(), SpanOptions{
			Component: "test",
			Operation: "metric-test",
		})
		time.Sleep(time.Millisecond)
		span.End(Result{})
	}

	// 收集 metrics
	var rm metricdata.ResourceMetrics
	err = reader.Collect(context.Background(), &rm)
	require.NoError(t, err)

	// 验证有 metric 数据
	assert.NotEmpty(t, rm.ScopeMetrics)
}

// ============================================================================
// 选项函数测试
// ============================================================================

func TestWithInstrumentationName(t *testing.T) {
	cfg := &otelConfig{}

	opt := WithInstrumentationName("custom-name")
	opt(cfg)

	assert.Equal(t, "custom-name", cfg.instrumentationName)
}

func TestWithInstrumentationName_Empty(t *testing.T) {
	cfg := &otelConfig{instrumentationName: "existing"}

	opt := WithInstrumentationName("")
	opt(cfg)

	assert.Equal(t, "existing", cfg.instrumentationName) // 不应该被覆盖
}

func TestWithTracerProvider(t *testing.T) {
	cfg := &otelConfig{}
	tp := otel.GetTracerProvider()

	opt := WithTracerProvider(tp)
	opt(cfg)

	assert.Equal(t, tp, cfg.tracerProvider)
}

func TestWithTracerProvider_Nil(t *testing.T) {
	originalTP := otel.GetTracerProvider()
	cfg := &otelConfig{tracerProvider: originalTP}

	opt := WithTracerProvider(nil)
	opt(cfg)

	assert.Equal(t, originalTP, cfg.tracerProvider) // 不应该被覆盖
}

func TestWithMeterProvider(t *testing.T) {
	cfg := &otelConfig{}
	mp := otel.GetMeterProvider()

	opt := WithMeterProvider(mp)
	opt(cfg)

	assert.Equal(t, mp, cfg.meterProvider)
}

func TestWithMeterProvider_Nil(t *testing.T) {
	originalMP := otel.GetMeterProvider()
	cfg := &otelConfig{meterProvider: originalMP}

	opt := WithMeterProvider(nil)
	opt(cfg)

	assert.Equal(t, originalMP, cfg.meterProvider) // 不应该被覆盖
}

func TestWithHistogramBuckets(t *testing.T) {
	cfg := &otelConfig{}
	buckets := []float64{0.01, 0.1, 1, 10}

	opt := WithHistogramBuckets(buckets)
	opt(cfg)

	assert.Equal(t, buckets, cfg.histogramBuckets)
}

func TestWithHistogramBuckets_Empty(t *testing.T) {
	original := []float64{0.01, 0.1, 1}
	cfg := &otelConfig{histogramBuckets: original}

	opt := WithHistogramBuckets(nil)
	opt(cfg)

	assert.Equal(t, original, cfg.histogramBuckets) // 空切片不应覆盖
}

func TestValidateBuckets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		buckets []float64
		wantErr bool
	}{
		{"valid_default", defaultDurationBuckets, false},
		{"valid_custom", []float64{0.01, 0.1, 1, 10}, false},
		{"valid_single", []float64{1.0}, false},
		{"nan", []float64{0.1, math.NaN(), 1.0}, true},
		{"positive_inf", []float64{0.1, math.Inf(1)}, true},
		{"negative_inf", []float64{math.Inf(-1), 0.1}, true},
		{"not_increasing", []float64{0.1, 0.5, 0.3}, true},
		{"duplicate", []float64{0.1, 0.5, 0.5}, true},
		{"descending", []float64{10, 5, 1}, true},
		{"negative_rejected", []float64{-1, 0, 1}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBuckets(tt.buckets)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrInvalidBuckets)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewOTelObserver_InvalidBuckets(t *testing.T) {
	tests := []struct {
		name    string
		buckets []float64
	}{
		{"nan", []float64{0.1, math.NaN(), 1.0}},
		{"inf", []float64{0.1, math.Inf(1)}},
		{"not_increasing", []float64{0.5, 0.3, 0.1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs, err := NewOTelObserver(WithHistogramBuckets(tt.buckets))
			assert.Nil(t, obs)
			assert.ErrorIs(t, err, ErrInvalidBuckets)
		})
	}
}

func TestWithHistogramBuckets_Integration(t *testing.T) {
	mp, _ := newTestMeterProvider()
	defer func() { _ = mp.Shutdown(context.Background()) }()

	customBuckets := []float64{0.005, 0.01, 0.05, 0.1, 0.5, 1}
	obs, err := NewOTelObserver(
		WithMeterProvider(mp),
		WithHistogramBuckets(customBuckets),
	)
	require.NoError(t, err)

	_, span := obs.Start(context.Background(), SpanOptions{
		Component: "test",
		Operation: "buckets-test",
	})
	span.End(Result{})
}

// ============================================================================
// 属性键常量测试
// ============================================================================

func TestAttrKeyConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "component", AttrKeyComponent)
	assert.Equal(t, "operation", AttrKeyOperation)
	assert.Equal(t, "status", AttrKeyStatus)
}

// ============================================================================
// ensureParentSpan 测试
// ============================================================================

func TestEnsureParentSpan_WithValidOTelSpan(t *testing.T) {
	// 已有有效 OTel span 的 context 应直接返回
	tp, _ := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "existing")
	defer span.End()

	result := ensureParentSpan(ctx)
	assert.Equal(t, ctx, result)
}

func TestEnsureParentSpan_WithXctxTraceInfo(t *testing.T) {
	// 无 OTel span，但 xctx 中有 trace/span ID 时应构建 remote parent
	ctx := context.Background()
	var err error

	ctx, err = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	require.NoError(t, err)
	ctx, err = xctx.WithSpanID(ctx, "b7ad6b7169203331")
	require.NoError(t, err)

	result := ensureParentSpan(ctx)
	sc := trace.SpanContextFromContext(result)

	assert.True(t, sc.IsValid())
	assert.True(t, sc.IsRemote())
	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", sc.TraceID().String())
	assert.Equal(t, "b7ad6b7169203331", sc.SpanID().String())
	// trace_flags 缺失时默认为 sampled（0x01），避免 ParentBased 采样器丢弃 trace
	assert.Equal(t, trace.TraceFlags(1), sc.TraceFlags())
}

func TestEnsureParentSpan_WithXctxTraceFlags(t *testing.T) {
	ctx := context.Background()
	var err error

	ctx, err = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	require.NoError(t, err)
	ctx, err = xctx.WithSpanID(ctx, "b7ad6b7169203331")
	require.NoError(t, err)
	ctx, err = xctx.WithTraceFlags(ctx, "01")
	require.NoError(t, err)

	result := ensureParentSpan(ctx)
	sc := trace.SpanContextFromContext(result)

	assert.True(t, sc.IsValid())
	assert.Equal(t, trace.TraceFlags(1), sc.TraceFlags())
}

func TestEnsureParentSpan_WithInvalidTraceFlags(t *testing.T) {
	ctx := context.Background()
	var err error

	ctx, err = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	require.NoError(t, err)
	ctx, err = xctx.WithSpanID(ctx, "b7ad6b7169203331")
	require.NoError(t, err)
	ctx, err = xctx.WithTraceFlags(ctx, "zz") // 无效十六进制
	require.NoError(t, err)

	result := ensureParentSpan(ctx)
	sc := trace.SpanContextFromContext(result)

	assert.True(t, sc.IsValid())
	// 无效 trace_flags 解析失败，回退为默认值 sampled（0x01）
	assert.Equal(t, trace.TraceFlags(1), sc.TraceFlags())
}

func TestEnsureParentSpan_WithExplicitUnsampled(t *testing.T) {
	// 显式设置 trace_flags="00"（unsampled）时应尊重调用方决策
	ctx := context.Background()
	var err error

	ctx, err = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	require.NoError(t, err)
	ctx, err = xctx.WithSpanID(ctx, "b7ad6b7169203331")
	require.NoError(t, err)
	ctx, err = xctx.WithTraceFlags(ctx, "00")
	require.NoError(t, err)

	result := ensureParentSpan(ctx)
	sc := trace.SpanContextFromContext(result)

	assert.True(t, sc.IsValid())
	assert.Equal(t, trace.TraceFlags(0), sc.TraceFlags()) // 显式 unsampled 应被保留
}

func TestEnsureParentSpan_NoTraceInfo(t *testing.T) {
	// 无 OTel span，xctx 也无 trace ID 时应直接返回原 context
	ctx := context.Background()
	result := ensureParentSpan(ctx)
	assert.Equal(t, ctx, result)
}

func TestEnsureParentSpan_PartialXctxInfo(t *testing.T) {
	// 只有 traceID 没有 spanID 时应直接返回
	ctx := context.Background()
	var err error
	ctx, err = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	require.NoError(t, err)

	result := ensureParentSpan(ctx)
	sc := trace.SpanContextFromContext(result)
	assert.False(t, sc.IsValid())
}

func TestEnsureParentSpan_InvalidTraceID(t *testing.T) {
	ctx := context.Background()
	var err error
	ctx, err = xctx.WithTraceID(ctx, "not-a-valid-trace-id")
	require.NoError(t, err)
	ctx, err = xctx.WithSpanID(ctx, "b7ad6b7169203331")
	require.NoError(t, err)

	result := ensureParentSpan(ctx)
	sc := trace.SpanContextFromContext(result)
	assert.False(t, sc.IsValid()) // 无效 traceID 应返回原 context
}

func TestEnsureParentSpan_InvalidSpanID(t *testing.T) {
	ctx := context.Background()
	var err error
	ctx, err = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	require.NoError(t, err)
	ctx, err = xctx.WithSpanID(ctx, "invalid-span-id")
	require.NoError(t, err)

	result := ensureParentSpan(ctx)
	sc := trace.SpanContextFromContext(result)
	assert.False(t, sc.IsValid()) // 无效 spanID 应返回原 context
}

// ============================================================================
// traceFlagsToHex 测试
// ============================================================================

func TestTraceFlagsToHex(t *testing.T) {
	tests := []struct {
		flags    trace.TraceFlags
		expected string
	}{
		{0, "00"},
		{1, "01"},
		{15, "0f"},
		{16, "10"},
		{255, "ff"},
		{170, "aa"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := traceFlagsToHex(tt.flags)
			assert.Equal(t, tt.expected, got)
		})
	}
}
