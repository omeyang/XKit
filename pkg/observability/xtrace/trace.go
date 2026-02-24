package xtrace

import (
	"context"
	"log/slog"
	"strings"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/observability/xlog"
)

// =============================================================================
// 选项配置（HTTP 和 gRPC 共用）
// =============================================================================

// Option 中间件/拦截器选项。
// HTTP 和 gRPC 共用同一套选项类型，避免重复定义。
type Option func(*config)

type config struct {
	autoGenerate bool // 是否自动生成缺失的追踪 ID
}

// WithAutoGenerate 设置是否自动生成缺失的追踪 ID。
//
// 默认为 true。设置为 false 时，不会自动生成追踪 ID。
// 适用于 HTTPMiddleware 和 GRPCUnaryServerInterceptor/GRPCStreamServerInterceptor。
func WithAutoGenerate(enabled bool) Option {
	return func(cfg *config) {
		cfg.autoGenerate = enabled
	}
}

func applyOptions(opts []Option) *config {
	cfg := &config{
		autoGenerate: true, // 默认自动生成
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// =============================================================================
// TraceInfo 追踪信息
// =============================================================================

// TraceInfo 链路追踪信息。
//
// 包含两层信息：
//   - 解析后的字段（TraceID、SpanID、TraceFlags）：用于业务逻辑和 context 注入
//   - 原始传输层字段（Traceparent、Tracestate）：用于透传和手动注入
//
// 设计决策: 当 traceparent 解析成功时，TraceID/SpanID/TraceFlags 来自 traceparent 解析结果，
// Traceparent 保留原始字符串用于透传场景（如 InjectTraceToHeader）。
// 两层字段服务于不同用途，不构成真正的不一致。
type TraceInfo struct {
	TraceID    string
	SpanID     string
	RequestID  string
	TraceFlags string // W3C trace-flags（如 "01" 表示已采样）

	// W3C Trace Context 扩展
	Traceparent string // 原始 traceparent 头，用于透传
	Tracestate  string
}

// IsEmpty 判断追踪信息是否为空
func (t TraceInfo) IsEmpty() bool {
	return t.TraceID == "" && t.SpanID == "" && t.RequestID == "" &&
		t.TraceFlags == "" && t.Traceparent == "" && t.Tracestate == ""
}

// =============================================================================
// Context 辅助函数
// =============================================================================

// 设计决策: TraceID/SpanID/RequestID/TraceFlags 代理函数让用户只需 import xtrace，
// 无需同时 import xctx。xctx 是底层存储层，xtrace 是面向用户的传播层。

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

// TraceFlags 从 context 获取 TraceFlags（代理到 xctx）
func TraceFlags(ctx context.Context) string {
	return xctx.TraceFlags(ctx)
}

// TraceInfoFromContext 从 context 提取完整的追踪信息。
//
// 与 ExtractFromHTTPHeader/ExtractFromMetadata 对称：
//   - ExtractFromHTTPHeader/ExtractFromMetadata: 从传输层提取到 TraceInfo
//   - TraceInfoFromContext: 从 context 提取到 TraceInfo
//
// 返回的 TraceInfo 不包含 Traceparent 和 Tracestate 字段，
// 因为 context 中只存储解析后的各字段，不存储原始传输层头。
func TraceInfoFromContext(ctx context.Context) TraceInfo {
	return TraceInfo{
		TraceID:    xctx.TraceID(ctx),
		SpanID:     xctx.SpanID(ctx),
		RequestID:  xctx.RequestID(ctx),
		TraceFlags: xctx.TraceFlags(ctx),
	}
}

// =============================================================================
// 内部辅助函数 — Context 注入
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
			if err != nil { // 防御性处理：正常流程不会触发（仅 nil context）
				xlog.Warn(ctx, "xtrace: failed to inject "+inj.name,
					slog.String(inj.name, value), slog.Any("error", err))
			}
		} else {
			xlog.Warn(ctx, "xtrace: invalid "+inj.name+" format, discarding",
				slog.String(inj.name, value))
			if autoGenerate {
				ctx, err = inj.ensure(ctx)
				if err != nil { // 防御性处理：正常流程不会触发（仅 nil context）
					xlog.Warn(ctx, "xtrace: failed to ensure "+inj.name, slog.Any("error", err))
				}
			}
		}
	} else if autoGenerate {
		ctx, err = inj.ensure(ctx)
		if err != nil { // 防御性处理：正常流程不会触发（仅 nil context）
			xlog.Warn(ctx, "xtrace: failed to ensure "+inj.name, slog.Any("error", err))
		}
	}
	return ctx
}

// 设计决策: idInjector 实例为 package-level var 而非 const，
// 因为 Go 不支持 const struct。它们初始化后不再修改，实质上是只读的。

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

// requestIDInjector RequestID 注入器。
// RequestID 没有格式限制，validate 始终返回 true。
var requestIDInjector = idInjector{
	name:     "request_id",
	validate: func(string) bool { return true },
	inject:   xctx.WithRequestID,
	ensure:   xctx.EnsureRequestID,
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
	return injectID(ctx, requestID, autoGenerate, requestIDInjector)
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
	if err != nil { // 防御性处理：正常流程不会触发（仅 nil context）
		xlog.Warn(ctx, "xtrace: failed to inject trace_flags",
			slog.String("trace_flags", traceFlags), slog.Any("error", err))
	}
	return ctx
}

// =============================================================================
// W3C Traceparent 解析与生成
// =============================================================================

// parseTraceparent 解析 W3C traceparent 格式
// 格式：{version}-{trace-id}-{parent-id}-{trace-flags}
// 示例：00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
//
// W3C 前向兼容性：
//   - 版本 "ff" 保留，始终无效（大小写不敏感）
//   - 未知版本（> "00"）按 version-00 格式解析前 4 个字段
//   - 未来版本可能包含额外字段（用 "-" 分隔），应忽略
//
// hasTraceparentSeparators 验证 traceparent 分隔符位于正确位置。
// 调用方保证 len(s) >= 55。
func hasTraceparentSeparators(s string) bool {
	return s[2] == '-' && s[35] == '-' && s[52] == '-'
}

// validateTraceparentStructure 验证 traceparent 的结构（长度、分隔符、版本、版本长度约束）。
func validateTraceparentStructure(traceparent string) bool {
	// W3C 规范：最小长度 55 字符（{2}-{32}-{16}-{2}）
	if len(traceparent) < 55 || !hasTraceparentSeparators(traceparent) {
		return false
	}
	version := traceparent[0:2]
	if !isValidTraceparentVersion(version) {
		return false
	}
	// W3C 规范：version 00 必须恰好 55 字符，不允许额外字段
	if version == "00" {
		return len(traceparent) == 55
	}
	// W3C 前向兼容：未知版本如果长度超过 55，第 56 位（索引 55）必须是 '-'
	// 这确保扩展字段使用正确的分隔符格式
	return len(traceparent) <= 55 || traceparent[55] == '-'
}

// 使用固定索引解析，避免 strings.SplitN 的堆分配。
func parseTraceparent(traceparent string) (traceID, spanID, traceFlags string, ok bool) {
	if !validateTraceparentStructure(traceparent) {
		return "", "", "", false
	}

	traceID = traceparent[3:35]
	if !isValidTraceID(traceID) {
		return "", "", "", false
	}

	spanID = traceparent[36:52]
	if !isValidSpanID(spanID) {
		return "", "", "", false
	}

	traceFlags = traceparent[53:55]
	if !isValidTraceFlags(traceFlags) {
		return "", "", "", false
	}

	return traceID, spanID, traceFlags, true
}

// isValidTraceparentVersion 验证 traceparent 版本格式
func isValidTraceparentVersion(version string) bool {
	// 验证版本格式（2个十六进制字符）
	if len(version) != 2 || !isValidHex(version) {
		return false
	}
	// W3C 规范：版本 "ff" 保留，始终无效（大小写不敏感）
	return !strings.EqualFold(version, "ff")
}

// isValidTraceFlags 验证 trace-flags 格式（2个十六进制字符）
func isValidTraceFlags(flags string) bool {
	return len(flags) == 2 && isValidHex(flags)
}

// isValidHex 验证字符串是否为有效的十六进制。
// 解析端容错：同时接受大写和小写，确保与不同实现的互操作性。
// 输出端（formatTraceparent）会统一转换为小写，确保符合 W3C 规范。
func isValidHex(s string) bool {
	if len(s) == 0 {
		return false
	}
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

// traceparentLen W3C traceparent 固定长度：00-{32}-{16}-{2} = 55 字符
const traceparentLen = 55

// formatTraceparent 生成 W3C traceparent 格式
// 注意：仅在 traceID 和 spanID 都有效时才生成
// traceFlags 为空时默认使用 "00"（未采样）
//
// 设计决策: 始终输出版本 "00"。即使收到未知版本的 traceparent，
// 本包作为 v00 实现，按 W3C 规范应以自身支持的版本重新生成。
// 未来版本的扩展字段不在 v00 传播范围内。
//
// W3C Trace Context 规范要求 trace-id、parent-id、trace-flags 必须是小写十六进制。
// 本函数使用固定大小的字节数组减少内存分配。
func formatTraceparent(traceID, spanID, traceFlags string) string {
	if !isValidTraceID(traceID) || !isValidSpanID(spanID) {
		return ""
	}

	// trace-flags 默认为 "00"（未采样）
	if traceFlags == "" || !isValidTraceFlags(traceFlags) {
		traceFlags = "00"
	}

	// traceparent 格式：00-{trace-id}-{span-id}-{trace-flags}
	// 使用 copy 将各部分写入固定大小的缓冲区
	var buf [traceparentLen]byte
	copy(buf[0:3], "00-")
	copy(buf[3:35], strings.ToLower(traceID))
	copy(buf[35:36], "-")
	copy(buf[36:52], strings.ToLower(spanID))
	copy(buf[52:53], "-")
	copy(buf[53:55], strings.ToLower(traceFlags))
	return string(buf[:])
}

// resolveTraceparent 解析 TraceInfo 中的 traceparent 并返回规范化的 v00 格式。
//
// 优先从 info.Traceparent 解析；解析失败或为空时，从 TraceID/SpanID/TraceFlags 生成。
// 返回空字符串表示无法生成有效的 traceparent。
//
// 设计决策: 无论 info.Traceparent 是否包含非 v00 版本，
// 始终以 v00 格式重新生成 traceparent。这与 formatTraceparent 的设计决策一致：
// 本包作为 v00 实现，按 W3C 规范应以自身支持的版本重新生成。
func resolveTraceparent(info TraceInfo) string {
	if info.Traceparent != "" {
		if traceID, spanID, traceFlags, ok := parseTraceparent(info.Traceparent); ok {
			return formatTraceparent(traceID, spanID, traceFlags)
		}
		// 无效时静默丢弃，回退到 TraceID/SpanID 生成
	}
	return formatTraceparent(info.TraceID, info.SpanID, info.TraceFlags)
}

// =============================================================================
// 传输层共享注入逻辑
// =============================================================================

// transportKeys 不同传输协议使用的 key 名称。
type transportKeys struct {
	traceID     string
	spanID      string
	requestID   string
	traceparent string
	tracestate  string
}

// injectTraceInfoTo 将 TraceInfo 的各字段通过 set 函数注入到传输层。
// 这是 InjectTraceToHeader 和 InjectTraceToMetadata 的共享实现。
//
// 设计决策: W3C 规范要求 tracestate 不得在无有效 traceparent 时发送。
// 仅当 traceparent 已成功写入时才注入 tracestate，避免下游收到不完整的 Trace Context。
func injectTraceInfoTo(set func(key, value string), info TraceInfo, keys transportKeys) {
	if info.TraceID != "" {
		set(keys.traceID, info.TraceID)
	}
	if info.SpanID != "" {
		set(keys.spanID, info.SpanID)
	}
	if info.RequestID != "" {
		set(keys.requestID, info.RequestID)
	}

	traceparent := resolveTraceparent(info)
	if traceparent != "" {
		set(keys.traceparent, traceparent)
	}

	if info.Tracestate != "" && traceparent != "" {
		set(keys.tracestate, info.Tracestate)
	}
}
