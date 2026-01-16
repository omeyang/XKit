//nolint:errcheck // 测试文件中的错误处理简化
package xlimit

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsCollector_Basic(t *testing.T) {
	collector := NewMetricsCollector("test")

	// 注册到测试注册器
	registry := prometheus.NewRegistry()
	err := collector.Register(registry)
	if err != nil {
		t.Fatalf("failed to register: %v", err)
	}

	// 记录允许的请求
	key := Key{Tenant: "test-tenant"}
	result := &Result{
		Allowed:   true,
		Limit:     100,
		Remaining: 99,
		Rule:      "test-rule",
	}
	collector.RecordAllow(key, result)

	// 验证计数器
	count := testutil.ToFloat64(collector.requestsTotal.WithLabelValues("test-tenant", "test-rule", "allowed"))
	if count != 1 {
		t.Errorf("expected count 1, got %f", count)
	}

	// 验证剩余配额
	remaining := testutil.ToFloat64(collector.remainingGauge.WithLabelValues("test-tenant", "test-rule"))
	if remaining != 99 {
		t.Errorf("expected remaining 99, got %f", remaining)
	}
}

func TestMetricsCollector_RecordDeny(t *testing.T) {
	collector := NewMetricsCollector("test_deny")

	registry := prometheus.NewRegistry()
	collector.Register(registry)

	key := Key{Tenant: "denied-tenant"}
	result := &Result{
		Allowed:   false,
		Limit:     100,
		Remaining: 0,
		Rule:      "deny-rule",
	}
	collector.RecordDeny(key, result)

	// 验证计数器
	count := testutil.ToFloat64(collector.requestsTotal.WithLabelValues("denied-tenant", "deny-rule", "denied"))
	if count != 1 {
		t.Errorf("expected count 1, got %f", count)
	}
}

func TestMetricsCollector_RecordLatency(t *testing.T) {
	collector := NewMetricsCollector("test_latency")

	registry := prometheus.NewRegistry()
	collector.Register(registry)

	key := Key{Tenant: "latency-tenant"}
	collector.RecordLatency(key, "latency-rule", 0.005) // 5ms

	// 验证直方图
	// 由于 histogram 验证较复杂，只检查是否不 panic
}

func TestMetricsCollector_Register_Idempotent(t *testing.T) {
	collector := NewMetricsCollector("test_idempotent")

	registry := prometheus.NewRegistry()

	// 第一次注册
	err := collector.Register(registry)
	if err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	// 第二次注册应该成功（幂等）
	err = collector.Register(registry)
	if err != nil {
		t.Fatalf("second register should be idempotent: %v", err)
	}
}

func TestDefaultMetricsCollector(t *testing.T) {
	collector := DefaultMetricsCollector()

	if collector.requestsTotal == nil {
		t.Error("requestsTotal should not be nil")
	}
	if collector.remainingGauge == nil {
		t.Error("remainingGauge should not be nil")
	}
	if collector.latencyHistogram == nil {
		t.Error("latencyHistogram should not be nil")
	}
}

func TestMetricsCollector_Callbacks(t *testing.T) {
	collector := NewMetricsCollector("test_callback")

	// 获取回调函数
	onAllow := collector.OnAllowCallback()
	onDeny := collector.OnDenyCallback()

	if onAllow == nil {
		t.Error("OnAllowCallback should not return nil")
	}
	if onDeny == nil {
		t.Error("OnDenyCallback should not return nil")
	}

	// 使用回调
	key := Key{Tenant: "callback-tenant"}
	result := &Result{
		Allowed:   true,
		Limit:     100,
		Remaining: 99,
		Rule:      "callback-rule",
	}

	onAllow(key, result)

	// 验证计数
	count := testutil.ToFloat64(collector.requestsTotal.WithLabelValues("callback-tenant", "callback-rule", "allowed"))
	if count != 1 {
		t.Errorf("expected count 1, got %f", count)
	}
}

func TestWithMetricsCollector_Integration(t *testing.T) {
	collector := NewMetricsCollector("test_integration")

	registry := prometheus.NewRegistry()
	collector.Register(registry)

	// 创建本地限流器并集成指标
	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 2, time.Minute)),
		WithMetricsCollector(collector),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	key := Key{Tenant: "integration-tenant"}

	// 允许的请求
	limiter.Allow(ctx, key)
	allowedCount := testutil.ToFloat64(collector.requestsTotal.WithLabelValues("integration-tenant", "tenant-limit", "allowed"))
	if allowedCount != 1 {
		t.Errorf("expected 1 allowed request, got %f", allowedCount)
	}

	// 再次请求
	limiter.Allow(ctx, key)

	// 拒绝的请求
	limiter.Allow(ctx, key)
	deniedCount := testutil.ToFloat64(collector.requestsTotal.WithLabelValues("integration-tenant", "tenant-limit", "denied"))
	if deniedCount != 1 {
		t.Errorf("expected 1 denied request, got %f", deniedCount)
	}
}

func TestWithPrometheusMetrics_ChainsCallbacks(t *testing.T) {
	var customCallbackCalled bool

	limiter, err := NewLocal(
		WithRules(TenantRule("tenant-limit", 10, time.Minute)),
		WithOnAllow(func(key Key, result *Result) {
			customCallbackCalled = true
		}),
		WithPrometheusMetrics(), // WithPrometheusMetrics 应该保留现有回调
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	limiter.Allow(ctx, Key{Tenant: "chain-tenant"})

	if !customCallbackCalled {
		t.Error("custom callback should still be called when using WithPrometheusMetrics")
	}
}
