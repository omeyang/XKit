package xlimit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// assertMetricExists 验证指定名称的指标已记录。
func assertMetricExists(t *testing.T, rm metricdata.ResourceMetrics, name string) {
	t.Helper()

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return
			}
		}
	}

	t.Errorf("expected metric %q to be recorded", name)
}

func TestNewMetrics(t *testing.T) {
	t.Run("nil meter provider returns nil", func(t *testing.T) {
		m, err := NewMetrics(nil)
		require.NoError(t, err)
		assert.Nil(t, m)
	})

	t.Run("valid meter provider creates metrics", func(t *testing.T) {
		reader := metric.NewManualReader()
		provider := metric.NewMeterProvider(metric.WithReader(reader))
		defer func() { _ = provider.Shutdown(context.Background()) }() //nolint:errcheck // defer cleanup

		m, err := NewMetrics(provider)
		require.NoError(t, err)
		assert.NotNil(t, m)
	})
}

func TestMetrics_RecordAllow(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }() //nolint:errcheck // defer cleanup

	m, err := NewMetrics(provider)
	require.NoError(t, err, "failed to create metrics")

	ctx := context.Background()

	t.Run("record allowed request", func(t *testing.T) {
		m.RecordAllow(ctx, "distributed", "test-rule", true, 100*time.Microsecond)

		var rm metricdata.ResourceMetrics
		require.NoError(t, reader.Collect(ctx, &rm), "failed to collect metrics")

		assertMetricExists(t, rm, metricNameRequestsTotal)
	})

	t.Run("record denied request", func(t *testing.T) {
		m.RecordAllow(ctx, "local", "test-rule", false, 50*time.Microsecond)

		var rm metricdata.ResourceMetrics
		require.NoError(t, reader.Collect(ctx, &rm), "failed to collect metrics")

		assertMetricExists(t, rm, metricNameDeniedTotal)
	})
}

func TestMetrics_RecordFallback(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }() //nolint:errcheck // defer cleanup

	m, err := NewMetrics(provider)
	require.NoError(t, err, "failed to create metrics")

	ctx := context.Background()

	m.RecordFallback(ctx, FallbackLocal, "connection refused")

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &rm), "failed to collect metrics")

	assertMetricExists(t, rm, metricNameFallbackTotal)
}

func TestMetrics_NilSafe(t *testing.T) {
	// 确保 nil metrics 不会 panic
	var m *Metrics

	ctx := context.Background()

	// 这些调用不应该 panic
	m.RecordAllow(ctx, "distributed", "test", true, time.Millisecond)
	m.RecordFallback(ctx, FallbackLocal, "test error")
}

func TestMetrics_CanceledContext(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }() //nolint:errcheck // defer cleanup

	m, err := NewMetrics(provider)
	require.NoError(t, err, "failed to create metrics")

	// 创建已取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// 即使 context 已取消，指标仍应记录（使用 context.WithoutCancel）
	m.RecordAllow(ctx, "distributed", "test-rule", true, time.Millisecond)

	// 验证指标已记录
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm), "failed to collect metrics")

	assertMetricExists(t, rm, metricNameRequestsTotal)
}
