package xtenant

import (
	"context"
	"strings"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/context/xplatform"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// =============================================================================
// gRPC Metadata 常量
// =============================================================================

// Metadata Key 名称（遵循小写加连字符的 gRPC 惯例）
const (
	MetaPlatformID      = "x-platform-id"
	MetaTenantID        = "x-tenant-id"
	MetaTenantName      = "x-tenant-name"
	MetaHasParent       = "x-has-parent"
	MetaUnclassRegionID = "x-unclass-region-id"

	// Trace 相关 Metadata Key
	MetaTraceID    = "x-trace-id"
	MetaSpanID     = "x-span-id"
	MetaRequestID  = "x-request-id"
	MetaTraceFlags = "x-trace-flags"
)

// =============================================================================
// gRPC Metadata 提取
// =============================================================================

// ExtractFromMetadata 从 gRPC Metadata 提取租户信息
//
// 提取以下 Key：
//   - x-tenant-id -> TenantID
//   - x-tenant-name -> TenantName
//
// 所有字段都是可选的，未设置的字段保持零值。
// Metadata 值会自动去除首尾空白。
func ExtractFromMetadata(md metadata.MD) TenantInfo {
	if md == nil {
		return TenantInfo{}
	}

	return TenantInfo{
		TenantID:   getMetadataValue(md, MetaTenantID),
		TenantName: getMetadataValue(md, MetaTenantName),
	}
}

// ExtractTraceFromMetadata 从 gRPC Metadata 提取追踪信息
//
// 提取以下 Key：
//   - x-trace-id -> TraceID
//   - x-span-id -> SpanID
//   - x-request-id -> RequestID
//   - x-trace-flags -> TraceFlags
//
// 所有字段都是可选的，未设置的字段保持零值。
func ExtractTraceFromMetadata(md metadata.MD) xctx.Trace {
	if md == nil {
		return xctx.Trace{}
	}

	return xctx.Trace{
		TraceID:    getMetadataValue(md, MetaTraceID),
		SpanID:     getMetadataValue(md, MetaSpanID),
		RequestID:  getMetadataValue(md, MetaRequestID),
		TraceFlags: getMetadataValue(md, MetaTraceFlags),
	}
}

// ExtractFromIncomingContext 从 incoming context 提取租户信息
//
// 等价于从 metadata.FromIncomingContext 获取 metadata 后调用 ExtractFromMetadata。
// 如果 ctx 为 nil，返回空 TenantInfo。
func ExtractFromIncomingContext(ctx context.Context) TenantInfo {
	if ctx == nil {
		return TenantInfo{}
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return TenantInfo{}
	}
	return ExtractFromMetadata(md)
}

// ExtractTraceFromIncomingContext 从 incoming context 提取追踪信息
//
// 等价于从 metadata.FromIncomingContext 获取 metadata 后调用 ExtractTraceFromMetadata。
// 如果 ctx 为 nil，返回空 Trace。
func ExtractTraceFromIncomingContext(ctx context.Context) xctx.Trace {
	if ctx == nil {
		return xctx.Trace{}
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return xctx.Trace{}
	}
	return ExtractTraceFromMetadata(md)
}

// =============================================================================
// gRPC 拦截器
// =============================================================================

// GRPCUnaryServerInterceptor 返回 gRPC 一元拦截器。
// 自动从 gRPC Metadata 提取租户信息并注入 context。
func GRPCUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return GRPCUnaryServerInterceptorWithOptions()
}

// GRPCInterceptorOption gRPC 拦截器选项
type GRPCInterceptorOption func(*grpcInterceptorConfig)

type grpcInterceptorConfig struct {
	requireTenant   bool
	requireTenantID bool
	ensureTrace     bool
}

// WithGRPCRequireTenant 设置是否要求租户信息必须存在
//
// 如果设置为 true，当 TenantID 或 TenantName 缺失时返回 InvalidArgument 错误。
// 默认为 false（不强制要求）。
//
// 与 WithGRPCRequireTenantID 互斥，后设置的选项生效。
func WithGRPCRequireTenant() GRPCInterceptorOption {
	return func(cfg *grpcInterceptorConfig) {
		cfg.requireTenant = true
		cfg.requireTenantID = false
	}
}

// WithGRPCRequireTenantID 设置只要求 TenantID 必须存在
//
// 如果设置为 true，当 TenantID 缺失时返回 InvalidArgument 错误，TenantName 不做要求。
// 适用于 TenantName 非必填的场景。
// 默认为 false（不强制要求）。
//
// 与 WithGRPCRequireTenant 互斥，后设置的选项生效。
func WithGRPCRequireTenantID() GRPCInterceptorOption {
	return func(cfg *grpcInterceptorConfig) {
		cfg.requireTenantID = true
		cfg.requireTenant = false
	}
}

// WithGRPCEnsureTrace 启用自动生成追踪信息
//
// 当上游未传递 trace metadata 时，自动生成新的 TraceID、SpanID、RequestID。
// 使当前服务成为分布式链路追踪的起点。
// 默认为 false（仅传播上游已有的追踪信息）。
//
// 典型场景：
//   - 网关服务：启用此选项，确保每个请求都有追踪信息
//   - 下游服务：不启用，只传播上游的追踪信息
func WithGRPCEnsureTrace() GRPCInterceptorOption {
	return func(cfg *grpcInterceptorConfig) {
		cfg.ensureTrace = true
	}
}

// GRPCUnaryServerInterceptorWithOptions 返回带选项的 gRPC 一元拦截器。
func GRPCUnaryServerInterceptorWithOptions(opts ...GRPCInterceptorOption) grpc.UnaryServerInterceptor {
	cfg := &grpcInterceptorConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		ctx, err := injectTenantToContext(ctx, cfg)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// GRPCStreamServerInterceptor 返回 gRPC 流式拦截器。
// 自动从 gRPC Metadata 提取租户信息并注入 context。
func GRPCStreamServerInterceptor() grpc.StreamServerInterceptor {
	return GRPCStreamServerInterceptorWithOptions()
}

// GRPCStreamServerInterceptorWithOptions 返回带选项的 gRPC 流式拦截器
func GRPCStreamServerInterceptorWithOptions(opts ...GRPCInterceptorOption) grpc.StreamServerInterceptor {
	cfg := &grpcInterceptorConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx, err := injectTenantToContext(ss.Context(), cfg)
		if err != nil {
			return err
		}
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
// gRPC Metadata 注入（跨服务传播）
// =============================================================================

// InjectToOutgoingContext 将租户信息注入 outgoing context。
// 从 context 提取租户信息并设置到 outgoing metadata，用于跨服务调用时传播租户信息。
// 同时也会注入服务级的平台信息（从 xplatform 获取）。
// 使用 Set 语义覆盖已存在的同名 key，防止租户信息串联（tenant leakage）。
func InjectToOutgoingContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	// 获取已有的 outgoing metadata（如果存在）
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.MD{}
	} else {
		// 复制一份，避免修改原始 metadata
		md = md.Copy()
	}

	injectPlatformMetadata(md)
	injectTenantMetadata(ctx, md)
	injectTraceMetadata(ctx, md)

	if len(md) == 0 {
		return ctx
	}

	return metadata.NewOutgoingContext(ctx, md)
}

// injectPlatformMetadata 注入服务级平台信息
func injectPlatformMetadata(md metadata.MD) {
	if !xplatform.IsInitialized() {
		return
	}
	if pid := xplatform.PlatformID(); pid != "" {
		md.Set(MetaPlatformID, pid)
	}
	if xplatform.HasParent() {
		md.Set(MetaHasParent, "true")
	} else {
		md.Set(MetaHasParent, "false")
	}
	if regionID := xplatform.UnclassRegionID(); regionID != "" {
		md.Set(MetaUnclassRegionID, regionID)
	}
}

// injectTenantMetadata 注入请求级租户信息
func injectTenantMetadata(ctx context.Context, md metadata.MD) {
	if tid := TenantID(ctx); tid != "" {
		md.Set(MetaTenantID, tid)
	}
	if tname := TenantName(ctx); tname != "" {
		md.Set(MetaTenantName, tname)
	}
}

// injectTraceMetadata 注入追踪信息
func injectTraceMetadata(ctx context.Context, md metadata.MD) {
	if tid := xctx.TraceID(ctx); tid != "" {
		md.Set(MetaTraceID, tid)
	}
	if sid := xctx.SpanID(ctx); sid != "" {
		md.Set(MetaSpanID, sid)
	}
	if rid := xctx.RequestID(ctx); rid != "" {
		md.Set(MetaRequestID, rid)
	}
	if flags := xctx.TraceFlags(ctx); flags != "" {
		md.Set(MetaTraceFlags, flags)
	}
}

// InjectTenantToMetadata 将 TenantInfo 注入 Metadata
//
// 用于手动构造 Metadata 的场景。
func InjectTenantToMetadata(md metadata.MD, info TenantInfo) {
	if md == nil {
		return
	}

	if info.TenantID != "" {
		md.Set(MetaTenantID, info.TenantID)
	}
	if info.TenantName != "" {
		md.Set(MetaTenantName, info.TenantName)
	}
}

// =============================================================================
// gRPC 客户端拦截器
// =============================================================================

// GRPCUnaryClientInterceptor 返回 gRPC 客户端一元拦截器。
// 自动将租户信息注入 outgoing context，用于跨服务调用传播。
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
// 自动将租户信息注入 outgoing context，用于跨服务调用传播。
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

// injectTenantToContext 从 incoming context 提取租户信息和追踪信息并注入
func injectTenantToContext(ctx context.Context, cfg *grpcInterceptorConfig) (context.Context, error) {
	// 提取并验证租户信息
	info := ExtractFromIncomingContext(ctx)
	if err := validateGRPCTenantInfo(info, cfg); err != nil {
		return nil, err
	}

	// 注入租户信息到 context
	ctx, err := injectGRPCTenantInfoToContext(ctx, info)
	if err != nil {
		return nil, err
	}

	// 处理追踪信息
	trace := ExtractTraceFromIncomingContext(ctx)
	ctx, err = injectGRPCTraceToContext(ctx, trace, cfg.ensureTrace)
	if err != nil {
		return nil, err
	}

	return ctx, nil
}

// validateGRPCTenantInfo 验证租户信息
func validateGRPCTenantInfo(info TenantInfo, cfg *grpcInterceptorConfig) error {
	if cfg.requireTenant {
		if err := info.Validate(); err != nil {
			return status.Error(codes.InvalidArgument, err.Error())
		}
	} else if cfg.requireTenantID {
		if info.TenantID == "" {
			return status.Error(codes.InvalidArgument, ErrEmptyTenantID.Error())
		}
	}
	return nil
}

// injectGRPCTenantInfoToContext 将租户信息注入 context
func injectGRPCTenantInfoToContext(ctx context.Context, info TenantInfo) (context.Context, error) {
	var err error
	if info.TenantID != "" {
		ctx, err = xctx.WithTenantID(ctx, info.TenantID)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	if info.TenantName != "" {
		ctx, err = xctx.WithTenantName(ctx, info.TenantName)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	return ctx, nil
}

// injectGRPCTraceToContext 处理追踪信息并注入 context
func injectGRPCTraceToContext(ctx context.Context, trace xctx.Trace, ensureTrace bool) (context.Context, error) {
	ctx, err := xctx.WithTrace(ctx, trace)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if ensureTrace {
		ctx, err = xctx.EnsureTrace(ctx)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	return ctx, nil
}
