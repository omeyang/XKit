package xsemaphore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric/noop"
)

func TestNewMetrics(t *testing.T) {
	t.Run("with noop provider", func(t *testing.T) {
		mp := noop.NewMeterProvider()
		metrics, err := NewMetrics(mp)
		require.NoError(t, err)
		assert.NotNil(t, metrics)
	})

	t.Run("nil provider returns nil", func(t *testing.T) {
		metrics, err := NewMetrics(nil)
		assert.NoError(t, err)
		assert.Nil(t, metrics)
	})
}

func TestMetrics_RecordAcquire(t *testing.T) {
	mp := noop.NewMeterProvider()
	metrics, err := NewMetrics(mp)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("successful acquire", func(t *testing.T) {
		// 不应 panic
		metrics.RecordAcquire(ctx, SemaphoreTypeDistributed, "test-resource", true, ReasonUnknown, 100*time.Millisecond)
	})

	t.Run("failed acquire - capacity full", func(t *testing.T) {
		metrics.RecordAcquire(ctx, SemaphoreTypeDistributed, "test-resource", false, ReasonCapacityFull, 50*time.Millisecond)
	})

	t.Run("failed acquire - tenant quota", func(t *testing.T) {
		metrics.RecordAcquire(ctx, SemaphoreTypeLocal, "test-resource", false, ReasonTenantQuotaExceeded, 50*time.Millisecond)
	})
}

func TestMetrics_RecordRelease(t *testing.T) {
	mp := noop.NewMeterProvider()
	metrics, err := NewMetrics(mp)
	require.NoError(t, err)

	ctx := context.Background()

	// 不应 panic
	metrics.RecordRelease(ctx, SemaphoreTypeDistributed, "test-resource")
	metrics.RecordRelease(ctx, SemaphoreTypeLocal, "test-resource")
}

func TestMetrics_RecordExtend(t *testing.T) {
	mp := noop.NewMeterProvider()
	metrics, err := NewMetrics(mp)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("successful extend", func(t *testing.T) {
		metrics.RecordExtend(ctx, SemaphoreTypeDistributed, "test-resource", true)
	})

	t.Run("failed extend", func(t *testing.T) {
		metrics.RecordExtend(ctx, SemaphoreTypeDistributed, "test-resource", false)
	})
}

func TestMetrics_RecordFallback(t *testing.T) {
	mp := noop.NewMeterProvider()
	metrics, err := NewMetrics(mp)
	require.NoError(t, err)

	ctx := context.Background()

	metrics.RecordFallback(ctx, FallbackLocal, "test-resource", "redis_unavailable")
	metrics.RecordFallback(ctx, FallbackOpen, "test-resource", "redis_unavailable")
	metrics.RecordFallback(ctx, FallbackClose, "test-resource", "redis_unavailable")
}

func TestMetrics_RecordQuery(t *testing.T) {
	mp := noop.NewMeterProvider()
	metrics, err := NewMetrics(mp)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("successful query", func(t *testing.T) {
		metrics.RecordQuery(ctx, SemaphoreTypeDistributed, "test-resource", true, 10*time.Millisecond)
	})

	t.Run("failed query", func(t *testing.T) {
		metrics.RecordQuery(ctx, SemaphoreTypeLocal, "test-resource", false, 5*time.Millisecond)
	})
}

func TestMetrics_NilReceiver(t *testing.T) {
	// 所有 Record* 方法在 nil receiver 上都应安全（不 panic）
	var m *Metrics
	ctx := context.Background()

	m.RecordAcquire(ctx, SemaphoreTypeDistributed, "r", true, ReasonUnknown, time.Millisecond)
	m.RecordRelease(ctx, SemaphoreTypeDistributed, "r")
	m.RecordExtend(ctx, SemaphoreTypeDistributed, "r", true)
	m.RecordFallback(ctx, FallbackLocal, "r", "test")
	m.RecordQuery(ctx, SemaphoreTypeDistributed, "r", true, time.Millisecond)
}

func TestSemaphore_WithMetrics(t *testing.T) {
	mr, client := setupRedis(t)
	_ = mr

	mp := noop.NewMeterProvider()
	sem, err := New(client,
		WithMeterProvider(mp),
	)
	require.NoError(t, err)
	defer closeSemaphore(t, sem)

	ctx := context.Background()

	// 测试指标被正确记录（不应 panic）
	permit, err := sem.TryAcquire(ctx, "metrics-test",
		WithCapacity(10),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	err = permit.Extend(ctx)
	require.NoError(t, err)

	err = permit.Release(ctx)
	assert.NoError(t, err)
}

func TestLocalSemaphore_WithMetrics(t *testing.T) {
	mp := noop.NewMeterProvider()
	metrics, err := NewMetrics(mp)
	require.NoError(t, err)

	opts := defaultOptions()
	opts.metrics = metrics
	opts.podCount = 1
	sem := newLocalSemaphore(opts)
	defer closeSemaphore(t, sem)

	ctx := context.Background()

	permit, err := sem.TryAcquire(ctx, "local-metrics-test",
		WithCapacity(10),
	)
	require.NoError(t, err)
	require.NotNil(t, permit)

	err = permit.Release(ctx)
	assert.NoError(t, err)
}
