package xmetrics_test

import (
	"context"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value bool
	}{
		{"true", "enabled", true},
		{"false", "disabled", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := xmetrics.Bool(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			assert.Equal(t, tt.value, attr.Value)
		})
	}
}

func TestInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value int
	}{
		{"positive", "count", 42},
		{"negative", "offset", -100},
		{"zero", "index", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := xmetrics.Int(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			assert.Equal(t, tt.value, attr.Value)
		})
	}
}

func TestInt64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value int64
	}{
		{"positive", "timestamp", 1704067200000},
		{"negative", "offset", -9223372036854775808},
		{"zero", "index", 0},
		{"max", "max", 9223372036854775807},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := xmetrics.Int64(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			assert.Equal(t, tt.value, attr.Value)
		})
	}
}

func TestUint64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value uint64
	}{
		{"positive", "bytes", 1024},
		{"zero", "count", 0},
		{"max", "max", 18446744073709551615},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := xmetrics.Uint64(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			assert.Equal(t, tt.value, attr.Value)
		})
	}
}

func TestFloat32(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value float32
	}{
		{"positive", "ratio", 3.14},
		{"negative", "offset", -2.5},
		{"zero", "rate", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := xmetrics.Float32(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			assert.Equal(t, tt.value, attr.Value)
		})
	}
}

func TestFloat64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value float64
	}{
		{"positive", "ratio", 3.14159},
		{"negative", "offset", -2.71828},
		{"zero", "rate", 0.0},
		{"fraction", "half", 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := xmetrics.Float64(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			assert.Equal(t, tt.value, attr.Value)
		})
	}
}

func TestDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value time.Duration
	}{
		{"millisecond", "latency_ms", time.Millisecond},
		{"second", "timeout", time.Second},
		{"minute", "interval", time.Minute},
		{"hour", "ttl", time.Hour},
		{"zero", "zero", 0},
		{"negative", "negative", -time.Second},
		{"composite", "composite", 2*time.Hour + 30*time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := xmetrics.Duration(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			assert.Equal(t, tt.value, attr.Value)
		})
	}
}

func TestAny(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value any
	}{
		{"string", "str", "hello"},
		{"int", "num", 42},
		{"float", "ratio", 3.14},
		{"bool", "flag", true},
		{"nil", "empty", nil},
		{"slice", "list", []int{1, 2, 3}},
		{"map", "dict", map[string]int{"a": 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := xmetrics.Any(tt.key, tt.value)
			assert.Equal(t, tt.key, attr.Key)
			assert.Equal(t, tt.value, attr.Value)
		})
	}

	// 单独测试 func 类型（不能用 Equal 比较）
	t.Run("func", func(t *testing.T) {
		fn := func() {}
		attr := xmetrics.Any("fn", fn)
		assert.Equal(t, "fn", attr.Key)
		assert.NotNil(t, attr.Value)
	})
}

// ============================================================================
// 属性组合使用测试
// ============================================================================

func TestAttrsInSlice(t *testing.T) {
	t.Parallel()

	attrs := []xmetrics.Attr{
		xmetrics.String("component", "xobs"),
		xmetrics.Int("version", 1),
		xmetrics.Bool("enabled", true),
		xmetrics.Float64("ratio", 0.95),
		xmetrics.Duration("timeout", 5*time.Second),
		xmetrics.Any("extra", map[string]string{"env": "test"}),
	}

	assert.Len(t, attrs, 6)
	assert.Equal(t, "component", attrs[0].Key)
	assert.Equal(t, "xobs", attrs[0].Value)
}

func TestAttrsInSpanOptions(t *testing.T) {
	t.Parallel()

	opts := xmetrics.SpanOptions{
		Component: "test",
		Operation: "attrs-test",
		Kind:      xmetrics.KindInternal,
		Attrs: []xmetrics.Attr{
			xmetrics.String("service", "my-service"),
			xmetrics.Int("port", 8080),
			xmetrics.Bool("tls", true),
		},
	}

	assert.Len(t, opts.Attrs, 3)
	assert.Equal(t, "service", opts.Attrs[0].Key)
	assert.Equal(t, "my-service", opts.Attrs[0].Value)
}

func TestAttrsInResult(t *testing.T) {
	t.Parallel()

	result := xmetrics.Result{
		Status: xmetrics.StatusOK,
		Attrs: []xmetrics.Attr{
			xmetrics.Int64("bytes_written", 1024),
			xmetrics.Duration("elapsed", 100*time.Millisecond),
			xmetrics.String("cache_status", "hit"),
		},
	}

	assert.Len(t, result.Attrs, 3)
	assert.Equal(t, "bytes_written", result.Attrs[0].Key)
	assert.Equal(t, int64(1024), result.Attrs[0].Value)
}

// ============================================================================
// 接口实现验证
// ============================================================================

func TestNoopObserver_ImplementsObserver(t *testing.T) {
	t.Parallel()

	var _ xmetrics.Observer = xmetrics.NoopObserver{}
	var _ xmetrics.Observer = &xmetrics.NoopObserver{}
}

func TestNoopSpan_ImplementsSpan(t *testing.T) {
	t.Parallel()

	var _ xmetrics.Span = xmetrics.NoopSpan{}
	var _ xmetrics.Span = &xmetrics.NoopSpan{}
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestNoopObserver_ConcurrentStart(t *testing.T) {
	t.Parallel()

	observer := xmetrics.NoopObserver{}
	ctx := context.Background()

	const goroutines = 100
	done := make(chan struct{}, goroutines)

	for range goroutines {
		go func() {
			defer func() { done <- struct{}{} }()

			_, span := observer.Start(ctx, xmetrics.SpanOptions{
				Component: "concurrent",
				Operation: "test",
			})
			span.End(xmetrics.Result{})
		}()
	}

	// 等待所有 goroutine 完成
	for range goroutines {
		<-done
	}
}

func TestOTelObserver_ConcurrentStart(t *testing.T) {
	t.Parallel()

	obs, err := xmetrics.NewOTelObserver()
	require.NoError(t, err)

	const goroutines = 100
	done := make(chan struct{}, goroutines)

	for i := range goroutines {
		go func(id int) {
			defer func() { done <- struct{}{} }()

			_, span := obs.Start(context.Background(), xmetrics.SpanOptions{
				Component: "concurrent",
				Operation: "test",
				Attrs:     []xmetrics.Attr{xmetrics.Int("id", id)},
			})
			span.End(xmetrics.Result{Status: xmetrics.StatusOK})
		}(i)
	}

	for range goroutines {
		<-done
	}
}

// ============================================================================
// 边界条件测试
// ============================================================================

func TestSpanOptions_LargeAttrs(t *testing.T) {
	t.Parallel()

	// 创建大量属性
	attrs := make([]xmetrics.Attr, 100)
	for i := range 100 {
		attrs[i] = xmetrics.Int("key", i)
	}

	opts := xmetrics.SpanOptions{
		Component: "test",
		Operation: "large-attrs",
		Attrs:     attrs,
	}

	obs, err := xmetrics.NewOTelObserver()
	require.NoError(t, err)

	_, span := obs.Start(context.Background(), opts)
	require.NotNil(t, span)
	span.End(xmetrics.Result{Attrs: attrs})
}

func TestResult_LargeAttrs(t *testing.T) {
	t.Parallel()

	attrs := make([]xmetrics.Attr, 100)
	for i := range 100 {
		attrs[i] = xmetrics.String("result-key", "value")
	}

	result := xmetrics.Result{
		Status: xmetrics.StatusOK,
		Attrs:  attrs,
	}

	assert.Len(t, result.Attrs, 100)
}

func TestAttrEdgeCases(t *testing.T) {
	t.Parallel()

	// 空字符串
	attr := xmetrics.String("", "")
	assert.Empty(t, attr.Key)
	assert.Empty(t, attr.Value)

	// 零值 Duration
	attr = xmetrics.Duration("zero", 0)
	assert.Equal(t, time.Duration(0), attr.Value)

	// 最大/最小整数
	attr = xmetrics.Int64("max", 9223372036854775807)
	assert.Equal(t, int64(9223372036854775807), attr.Value)

	attr = xmetrics.Int64("min", -9223372036854775808)
	assert.Equal(t, int64(-9223372036854775808), attr.Value)

	// 最大 uint64
	attr = xmetrics.Uint64("max", 18446744073709551615)
	assert.Equal(t, uint64(18446744073709551615), attr.Value)
}
