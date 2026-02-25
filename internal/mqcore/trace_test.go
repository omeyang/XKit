package mqcore

import (
	"context"
	"errors"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"go.opentelemetry.io/otel/trace"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// MergeTraceContext Tests
// =============================================================================

func TestMergeTraceContext_NilBase(t *testing.T) {
	var nilCtx context.Context
	extracted := context.Background()
	result := MergeTraceContext(nilCtx, extracted)
	assert.Equal(t, extracted, result)
}

func TestMergeTraceContext_NilExtracted(t *testing.T) {
	base := context.Background()
	result := MergeTraceContext(base, nil)
	assert.Equal(t, base, result)
}

func TestMergeTraceContext_BothNil(t *testing.T) {
	var nilCtx context.Context
	result := MergeTraceContext(nilCtx, nilCtx)
	assert.NotNil(t, result, "both nil should return context.Background(), not nil")
}

func TestMergeTraceContext_WithTraceID(t *testing.T) {
	base := context.Background()
	extracted, err := xctx.WithTraceID(context.Background(), "0af7651916cd43dd8448eb211c80319c")
	require.NoError(t, err)

	result := MergeTraceContext(base, extracted)

	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", xctx.TraceID(result))
}

func TestMergeTraceContext_WithSpanID(t *testing.T) {
	base := context.Background()
	extracted, err := xctx.WithSpanID(context.Background(), "b7ad6b7169203331")
	require.NoError(t, err)

	result := MergeTraceContext(base, extracted)

	assert.Equal(t, "b7ad6b7169203331", xctx.SpanID(result))
}

func TestMergeTraceContext_WithRequestID(t *testing.T) {
	base := context.Background()
	extracted, err := xctx.WithRequestID(context.Background(), "req-12345")
	require.NoError(t, err)

	result := MergeTraceContext(base, extracted)

	assert.Equal(t, "req-12345", xctx.RequestID(result))
}

func TestMergeTraceContext_WithTraceFlags(t *testing.T) {
	base := context.Background()
	extracted, err := xctx.WithTraceFlags(context.Background(), "01")
	require.NoError(t, err)

	result := MergeTraceContext(base, extracted)

	assert.Equal(t, "01", xctx.TraceFlags(result))
}

func TestMergeTraceContext_WithAllFields(t *testing.T) {
	base := context.Background()
	extracted := context.Background()
	var err error
	extracted, err = xctx.WithTraceID(extracted, "0af7651916cd43dd8448eb211c80319c")
	require.NoError(t, err)
	extracted, err = xctx.WithSpanID(extracted, "b7ad6b7169203331")
	require.NoError(t, err)
	extracted, err = xctx.WithRequestID(extracted, "req-12345")
	require.NoError(t, err)
	extracted, err = xctx.WithTraceFlags(extracted, "01")
	require.NoError(t, err)

	result := MergeTraceContext(base, extracted)

	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", xctx.TraceID(result))
	assert.Equal(t, "b7ad6b7169203331", xctx.SpanID(result))
	assert.Equal(t, "req-12345", xctx.RequestID(result))
	assert.Equal(t, "01", xctx.TraceFlags(result))
}

func TestMergeTraceField_SetterError(t *testing.T) {
	// 测试 setter 返回错误时的优雅降级行为
	base := context.Background()
	failingSetter := func(_ context.Context, _ string) (context.Context, error) {
		return nil, errors.New("setter failed")
	}

	result := mergeTraceField(base, "some-value", failingSetter)

	// setter 失败时应保留原 context
	assert.Equal(t, base, result)
}

func TestMergeTraceContext_EmptyExtracted(t *testing.T) {
	base, _ := xctx.WithTraceID(context.Background(), "original-trace-id")
	extracted := context.Background()

	result := MergeTraceContext(base, extracted)

	// base 的值应保留
	assert.Equal(t, "original-trace-id", xctx.TraceID(result))
}

// =============================================================================
// OTel SpanContext Preservation Tests
// =============================================================================

func TestMergeTraceContext_PreservesOTelSpanContext(t *testing.T) {
	// 模拟 OTelTracer.Extract 返回的 context：包含有效 OTel SpanContext + xctx 字段
	traceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	spanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	extracted := trace.ContextWithSpanContext(context.Background(), sc)
	extracted, _ = xctx.WithTraceID(extracted, traceID.String())
	extracted, _ = xctx.WithSpanID(extracted, spanID.String())

	// base 是消费者的 context（如带有 deadline 的 ctx）
	base := context.Background()

	result := MergeTraceContext(base, extracted)

	// 验证 xctx 字段已合并
	assert.Equal(t, traceID.String(), xctx.TraceID(result))
	assert.Equal(t, spanID.String(), xctx.SpanID(result))

	// 验证 OTel SpanContext 已保留（这是 FG-M1 修复的核心断言）
	resultSC := trace.SpanContextFromContext(result)
	assert.True(t, resultSC.IsValid(), "OTel SpanContext should be preserved after merge")
	assert.Equal(t, traceID, resultSC.TraceID())
	assert.Equal(t, spanID, resultSC.SpanID())
	assert.True(t, resultSC.IsSampled())
}

func TestMergeTraceContext_NoOTelSpanContext(t *testing.T) {
	// extracted 没有 OTel SpanContext（仅有 xctx 字段）
	extracted, _ := xctx.WithTraceID(context.Background(), "0af7651916cd43dd8448eb211c80319c")
	base := context.Background()

	result := MergeTraceContext(base, extracted)

	// xctx 字段应合并
	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", xctx.TraceID(result))

	// 没有 OTel SpanContext，不应注入无效的
	resultSC := trace.SpanContextFromContext(result)
	assert.False(t, resultSC.IsValid())
}
