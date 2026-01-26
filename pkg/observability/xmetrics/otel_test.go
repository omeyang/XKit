package xmetrics

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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
