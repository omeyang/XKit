package mqcore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoopTracer_Inject(t *testing.T) {
	tracer := NoopTracer{}
	headers := make(map[string]string)

	tracer.Inject(context.Background(), headers)

	assert.Empty(t, headers)
}

func TestNoopTracer_Extract(t *testing.T) {
	tracer := NoopTracer{}
	headers := map[string]string{
		"traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
	}

	ctx := tracer.Extract(headers)

	assert.NotNil(t, ctx)
	// NoopTracer 返回 Background Context，不包含任何追踪信息
}

func TestNoopTracer_ImplementsTracer(t *testing.T) {
	var tracer Tracer = NoopTracer{}
	assert.NotNil(t, tracer)
}

func TestNoopTracer_Inject_NilHeaders(t *testing.T) {
	tracer := NoopTracer{}

	// 不应 panic
	assert.NotPanics(t, func() {
		tracer.Inject(context.Background(), nil)
	})
}

func TestNoopTracer_Extract_NilHeaders(t *testing.T) {
	tracer := NoopTracer{}

	ctx := tracer.Extract(nil)

	assert.NotNil(t, ctx)
	assert.Equal(t, context.Background(), ctx)
}

func TestNoopTracer_Extract_EmptyHeaders(t *testing.T) {
	tracer := NoopTracer{}

	ctx := tracer.Extract(map[string]string{})

	assert.NotNil(t, ctx)
	assert.Equal(t, context.Background(), ctx)
}

func TestNoopTracer_Inject_WithContext(t *testing.T) {
	tracer := NoopTracer{}
	headers := make(map[string]string)

	// 使用带有值的 context
	ctx := context.WithValue(context.Background(), "key", "value")

	// Inject 不应修改 headers
	tracer.Inject(ctx, headers)

	assert.Empty(t, headers)
}
