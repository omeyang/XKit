package xhealth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkCheck_NoChecks(b *testing.B) {
	h := &Health{
		opts:   defaultOptions(),
		stopCh: make(chan struct{}),
	}
	for i := range h.statuses {
		h.statuses[i] = StatusUp
	}
	ctx := context.Background()

	b.ResetTimer()
	for range b.N {
		h.check(ctx, endpointReadiness)
	}
}

func BenchmarkCheck_SingleSync(b *testing.B) {
	h, err := New(WithCacheTTL(0))
	require.NoError(b, err)
	require.NoError(b, h.AddReadinessCheck("noop", CheckConfig{
		Check: func(_ context.Context) error { return nil },
	}))
	ctx := context.Background()

	b.ResetTimer()
	for range b.N {
		h.check(ctx, endpointReadiness)
	}
}

func BenchmarkCheck_MultipleSync(b *testing.B) {
	h, err := New(WithCacheTTL(0))
	require.NoError(b, err)
	for _, name := range []string{"db", "redis", "kafka"} {
		require.NoError(b, h.AddReadinessCheck(name, CheckConfig{
			Check: func(_ context.Context) error { return nil },
		}))
	}
	ctx := context.Background()

	b.ResetTimer()
	for range b.N {
		h.check(ctx, endpointReadiness)
	}
}

func BenchmarkCheck_WithCache(b *testing.B) {
	h, err := New(WithCacheTTL(defaultCacheTTL))
	require.NoError(b, err)
	require.NoError(b, h.AddReadinessCheck("cached", CheckConfig{
		Check: func(_ context.Context) error { return nil },
	}))
	ctx := context.Background()

	// 预热缓存
	h.check(ctx, endpointReadiness)

	b.ResetTimer()
	for range b.N {
		h.check(ctx, endpointReadiness)
	}
}

func BenchmarkStatusCode(b *testing.B) {
	for range b.N {
		statusCode(StatusUp)
		statusCode(StatusDegraded)
		statusCode(StatusDown)
	}
}
