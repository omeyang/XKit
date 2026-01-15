package xtrace

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/observability/xlog"
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

// TraceInfo 链路追踪信息
type TraceInfo struct {
	TraceID    string
	SpanID     string
	RequestID  string
	TraceFlags string // W3C trace-flags（如 "01" 表示已采样）

	// W3C Trace Context 扩展
	Traceparent string
	Tracestate  string
}

// IsEmpty 判断追踪信息是否为空
func (t TraceInfo) IsEmpty() bool {
	return t.TraceID == "" && t.SpanID == "" && t.RequestID == "" &&
		t.TraceFlags == "" && t.Traceparent == "" && t.Tracestate == ""
}

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
func HTTPMiddleware() func(http.Handler) http.Handler {
	return HTTPMiddlewareWithOptions()
}

// MiddlewareOption 中间件选项
type MiddlewareOption func(*middlewareConfig)

type middlewareConfig struct {
	autoGenerate bool // 是否自动生成缺失的追踪 ID
}

// WithAutoGenerate 设置是否自动生成缺失的追踪 ID
//
// 默认为 true。设置为 false 时，不会自动生成追踪 ID。
func WithAutoGenerate(enabled bool) MiddlewareOption {
	return func(cfg *middlewareConfig) {
		cfg.autoGenerate = enabled
	}
}

// HTTPMiddlewareWithOptions 返回带选项的 HTTP 中间件
func HTTPMiddlewareWithOptions(opts ...MiddlewareOption) func(http.Handler) http.Handler {
	cfg := &middlewareConfig{
		autoGenerate: true, // 默认自动生成
	}
	for _, opt := range opts {
		opt(cfg)
	}

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

	// 如果已有 Traceparent，验证后再透传，避免传播无效 traceparent
	if info.Traceparent != "" {
		if _, _, _, ok := parseTraceparent(info.Traceparent); ok {
			h.Set(HeaderTraceparent, info.Traceparent)
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

// =============================================================================
// Context 辅助函数
// =============================================================================

// TraceID 从 context 获取 TraceID（代理到 xctx）
func TraceID(ctx context.Context) string {
	return xctx.TraceID(ctx)
}

// SpanID 从 context 获取 SpanID（代理到 xctx）
func SpanID(ctx context.Context) string {
	return xctx.SpanID(ctx)
}

// RequestID 从 context 获取 RequestID（代理到 xctx）
func RequestID(ctx context.Context) string {
	return xctx.RequestID(ctx)
}

// =============================================================================
// 内部辅助函数
// =============================================================================

// injectTraceToContext 将追踪信息注入 context
func injectTraceToContext(ctx context.Context, info TraceInfo, autoGenerate bool) context.Context {
	ctx = injectTraceID(ctx, info.TraceID, autoGenerate)
	ctx = injectSpanID(ctx, info.SpanID, autoGenerate)
	ctx = injectRequestID(ctx, info.RequestID, autoGenerate)
	ctx = injectTraceFlags(ctx, info.TraceFlags)
	return ctx
}

// idInjector 定义 ID 注入的行为
type idInjector struct {
	name     string                                                 // ID 名称，用于日志
	validate func(string) bool                                      // 验证函数
	inject   func(context.Context, string) (context.Context, error) // 注入函数
	ensure   func(context.Context) (context.Context, error)         // 确保存在函数
}

// injectID 通用的 ID 注入逻辑
func injectID(ctx context.Context, value string, autoGenerate bool, inj idInjector) context.Context {
	var err error
	if value != "" {
		if inj.validate(value) {
			ctx, err = inj.inject(ctx, value)
			if err != nil {
				xlog.Warn(ctx, "xtrace: failed to inject "+inj.name,
					slog.String(inj.name, value), slog.Any("error", err))
			}
		} else {
			xlog.Warn(ctx, "xtrace: invalid "+inj.name+" format, discarding",
				slog.String(inj.name, value))
			if autoGenerate {
				ctx, err = inj.ensure(ctx)
				if err != nil {
					xlog.Warn(ctx, "xtrace: failed to ensure "+inj.name, slog.Any("error", err))
				}
			}
		}
	} else if autoGenerate {
		ctx, err = inj.ensure(ctx)
		if err != nil {
			xlog.Warn(ctx, "xtrace: failed to ensure "+inj.name, slog.Any("error", err))
		}
	}
	return ctx
}

// traceIDInjector TraceID 注入器
var traceIDInjector = idInjector{
	name:     "trace_id",
	validate: isValidTraceID,
	inject:   xctx.WithTraceID,
	ensure:   xctx.EnsureTraceID,
}

// spanIDInjector SpanID 注入器
var spanIDInjector = idInjector{
	name:     "span_id",
	validate: isValidSpanID,
	inject:   xctx.WithSpanID,
	ensure:   xctx.EnsureSpanID,
}

// injectTraceID 注入或生成 TraceID
func injectTraceID(ctx context.Context, traceID string, autoGenerate bool) context.Context {
	return injectID(ctx, traceID, autoGenerate, traceIDInjector)
}

// injectSpanID 注入或生成 SpanID
func injectSpanID(ctx context.Context, spanID string, autoGenerate bool) context.Context {
	return injectID(ctx, spanID, autoGenerate, spanIDInjector)
}

// injectRequestID 注入或生成 RequestID
func injectRequestID(ctx context.Context, requestID string, autoGenerate bool) context.Context {
	var err error
	if requestID != "" {
		ctx, err = xctx.WithRequestID(ctx, requestID)
		if err != nil {
			xlog.Warn(ctx, "xtrace: failed to inject request_id",
				slog.String("request_id", requestID), slog.Any("error", err))
		}
	} else if autoGenerate {
		ctx, err = xctx.EnsureRequestID(ctx)
		if err != nil {
			xlog.Warn(ctx, "xtrace: failed to ensure request_id", slog.Any("error", err))
		}
	}
	return ctx
}

// injectTraceFlags 注入 TraceFlags
// TraceFlags 不需要自动生成，仅在从上游收到时注入
//
// 格式校验：trace_flags 必须是 2 位十六进制字符（如 "00"、"01"、"ff"）。
// 无效格式会记录警告日志并跳过注入，避免污染 context。
func injectTraceFlags(ctx context.Context, traceFlags string) context.Context {
	if traceFlags == "" {
		return ctx
	}
	// 格式校验：必须是 2 位十六进制
	if !isValidTraceFlags(traceFlags) {
		xlog.Warn(ctx, "xtrace: invalid trace_flags format, discarding",
			slog.String("trace_flags", traceFlags),
			slog.String("expected", "2-character hex string (e.g., \"00\", \"01\")"))
		return ctx
	}
	var err error
	ctx, err = xctx.WithTraceFlags(ctx, strings.ToLower(traceFlags))
	if err != nil {
		xlog.Warn(ctx, "xtrace: failed to inject trace_flags",
			slog.String("trace_flags", traceFlags), slog.Any("error", err))
	}
	return ctx
}

// parseTraceparent 解析 W3C traceparent 格式
// 格式：{version}-{trace-id}-{parent-id}-{trace-flags}
// 示例：00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
//
// W3C 前向兼容性：
//   - 版本 "ff" 保留，始终无效
//   - 未知版本（> "00"）按 version-00 格式解析前 4 个字段
//   - 未来版本可能包含额外字段（用 "-" 分隔），应忽略
func parseTraceparent(traceparent string) (traceID, spanID, traceFlags string, ok bool) {
	// W3C 规范：最小长度 55 字符（00-{32}-{16}-{2}）
	if len(traceparent) < 55 {
		return "", "", "", false
	}

	// 允许未来版本包含额外字段，只取前 4 个
	parts := strings.SplitN(traceparent, "-", 5)
	if len(parts) < 4 {
		return "", "", "", false
	}

	if !isValidTraceparentVersion(parts[0]) {
		return "", "", "", false
	}
	// W3C 规范：version 00 必须恰好 55 字符，不允许额外字段
	// 未来版本（> 00）可以包含额外字段
	if parts[0] == "00" && len(traceparent) != 55 {
		return "", "", "", false
	}
	if !isValidTraceID(parts[1]) {
		return "", "", "", false
	}
	if !isValidSpanID(parts[2]) {
		return "", "", "", false
	}
	if !isValidTraceFlags(parts[3]) {
		return "", "", "", false
	}

	return parts[1], parts[2], parts[3], true
}

// isValidTraceparentVersion 验证 traceparent 版本格式
func isValidTraceparentVersion(version string) bool {
	// 验证版本格式（2个十六进制字符）
	if len(version) != 2 || !isValidHex(version) {
		return false
	}
	// W3C 规范：版本 "ff" 保留，始终无效
	return version != "ff"
}

// isValidTraceFlags 验证 trace-flags 格式（2个十六进制字符）
func isValidTraceFlags(flags string) bool {
	return len(flags) == 2 && isValidHex(flags)
}

// isValidHex 验证字符串是否为有效的十六进制
func isValidHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		isDigit := c >= '0' && c <= '9'
		isLowerHex := c >= 'a' && c <= 'f'
		isUpperHex := c >= 'A' && c <= 'F'
		if !isDigit && !isLowerHex && !isUpperHex {
			return false
		}
	}
	return true
}

// isValidTraceID 验证 trace ID 格式（32位十六进制，非全零）
func isValidTraceID(id string) bool {
	if len(id) != 32 || !isValidHex(id) {
		return false
	}
	return id != "00000000000000000000000000000000"
}

// isValidSpanID 验证 span ID 格式（16位十六进制，非全零）
func isValidSpanID(id string) bool {
	if len(id) != 16 || !isValidHex(id) {
		return false
	}
	return id != "0000000000000000"
}

// formatTraceparent 生成 W3C traceparent 格式
// 注意：仅在 traceID 和 spanID 都有效时才生成
// traceFlags 为空时默认使用 "00"（未采样）
//
// W3C Trace Context 规范要求 trace-id、parent-id、trace-flags 必须是小写十六进制。
// 本函数会自动将输入转换为小写，确保输出符合规范。
func formatTraceparent(traceID, spanID, traceFlags string) string {
	// trace-id 必须是 32 位十六进制且非全零
	if len(traceID) != 32 || !isValidHex(traceID) {
		return ""
	}
	if traceID == "00000000000000000000000000000000" {
		return ""
	}

	// span-id 必须是 16 位十六进制且非全零
	if len(spanID) != 16 || !isValidHex(spanID) || spanID == "0000000000000000" {
		return ""
	}

	// trace-flags 默认为 "00"（未采样）
	if traceFlags == "" || len(traceFlags) != 2 || !isValidHex(traceFlags) {
		traceFlags = "00"
	}

	// W3C 规范要求小写，统一转换确保兼容性
	return "00-" + strings.ToLower(traceID) + "-" + strings.ToLower(spanID) + "-" + strings.ToLower(traceFlags)
}
