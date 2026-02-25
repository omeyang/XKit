package xctx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// =============================================================================
// Require 函数：强制获取模式
// 追踪信息通常由 EnsureXxx 自动生成，但在需要确认特定字段存在的场景下
// 提供单字段级的强制获取。如需批量校验，可使用 Trace.Validate()。
// =============================================================================

// RequireTraceID 从 context 获取 trace ID，不存在则返回错误。
//
// 语义：值必须存在，缺失时返回 ErrMissingTraceID。
// 如果 ctx 为 nil，返回 ErrNilContext。
func RequireTraceID(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", ErrNilContext
	}
	v := TraceID(ctx)
	if v == "" {
		return "", ErrMissingTraceID
	}
	return v, nil
}

// RequireSpanID 从 context 获取 span ID，不存在则返回错误。
//
// 语义：值必须存在，缺失时返回 ErrMissingSpanID。
// 如果 ctx 为 nil，返回 ErrNilContext。
func RequireSpanID(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", ErrNilContext
	}
	v := SpanID(ctx)
	if v == "" {
		return "", ErrMissingSpanID
	}
	return v, nil
}

// RequireRequestID 从 context 获取 request ID，不存在则返回错误。
//
// 语义：值必须存在，缺失时返回 ErrMissingRequestID。
// 如果 ctx 为 nil，返回 ErrNilContext。
func RequireRequestID(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", ErrNilContext
	}
	v := RequestID(ctx)
	if v == "" {
		return "", ErrMissingRequestID
	}
	return v, nil
}

// =============================================================================
// ID 格式常量（遵循 W3C Trace Context 规范）
// =============================================================================

const (
	// TraceIDSize W3C 规范: 128-bit (16 bytes) -> 32 hex chars
	TraceIDSize = 16

	// SpanIDSize W3C 规范: 64-bit (8 bytes) -> 16 hex chars
	SpanIDSize = 8
)

// =============================================================================
// Trace 日志属性 Key 常量
// =============================================================================

// Trace Key 常量，遵循 OpenTelemetry 语义约定（下划线分隔）
const (
	KeyTraceID    = "trace_id"
	KeySpanID     = "span_id"
	KeyRequestID  = "request_id"
	KeyTraceFlags = "trace_flags"

	// traceFieldCount 追踪字段数量（用于 slog 属性预分配，不导出以避免脆弱的 API 契约）
	traceFieldCount = 4
)

// =============================================================================
// Trace Context Key 定义
// =============================================================================

const (
	keyTraceID    = contextKey("xctx:trace_id")
	keySpanID     = contextKey("xctx:span_id")
	keyRequestID  = contextKey("xctx:request_id")
	keyTraceFlags = contextKey("xctx:trace_flags")
)

// =============================================================================
// TraceID 操作
// =============================================================================

// WithTraceID 将 trace ID 注入 context
//
// 如果 ctx 为 nil，返回 ErrNilContext。
// 未来 OpenTelemetry 集成时，此函数可改为从 otel span 中提取 trace ID，
// 或者提供 WithTraceIDFromSpan 等扩展方法。
func WithTraceID(ctx context.Context, traceID string) (context.Context, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	return context.WithValue(ctx, keyTraceID, traceID), nil
}

// TraceID 从 context 提取 trace ID，不存在返回空字符串
// 未来 OpenTelemetry 集成时，可优先从 otel span context 中提取。
func TraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(keyTraceID).(string); ok {
		return v
	}
	return ""
}

// =============================================================================
// SpanID 操作
// =============================================================================

// WithSpanID 将 span ID 注入 context
//
// 如果 ctx 为 nil，返回 ErrNilContext。
func WithSpanID(ctx context.Context, spanID string) (context.Context, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	return context.WithValue(ctx, keySpanID, spanID), nil
}

// SpanID 从 context 提取 span ID，不存在返回空字符串
func SpanID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(keySpanID).(string); ok {
		return v
	}
	return ""
}

// =============================================================================
// RequestID 操作
// =============================================================================

// WithRequestID 将 request ID 注入 context
//
// 如果 ctx 为 nil，返回 ErrNilContext。
func WithRequestID(ctx context.Context, requestID string) (context.Context, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	return context.WithValue(ctx, keyRequestID, requestID), nil
}

// RequestID 从 context 提取 request ID，不存在返回空字符串
func RequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(keyRequestID).(string); ok {
		return v
	}
	return ""
}

// =============================================================================
// TraceFlags 操作（W3C Trace Context trace-flags 字段）
// =============================================================================

// WithTraceFlags 将 trace flags 注入 context
//
// trace-flags 是 W3C Trace Context 的一部分，用于传递采样决策等信息。
// 格式: 2位十六进制字符串（如 "01" 表示已采样，"00" 表示未采样）
// 如果 ctx 为 nil，返回 ErrNilContext。
func WithTraceFlags(ctx context.Context, flags string) (context.Context, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	return context.WithValue(ctx, keyTraceFlags, flags), nil
}

// TraceFlags 从 context 提取 trace flags，不存在返回空字符串
//
// 返回值格式为 2 位十六进制字符串（如 "01"、"00"）。
// 如果未设置，返回空字符串，调用方应根据业务需求决定默认行为。
func TraceFlags(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(keyTraceFlags).(string); ok {
		return v
	}
	return ""
}

// =============================================================================
// ID 生成函数（遵循 W3C Trace Context 规范）
// 参考: https://www.w3.org/TR/trace-context/
// 参考: https://github.com/open-telemetry/opentelemetry-go/blob/main/sdk/trace/id_generator.go
// =============================================================================

// isAllZeros 检查字节切片是否全为零
// W3C Trace Context 规范禁止全零的 trace-id 和 span-id
//
// 设计决策: 未引入可替换的随机源注入点，因为：
//   - crypto/rand 失败属于系统级故障，测试价值有限
//   - 全零概率极低（2^-128 / 2^-64），不值得为此增加生产复杂度
//   - 覆盖率 ~83% 对这类极端分支是可接受的
func isAllZeros(buf []byte) bool {
	for _, b := range buf {
		if b != 0 {
			return false
		}
	}
	return true
}

// GenerateTraceID 生成符合 W3C Trace Context 规范的 TraceID
//
// 格式: 32位小写十六进制字符串 (128-bit)
// 示例: "0af7651916cd43dd8448eb211c80319c"
// 使用 crypto/rand 保证随机性，适用于分布式追踪场景。
//
// W3C 规范要求 trace-id 不能为全零，虽然概率极低（2^-128），但本函数
// 遵循规范进行检查，若出现全零则重新生成。
//
// Panic 策略说明：如果底层熵源不可用（极罕见的系统级错误），函数会 panic。
// 这是有意的设计选择，原因如下：
//  1. crypto/rand 失败意味着系统无法提供安全随机数，继续运行会导致安全隐患
//  2. 这与 OpenTelemetry 等标准库采用相同的策略
//  3. 此错误在正常运行环境中几乎不可能发生（需要内核级故障）
//  4. 服务在此状态下应立即终止，而非静默降级
func GenerateTraceID() string {
	var buf [TraceIDSize]byte
	for {
		if _, err := rand.Read(buf[:]); err != nil {
			panic("xctx: crypto/rand.Read failed: " + err.Error())
		}
		if !isAllZeros(buf[:]) {
			return hex.EncodeToString(buf[:])
		}
		// 全零情况极其罕见（概率 2^-128），重新生成
	}
}

// GenerateSpanID 生成符合 W3C Trace Context 规范的 SpanID
//
// 格式: 16位小写十六进制字符串 (64-bit)
// 示例: "b7ad6b7169203331"
// 使用 crypto/rand 保证随机性。
//
// W3C 规范要求 span-id 不能为全零，虽然概率极低（2^-64），但本函数
// 遵循规范进行检查，若出现全零则重新生成。
//
// Panic 策略：与 GenerateTraceID 相同，熵源不可用时会 panic。
// 详见 GenerateTraceID 的文档说明。
func GenerateSpanID() string {
	var buf [SpanIDSize]byte
	for {
		if _, err := rand.Read(buf[:]); err != nil {
			panic("xctx: crypto/rand.Read failed: " + err.Error())
		}
		if !isAllZeros(buf[:]) {
			return hex.EncodeToString(buf[:])
		}
		// 全零情况极其罕见（概率 2^-64），重新生成
	}
}

// GenerateRequestID 生成 RequestID
//
// 格式: 32位小写十六进制字符串（与 TraceID 格式一致）
// 示例: "550e8400e29b41d4a716446655440000"
//
// RequestID 不在 W3C 标准中，这里采用与 TraceID 相同的格式保持一致性。
func GenerateRequestID() string {
	return GenerateTraceID()
}

// =============================================================================
// Ensure 函数：自动补全模式（有则沿用，无则生成）
// 用于请求入口，使当前服务成为分布式链路追踪的起点
// =============================================================================

// EnsureTraceID 确保 context 中存在 TraceID。
//
// 语义：确保非空。如果 context 中已有 TraceID，原样返回（不验证/不纠正）；
// 否则自动生成新的并注入。
// 适用于 HTTP/gRPC 入口中间件，确保每个请求都有追踪标识。
// 如果 ctx 为 nil，返回 ErrNilContext。
func EnsureTraceID(ctx context.Context) (context.Context, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if TraceID(ctx) != "" {
		return ctx, nil
	}
	return WithTraceID(ctx, GenerateTraceID())
}

// EnsureSpanID 确保 context 中存在 SpanID。
//
// 语义：确保非空。如果 context 中已有 SpanID，原样返回（不验证/不纠正）；
// 否则自动生成新的并注入。
// 如果 ctx 为 nil，返回 ErrNilContext。
func EnsureSpanID(ctx context.Context) (context.Context, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if SpanID(ctx) != "" {
		return ctx, nil
	}
	return WithSpanID(ctx, GenerateSpanID())
}

// EnsureRequestID 确保 context 中存在 RequestID。
//
// 语义：确保非空。如果 context 中已有 RequestID，原样返回（不验证/不纠正）；
// 否则自动生成新的并注入。
// 如果 ctx 为 nil，返回 ErrNilContext。
func EnsureRequestID(ctx context.Context) (context.Context, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if RequestID(ctx) != "" {
		return ctx, nil
	}
	return WithRequestID(ctx, GenerateRequestID())
}

// EnsureTrace 确保 context 中存在所有追踪字段。
//
// 语义：确保非空。批量检查并补全 TraceID、SpanID、RequestID。
// 对于已存在的字段，原样保留（不验证/不纠正）；仅补全缺失的字段。
// 适用于请求入口，一次调用确保所有追踪信息就绪。
// 如果 ctx 为 nil，返回 ErrNilContext。
//
// 注意：此函数不处理 TraceFlags。这是有意的设计：
//   - TraceFlags 包含采样决策（如 "01" 表示已采样，"00" 表示未采样）
//   - 采样决策应从上游请求传播，不应由当前服务自动生成
//   - 如需设置 TraceFlags，请使用 WithTraceFlags 显式设置
//
// 如果需要同时确保 TraceFlags，请使用组合调用：
//
//	ctx, err = xctx.EnsureTrace(ctx)
//	if err != nil {
//	    return err
//	}
//	if xctx.TraceFlags(ctx) == "" {
//	    ctx, err = xctx.WithTraceFlags(ctx, "00") // 默认未采样
//	}
func EnsureTrace(ctx context.Context) (context.Context, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}

	// 一次性检查所有字段，避免重复 context.Value 查找
	hasTraceID := TraceID(ctx) != ""
	hasSpanID := SpanID(ctx) != ""
	hasRequestID := RequestID(ctx) != ""

	// 快速路径：所有字段都存在时直接返回
	if hasTraceID && hasSpanID && hasRequestID {
		return ctx, nil
	}

	// 设计决策: 构建仅含缺失字段的 Trace 后调用 WithTrace 批量注入。
	// WithTrace 内部通过 applyOptionalFields 跳过空值字段，不会覆盖已存在的值。
	// 相比逐个调用 EnsureXxx，减少了重复的 nil/存在性检查和函数调用开销。
	var trace Trace
	if !hasTraceID {
		trace.TraceID = GenerateTraceID()
	}
	if !hasSpanID {
		trace.SpanID = GenerateSpanID()
	}
	if !hasRequestID {
		trace.RequestID = GenerateRequestID()
	}

	return WithTrace(ctx, trace)
}

// =============================================================================
// Trace 结构体（批量获取模式）
// =============================================================================

// Trace 追踪信息结构体
//
// 用于批量获取追踪信息，替代多参数函数。
// TraceFlags 是 W3C Trace Context 的采样标志（如 "01" 表示已采样）。
type Trace struct {
	TraceID    string
	SpanID     string
	RequestID  string
	TraceFlags string
}

// GetTrace 从 context 批量获取所有追踪信息
//
// 返回 Trace 结构体，字段可能为空字符串。
// 使用 IsComplete() 检查是否全部存在。
func GetTrace(ctx context.Context) Trace {
	return Trace{
		TraceID:    TraceID(ctx),
		SpanID:     SpanID(ctx),
		RequestID:  RequestID(ctx),
		TraceFlags: TraceFlags(ctx),
	}
}

// Validate 校验 Trace 必填字段是否完整，缺失时返回对应的哨兵错误。
//
// 采用 fail-fast 策略：仅返回第一个缺失字段的错误（按 TraceID → SpanID → RequestID 顺序）。
// 如需一次性获取所有缺失字段，请逐字段调用 RequireXxx 或自行遍历检查。
//
// 与 IsComplete() 检查相同条件，区别在于返回类型：
//   - Validate() 返回 error，适用于中间件/业务层的错误处理链
//   - IsComplete() 返回 bool，适用于条件判断和日志记录
//
// 约束：
//   - TraceID 必须存在
//   - SpanID 必须存在
//   - RequestID 必须存在
//   - TraceFlags 不参与校验（可选的采样决策字段，由上游传播）
func (t Trace) Validate() error {
	if t.TraceID == "" {
		return ErrMissingTraceID
	}
	if t.SpanID == "" {
		return ErrMissingSpanID
	}
	if t.RequestID == "" {
		return ErrMissingRequestID
	}
	return nil
}

// IsComplete 检查追踪信息是否完整
//
// TraceID、SpanID、RequestID 三个核心字段都非空时返回 true。
// TraceFlags 不参与完整性检查，因为它是可选的采样决策字段，由上游传播。
func (t Trace) IsComplete() bool {
	return t.TraceID != "" && t.SpanID != "" && t.RequestID != ""
}

// WithTrace 将 Trace 结构体中的非空字段批量注入 context。
//
// 仅注入非空字段，空字符串字段会被跳过。
// 适用于从上游请求（如 HTTP Header、gRPC Metadata）解析追踪信息后一次性注入。
// 如果 ctx 为 nil，返回 ErrNilContext。
func WithTrace(ctx context.Context, tr Trace) (context.Context, error) {
	return applyOptionalFields(ctx, []contextFieldSetter{
		{value: tr.TraceID, set: WithTraceID},
		{value: tr.SpanID, set: WithSpanID},
		{value: tr.RequestID, set: WithRequestID},
		{value: tr.TraceFlags, set: WithTraceFlags},
	})
}
