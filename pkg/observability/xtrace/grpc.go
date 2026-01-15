package xtrace

import (
	"context"
	"strings"

	"github.com/omeyang/xkit/pkg/context/xctx"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// =============================================================================
// gRPC Metadata 常量
// =============================================================================

// Metadata Key 名称（遵循小写加连字符的 gRPC 惯例）
const (
	// 自定义 Metadata Key
	MetaTraceID   = "x-trace-id"
	MetaSpanID    = "x-span-id"
	MetaRequestID = "x-request-id"

	// W3C Trace Context 标准 Key
	MetaTraceparent = "traceparent"
	MetaTracestate  = "tracestate"
)

// =============================================================================
// gRPC Metadata 提取
// =============================================================================

// ExtractFromMetadata 从 gRPC Metadata 提取追踪信息
//
// 提取以下 Key：
//   - x-trace-id -> TraceID
//   - x-span-id -> SpanID
//   - x-request-id -> RequestID
//   - traceparent -> Traceparent (W3C)
//   - tracestate -> Tracestate (W3C)
//
// 如果存在 traceparent，会自动解析出 TraceID 和 SpanID。
func ExtractFromMetadata(md metadata.MD) TraceInfo {
	if md == nil {
		return TraceInfo{}
	}

	info := TraceInfo{
		TraceID:     getMetadataValue(md, MetaTraceID),
		SpanID:      getMetadataValue(md, MetaSpanID),
		RequestID:   getMetadataValue(md, MetaRequestID),
		Traceparent: getMetadataValue(md, MetaTraceparent),
		Tracestate:  getMetadataValue(md, MetaTracestate),
	}

	// 如果有 traceparent，解析出 TraceID、SpanID 和 TraceFlags
	// W3C traceparent 优先级最高，直接覆盖自定义 metadata 的值
	if info.Traceparent != "" {
		if traceID, spanID, traceFlags, ok := parseTraceparent(info.Traceparent); ok {
			info.TraceID = traceID
			info.SpanID = spanID
			info.TraceFlags = traceFlags
		}
	}

	return info
}

// ExtractFromIncomingContext 从 incoming context 提取追踪信息
func ExtractFromIncomingContext(ctx context.Context) TraceInfo {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return TraceInfo{}
	}
	return ExtractFromMetadata(md)
}

// =============================================================================
// gRPC 服务端拦截器
// =============================================================================

// GRPCUnaryServerInterceptor 返回 gRPC 一元服务端拦截器。
// 自动从 gRPC Metadata 提取追踪信息并注入 context，缺失时自动生成。
func GRPCUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return GRPCUnaryServerInterceptorWithOptions()
}

// GRPCInterceptorOption gRPC 拦截器选项
type GRPCInterceptorOption func(*grpcInterceptorConfig)

type grpcInterceptorConfig struct {
	autoGenerate bool
}

// WithGRPCAutoGenerate 设置是否自动生成缺失的追踪 ID
func WithGRPCAutoGenerate(enabled bool) GRPCInterceptorOption {
	return func(cfg *grpcInterceptorConfig) {
		cfg.autoGenerate = enabled
	}
}

// GRPCUnaryServerInterceptorWithOptions 返回带选项的 gRPC 一元服务端拦截器
func GRPCUnaryServerInterceptorWithOptions(opts ...GRPCInterceptorOption) grpc.UnaryServerInterceptor {
	cfg := &grpcInterceptorConfig{
		autoGenerate: true,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		// 提取追踪信息
		traceInfo := ExtractFromIncomingContext(ctx)

		// 注入到 context
		ctx = injectTraceToContext(ctx, traceInfo, cfg.autoGenerate)

		return handler(ctx, req)
	}
}

// GRPCStreamServerInterceptor 返回 gRPC 流式服务端拦截器。
// 自动从 gRPC Metadata 提取追踪信息并注入 context。
func GRPCStreamServerInterceptor() grpc.StreamServerInterceptor {
	return GRPCStreamServerInterceptorWithOptions()
}

// GRPCStreamServerInterceptorWithOptions 返回带选项的 gRPC 流式服务端拦截器
func GRPCStreamServerInterceptorWithOptions(opts ...GRPCInterceptorOption) grpc.StreamServerInterceptor {
	cfg := &grpcInterceptorConfig{
		autoGenerate: true,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// 提取追踪信息
		traceInfo := ExtractFromIncomingContext(ss.Context())

		// 注入到 context
		ctx := injectTraceToContext(ss.Context(), traceInfo, cfg.autoGenerate)

		return handler(srv, &wrappedServerStream{ServerStream: ss, ctx: ctx})
	}
}

// wrappedServerStream 包装 ServerStream 以覆盖 Context
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context 返回包装后的 context
func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

// =============================================================================
// gRPC 客户端拦截器
// =============================================================================

// GRPCUnaryClientInterceptor 返回 gRPC 客户端一元拦截器。
// 自动将追踪信息注入 outgoing context，用于跨服务调用传播。
func GRPCUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		ctx = InjectToOutgoingContext(ctx)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// GRPCStreamClientInterceptor 返回 gRPC 客户端流式拦截器。
// 自动将追踪信息注入 outgoing context，用于跨服务调用传播。
func GRPCStreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx = InjectToOutgoingContext(ctx)
		return streamer(ctx, desc, cc, method, opts...)
	}
}

// =============================================================================
// gRPC Metadata 注入（跨服务传播）
// =============================================================================

// InjectToOutgoingContext 将追踪信息注入 outgoing context。
// 从 context 提取追踪信息并设置到 outgoing metadata，用于跨服务调用时传播。
// 会正确传递上游的 trace-flags（采样决策）。
func InjectToOutgoingContext(ctx context.Context) context.Context {
	traceID := xctx.TraceID(ctx)
	spanID := xctx.SpanID(ctx)
	requestID := xctx.RequestID(ctx)
	traceFlags := xctx.TraceFlags(ctx)

	// 如果没有任何信息，直接返回
	if traceID == "" && spanID == "" && requestID == "" {
		return ctx
	}

	// 获取现有 metadata 并复制，避免修改原 metadata
	md, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = md.Copy()
	} else {
		md = metadata.New(nil)
	}

	// 使用 Set 覆盖（而非追加），避免多次调用产生重复值
	if traceID != "" {
		md.Set(MetaTraceID, traceID)
	}
	if spanID != "" {
		md.Set(MetaSpanID, spanID)
	}
	if requestID != "" {
		md.Set(MetaRequestID, requestID)
	}

	// 生成 W3C traceparent（仅在 traceID 和 spanID 都有效时）
	// 使用 context 中的 traceFlags，若无则默认 "00"
	if traceparent := formatTraceparent(traceID, spanID, traceFlags); traceparent != "" {
		md.Set(MetaTraceparent, traceparent)
	}

	return metadata.NewOutgoingContext(ctx, md)
}

// InjectTraceToMetadata 将 TraceInfo 注入 Metadata
//
// 用于手动构造 Metadata 的场景。
// 如果 TraceInfo.Traceparent 为空但有有效的 TraceID 和 SpanID，
// 会自动生成 traceparent（使用 TraceFlags，若为空则默认 "00"）。
func InjectTraceToMetadata(md metadata.MD, info TraceInfo) {
	if md == nil {
		return
	}

	if info.TraceID != "" {
		md.Set(MetaTraceID, info.TraceID)
	}
	if info.SpanID != "" {
		md.Set(MetaSpanID, info.SpanID)
	}
	if info.RequestID != "" {
		md.Set(MetaRequestID, info.RequestID)
	}

	// 如果已有 Traceparent，验证后再透传，避免传播无效 traceparent
	if info.Traceparent != "" {
		if _, _, _, ok := parseTraceparent(info.Traceparent); ok {
			md.Set(MetaTraceparent, info.Traceparent)
		}
		// 无效时静默丢弃，尝试从 TraceID/SpanID 生成
	}

	// 如果没有设置有效的 Traceparent，尝试从 TraceID/SpanID 生成
	if len(md.Get(MetaTraceparent)) == 0 {
		if traceparent := formatTraceparent(info.TraceID, info.SpanID, info.TraceFlags); traceparent != "" {
			md.Set(MetaTraceparent, traceparent)
		}
	}

	if info.Tracestate != "" {
		md.Set(MetaTracestate, info.Tracestate)
	}
}

// =============================================================================
// 内部辅助函数
// =============================================================================

// getMetadataValue 获取 metadata 中的值（取第一个，去除空白）
func getMetadataValue(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}
