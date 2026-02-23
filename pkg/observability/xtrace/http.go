package xtrace

import (
	"context"
	"net/http"
	"strings"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

// =============================================================================
// HTTP Header 常量
// =============================================================================

// HTTP Header 名称
const (
	// 自定义 Header（兼容常见实现）
	HeaderTraceID   = "X-Trace-ID"
	HeaderSpanID    = "X-Span-ID"
	HeaderRequestID = "X-Request-ID"

	// W3C Trace Context 标准 Header
	HeaderTraceparent = "traceparent"
	HeaderTracestate  = "tracestate"
)

// =============================================================================
// HTTP Header 提取
// =============================================================================

// ExtractFromHTTPHeader 从 HTTP Header 提取追踪信息
//
// 提取以下 Header：
//   - X-Trace-ID -> TraceID
//   - X-Span-ID -> SpanID
//   - X-Request-ID -> RequestID
//   - traceparent -> Traceparent (W3C)
//   - tracestate -> Tracestate (W3C)
//
// 如果存在 traceparent，会自动解析出 TraceID 和 SpanID。
func ExtractFromHTTPHeader(h http.Header) TraceInfo {
	if h == nil {
		return TraceInfo{}
	}

	info := TraceInfo{
		TraceID:     strings.TrimSpace(h.Get(HeaderTraceID)),
		SpanID:      strings.TrimSpace(h.Get(HeaderSpanID)),
		RequestID:   strings.TrimSpace(h.Get(HeaderRequestID)),
		Traceparent: strings.TrimSpace(h.Get(HeaderTraceparent)),
		Tracestate:  strings.TrimSpace(h.Get(HeaderTracestate)),
	}

	// 如果有 traceparent，解析出 TraceID、SpanID 和 TraceFlags
	// W3C traceparent 优先级最高，直接覆盖自定义头的值
	if info.Traceparent != "" {
		if traceID, spanID, traceFlags, ok := parseTraceparent(info.Traceparent); ok {
			info.TraceID = traceID
			info.SpanID = spanID
			info.TraceFlags = traceFlags
		}
	}

	return info
}

// ExtractFromHTTPRequest 从 HTTP Request 提取追踪信息
func ExtractFromHTTPRequest(r *http.Request) TraceInfo {
	if r == nil {
		return TraceInfo{}
	}
	return ExtractFromHTTPHeader(r.Header)
}

// =============================================================================
// HTTP 中间件
// =============================================================================

// HTTPMiddleware 返回 HTTP 中间件。
// 自动从 HTTP Header 提取追踪信息并注入 context，缺失时自动生成。
//
// 设计决策: xtrace 只做传输层适配（提取/注入追踪标识），不创建 OTel Span。
// Span 生命周期管理由 OTel SDK 的 otelhttp 中间件负责。
func HTTPMiddleware(opts ...Option) func(http.Handler) http.Handler {
	cfg := applyOptions(opts)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// 提取追踪信息
			info := ExtractFromHTTPHeader(r.Header)

			// 注入到 context
			ctx = injectTraceToContext(ctx, info, cfg.autoGenerate)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// =============================================================================
// HTTP Header 注入（跨服务传播）
// =============================================================================

// InjectToRequest 将追踪信息注入 HTTP 请求。
// 从 context 提取追踪信息并设置到请求 Header，用于跨服务调用时传播。
// 会正确传递上游的 trace-flags（采样决策）。
//
// 注意：本函数不传播 tracestate。如需传播 tracestate，
// 请使用 InjectTraceToHeader 手动设置，或使用 OpenTelemetry SDK。
func InjectToRequest(ctx context.Context, req *http.Request) {
	if req == nil {
		return
	}

	// 防止调用方构造 &http.Request{} 导致 nil Header panic
	if req.Header == nil {
		req.Header = make(http.Header)
	}

	traceID := xctx.TraceID(ctx)
	spanID := xctx.SpanID(ctx)
	requestID := xctx.RequestID(ctx)
	traceFlags := xctx.TraceFlags(ctx)

	// 注入追踪信息
	if traceID != "" {
		req.Header.Set(HeaderTraceID, traceID)
	}
	if spanID != "" {
		req.Header.Set(HeaderSpanID, spanID)
	}
	if requestID != "" {
		req.Header.Set(HeaderRequestID, requestID)
	}

	// 生成 W3C traceparent（仅在 traceID 和 spanID 都有效时）
	// 使用 context 中的 traceFlags，若无则默认 "00"
	if traceparent := formatTraceparent(traceID, spanID, traceFlags); traceparent != "" {
		req.Header.Set(HeaderTraceparent, traceparent)
	}
}

// InjectTraceToHeader 将 TraceInfo 注入 HTTP Header
//
// 用于手动构造 HTTP Header 的场景。
// 如果 TraceInfo.Traceparent 为空但有有效的 TraceID 和 SpanID，
// 会自动生成 traceparent（使用 TraceFlags，若为空则默认 "00"）。
//
// 注意：如果 Traceparent 格式无效，会静默丢弃并尝试从 TraceID/SpanID 生成。
// 如果同时设置了 TraceID 和 Traceparent，请确保两者一致以避免下游混淆。
func InjectTraceToHeader(h http.Header, info TraceInfo) {
	if h == nil {
		return
	}

	if info.TraceID != "" {
		h.Set(HeaderTraceID, info.TraceID)
	}
	if info.SpanID != "" {
		h.Set(HeaderSpanID, info.SpanID)
	}
	if info.RequestID != "" {
		h.Set(HeaderRequestID, info.RequestID)
	}

	// 设计决策: 无论 info.Traceparent 是否包含非 v00 版本，
	// 始终以 v00 格式重新生成 traceparent。这与 formatTraceparent 的设计决策一致：
	// 本包作为 v00 实现，按 W3C 规范应以自身支持的版本重新生成。
	if info.Traceparent != "" {
		if traceID, spanID, traceFlags, ok := parseTraceparent(info.Traceparent); ok {
			h.Set(HeaderTraceparent, formatTraceparent(traceID, spanID, traceFlags))
		}
		// 无效时静默丢弃，尝试从 TraceID/SpanID 生成
	}

	// 如果没有设置有效的 Traceparent，尝试从 TraceID/SpanID 生成
	if h.Get(HeaderTraceparent) == "" {
		if traceparent := formatTraceparent(info.TraceID, info.SpanID, info.TraceFlags); traceparent != "" {
			h.Set(HeaderTraceparent, traceparent)
		}
	}

	if info.Tracestate != "" {
		h.Set(HeaderTracestate, info.Tracestate)
	}
}
