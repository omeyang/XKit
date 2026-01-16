package xlimit

import (
	"github.com/prometheus/client_golang/prometheus"
)

// 默认的 Prometheus 指标
var (
	// RateLimitRequestsTotal 限流请求总数
	RateLimitRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xlimit",
			Name:      "requests_total",
			Help:      "Total number of rate limit requests",
		},
		[]string{"tenant", "rule", "result"},
	)

	// RateLimitRemainingGauge 剩余配额
	RateLimitRemainingGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "xlimit",
			Name:      "remaining",
			Help:      "Remaining rate limit quota",
		},
		[]string{"tenant", "rule"},
	)

	// RateLimitLatencyHistogram 限流检查延迟
	RateLimitLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "xlimit",
			Name:      "check_duration_seconds",
			Help:      "Rate limit check duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"tenant", "rule"},
	)
)

// MetricsCollector 指标收集器
type MetricsCollector struct {
	requestsTotal    *prometheus.CounterVec
	remainingGauge   *prometheus.GaugeVec
	latencyHistogram *prometheus.HistogramVec
	registered       bool
}

// DefaultMetricsCollector 创建使用默认指标的收集器
func DefaultMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		requestsTotal:    RateLimitRequestsTotal,
		remainingGauge:   RateLimitRemainingGauge,
		latencyHistogram: RateLimitLatencyHistogram,
	}
}

// NewMetricsCollector 创建自定义指标收集器
func NewMetricsCollector(namespace string) *MetricsCollector {
	return &MetricsCollector{
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rate_limit_requests_total",
				Help:      "Total number of rate limit requests",
			},
			[]string{"tenant", "rule", "result"},
		),
		remainingGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rate_limit_remaining",
				Help:      "Remaining rate limit quota",
			},
			[]string{"tenant", "rule"},
		),
		latencyHistogram: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "rate_limit_check_duration_seconds",
				Help:      "Rate limit check duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"tenant", "rule"},
		),
	}
}

// Register 注册指标到 Prometheus
func (m *MetricsCollector) Register(registerer prometheus.Registerer) error {
	if m.registered {
		return nil
	}

	if err := registerer.Register(m.requestsTotal); err != nil {
		// 忽略已注册错误
		if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
			return err
		}
	}

	if err := registerer.Register(m.remainingGauge); err != nil {
		if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
			return err
		}
	}

	if err := registerer.Register(m.latencyHistogram); err != nil {
		if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
			return err
		}
	}

	m.registered = true
	return nil
}

// RecordAllow 记录允许的请求
func (m *MetricsCollector) RecordAllow(key Key, result *Result) {
	labels := prometheus.Labels{
		"tenant": key.Tenant,
		"rule":   result.Rule,
		"result": "allowed",
	}
	m.requestsTotal.With(labels).Inc()
	m.remainingGauge.With(prometheus.Labels{
		"tenant": key.Tenant,
		"rule":   result.Rule,
	}).Set(float64(result.Remaining))
}

// RecordDeny 记录拒绝的请求
func (m *MetricsCollector) RecordDeny(key Key, result *Result) {
	labels := prometheus.Labels{
		"tenant": key.Tenant,
		"rule":   result.Rule,
		"result": "denied",
	}
	m.requestsTotal.With(labels).Inc()
	m.remainingGauge.With(prometheus.Labels{
		"tenant": key.Tenant,
		"rule":   result.Rule,
	}).Set(float64(result.Remaining))
}

// RecordLatency 记录检查延迟
func (m *MetricsCollector) RecordLatency(key Key, rule string, durationSeconds float64) {
	m.latencyHistogram.With(prometheus.Labels{
		"tenant": key.Tenant,
		"rule":   rule,
	}).Observe(durationSeconds)
}

// OnAllowCallback 返回用于 WithOnAllow 的回调函数
func (m *MetricsCollector) OnAllowCallback() func(Key, *Result) {
	return m.RecordAllow
}

// OnDenyCallback 返回用于 WithOnDeny 的回调函数
func (m *MetricsCollector) OnDenyCallback() func(Key, *Result) {
	return m.RecordDeny
}

// WithPrometheusMetrics 创建配置 Prometheus 指标收集的 Option
// 使用默认的 Prometheus 注册器
func WithPrometheusMetrics() Option {
	collector := DefaultMetricsCollector()
	// 尝试注册到默认注册器
	// 注册失败通常是指标名称冲突，此时使用未注册的 collector 仍可工作
	// （指标数据不会被暴露，但不影响限流功能）
	tryRegisterCollector(collector, prometheus.DefaultRegisterer)

	return func(opts *options) {
		// 组合现有回调
		existingOnAllow := opts.onAllow
		existingOnDeny := opts.onDeny

		opts.onAllow = func(key Key, result *Result) {
			collector.RecordAllow(key, result)
			if existingOnAllow != nil {
				existingOnAllow(key, result)
			}
		}

		opts.onDeny = func(key Key, result *Result) {
			collector.RecordDeny(key, result)
			if existingOnDeny != nil {
				existingOnDeny(key, result)
			}
		}
	}
}

// WithMetricsCollector 使用指定的指标收集器
func WithMetricsCollector(collector *MetricsCollector) Option {
	return func(opts *options) {
		existingOnAllow := opts.onAllow
		existingOnDeny := opts.onDeny

		opts.onAllow = func(key Key, result *Result) {
			collector.RecordAllow(key, result)
			if existingOnAllow != nil {
				existingOnAllow(key, result)
			}
		}

		opts.onDeny = func(key Key, result *Result) {
			collector.RecordDeny(key, result)
			if existingOnDeny != nil {
				existingOnDeny(key, result)
			}
		}
	}
}

// tryRegisterCollector 尝试注册指标收集器。
// 注册失败时不影响程序运行，限流功能仍可正常工作。
func tryRegisterCollector(collector *MetricsCollector, registerer prometheus.Registerer) {
	if err := collector.Register(registerer); err != nil {
		// 注册失败通常是指标名称冲突
		// 此时 collector 仍可正常记录指标，只是数据不会被 Prometheus 采集
		return
	}
}
