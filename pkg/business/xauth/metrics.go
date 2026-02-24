package xauth

// =============================================================================
// 指标名称常量
// =============================================================================

const (
	// MetricsComponent 组件名称。
	MetricsComponent = "xauth"

	// 操作名称
	MetricsOpGetToken          = "GetToken"
	MetricsOpVerifyToken       = "VerifyToken"
	MetricsOpRefreshToken      = "RefreshToken"
	MetricsOpGetPlatformData   = "GetPlatformData"
	MetricsOpGetPlatformID     = "GetPlatformID"
	MetricsOpHasParentPlatform = "HasParentPlatform"
	MetricsOpGetUnclassRegion  = "GetUnclassRegionID"
	MetricsOpHTTPRequest       = "HTTP"

	// 属性 Key
	MetricsAttrTenantID   = "tenant_id"
	MetricsAttrCacheHit   = "cache_hit"
	MetricsAttrTokenType  = "token_type"
	MetricsAttrHTTPPath   = "http.path"
	MetricsAttrHTTPMethod = "http.method"
	MetricsAttrHTTPStatus = "http.status"
)
