package xtrace

import (
	"context"
	"strings"

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
//
// 设计决策: xtrace 只做传输层适配（提取/注入追踪标识），不创建 OTel Span。
// Span 生命周期管理由 OTel SDK 的 otelgrpc 拦截器负责。
func GRPCUnaryServerInterceptor(opts ...Option) grpc.UnaryServerInterceptor {
	cfg := applyOptions(opts)

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
func GRPCStreamServerInterceptor(opts ...Option) grpc.StreamServerInterceptor {
	cfg := applyOptions(opts)

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// 缓存 context（与 HTTP 中间件的 ctx := r.Context() 写法保持一致）
		ctx := ss.Context()

		// 提取追踪信息
		traceInfo := ExtractFromIncomingContext(ctx)

		// 注入到 context
		ctx = injectTraceToContext(ctx, traceInfo, cfg.autoGenerate)

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

// grpcTransportKeys gRPC 传输层使用的 key 名称。
//
// 设计决策: 提取为包级变量，供 InjectToOutgoingContext 和 InjectTraceToMetadata 共享，
// 确保两条注入路径使用相同的 traceparent 生成逻辑（resolveTraceparent）。
var grpcTransportKeys = transportKeys{
	traceID:     MetaTraceID,
	spanID:      MetaSpanID,
	requestID:   MetaRequestID,
	traceparent: MetaTraceparent,
	tracestate:  MetaTracestate,
}

// InjectToOutgoingContext 将追踪信息注入 outgoing context。
// 从 context 提取追踪信息并设置到 outgoing metadata，用于跨服务调用时传播。
// 会正确传递上游的 trace-flags（采样决策）。
//
// 注意：本函数不传播 tracestate（因为 context 中不存储 tracestate）。
// 如需传播 tracestate，请使用 InjectTraceToMetadata 手动设置，或使用 OpenTelemetry SDK。
func InjectToOutgoingContext(ctx context.Context) context.Context {
	info := TraceInfoFromContext(ctx)

	// 如果没有任何追踪信息，直接返回
	if info.IsEmpty() {
		return ctx
	}

	// 获取现有 metadata 并复制，避免修改原 metadata
	md, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = md.Copy()
	} else {
		md = metadata.New(nil)
	}

	injectTraceInfoTo(func(k, v string) { md.Set(k, v) }, info, grpcTransportKeys)

	return metadata.NewOutgoingContext(ctx, md)
}

// InjectTraceToMetadata 将 TraceInfo 注入 Metadata
//
// 用于手动构造 Metadata 的场景。
// 如果 TraceInfo.Traceparent 为空但有有效的 TraceID 和 SpanID，
// 会自动生成 traceparent（使用 TraceFlags，若为空则默认 "00"）。
//
// 注意：如果同时设置了 TraceID 和 Traceparent，请确保两者一致以避免下游混淆。
func InjectTraceToMetadata(md metadata.MD, info TraceInfo) {
	if md == nil {
		return
	}
	injectTraceInfoTo(func(k, v string) { md.Set(k, v) }, info, grpcTransportKeys)
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
