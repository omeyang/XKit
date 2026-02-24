package xlimit

import (
	"context"
	"math"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/omeyang/xkit/pkg/context/xtenant"
)

// GRPCKeyExtractor 从 gRPC 请求中提取限流键
type GRPCKeyExtractor struct {
	tenantHeader      string
	callerHeader      string
	resourceExtractor func(context.Context, *grpc.UnaryServerInfo) string
}

// GRPCKeyExtractorOption gRPC 键提取器选项
type GRPCKeyExtractorOption func(*GRPCKeyExtractor)

// DefaultGRPCKeyExtractor 创建默认的 gRPC 键提取器
func DefaultGRPCKeyExtractor() *GRPCKeyExtractor {
	return &GRPCKeyExtractor{
		tenantHeader: xtenant.MetaTenantID,
		callerHeader: "x-caller-id",
	}
}

// NewGRPCKeyExtractor 创建自定义的 gRPC 键提取器
func NewGRPCKeyExtractor(opts ...GRPCKeyExtractorOption) *GRPCKeyExtractor {
	e := DefaultGRPCKeyExtractor()
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Extract 从 gRPC 上下文中提取限流键
func (e *GRPCKeyExtractor) Extract(ctx context.Context, info *grpc.UnaryServerInfo) Key {
	key := e.extractFromMetadata(ctx)

	// 从 info 提取 method
	if info != nil {
		key.Method = info.FullMethod
	}

	// 提取资源信息
	if e.resourceExtractor != nil {
		key.Resource = e.resourceExtractor(ctx, info)
	}

	return key
}

// ExtractStream 从 gRPC Stream 上下文中提取限流键
func (e *GRPCKeyExtractor) ExtractStream(ctx context.Context, info *grpc.StreamServerInfo) Key {
	key := e.extractFromMetadata(ctx)

	// 从 info 提取 method
	if info != nil {
		key.Method = info.FullMethod
	}

	return key
}

// extractFromMetadata 从 gRPC metadata 提取 tenant 和 caller
func (e *GRPCKeyExtractor) extractFromMetadata(ctx context.Context) Key {
	var key Key
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get(e.tenantHeader); len(values) > 0 {
			key.Tenant = values[0]
		}
		if values := md.Get(e.callerHeader); len(values) > 0 {
			key.Caller = values[0]
		}
	}
	return key
}

// WithGRPCTenantHeader 设置租户 ID 的 header 名称
func WithGRPCTenantHeader(header string) GRPCKeyExtractorOption {
	return func(e *GRPCKeyExtractor) {
		e.tenantHeader = header
	}
}

// WithGRPCCallerHeader 设置调用方 ID 的 header 名称
func WithGRPCCallerHeader(header string) GRPCKeyExtractorOption {
	return func(e *GRPCKeyExtractor) {
		e.callerHeader = header
	}
}

// WithGRPCResourceExtractor 设置资源提取器
func WithGRPCResourceExtractor(extractor func(context.Context, *grpc.UnaryServerInfo) string) GRPCKeyExtractorOption {
	return func(e *GRPCKeyExtractor) {
		e.resourceExtractor = extractor
	}
}

// GRPCInterceptorOptions gRPC 拦截器选项
type GRPCInterceptorOptions struct {
	KeyExtractor   *GRPCKeyExtractor
	SkipFunc       func(ctx context.Context, info *grpc.UnaryServerInfo) bool
	StreamSkipFunc func(ctx context.Context, info *grpc.StreamServerInfo) bool
}

// GRPCInterceptorOption gRPC 拦截器选项函数
type GRPCInterceptorOption func(*GRPCInterceptorOptions)

// defaultGRPCInterceptorOptions 返回默认的 gRPC 拦截器选项
func defaultGRPCInterceptorOptions() *GRPCInterceptorOptions {
	return &GRPCInterceptorOptions{
		KeyExtractor: DefaultGRPCKeyExtractor(),
	}
}

// WithGRPCKeyExtractor 设置 gRPC 键提取器
func WithGRPCKeyExtractor(extractor *GRPCKeyExtractor) GRPCInterceptorOption {
	return func(opts *GRPCInterceptorOptions) {
		opts.KeyExtractor = extractor
	}
}

// WithGRPCSkipFunc 设置 gRPC 跳过函数
func WithGRPCSkipFunc(skipFunc func(ctx context.Context, info *grpc.UnaryServerInfo) bool) GRPCInterceptorOption {
	return func(opts *GRPCInterceptorOptions) {
		opts.SkipFunc = skipFunc
	}
}

// WithGRPCStreamSkipFunc 设置 gRPC Stream 跳过函数
func WithGRPCStreamSkipFunc(skipFunc func(ctx context.Context, info *grpc.StreamServerInfo) bool) GRPCInterceptorOption {
	return func(opts *GRPCInterceptorOptions) {
		opts.StreamSkipFunc = skipFunc
	}
}

// UnaryServerInterceptor 创建 gRPC 一元服务端拦截器
//
// 示例:
//
//	limiter, _ := xlimit.New(redisClient, xlimit.WithRules(...))
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(xlimit.UnaryServerInterceptor(limiter)),
//	)
func UnaryServerInterceptor(limiter Limiter, opts ...GRPCInterceptorOption) grpc.UnaryServerInterceptor {
	// 设计决策: nil limiter 使用 panic（同 HTTPMiddleware，见 middleware_http.go 注释）。
	if limiter == nil {
		panic("xlimit: UnaryServerInterceptor requires a non-nil Limiter")
	}

	options := defaultGRPCInterceptorOptions()
	for _, opt := range opts {
		opt(options)
	}
	if options.KeyExtractor == nil {
		options.KeyExtractor = DefaultGRPCKeyExtractor()
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// 检查是否跳过
		if options.SkipFunc != nil && options.SkipFunc(ctx, info) {
			return handler(ctx, req)
		}

		// 提取限流键
		key := options.KeyExtractor.Extract(ctx, info)

		// 执行限流检查
		result, err := limiter.Allow(ctx, key)
		if err != nil {
			// 设计决策: 优先检查 result 是否携带拒绝信息（如 FallbackClose 策略
			// 返回 Allowed=false + ErrRedisUnavailable）。仅当 result 为空时
			// 才 fail-open（限流器内部错误不阻塞业务请求）。
			if result != nil && !result.Allowed {
				return nil, grpcRateLimitError(ctx, result)
			}
			return handler(ctx, req)
		}

		// 检查是否被限流
		if !result.Allowed {
			return nil, grpcRateLimitError(ctx, result)
		}

		return handler(ctx, req)
	}
}

// StreamServerInterceptor 创建 gRPC 流式服务端拦截器
//
// 示例:
//
//	limiter, _ := xlimit.New(redisClient, xlimit.WithRules(...))
//	server := grpc.NewServer(
//	    grpc.StreamInterceptor(xlimit.StreamServerInterceptor(limiter)),
//	)
func StreamServerInterceptor(limiter Limiter, opts ...GRPCInterceptorOption) grpc.StreamServerInterceptor {
	// 设计决策: nil limiter 使用 panic（同 HTTPMiddleware，见 middleware_http.go 注释）。
	if limiter == nil {
		panic("xlimit: StreamServerInterceptor requires a non-nil Limiter")
	}

	options := defaultGRPCInterceptorOptions()
	for _, opt := range opts {
		opt(options)
	}
	if options.KeyExtractor == nil {
		options.KeyExtractor = DefaultGRPCKeyExtractor()
	}

	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := stream.Context()

		// 检查是否跳过
		if options.StreamSkipFunc != nil && options.StreamSkipFunc(ctx, info) {
			return handler(srv, stream)
		}

		// 提取限流键
		key := options.KeyExtractor.ExtractStream(ctx, info)

		// 执行限流检查
		result, err := limiter.Allow(ctx, key)
		if err != nil {
			// 设计决策: 同 UnaryServerInterceptor，优先检查 result 拒绝信息。
			if result != nil && !result.Allowed {
				return grpcRateLimitError(ctx, result)
			}
			return handler(srv, stream)
		}

		// 检查是否被限流
		if !result.Allowed {
			return grpcRateLimitError(ctx, result)
		}

		return handler(srv, stream)
	}
}

// grpcRateLimitError 创建 gRPC 限流错误并设置 Retry-After trailer metadata。
// SetTrailer 失败仅表示 transport 不可用，此时 error 也无法投递，不影响限流语义。
func grpcRateLimitError(ctx context.Context, result *Result) error {
	setRetryAfterTrailer(ctx, result)
	return status.Errorf(codes.ResourceExhausted,
		"rate limit exceeded: limit=%d, retry_after=%v",
		result.Limit, result.RetryAfter)
}

// setRetryAfterTrailer 尽力设置 Retry-After trailer metadata
func setRetryAfterTrailer(ctx context.Context, result *Result) {
	if result.RetryAfter <= 0 {
		return
	}
	retryAfterSec := int64(math.Ceil(result.RetryAfter.Seconds()))
	if err := grpc.SetTrailer(ctx, metadata.Pairs("retry-after",
		strconv.FormatInt(retryAfterSec, 10))); err != nil {
		return // transport 不可用时无法设置 trailer，继续返回限流错误
	}
}
