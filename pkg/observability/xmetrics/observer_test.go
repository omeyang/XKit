package xmetrics

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Kind 和 Status 常量测试
// ============================================================================

func TestKindConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind Kind
		want int
	}{
		{"KindInternal", KindInternal, 0},
		{"KindServer", KindServer, 1},
		{"KindClient", KindClient, 2},
		{"KindProducer", KindProducer, 3},
		{"KindConsumer", KindConsumer, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, int(tt.kind))
		})
	}
}

func TestStatusConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Status("ok"), StatusOK)
	assert.Equal(t, Status("error"), StatusError)
}

// ============================================================================
// Attr 结构测试
// ============================================================================

func TestAttr_Struct(t *testing.T) {
	t.Parallel()

	attr := Attr{Key: "test-key", Value: "test-value"}
	assert.Equal(t, "test-key", attr.Key)
	assert.Equal(t, "test-value", attr.Value)
}

func TestAttr_EmptyKey(t *testing.T) {
	t.Parallel()

	attr := Attr{Key: "", Value: "value"}
	assert.Empty(t, attr.Key)
	assert.NotEmpty(t, attr.Value)
}

func TestAttr_NilValue(t *testing.T) {
	t.Parallel()

	attr := Attr{Key: "key", Value: nil}
	assert.Equal(t, "key", attr.Key)
	assert.Nil(t, attr.Value)
}

// ============================================================================
// SpanOptions 结构测试
// ============================================================================

func TestSpanOptions_Struct(t *testing.T) {
	t.Parallel()

	opts := SpanOptions{
		Component: "test-component",
		Operation: "test-operation",
		Kind:      KindServer,
		Attrs:     []Attr{{Key: "k1", Value: "v1"}},
	}

	assert.Equal(t, "test-component", opts.Component)
	assert.Equal(t, "test-operation", opts.Operation)
	assert.Equal(t, KindServer, opts.Kind)
	assert.Len(t, opts.Attrs, 1)
}

func TestSpanOptions_Empty(t *testing.T) {
	t.Parallel()

	opts := SpanOptions{}
	assert.Empty(t, opts.Component)
	assert.Empty(t, opts.Operation)
	assert.Equal(t, KindInternal, opts.Kind)
	assert.Nil(t, opts.Attrs)
}

// ============================================================================
// Result 结构测试
// ============================================================================

func TestResult_WithStatus(t *testing.T) {
	t.Parallel()

	result := Result{Status: StatusOK}
	assert.Equal(t, StatusOK, result.Status)
	assert.Nil(t, result.Err)
}

func TestResult_WithError(t *testing.T) {
	t.Parallel()

	testErr := errors.New("test error")
	result := Result{Err: testErr}

	assert.Empty(t, result.Status)
	assert.Equal(t, testErr, result.Err)
}

func TestResult_WithStatusAndError(t *testing.T) {
	t.Parallel()

	testErr := errors.New("test error")
	result := Result{
		Status: StatusError,
		Err:    testErr,
		Attrs:  []Attr{{Key: "detail", Value: "extra info"}},
	}

	assert.Equal(t, StatusError, result.Status)
	assert.Equal(t, testErr, result.Err)
	assert.Len(t, result.Attrs, 1)
}

// ============================================================================
// NoopObserver 测试
// ============================================================================

func TestNoopObserver_Start(t *testing.T) {
	t.Parallel()

	observer := NoopObserver{}
	ctx := context.Background()

	newCtx, span := observer.Start(ctx, SpanOptions{
		Component: "test",
		Operation: "op",
	})

	assert.NotNil(t, newCtx)
	assert.NotNil(t, span)
	assert.Equal(t, ctx, newCtx) // NoopObserver 返回原始 ctx
}

func TestNoopObserver_Start_NilContext(t *testing.T) {
	t.Parallel()

	var nilCtx context.Context
	observer := NoopObserver{}

	// NoopObserver 对 nil ctx 也应该安全
	newCtx, span := observer.Start(nilCtx, SpanOptions{})

	assert.Nil(t, newCtx)
	assert.NotNil(t, span)
}

func TestNoopObserver_Start_EmptyOptions(t *testing.T) {
	t.Parallel()

	observer := NoopObserver{}
	ctx := context.Background()

	newCtx, span := observer.Start(ctx, SpanOptions{})

	assert.NotNil(t, newCtx)
	assert.NotNil(t, span)
}

func TestNoopObserver_Start_AllKinds(t *testing.T) {
	t.Parallel()

	observer := NoopObserver{}
	ctx := context.Background()

	kinds := []Kind{KindInternal, KindServer, KindClient, KindProducer, KindConsumer}

	for _, kind := range kinds {
		t.Run("Kind_"+string(rune('0'+kind)), func(t *testing.T) {
			_, span := observer.Start(ctx, SpanOptions{Kind: kind})
			assert.NotNil(t, span)
		})
	}
}

// ============================================================================
// NoopSpan 测试
// ============================================================================

func TestNoopSpan_End(t *testing.T) {
	t.Parallel()

	span := NoopSpan{}

	// End 应该不 panic
	assert.NotPanics(t, func() {
		span.End(Result{})
	})
}

func TestNoopSpan_End_WithResult(t *testing.T) {
	t.Parallel()

	span := NoopSpan{}

	// 带各种结果调用 End 都不应该 panic
	results := []Result{
		{},
		{Status: StatusOK},
		{Status: StatusError},
		{Err: errors.New("error")},
		{Status: StatusError, Err: errors.New("error")},
		{Attrs: []Attr{{Key: "k", Value: "v"}}},
	}

	for i, result := range results {
		t.Run("Result_"+string(rune('0'+i)), func(t *testing.T) {
			assert.NotPanics(t, func() {
				span.End(result)
			})
		})
	}
}

func TestNoopSpan_End_MultipleTimes(t *testing.T) {
	t.Parallel()

	span := NoopSpan{}

	// 多次调用 End 不应该 panic
	assert.NotPanics(t, func() {
		span.End(Result{})
		span.End(Result{})
		span.End(Result{})
	})
}

// ============================================================================
// Start 辅助函数测试
// ============================================================================

func TestStart_NilObserver(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	newCtx, span := Start(ctx, nil, SpanOptions{
		Component: "test",
		Operation: "op",
	})

	assert.NotNil(t, newCtx)
	assert.NotNil(t, span)
	assert.Equal(t, ctx, newCtx) // nil observer 返回原始 ctx

	// span 是 NoopSpan
	_, ok := span.(NoopSpan)
	assert.True(t, ok)
}

func TestStart_WithNoopObserver(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	observer := NoopObserver{}

	newCtx, span := Start(ctx, observer, SpanOptions{
		Component: "test",
		Operation: "op",
	})

	assert.NotNil(t, newCtx)
	assert.NotNil(t, span)
}

func TestStart_NilContext(t *testing.T) {
	t.Parallel()

	var nilCtx context.Context
	// nil context + nil observer
	newCtx, span := Start(nilCtx, nil, SpanOptions{})

	assert.Nil(t, newCtx)
	assert.NotNil(t, span)
}

// ============================================================================
// 接口实现验证
// ============================================================================

func TestNoopObserver_ImplementsObserver(t *testing.T) {
	t.Parallel()

	var _ Observer = NoopObserver{}
	var _ Observer = &NoopObserver{}
}

func TestNoopSpan_ImplementsSpan(t *testing.T) {
	t.Parallel()

	var _ Span = NoopSpan{}
	var _ Span = &NoopSpan{}
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestNoopObserver_ConcurrentStart(t *testing.T) {
	t.Parallel()

	observer := NoopObserver{}
	ctx := context.Background()

	const goroutines = 100
	done := make(chan struct{}, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer func() { done <- struct{}{} }()

			_, span := observer.Start(ctx, SpanOptions{
				Component: "concurrent",
				Operation: "test",
			})
			span.End(Result{})
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < goroutines; i++ {
		<-done
	}
}

func TestNoopSpan_ConcurrentEnd(t *testing.T) {
	t.Parallel()

	span := NoopSpan{}

	const goroutines = 100
	done := make(chan struct{}, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			span.End(Result{})
		}()
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}
}

// ============================================================================
// 边界条件测试
// ============================================================================

func TestSpanOptions_LargeAttrs(t *testing.T) {
	t.Parallel()

	// 创建大量属性
	attrs := make([]Attr, 1000)
	for i := 0; i < 1000; i++ {
		attrs[i] = Attr{Key: "key", Value: i}
	}

	opts := SpanOptions{
		Component: "test",
		Operation: "large-attrs",
		Attrs:     attrs,
	}

	observer := NoopObserver{}
	ctx := context.Background()

	_, span := observer.Start(ctx, opts)
	require.NotNil(t, span)

	span.End(Result{Attrs: attrs})
}

func TestResult_LargeAttrs(t *testing.T) {
	t.Parallel()

	attrs := make([]Attr, 1000)
	for i := 0; i < 1000; i++ {
		attrs[i] = Attr{Key: "result-key", Value: i}
	}

	result := Result{
		Status: StatusOK,
		Attrs:  attrs,
	}

	assert.Len(t, result.Attrs, 1000)
}
