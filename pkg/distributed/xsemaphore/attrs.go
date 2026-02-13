package xsemaphore

import (
	"log/slog"
	"time"
)

// =============================================================================
// 日志属性键常量
// =============================================================================

const (
	attrKeyPermitID    = "permit_id"
	attrKeyResource    = "resource"
	attrKeyTenantID    = "tenant_id"
	attrKeyCapacity    = "capacity"
	attrKeyUsed        = "used"
	attrKeyAvailable   = "available"
	attrKeyError       = "error"
	attrKeyReason      = "reason"
	attrKeyDuration    = "duration"
	attrKeyGlobalCount = "global_count"
	attrKeyTenantCount = "tenant_count"
	attrKeyRetry       = "retry"
	attrKeyMaxRetries  = "max_retries"
	attrKeySemType     = "sem_type"
	attrKeyStrategy    = "strategy"
	attrKeyStatusCode  = "status_code"
)

// =============================================================================
// 日志属性构造函数
// =============================================================================

// AttrPermitID 返回许可 ID 属性
func AttrPermitID(id string) slog.Attr {
	return slog.String(attrKeyPermitID, id)
}

// AttrResource 返回资源属性
func AttrResource(resource string) slog.Attr {
	return slog.String(attrKeyResource, resource)
}

// AttrTenantID 返回租户 ID 属性
func AttrTenantID(tenantID string) slog.Attr {
	return slog.String(attrKeyTenantID, tenantID)
}

// AttrCapacity 返回容量属性
func AttrCapacity(capacity int) slog.Attr {
	return slog.Int(attrKeyCapacity, capacity)
}

// AttrUsed 返回已使用数属性
func AttrUsed(used int) slog.Attr {
	return slog.Int(attrKeyUsed, used)
}

// AttrAvailable 返回可用数属性
func AttrAvailable(available int) slog.Attr {
	return slog.Int(attrKeyAvailable, available)
}

// AttrError 返回错误属性
func AttrError(err error) slog.Attr {
	if err == nil {
		return slog.String(attrKeyError, "")
	}
	return slog.String(attrKeyError, err.Error())
}

// AttrReason 返回原因属性
func AttrReason(reason string) slog.Attr {
	return slog.String(attrKeyReason, reason)
}

// AttrDuration 返回持续时间属性
func AttrDuration(d time.Duration) slog.Attr {
	return slog.Duration(attrKeyDuration, d)
}

// AttrGlobalCount 返回全局计数属性
func AttrGlobalCount(count int) slog.Attr {
	return slog.Int(attrKeyGlobalCount, count)
}

// AttrTenantCount 返回租户计数属性
func AttrTenantCount(count int) slog.Attr {
	return slog.Int(attrKeyTenantCount, count)
}

// AttrRetry 返回重试次数属性
func AttrRetry(n int) slog.Attr {
	return slog.Int(attrKeyRetry, n)
}

// AttrMaxRetries 返回最大重试次数属性
func AttrMaxRetries(n int) slog.Attr {
	return slog.Int(attrKeyMaxRetries, n)
}

// AttrSemType 返回信号量类型属性
func AttrSemType(semType string) slog.Attr {
	return slog.String(attrKeySemType, semType)
}

// AttrStrategy 返回策略属性
func AttrStrategy(strategy FallbackStrategy) slog.Attr {
	return slog.String(attrKeyStrategy, string(strategy))
}

// AttrStatusCode 返回状态码属性
func AttrStatusCode(code int) slog.Attr {
	return slog.Int(attrKeyStatusCode, code)
}
