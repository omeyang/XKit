package xtenant

import (
	"context"
	"net/http"
	"strings"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/context/xplatform"
)

// =============================================================================
// HTTP Header 常量
// =============================================================================

// HTTP Header 名称（遵循 X- 前缀约定）
const (
	HeaderPlatformID      = "X-Platform-ID"
	HeaderTenantID        = "X-Tenant-ID"
	HeaderTenantName      = "X-Tenant-Name"
	HeaderHasParent       = "X-Has-Parent"
	HeaderUnclassRegionID = "X-Unclass-Region-ID"

	// Trace 相关 Header
	HeaderTraceID    = "X-Trace-ID"
	HeaderSpanID     = "X-Span-ID"
	HeaderRequestID  = "X-Request-ID"
	HeaderTraceFlags = "X-Trace-Flags"
)

// =============================================================================
// HTTP Header 提取
// =============================================================================

// ExtractFromHTTPHeader 从 HTTP Header 提取租户信息
//
// 提取以下 Header：
//   - X-Tenant-ID -> TenantID
//   - X-Tenant-Name -> TenantName
//
// 所有字段都是可选的，未设置的字段保持零值。
// Header 值会自动去除首尾空白。
func ExtractFromHTTPHeader(h http.Header) TenantInfo {
	if h == nil {
		return TenantInfo{}
	}

	return TenantInfo{
		TenantID:   strings.TrimSpace(h.Get(HeaderTenantID)),
		TenantName: strings.TrimSpace(h.Get(HeaderTenantName)),
	}
}

// ExtractTraceFromHTTPHeader 从 HTTP Header 提取追踪信息
//
// 提取以下 Header：
//   - X-Trace-ID -> TraceID
//   - X-Span-ID -> SpanID
//   - X-Request-ID -> RequestID
//   - X-Trace-Flags -> TraceFlags
//
// 所有字段都是可选的，未设置的字段保持零值。
func ExtractTraceFromHTTPHeader(h http.Header) xctx.Trace {
	if h == nil {
		return xctx.Trace{}
	}

	return xctx.Trace{
		TraceID:    strings.TrimSpace(h.Get(HeaderTraceID)),
		SpanID:     strings.TrimSpace(h.Get(HeaderSpanID)),
		RequestID:  strings.TrimSpace(h.Get(HeaderRequestID)),
		TraceFlags: strings.TrimSpace(h.Get(HeaderTraceFlags)),
	}
}

// ExtractTraceFromHTTPRequest 从 HTTP Request 提取追踪信息
//
// 等价于 ExtractTraceFromHTTPHeader(r.Header)。
func ExtractTraceFromHTTPRequest(r *http.Request) xctx.Trace {
	if r == nil {
		return xctx.Trace{}
	}
	return ExtractTraceFromHTTPHeader(r.Header)
}

// ExtractFromHTTPRequest 从 HTTP Request 提取租户信息
//
// 等价于 ExtractFromHTTPHeader(r.Header)。
func ExtractFromHTTPRequest(r *http.Request) TenantInfo {
	if r == nil {
		return TenantInfo{}
	}
	return ExtractFromHTTPHeader(r.Header)
}

// =============================================================================
// HTTP 中间件
// =============================================================================

// HTTPMiddleware 返回 HTTP 中间件。
// 自动从 HTTP Header 提取租户信息并注入 context。
func HTTPMiddleware() func(http.Handler) http.Handler {
	return HTTPMiddlewareWithOptions()
}

// MiddlewareOption 中间件选项
type MiddlewareOption func(*middlewareConfig)

type middlewareConfig struct {
	requireTenant bool
}

// WithRequireTenant 设置是否要求租户信息必须存在
//
// 如果设置为 true，当租户信息缺失时返回 400 错误。
// 默认为 false（不强制要求）。
func WithRequireTenant() MiddlewareOption {
	return func(cfg *middlewareConfig) {
		cfg.requireTenant = true
	}
}

// HTTPMiddlewareWithOptions 返回带选项的 HTTP 中间件。
func HTTPMiddlewareWithOptions(opts ...MiddlewareOption) func(http.Handler) http.Handler {
	cfg := &middlewareConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// 提取租户信息
			info := ExtractFromHTTPHeader(r.Header)

			// 如果要求租户信息必须存在，进行验证
			if cfg.requireTenant {
				if err := info.Validate(); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}

			// 注入租户信息到 context
			var err error
			if info.TenantID != "" {
				ctx, err = xctx.WithTenantID(ctx, info.TenantID)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
			if info.TenantName != "" {
				ctx, err = xctx.WithTenantName(ctx, info.TenantName)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}

			// 提取并注入追踪信息（确保链路追踪连续性）
			trace := ExtractTraceFromHTTPHeader(r.Header)
			ctx, err = xctx.WithTrace(ctx, trace)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// =============================================================================
// HTTP Header 注入（跨服务传播）
// =============================================================================

// InjectToRequest 将租户信息注入 HTTP 请求。
// 从 context 提取租户信息并设置到请求 Header，用于跨服务调用时传播。
// 同时也会注入服务级的平台信息（从 xplatform 获取）。
func InjectToRequest(ctx context.Context, req *http.Request) {
	if req == nil || req.Header == nil {
		return
	}

	injectPlatformHeaders(req.Header)
	injectTenantHeaders(ctx, req.Header)
	injectTraceHeaders(ctx, req.Header)
}

// injectPlatformHeaders 注入服务级平台信息
func injectPlatformHeaders(h http.Header) {
	if !xplatform.IsInitialized() {
		return
	}
	if pid := xplatform.PlatformID(); pid != "" {
		h.Set(HeaderPlatformID, pid)
	}
	if xplatform.HasParent() {
		h.Set(HeaderHasParent, "true")
	} else {
		h.Set(HeaderHasParent, "false")
	}
	if regionID := xplatform.UnclassRegionID(); regionID != "" {
		h.Set(HeaderUnclassRegionID, regionID)
	}
}

// injectTenantHeaders 注入请求级租户信息
func injectTenantHeaders(ctx context.Context, h http.Header) {
	if tid := TenantID(ctx); tid != "" {
		h.Set(HeaderTenantID, tid)
	}
	if tname := TenantName(ctx); tname != "" {
		h.Set(HeaderTenantName, tname)
	}
}

// injectTraceHeaders 注入追踪信息
func injectTraceHeaders(ctx context.Context, h http.Header) {
	if tid := xctx.TraceID(ctx); tid != "" {
		h.Set(HeaderTraceID, tid)
	}
	if sid := xctx.SpanID(ctx); sid != "" {
		h.Set(HeaderSpanID, sid)
	}
	if rid := xctx.RequestID(ctx); rid != "" {
		h.Set(HeaderRequestID, rid)
	}
	if flags := xctx.TraceFlags(ctx); flags != "" {
		h.Set(HeaderTraceFlags, flags)
	}
}

// InjectTenantToHeader 将 TenantInfo 注入 HTTP Header
//
// 用于手动构造 HTTP Header 的场景。
func InjectTenantToHeader(h http.Header, info TenantInfo) {
	if h == nil {
		return
	}

	if info.TenantID != "" {
		h.Set(HeaderTenantID, info.TenantID)
	}
	if info.TenantName != "" {
		h.Set(HeaderTenantName, info.TenantName)
	}
}
