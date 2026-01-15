package mqcore

import (
	"context"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// MergeTraceContext Tests
// =============================================================================

func TestMergeTraceContext_NilBase(t *testing.T) {
	extracted := context.Background()
	result := MergeTraceContext(nil, extracted)
	assert.Equal(t, extracted, result)
}

func TestMergeTraceContext_NilExtracted(t *testing.T) {
	base := context.Background()
	result := MergeTraceContext(base, nil)
	assert.Equal(t, base, result)
}

func TestMergeTraceContext_BothNil(t *testing.T) {
	result := MergeTraceContext(nil, nil)
	assert.Nil(t, result)
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

	result := MergeTraceContext(base, extracted)

	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", xctx.TraceID(result))
	assert.Equal(t, "b7ad6b7169203331", xctx.SpanID(result))
	assert.Equal(t, "req-12345", xctx.RequestID(result))
}

func TestMergeTraceContext_EmptyExtracted(t *testing.T) {
	base, _ := xctx.WithTraceID(context.Background(), "original-trace-id")
	extracted := context.Background()

	result := MergeTraceContext(base, extracted)

	// base 的值应保留
	assert.Equal(t, "original-trace-id", xctx.TraceID(result))
}
