package xmetrics

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// newTestTracerProvider 创建用于测试的 TracerProvider
func newTestTracerProvider() (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	return tp, exporter
}

// newTestMeterProvider 创建用于测试的 MeterProvider
func newTestMeterProvider() (*sdkmetric.MeterProvider, *sdkmetric.ManualReader) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
	)
	return mp, reader
}

// ============================================================================
// NewOTelObserver 测试
// ============================================================================

func TestNewOTelObserver_Default(t *testing.T) {
	obs, err := NewOTelObserver()
	require.NoError(t, err)
	require.NotNil(t, obs)
}

func TestNewOTelObserver_WithOptions(t *testing.T) {
	tp, _ := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	mp, _ := newTestMeterProvider()
	defer func() { _ = mp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(
		WithInstrumentationName("test-instrumentation"),
		WithTracerProvider(tp),
		WithMeterProvider(mp),
	)

	require.NoError(t, err)
	require.NotNil(t, obs)
}

func TestNewOTelObserver_WithEmptyInstrumentationName(t *testing.T) {
	// 空名称应该使用默认值
	obs, err := NewOTelObserver(WithInstrumentationName(""))
	require.NoError(t, err)
	require.NotNil(t, obs)
}

func TestNewOTelObserver_WithNilProviders(t *testing.T) {
	// nil provider 应该使用全局默认
	obs, err := NewOTelObserver(
		WithTracerProvider(nil),
		WithMeterProvider(nil),
	)
	require.NoError(t, err)
	require.NotNil(t, obs)
}

// ============================================================================
// Observer.Start 测试
// ============================================================================

func TestOTelObserver_Start_Basic(t *testing.T) {
	tp, exporter := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(WithTracerProvider(tp))
	require.NoError(t, err)

	ctx := context.Background()
	newCtx, span := obs.Start(ctx, SpanOptions{
		Component: "test-component",
		Operation: "test-operation",
	})

	require.NotNil(t, newCtx)
	require.NotNil(t, span)

	span.End(Result{})

	// 验证 span 被记录
	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "test-operation", spans[0].Name)
}

func TestOTelObserver_Start_NilContext(t *testing.T) {
	tp, _ := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(WithTracerProvider(tp))
	require.NoError(t, err)

	// nil context 应该被安全处理
	var nilCtx context.Context
	newCtx, span := obs.Start(nilCtx, SpanOptions{
		Component: "test",
		Operation: "nil-ctx",
	})

	require.NotNil(t, newCtx) // 应该返回 background context
	require.NotNil(t, span)

	span.End(Result{})
}

func TestOTelObserver_Start_EmptyOptions(t *testing.T) {
	tp, exporter := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(WithTracerProvider(tp))
	require.NoError(t, err)

	ctx := context.Background()
	_, span := obs.Start(ctx, SpanOptions{})

	span.End(Result{})

	// 应该使用默认值
	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "unknown", spans[0].Name) // unknownOperation
}

func TestOTelObserver_Start_AllKinds(t *testing.T) {
	tp, exporter := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(WithTracerProvider(tp))
	require.NoError(t, err)

	tests := []struct {
		kind         Kind
		expectedKind trace.SpanKind
	}{
		{KindInternal, trace.SpanKindInternal},
		{KindServer, trace.SpanKindServer},
		{KindClient, trace.SpanKindClient},
		{KindProducer, trace.SpanKindProducer},
		{KindConsumer, trace.SpanKindConsumer},
	}

	for _, tt := range tests {
		t.Run(tt.expectedKind.String(), func(t *testing.T) {
			exporter.Reset()

			_, span := obs.Start(context.Background(), SpanOptions{
				Component: "test",
				Operation: "kind-test",
				Kind:      tt.kind,
			})
			span.End(Result{})

			spans := exporter.GetSpans()
			require.Len(t, spans, 1)
			assert.Equal(t, tt.expectedKind, spans[0].SpanKind)
		})
	}
}

func TestOTelObserver_Start_WithAttrs(t *testing.T) {
	tp, exporter := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(WithTracerProvider(tp))
	require.NoError(t, err)

	_, span := obs.Start(context.Background(), SpanOptions{
		Component: "test",
		Operation: "attrs-test",
		Attrs: []Attr{
			String("service", "my-service"),
			Int("port", 8080),
			Bool("enabled", true),
		},
	})
	span.End(Result{})

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	// 验证属性（包括默认的 component 和 operation）
	attrs := spans[0].Attributes
	assert.True(t, len(attrs) >= 5) // component, operation + 3 custom
}

// ============================================================================
// Span.End 测试
// ============================================================================

func TestOTelSpan_End_Success(t *testing.T) {
	tp, exporter := newTestTracerProvider()
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
		Operation: "success",
	})
	span.End(Result{Status: StatusOK})

	// 验证 trace
	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	// 验证 metrics
	var rm metricdata.ResourceMetrics
	err = reader.Collect(context.Background(), &rm)
	require.NoError(t, err)
}

func TestOTelSpan_End_WithError(t *testing.T) {
	tp, exporter := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(WithTracerProvider(tp))
	require.NoError(t, err)

	_, span := obs.Start(context.Background(), SpanOptions{
		Component: "test",
		Operation: "error",
	})

	testErr := errors.New("test error")
	span.End(Result{Err: testErr})

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	// 验证错误被记录
	events := spans[0].Events
	assert.NotEmpty(t, events) // 应该有错误事件
}

func TestOTelSpan_End_WithResultAttrs(t *testing.T) {
	tp, exporter := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(WithTracerProvider(tp))
	require.NoError(t, err)

	_, span := obs.Start(context.Background(), SpanOptions{
		Component: "test",
		Operation: "result-attrs",
	})
	span.End(Result{
		Status: StatusOK,
		Attrs: []Attr{
			Int64("bytes", 1024),
			String("cache", "hit"),
		},
	})

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
}

func TestOTelSpan_End_Nil(t *testing.T) {
	// nil span 的 End 不应该 panic
	var span *otelSpan
	assert.NotPanics(t, func() {
		span.End(Result{})
	})
}

func TestOTelSpan_End_MultipleTimes(t *testing.T) {
	tp, _ := newTestTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	obs, err := NewOTelObserver(WithTracerProvider(tp))
	require.NoError(t, err)

	_, span := obs.Start(context.Background(), SpanOptions{
		Component: "test",
		Operation: "multi-end",
	})

	// 多次 End 不应该 panic
	assert.NotPanics(t, func() {
		span.End(Result{})
		span.End(Result{})
		span.End(Result{})
	})
}

// ============================================================================
// resolveStatus 测试
// ============================================================================

func TestResolveStatus(t *testing.T) {
	tests := []struct {
		name     string
		result   Result
		expected Status
	}{
		{
			name:     "explicit_ok",
			result:   Result{Status: StatusOK},
			expected: StatusOK,
		},
		{
			name:     "explicit_error",
			result:   Result{Status: StatusError},
			expected: StatusError,
		},
		{
			name:     "infer_error_from_err",
			result:   Result{Err: errors.New("error")},
			expected: StatusError,
		},
		{
			name:     "infer_ok_from_empty",
			result:   Result{},
			expected: StatusOK,
		},
		{
			name:     "explicit_overrides_err",
			result:   Result{Status: StatusOK, Err: errors.New("ignored")},
			expected: StatusOK, // 显式状态优先
		},
		{
			name:     "unknown_status_no_err_falls_back_to_ok",
			result:   Result{Status: Status("timeout")},
			expected: StatusOK, // 未知状态回退到 Err 推导
		},
		{
			name:     "unknown_status_with_err_falls_back_to_error",
			result:   Result{Status: Status("partial"), Err: errors.New("partial failure")},
			expected: StatusError, // 未知状态 + 有 Err → error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveStatus(tt.result)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ============================================================================
// mapSpanKind 测试
// ============================================================================

func TestMapSpanKind(t *testing.T) {
	tests := []struct {
		input    Kind
		expected trace.SpanKind
	}{
		{KindInternal, trace.SpanKindInternal},
		{KindServer, trace.SpanKindServer},
		{KindClient, trace.SpanKindClient},
		{KindProducer, trace.SpanKindProducer},
		{KindConsumer, trace.SpanKindConsumer},
		{Kind(99), trace.SpanKindInternal}, // 未知类型默认为 Internal
	}

	for _, tt := range tests {
		t.Run(tt.expected.String(), func(t *testing.T) {
			got := mapSpanKind(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ============================================================================
// attrsToOTel 测试
// ============================================================================

func TestAttrsToOTel(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		result := attrsToOTel(nil)
		assert.Nil(t, result)

		result = attrsToOTel([]Attr{})
		assert.Nil(t, result)
	})

	t.Run("skip_empty_key", func(t *testing.T) {
		attrs := []Attr{
			{Key: "", Value: "value"},
			{Key: "valid", Value: "value"},
		}
		result := attrsToOTel(attrs)
		assert.Len(t, result, 1)
		assert.Equal(t, "valid", string(result[0].Key))
	})

	t.Run("skip_nil_value", func(t *testing.T) {
		attrs := []Attr{
			{Key: "nil", Value: nil},
			{Key: "valid", Value: "value"},
		}
		result := attrsToOTel(attrs)
		assert.Len(t, result, 1)
	})

	t.Run("skip_reserved_keys", func(t *testing.T) {
		attrs := []Attr{
			{Key: AttrKeyComponent, Value: "override"},
			{Key: AttrKeyOperation, Value: "override"},
			{Key: AttrKeyStatus, Value: "override"},
			{Key: "valid", Value: "value"},
		}
		result := attrsToOTel(attrs)
		assert.Len(t, result, 1)
		assert.Equal(t, attribute.Key("valid"), result[0].Key)
	})

	t.Run("all_reserved_returns_nil", func(t *testing.T) {
		attrs := []Attr{
			{Key: AttrKeyComponent, Value: "override"},
			{Key: AttrKeyOperation, Value: "override"},
			{Key: AttrKeyStatus, Value: "override"},
		}
		result := attrsToOTel(attrs)
		assert.Nil(t, result)
	})

	t.Run("all_types", func(t *testing.T) {
		attrs := []Attr{
			String("str", "value"),
			Bool("bool", true),
			Int("int", 42),
			Int64("int64", 100),
			Uint64("uint64", 200),
			Float64("float64", 3.14),
			Duration("duration", time.Second),
		}
		result := attrsToOTel(attrs)
		assert.Len(t, result, 7)
	})
}

// ============================================================================
// isReservedAttrKey 测试
// ============================================================================

func TestIsReservedAttrKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key      string
		reserved bool
	}{
		{AttrKeyComponent, true},
		{AttrKeyOperation, true},
		{AttrKeyStatus, true},
		{"service", false},
		{"db.system", false},
		{"", false},
		{"Component", false}, // 大小写敏感
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			assert.Equal(t, tt.reserved, isReservedAttrKey(tt.key))
		})
	}
}

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
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < 10; j++ {
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
		}(i)
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
	for i := 0; i < 5; i++ {
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
