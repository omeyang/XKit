package xauth

import (
	"context"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"
)

// =============================================================================
// 指标名称常量
// =============================================================================

const (
	// MetricsComponent 组件名称。
	MetricsComponent = "xauth"

	// 操作名称
	MetricsOpGetToken         = "GetToken"
	MetricsOpVerifyToken      = "VerifyToken"
	MetricsOpRefreshToken     = "RefreshToken"
	MetricsOpGetPlatformID    = "GetPlatformID"
	MetricsOpHasParentPlatform = "HasParentPlatform"
	MetricsOpGetUnclassRegion = "GetUnclassRegionID"
	MetricsOpHTTPRequest      = "HTTPRequest"

	// 属性 Key
	MetricsAttrTenantID  = "tenant_id"
	MetricsAttrCacheHit  = "cache_hit"
	MetricsAttrTokenType = "token_type"
	MetricsAttrPath      = "path"
	MetricsAttrMethod    = "method"
	MetricsAttrStatus    = "status"
)

// =============================================================================
// 观测辅助函数
// =============================================================================

// startSpan 开始一个观测跨度。
func startSpan(ctx context.Context, observer xmetrics.Observer, operation string, attrs ...xmetrics.Attr) (context.Context, xmetrics.Span) {
	return xmetrics.Start(ctx, observer, xmetrics.SpanOptions{
		Component: MetricsComponent,
		Operation: operation,
		Kind:      xmetrics.KindClient,
		Attrs:     attrs,
	})
}

// endSpan 结束观测跨度。
func endSpan(span xmetrics.Span, err error, attrs ...xmetrics.Attr) {
	result := xmetrics.Result{
		Err:   err,
		Attrs: attrs,
	}
	if err != nil {
		result.Status = xmetrics.StatusError
	} else {
		result.Status = xmetrics.StatusOK
	}
	span.End(result)
}

// =============================================================================
// 指标记录器
// =============================================================================

// MetricsRecorder 指标记录器接口。
// 用于记录 xauth 操作的详细指标。
//
// 设计说明：
// MetricsRecorder 是一个扩展点，供用户自定义更细粒度的指标收集。
// 默认情况下，xauth 通过 xmetrics.Observer 提供基础可观测性（追踪、延迟、错误率）。
// MetricsRecorder 用于需要更详细指标的高级场景，例如：
//   - 按租户统计 Token 获取次数
//   - 按 Token 类型（API Key / Client Credentials）分类统计
//   - 自定义缓存命中率指标
//   - 与业务指标系统集成
//
// 使用示例：
//
//	type myMetrics struct {
//	    tokenObtainCounter *prometheus.CounterVec
//	}
//
//	func (m *myMetrics) RecordTokenObtain(ctx context.Context, tenantID, tokenType string, duration time.Duration, err error) {
//	    status := "success"
//	    if err != nil {
//	        status = "error"
//	    }
//	    m.tokenObtainCounter.WithLabelValues(tenantID, tokenType, status).Inc()
//	}
type MetricsRecorder interface {
	// RecordTokenObtain 记录 Token 获取。
	RecordTokenObtain(ctx context.Context, tenantID string, tokenType string, duration time.Duration, err error)

	// RecordTokenVerify 记录 Token 验证。
	RecordTokenVerify(ctx context.Context, duration time.Duration, err error)

	// RecordTokenRefresh 记录 Token 刷新。
	RecordTokenRefresh(ctx context.Context, tenantID string, duration time.Duration, err error)

	// RecordCacheHit 记录缓存命中。
	RecordCacheHit(ctx context.Context, operation string, tenantID string, hit bool)

	// RecordHTTPRequest 记录 HTTP 请求。
	RecordHTTPRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration, err error)
}

// NoopMetricsRecorder 空指标记录器。
type NoopMetricsRecorder struct{}

func (NoopMetricsRecorder) RecordTokenObtain(_ context.Context, _, _ string, _ time.Duration, _ error) {}
func (NoopMetricsRecorder) RecordTokenVerify(_ context.Context, _ time.Duration, _ error)             {}
func (NoopMetricsRecorder) RecordTokenRefresh(_ context.Context, _ string, _ time.Duration, _ error)  {}
func (NoopMetricsRecorder) RecordCacheHit(_ context.Context, _, _ string, _ bool)                     {}
func (NoopMetricsRecorder) RecordHTTPRequest(_ context.Context, _, _ string, _ int, _ time.Duration, _ error) {
}

// =============================================================================
// 基于 Observer 的指标记录器
// =============================================================================

// ObserverMetricsRecorder 基于 Observer 的指标记录器。
type ObserverMetricsRecorder struct {
	observer xmetrics.Observer
}

// NewObserverMetricsRecorder 创建基于 Observer 的指标记录器。
func NewObserverMetricsRecorder(observer xmetrics.Observer) *ObserverMetricsRecorder {
	if observer == nil {
		observer = xmetrics.NoopObserver{}
	}
	return &ObserverMetricsRecorder{observer: observer}
}

// RecordTokenObtain 记录 Token 获取。
func (r *ObserverMetricsRecorder) RecordTokenObtain(ctx context.Context, tenantID string, tokenType string, _ time.Duration, err error) {
	_, span := startSpan(ctx, r.observer, MetricsOpGetToken,
		xmetrics.Attr{Key: MetricsAttrTenantID, Value: tenantID},
		xmetrics.Attr{Key: MetricsAttrTokenType, Value: tokenType},
	)
	endSpan(span, err)
}

// RecordTokenVerify 记录 Token 验证。
func (r *ObserverMetricsRecorder) RecordTokenVerify(ctx context.Context, _ time.Duration, err error) {
	_, span := startSpan(ctx, r.observer, MetricsOpVerifyToken)
	endSpan(span, err)
}

// RecordTokenRefresh 记录 Token 刷新。
func (r *ObserverMetricsRecorder) RecordTokenRefresh(ctx context.Context, tenantID string, _ time.Duration, err error) {
	_, span := startSpan(ctx, r.observer, MetricsOpRefreshToken,
		xmetrics.Attr{Key: MetricsAttrTenantID, Value: tenantID},
	)
	endSpan(span, err)
}

// RecordCacheHit 记录缓存命中。
func (r *ObserverMetricsRecorder) RecordCacheHit(ctx context.Context, operation string, tenantID string, hit bool) {
	_, span := startSpan(ctx, r.observer, operation,
		xmetrics.Attr{Key: MetricsAttrTenantID, Value: tenantID},
		xmetrics.Attr{Key: MetricsAttrCacheHit, Value: hit},
	)
	endSpan(span, nil)
}

// RecordHTTPRequest 记录 HTTP 请求。
func (r *ObserverMetricsRecorder) RecordHTTPRequest(ctx context.Context, method, path string, statusCode int, _ time.Duration, err error) {
	_, span := startSpan(ctx, r.observer, MetricsOpHTTPRequest,
		xmetrics.Attr{Key: MetricsAttrMethod, Value: method},
		xmetrics.Attr{Key: MetricsAttrPath, Value: path},
		xmetrics.Attr{Key: MetricsAttrStatus, Value: statusCode},
	)
	endSpan(span, err)
}
