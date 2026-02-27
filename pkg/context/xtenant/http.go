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
//
// 设计决策: 本函数仅做 TrimSpace，不校验长度、字符集或控制字符。
// 租户 ID/名称的格式因系统而异，格式校验应由中间件选项或业务层负责，
// Extract 函数保持为无策略的薄提取层。
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

// ExtractFromHTTPRequest 从 HTTP Request 提取租户信息
//
// 等价于 ExtractFromHTTPHeader(r.Header)。
func ExtractFromHTTPRequest(r *http.Request) TenantInfo {
	if r == nil {
		return TenantInfo{}
	}
	return ExtractFromHTTPHeader(r.Header)
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

// =============================================================================
// HTTP 中间件
// =============================================================================

// HTTPMiddleware 返回 HTTP 中间件。
// 自动从 HTTP Header 提取租户信息并注入 context。
func HTTPMiddleware() func(http.Handler) http.Handler {
	return HTTPMiddlewareWithOptions()
}

// MiddlewareOption 中间件选项
//
// 设计决策: middlewareConfig 与 grpcInterceptorConfig 字段相同但独立定义，
// 保持 HTTP 和 gRPC 协议选项的类型独立，允许各自独立演进。
type MiddlewareOption func(*middlewareConfig)

type middlewareConfig struct {
	requireTenant   bool
	requireTenantID bool
	ensureTrace     bool
}

// WithRequireTenant 设置是否要求租户信息必须存在
//
// 如果设置为 true，当 TenantID 或 TenantName 缺失时返回 400 错误。
// 默认为 false（不强制要求）。
//
// 与 WithRequireTenantID 互斥，后设置的选项生效。
func WithRequireTenant() MiddlewareOption {
	return func(cfg *middlewareConfig) {
		cfg.requireTenant = true
		cfg.requireTenantID = false
	}
}

// WithRequireTenantID 设置只要求 TenantID 必须存在
//
// 如果设置为 true，当 TenantID 缺失时返回 400 错误，TenantName 不做要求。
// 适用于 TenantName 非必填的场景。
// 默认为 false（不强制要求）。
//
// 与 WithRequireTenant 互斥，后设置的选项生效。
func WithRequireTenantID() MiddlewareOption {
	return func(cfg *middlewareConfig) {
		cfg.requireTenantID = true
		cfg.requireTenant = false
	}
}

// WithEnsureTrace 启用自动生成追踪信息
//
// 当上游未传递 trace header 时，自动生成新的 TraceID、SpanID、RequestID。
// 使当前服务成为分布式链路追踪的起点。
// 默认为 false（仅传播上游已有的追踪信息）。
//
// 典型场景：
//   - 网关服务：启用此选项，确保每个请求都有追踪信息
//   - 下游服务：不启用，只传播上游的追踪信息
func WithEnsureTrace() MiddlewareOption {
	return func(cfg *middlewareConfig) {
		cfg.ensureTrace = true
	}
}

// HTTPMiddlewareWithOptions 返回带选项的 HTTP 中间件。
func HTTPMiddlewareWithOptions(opts ...MiddlewareOption) func(http.Handler) http.Handler {
	cfg := &middlewareConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, code, err := injectTenantToHTTPContext(r, cfg)
			if err != nil {
				// 设计决策: 500 错误在正常流程中不可达（r.Context() 始终非 nil），
				// 这里的错误信息来自 xctx，不含敏感数据，故直接返回以便调试。
				http.Error(w, err.Error(), code)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// injectTenantToHTTPContext 从 HTTP 请求提取租户信息和追踪信息并注入 context
// 返回注入后的 context、HTTP 状态码（仅错误时使用）、错误
func injectTenantToHTTPContext(r *http.Request, cfg *middlewareConfig) (context.Context, int, error) {
	ctx := r.Context()

	// 提取并验证租户信息
	info := ExtractFromHTTPHeader(r.Header)
	if err := validateHTTPTenantInfo(info, cfg); err != nil {
		return nil, http.StatusBadRequest, err
	}

	// 注入租户信息到 context（复用公开 API）
	ctx, err := WithTenantInfo(ctx, info)
	if err != nil { // 防御性处理：当前 xctx 实现下不可达（r.Context() 始终非 nil）
		return nil, http.StatusInternalServerError, err
	}

	// 处理追踪信息
	trace := ExtractTraceFromHTTPHeader(r.Header)
	ctx, err = injectHTTPTraceToContext(ctx, trace, cfg.ensureTrace)
	if err != nil { // 防御性处理：当前 xctx 实现下不可达
		return nil, http.StatusInternalServerError, err
	}

	return ctx, 0, nil
}

// validateHTTPTenantInfo 验证租户信息
func validateHTTPTenantInfo(info TenantInfo, cfg *middlewareConfig) error {
	if cfg.requireTenant {
		return info.Validate()
	}
	if cfg.requireTenantID && info.TenantID == "" {
		return ErrEmptyTenantID
	}
	return nil
}

// injectHTTPTraceToContext 处理追踪信息并注入 context
func injectHTTPTraceToContext(ctx context.Context, trace xctx.Trace, ensureTrace bool) (context.Context, error) {
	ctx, err := xctx.WithTrace(ctx, trace)
	if err != nil { // 防御性处理：当前 xctx 实现下不可达
		return nil, err
	}
	if ensureTrace {
		return xctx.EnsureTrace(ctx)
	}
	return ctx, nil
}

// =============================================================================
// HTTP Header 注入（跨服务传播）
// =============================================================================

// InjectToRequest 将租户信息注入 HTTP 请求。
// 从 context 提取租户信息并设置到请求 Header，用于跨服务调用时传播。
// 同时也会注入服务级的平台信息（从 xplatform 获取）。
//
// 如果 req 为 nil 或 req.Header 为 nil，函数静默返回不执行任何操作。
// 这是防御性设计：http.NewRequest 保证 Header 非空，但某些测试场景或
// 手动构造的 Request 可能出现 nil Header，此时静默跳过比 panic 更安全。
func InjectToRequest(ctx context.Context, req *http.Request) {
	if req == nil || req.Header == nil {
		return
	}

	injectPlatformHeaders(req.Header)
	injectTenantHeaders(ctx, req.Header)
	injectTraceHeaders(ctx, req.Header)
}

// injectPlatformHeaders 注入服务级平台信息
//
// 设计决策: 使用与租户/追踪字段一致的"以源为准"语义——
// xplatform 未初始化时删除平台键，防止请求对象复用时旧平台信息泄漏到下游。
func injectPlatformHeaders(h http.Header) {
	if !xplatform.IsInitialized() {
		h.Del(HeaderPlatformID)
		h.Del(HeaderHasParent)
		h.Del(HeaderUnclassRegionID)
		return
	}
	h.Set(HeaderPlatformID, xplatform.PlatformID())
	if xplatform.HasParent() {
		h.Set(HeaderHasParent, "true")
	} else {
		h.Set(HeaderHasParent, "false")
	}
	if regionID := xplatform.UnclassRegionID(); regionID != "" {
		h.Set(HeaderUnclassRegionID, regionID)
	} else {
		h.Del(HeaderUnclassRegionID)
	}
}

// injectTenantHeaders 注入请求级租户信息
//
// 使用"以 context 为准"的语义：有值则 Set，无值则 Del。
// 防止请求对象复用时旧租户信息泄漏到下游。
func injectTenantHeaders(ctx context.Context, h http.Header) {
	if tid := TenantID(ctx); tid != "" {
		h.Set(HeaderTenantID, tid)
	} else {
		h.Del(HeaderTenantID)
	}
	if tname := TenantName(ctx); tname != "" {
		h.Set(HeaderTenantName, tname)
	} else {
		h.Del(HeaderTenantName)
	}
}

// injectTraceHeaders 注入追踪信息
//
// 使用"以 context 为准"的语义：有值则 Set，无值则 Del。
func injectTraceHeaders(ctx context.Context, h http.Header) {
	if tid := xctx.TraceID(ctx); tid != "" {
		h.Set(HeaderTraceID, tid)
	} else {
		h.Del(HeaderTraceID)
	}
	if sid := xctx.SpanID(ctx); sid != "" {
		h.Set(HeaderSpanID, sid)
	} else {
		h.Del(HeaderSpanID)
	}
	if rid := xctx.RequestID(ctx); rid != "" {
		h.Set(HeaderRequestID, rid)
	} else {
		h.Del(HeaderRequestID)
	}
	if flags := xctx.TraceFlags(ctx); flags != "" {
		h.Set(HeaderTraceFlags, flags)
	} else {
		h.Del(HeaderTraceFlags)
	}
}

// InjectTenantToHeader 将 TenantInfo 注入 HTTP Header
//
// 用于手动构造 HTTP Header 的场景。
// 采用增量写入语义：只 Set 非空字段，不清除已有的键。
// 如需"以 context 为准"的清理语义，请使用 InjectToRequest。
//
// 对 TenantID/TenantName 做 TrimSpace 后再判空和 Set，
// 与包内其他写入路径（WithTenantID、ExtractFromHTTPHeader 等）的归一化语义一致。
func InjectTenantToHeader(h http.Header, info TenantInfo) {
	if h == nil {
		return
	}

	if tid := strings.TrimSpace(info.TenantID); tid != "" {
		h.Set(HeaderTenantID, tid)
	}
	if tname := strings.TrimSpace(info.TenantName); tname != "" {
		h.Set(HeaderTenantName, tname)
	}
}
