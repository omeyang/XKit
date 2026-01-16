//nolint:errcheck // 测试代码中 defer 调用忽略 Shutdown 错误
package xlimit

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestNewMetrics(t *testing.T) {
	t.Run("nil meter provider returns nil", func(t *testing.T) {
		m, err := NewMetrics(nil)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if m != nil {
			t.Error("expected nil metrics")
		}
	})

	t.Run("valid meter provider creates metrics", func(t *testing.T) {
		reader := metric.NewManualReader()
		provider := metric.NewMeterProvider(metric.WithReader(reader))
		defer func() { _ = provider.Shutdown(context.Background()) }()

		m, err := NewMetrics(provider)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if m == nil {
			t.Error("expected metrics to be created")
		}
	})
}

func TestMetrics_RecordAllow(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	m, err := NewMetrics(provider)
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	ctx := context.Background()

	t.Run("record allowed request", func(t *testing.T) {
		m.RecordAllow(ctx, "distributed", "test-rule", true, 100*time.Microsecond)

		var rm metricdata.ResourceMetrics
		if err := reader.Collect(ctx, &rm); err != nil {
			t.Fatalf("failed to collect metrics: %v", err)
		}

		// 验证指标已记录
		found := false
		for _, sm := range rm.ScopeMetrics {
			for _, metric := range sm.Metrics {
				if metric.Name == metricNameRequestsTotal {
					found = true
				}
			}
		}
		if !found {
			t.Error("expected requests_total metric to be recorded")
		}
	})

	t.Run("record denied request", func(t *testing.T) {
		m.RecordAllow(ctx, "local", "test-rule", false, 50*time.Microsecond)

		var rm metricdata.ResourceMetrics
		if err := reader.Collect(ctx, &rm); err != nil {
			t.Fatalf("failed to collect metrics: %v", err)
		}

		// 验证 denied 指标已记录
		found := false
		for _, sm := range rm.ScopeMetrics {
			for _, metric := range sm.Metrics {
				if metric.Name == metricNameDeniedTotal {
					found = true
				}
			}
		}
		if !found {
			t.Error("expected denied_total metric to be recorded")
		}
	})
}

func TestMetrics_RecordFallback(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	m, err := NewMetrics(provider)
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	ctx := context.Background()

	m.RecordFallback(ctx, FallbackLocal, "connection refused")

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("failed to collect metrics: %v", err)
	}

	// 验证 fallback 指标已记录
	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, metric := range sm.Metrics {
			if metric.Name == metricNameFallbackTotal {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected fallback_total metric to be recorded")
	}
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
	defer func() { _ = provider.Shutdown(context.Background()) }()

	m, err := NewMetrics(provider)
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	// 创建已取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// 即使 context 已取消，指标仍应记录（使用 context.WithoutCancel）
	m.RecordAllow(ctx, "distributed", "test-rule", true, time.Millisecond)

	// 验证指标已记录
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("failed to collect metrics: %v", err)
	}

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, metric := range sm.Metrics {
			if metric.Name == metricNameRequestsTotal {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected metrics to be recorded even with canceled context")
	}
}
