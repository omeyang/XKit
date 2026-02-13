package xsemaphore

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// =============================================================================
// Tracer 相关常量
// =============================================================================

const (
	// tracerName 追踪器名称
	tracerName = "xsemaphore"
)

// Span 操作名称
const (
	spanNameTryAcquire = "xsemaphore.TryAcquire"
	spanNameAcquire    = "xsemaphore.Acquire"
	spanNameRelease    = "xsemaphore.Release"
	spanNameExtend     = "xsemaphore.Extend"
	spanNameQuery      = "xsemaphore.Query"
)

// Span 属性名称（Metrics 也复用这些常量，确保 trace 与 metrics 键名一致）
const (
	attrSemType      = "xsemaphore.type"
	attrResource     = "xsemaphore.resource"
	attrTenantID     = "xsemaphore.tenant_id"
	attrCapacity     = "xsemaphore.capacity"
	attrTenantQuota  = "xsemaphore.tenant_quota"
	attrAcquired     = "xsemaphore.acquired"
	attrPermitID     = "xsemaphore.permit_id"
	attrFailReason   = "xsemaphore.fail_reason"
	attrGlobalUsed   = "xsemaphore.global_used"
	attrTenantUsed   = "xsemaphore.tenant_used"
	attrFallbackUsed = "xsemaphore.fallback_used"
	attrRetryCount   = "xsemaphore.retry_count"
	attrSuccess      = "xsemaphore.success"
	attrStrategy     = "xsemaphore.strategy"
)

// =============================================================================
// Tracer 管理
// =============================================================================

// getTracer 获取 tracer 实例
// 如果配置了 TracerProvider 则使用它，否则使用全局默认
func getTracer(tp trace.TracerProvider) trace.Tracer {
	if tp == nil {
		tp = otel.GetTracerProvider()
	}
	return tp.Tracer(tracerName, trace.WithInstrumentationVersion(instrumentationVersion))
}

// =============================================================================
// Span 创建辅助函数
// =============================================================================

// startSpan 创建新的 span
// 如果 tracer 为 nil，使用全局 tracer（可能是 noop tracer）
//
//nolint:unparam // opts 参数保留用于未来扩展（如设置 span kind、attributes）
func startSpan(ctx context.Context, tracer trace.Tracer, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if tracer == nil {
		tracer = otel.GetTracerProvider().Tracer(tracerName)
	}
	return tracer.Start(ctx, name, opts...)
}

// setSpanError 设置 span 错误状态
func setSpanError(span trace.Span, err error) {
	if err != nil && span != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// setSpanOK 设置 span 成功状态
func setSpanOK(span trace.Span) {
	if span != nil {
		span.SetStatus(codes.Ok, "")
	}
}

// =============================================================================
// 通用属性构建
// =============================================================================

// acquireSpanAttributes 构建 acquire 操作的 span 属性
func acquireSpanAttributes(semType, resource, tenantID string, capacity, tenantQuota int) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String(attrSemType, semType),
		attribute.String(attrResource, resource),
		attribute.Int(attrCapacity, capacity),
	}
	if tenantID != "" {
		attrs = append(attrs, attribute.String(attrTenantID, tenantID))
	}
	if tenantQuota > 0 {
		attrs = append(attrs, attribute.Int(attrTenantQuota, tenantQuota))
	}
	return attrs
}

// releaseSpanAttributes 构建 release 操作的 span 属性
func releaseSpanAttributes(semType, resource, tenantID, permitID string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String(attrSemType, semType),
		attribute.String(attrResource, resource),
		attribute.String(attrPermitID, permitID),
	}
	if tenantID != "" {
		attrs = append(attrs, attribute.String(attrTenantID, tenantID))
	}
	return attrs
}

// extendSpanAttributes 构建 extend 操作的 span 属性
func extendSpanAttributes(semType, resource, tenantID, permitID string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String(attrSemType, semType),
		attribute.String(attrResource, resource),
		attribute.String(attrPermitID, permitID),
	}
	if tenantID != "" {
		attrs = append(attrs, attribute.String(attrTenantID, tenantID))
	}
	return attrs
}
