package xmetrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	t.Run("skip_typed_nil_value", func(t *testing.T) {
		var typedNilStr *string
		attrs := []Attr{
			{Key: "typed-nil", Value: typedNilStr},
			{Key: "valid", Value: "value"},
		}
		result := attrsToOTel(attrs)
		assert.Len(t, result, 1, "typed-nil value should be filtered like nil")
		assert.Equal(t, attribute.Key("valid"), result[0].Key)
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
