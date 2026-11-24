package xmetrics_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// ============================================================================
// Kind 常量测试
// ============================================================================

func TestKindConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind xmetrics.Kind
		want int
	}{
		{"KindInternal", xmetrics.KindInternal, 0},
		{"KindServer", xmetrics.KindServer, 1},
		{"KindClient", xmetrics.KindClient, 2},
		{"KindProducer", xmetrics.KindProducer, 3},
		{"KindConsumer", xmetrics.KindConsumer, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, int(tt.kind))
		})
	}
}

// ============================================================================
// Status 常量测试
// ============================================================================

func TestStatusConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, xmetrics.Status("ok"), xmetrics.StatusOK)
	assert.Equal(t, xmetrics.Status("error"), xmetrics.StatusError)
}

// ============================================================================
// 类型别名验证
// ============================================================================

func TestTypeAliases(t *testing.T) {
	t.Parallel()

	// 验证 Attr 类型
	attr := xmetrics.Attr{Key: "test", Value: "value"}
	assert.Equal(t, "test", attr.Key)
	assert.Equal(t, "value", attr.Value)

	// 验证 SpanOptions 类型
	opts := xmetrics.SpanOptions{
		Component: "comp",
		Operation: "op",
		Kind:      xmetrics.KindServer,
		Attrs:     []xmetrics.Attr{{Key: "k", Value: "v"}},
	}
	assert.Equal(t, "comp", opts.Component)
	assert.Equal(t, "op", opts.Operation)
	assert.Equal(t, xmetrics.KindServer, opts.Kind)
	assert.Len(t, opts.Attrs, 1)

	// 验证 Result 类型
	result := xmetrics.Result{
		Status: xmetrics.StatusOK,
		Err:    nil,
		Attrs:  []xmetrics.Attr{{Key: "k", Value: "v"}},
	}
	assert.Equal(t, xmetrics.StatusOK, result.Status)
	assert.Nil(t, result.Err)
}

// ============================================================================
// Start 函数测试
// ============================================================================

func TestStart_NilObserver(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	newCtx, span := xmetrics.Start(ctx, nil, xmetrics.SpanOptions{
		Component: "test",
		Operation: "nil-observer",
	})

	assert.NotNil(t, newCtx)
	assert.NotNil(t, span)
	assert.Equal(t, ctx, newCtx) // nil observer 返回原始 ctx

	// span 应该是 NoopSpan
	_, ok := span.(xmetrics.NoopSpan)
	assert.True(t, ok)

	// 调用 End 不应 panic
	assert.NotPanics(t, func() {
		span.End(xmetrics.Result{})
	})
}

func TestStart_NoopObserver(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	_, span := xmetrics.Start(ctx, xmetrics.NoopObserver{}, xmetrics.SpanOptions{
		Component: "test",
		Operation: "noop",
	})

	require.NotNil(t, span)
	require.NotPanics(t, func() {
		span.End(xmetrics.Result{})
	})
}

func TestStart_WithAllKinds(t *testing.T) {
	t.Parallel()

	kinds := []xmetrics.Kind{
		xmetrics.KindInternal,
		xmetrics.KindServer,
		xmetrics.KindClient,
		xmetrics.KindProducer,
		xmetrics.KindConsumer,
	}

	for _, kind := range kinds {
		t.Run("Kind_"+kind.String(), func(t *testing.T) {
			ctx := context.Background()
			_, span := xmetrics.Start(ctx, xmetrics.NoopObserver{}, xmetrics.SpanOptions{
				Component: "test",
				Operation: "kind-test",
				Kind:      kind,
			})
			assert.NotNil(t, span)
			span.End(xmetrics.Result{})
		})
	}
}

func TestStart_NilContext(t *testing.T) {
	t.Parallel()

	var nilCtx context.Context
	// nil context + nil observer → ctx 归一化为 context.Background()
	newCtx, span := xmetrics.Start(nilCtx, nil, xmetrics.SpanOptions{})

	assert.NotNil(t, newCtx) // nil ctx 被归一化为 context.Background()
	assert.NotNil(t, span)
}

// ============================================================================
// NoopObserver 和 NoopSpan 测试
// ============================================================================

func TestNoopObserver_Start(t *testing.T) {
	t.Parallel()

	observer := xmetrics.NoopObserver{}
	ctx := context.Background()

	newCtx, span := observer.Start(ctx, xmetrics.SpanOptions{
		Component: "test",
		Operation: "op",
	})

	assert.NotNil(t, newCtx)
	assert.NotNil(t, span)
	assert.Equal(t, ctx, newCtx)
}

func TestNoopSpan_End(t *testing.T) {
	t.Parallel()

	span := xmetrics.NoopSpan{}

	// 各种结果调用 End 都不应该 panic
	results := []xmetrics.Result{
		{},
		{Status: xmetrics.StatusOK},
		{Status: xmetrics.StatusError},
		{Err: errors.New("error")},
		{Status: xmetrics.StatusError, Err: errors.New("error")},
		{Attrs: []xmetrics.Attr{{Key: "k", Value: "v"}}},
	}

	for i, result := range results {
		t.Run("Result_"+strconv.Itoa(i), func(t *testing.T) {
			assert.NotPanics(t, func() {
				span.End(result)
			})
		})
	}
}

func TestNoopSpan_End_MultipleTimes(t *testing.T) {
	t.Parallel()

	span := xmetrics.NoopSpan{}

	// 多次调用 End 不应该 panic
	assert.NotPanics(t, func() {
		span.End(xmetrics.Result{})
		span.End(xmetrics.Result{})
		span.End(xmetrics.Result{})
	})
}

// ============================================================================
// NewOTelObserver 测试
// ============================================================================

func TestNewOTelObserver_Default(t *testing.T) {
	obs, err := xmetrics.NewOTelObserver()
	require.NoError(t, err)
	require.NotNil(t, obs)
}

func TestNewOTelObserver_StartEnd(t *testing.T) {
	obs, err := xmetrics.NewOTelObserver()
	require.NoError(t, err)
	require.NotNil(t, obs)

	ctx, span := xmetrics.Start(context.Background(), obs, xmetrics.SpanOptions{
		Component: "test",
		Operation: "op",
	})
	require.NotNil(t, ctx)
	require.NotNil(t, span)

	require.NotPanics(t, func() {
		span.End(xmetrics.Result{})
	})
}

func TestNewOTelObserver_WithInstrumentationName(t *testing.T) {
	obs, err := xmetrics.NewOTelObserver(
		xmetrics.WithInstrumentationName("my-instrumentation"),
	)
	require.NoError(t, err)
	require.NotNil(t, obs)

	ctx, span := obs.Start(context.Background(), xmetrics.SpanOptions{
		Component: "test",
		Operation: "op",
	})
	require.NotNil(t, ctx)
	require.NotNil(t, span)
	span.End(xmetrics.Result{})
}

func TestNewOTelObserver_WithTracerProvider(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	obs, err := xmetrics.NewOTelObserver(
		xmetrics.WithTracerProvider(tp),
	)
	require.NoError(t, err)
	require.NotNil(t, obs)

	ctx, span := obs.Start(context.Background(), xmetrics.SpanOptions{
		Component: "test",
		Operation: "op",
	})
	require.NotNil(t, ctx)
	require.NotNil(t, span)
	span.End(xmetrics.Result{})
}

func TestNewOTelObserver_WithMeterProvider(t *testing.T) {
	mp := sdkmetric.NewMeterProvider()
	defer func() { _ = mp.Shutdown(context.Background()) }()

	obs, err := xmetrics.NewOTelObserver(
		xmetrics.WithMeterProvider(mp),
	)
	require.NoError(t, err)
	require.NotNil(t, obs)

	ctx, span := obs.Start(context.Background(), xmetrics.SpanOptions{
		Component: "test",
		Operation: "op",
	})
	require.NotNil(t, ctx)
	require.NotNil(t, span)
	span.End(xmetrics.Result{})
}

func TestNewOTelObserver_WithAllOptions(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	mp := sdkmetric.NewMeterProvider()
	defer func() { _ = mp.Shutdown(context.Background()) }()

	obs, err := xmetrics.NewOTelObserver(
		xmetrics.WithInstrumentationName("full-test"),
		xmetrics.WithTracerProvider(tp),
		xmetrics.WithMeterProvider(mp),
	)
	require.NoError(t, err)
	require.NotNil(t, obs)
}

func TestNewOTelObserver_AllKinds(t *testing.T) {
	obs, err := xmetrics.NewOTelObserver()
	require.NoError(t, err)

	kinds := []xmetrics.Kind{
		xmetrics.KindInternal,
		xmetrics.KindServer,
		xmetrics.KindClient,
		xmetrics.KindProducer,
		xmetrics.KindConsumer,
	}

	for _, kind := range kinds {
		t.Run("Kind_"+kind.String(), func(t *testing.T) {
			ctx, span := obs.Start(context.Background(), xmetrics.SpanOptions{
				Component: "test",
				Operation: "op",
				Kind:      kind,
			})
			assert.NotNil(t, ctx)
			assert.NotNil(t, span)
			span.End(xmetrics.Result{})
		})
	}
}

func TestNewOTelObserver_WithAttrsAndResult(t *testing.T) {
	obs, err := xmetrics.NewOTelObserver()
	require.NoError(t, err)

	ctx, span := obs.Start(context.Background(), xmetrics.SpanOptions{
		Component: "test",
		Operation: "attrs-test",
		Attrs: []xmetrics.Attr{
			xmetrics.String("service", "test-service"),
			xmetrics.Int("port", 8080),
		},
	})
	require.NotNil(t, ctx)
	require.NotNil(t, span)

	// End with result attrs
	span.End(xmetrics.Result{
		Status: xmetrics.StatusOK,
		Attrs: []xmetrics.Attr{
			xmetrics.Duration("elapsed", 100*time.Millisecond),
			xmetrics.Bool("cached", true),
		},
	})
}

func TestNewOTelObserver_WithError(t *testing.T) {
	obs, err := xmetrics.NewOTelObserver()
	require.NoError(t, err)

	_, span := obs.Start(context.Background(), xmetrics.SpanOptions{
		Component: "test",
		Operation: "error-test",
	})

	span.End(xmetrics.Result{
		Status: xmetrics.StatusError,
		Err:    errors.New("test error"),
	})
}

// ============================================================================
// 属性辅助函数测试
// ============================================================================

func TestString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"normal", "key", "value"},
		{"empty_key", "", "value"},
		{"empty_value", "key", ""},
		{"both_empty", "", ""},
		{"unicode", "键", "值"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := xmetrics.String(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			assert.Equal(t, tt.value, attr.Value)
		})
	}
}
