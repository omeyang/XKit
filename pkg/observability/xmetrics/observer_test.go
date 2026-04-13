package xmetrics

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Kind 和 Status 常量测试已在外部测试文件 xmetrics_test.go 中覆盖，
// 此处不重复（常量属于公开 API，由外部测试验证更合适）。

// ============================================================================
// Kind.String() 测试
// ============================================================================

func TestKind_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind Kind
		want string
	}{
		{KindInternal, "Internal"},
		{KindServer, "Server"},
		{KindClient, "Client"},
		{KindProducer, "Producer"},
		{KindConsumer, "Consumer"},
		{Kind(99), "Kind(99)"},
		{Kind(-1), "Kind(-1)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.kind.String())
		})
	}
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

	// NoopObserver 对 nil ctx 也应该安全，返回 context.Background()
	newCtx, span := observer.Start(nilCtx, SpanOptions{})

	assert.NotNil(t, newCtx) // nil ctx 被归一化为 context.Background()
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
		t.Run(kind.String(), func(t *testing.T) {
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
		t.Run("Result_"+strconv.Itoa(i), func(t *testing.T) {
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
	// nil context + nil observer → ctx 归一化为 context.Background()
	newCtx, span := Start(nilCtx, nil, SpanOptions{})

	assert.NotNil(t, newCtx) // nil ctx 被归一化为 context.Background()
	assert.NotNil(t, span)
}

// nilCtxObserver 是返回 nil context 的测试用 Observer，用于验证 Start 的兜底逻辑。
type nilCtxObserver struct{}

func (nilCtxObserver) Start(_ context.Context, _ SpanOptions) (context.Context, Span) {
	var nilCtx context.Context // 故意返回 nil context 以测试兜底逻辑
	return nilCtx, NoopSpan{}
}

func TestStart_ObserverReturnsNilContext(t *testing.T) {
	t.Parallel()

	// 自定义 Observer 返回 nil context，Start 应该兜底为原始 ctx
	ctx := context.Background()
	newCtx, span := Start(ctx, nilCtxObserver{}, SpanOptions{
		Component: "test",
		Operation: "nil-ctx-observer",
	})

	assert.NotNil(t, newCtx) // 兜底返回传入的 ctx
	assert.NotNil(t, span)
}

// nilSpanObserver 是返回 nil span 的测试用 Observer，用于验证 Start 的兜底逻辑。
type nilSpanObserver struct{}

func (nilSpanObserver) Start(ctx context.Context, _ SpanOptions) (context.Context, Span) {
	var nilSpan Span // 故意返回 nil span 以测试兜底逻辑
	return ctx, nilSpan
}

func TestStart_ObserverReturnsNilSpan(t *testing.T) {
	t.Parallel()

	// 自定义 Observer 返回 nil span，Start 应该兜底为 NoopSpan
	ctx := context.Background()
	newCtx, span := Start(ctx, nilSpanObserver{}, SpanOptions{
		Component: "test",
		Operation: "nil-span-observer",
	})

	assert.NotNil(t, newCtx)
	require.NotNil(t, span)

	// span 应被兜底为 NoopSpan
	_, ok := span.(NoopSpan)
	assert.True(t, ok)

	// End 不应 panic
	assert.NotPanics(t, func() {
		span.End(Result{})
	})
}

// ============================================================================
// isNilInterface 测试
// ============================================================================

// customSpan 是用于测试的自定义 Span 实现。
type customSpan struct{}

func (*customSpan) End(_ Result) {}

func TestIsNilInterface(t *testing.T) {
	t.Parallel()

	t.Run("untyped_nil", func(t *testing.T) {
		assert.True(t, isNilInterface(nil))
	})

	t.Run("nil_interface", func(t *testing.T) {
		var span Span
		assert.True(t, isNilInterface(span))
	})

	t.Run("typed_nil_pointer", func(t *testing.T) {
		var s *customSpan
		var span Span = s
		assert.True(t, isNilInterface(span))
	})

	t.Run("non_nil_pointer", func(t *testing.T) {
		s := &customSpan{}
		var span Span = s
		assert.False(t, isNilInterface(span))
	})

	t.Run("struct_value", func(t *testing.T) {
		var span Span = NoopSpan{}
		assert.False(t, isNilInterface(span))
	})
}

// ============================================================================
// FG-S1: typed-nil 防御回归测试
// ============================================================================

// typedNilSpanObserver 返回 typed-nil span（接口 type 非空但 value 为 nil），
// 用于验证 Start 的 typed-nil 兜底逻辑。
type typedNilSpanObserver struct{}

func (typedNilSpanObserver) Start(ctx context.Context, _ SpanOptions) (context.Context, Span) {
	var s *customSpan // typed-nil：接口内部 type=*customSpan, value=nil
	return ctx, s
}

// typedNilObserver 是 typed-nil Observer，用于验证 Start 的 typed-nil observer 兜底逻辑。
type typedNilObserver struct{}

func (*typedNilObserver) Start(ctx context.Context, _ SpanOptions) (context.Context, Span) {
	return ctx, NoopSpan{}
}

func TestStart_TypedNilObserver(t *testing.T) {
	t.Parallel()

	// typed-nil observer：接口内部 type=*typedNilObserver, value=nil
	var obs *typedNilObserver
	var observer Observer = obs

	ctx := context.Background()
	newCtx, span := Start(ctx, observer, SpanOptions{
		Component: "test",
		Operation: "typed-nil-observer",
	})

	assert.NotNil(t, newCtx)
	require.NotNil(t, span)

	// span 应被兜底为 NoopSpan
	_, ok := span.(NoopSpan)
	assert.True(t, ok)

	// End 不应 panic
	assert.NotPanics(t, func() {
		span.End(Result{})
	})
}

func TestStart_TypedNilSpan(t *testing.T) {
	t.Parallel()

	// 自定义 Observer 返回 typed-nil span，Start 应兜底为 NoopSpan
	ctx := context.Background()
	newCtx, span := Start(ctx, typedNilSpanObserver{}, SpanOptions{
		Component: "test",
		Operation: "typed-nil-span",
	})

	assert.NotNil(t, newCtx)
	require.NotNil(t, span)

	// span 应被兜底为 NoopSpan
	_, ok := span.(NoopSpan)
	assert.True(t, ok)

	// End 不应 panic
	assert.NotPanics(t, func() {
		span.End(Result{})
	})
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
