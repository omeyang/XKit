package storageopt

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHealthContext_PositiveTimeout(t *testing.T) {
	ctx := context.Background()
	hctx, cancel := HealthContext(ctx, 5*time.Second)
	defer cancel()

	deadline, ok := hctx.Deadline()
	assert.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(5*time.Second), deadline, 100*time.Millisecond)
}

func TestHealthContext_ZeroTimeout(t *testing.T) {
	ctx := context.Background()
	hctx, cancel := HealthContext(ctx, 0)
	defer cancel()

	// 无超时，应返回原始 context
	_, ok := hctx.Deadline()
	assert.False(t, ok)
	assert.Equal(t, ctx, hctx)
}

func TestHealthContext_NilContext(t *testing.T) {
	// nil ctx 应归一化为 context.Background()，不 panic
	hctx, cancel := HealthContext(nil, 5*time.Second) //nolint:staticcheck // 测试 nil ctx 归一化
	defer cancel()

	deadline, ok := hctx.Deadline()
	assert.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(5*time.Second), deadline, 100*time.Millisecond)
}

func TestHealthContext_NilContext_ZeroTimeout(t *testing.T) {
	// nil ctx + zero timeout 应返回 Background()，不 panic
	hctx, cancel := HealthContext(nil, 0) //nolint:staticcheck // 测试 nil ctx 归一化
	defer cancel()

	assert.NotNil(t, hctx)
}

func TestHealthContext_NegativeTimeout(t *testing.T) {
	ctx := context.Background()
	hctx, cancel := HealthContext(ctx, -1*time.Second)
	defer cancel()

	// 负超时，应返回原始 context
	_, ok := hctx.Deadline()
	assert.False(t, ok)
	assert.Equal(t, ctx, hctx)
}
