// Package xtenant 提供租户信息的提取和传播功能。
//
// # 核心理念
//
// xtenant 专注于请求级别的租户信息管理：
//
//   - TenantID: 租户 ID（每个请求携带）
//   - TenantName: 租户名称（每个请求携带）
//
// 服务级别的平台信息（PlatformID、HasParent 等）由 xplatform 包管理，
// xtenant 在跨服务传播时会自动包含这些信息。
//
// # 快速开始
//
// HTTP 服务使用 HTTPMiddleware() 中间件，gRPC 服务使用 GRPCUnaryServerInterceptor() 拦截器。
// 在业务代码中通过 TenantID(ctx) 和 TenantName(ctx) 获取租户信息。
// 跨服务调用时使用 InjectToRequest(ctx, req) 或 InjectToOutgoingContext(ctx) 传播。
//
// # 传输协议
//
// HTTP Header 约定（遵循 X- 前缀）：
//
// 租户和平台信息：
//   - X-Platform-ID: 平台 ID（来自 xplatform）
//   - X-Tenant-ID: 租户 ID
//   - X-Tenant-Name: 租户名称
//   - X-Has-Parent: 是否有上级平台（来自 xplatform）
//   - X-Unclass-Region-ID: 未分类区域 ID（来自 xplatform）
//
// 追踪信息（来自 xctx）：
//   - X-Trace-ID: 追踪标识（W3C 规范，128-bit）
//   - X-Span-ID: 跨度标识（W3C 规范，64-bit）
//   - X-Request-ID: 请求标识
//   - X-Trace-Flags: 追踪标志（W3C 规范，采样决策）
//
// gRPC Metadata Key 约定（小写连字符）：
//
// 租户和平台信息：
//   - x-platform-id
//   - x-tenant-id
//   - x-tenant-name
//   - x-has-parent
//   - x-unclass-region-id
//
// 追踪信息：
//   - x-trace-id
//   - x-span-id
//   - x-request-id
//   - x-trace-flags
//
// # 跨服务传播
//
// HTTP 客户端使用 InjectToRequest()，gRPC 客户端使用 InjectToOutgoingContext()
// 或客户端拦截器 GRPCUnaryClientInterceptor()。
//
// # 与 xplatform、xctx 的关系
//
//   - xplatform: 管理进程级别的平台信息（PlatformID、HasParent、UnclassRegionID）
//   - xctx: 提供纯粹的 context 操作（底层实现）
//   - xtenant: 提供租户信息的提取和传播（本包）
//
// xtenant 在内部使用 xctx 进行 context 操作，在传播时从 xplatform 获取平台信息。
//
// # 中间件选项
//
// 租户验证选项（互斥，后设置的选项生效）：
//
//   - WithRequireTenant() / WithGRPCRequireTenant():
//     要求 TenantID 和 TenantName 都存在，缺失时返回 400/InvalidArgument
//
//   - WithRequireTenantID() / WithGRPCRequireTenantID():
//     只要求 TenantID 存在，TenantName 不做强制要求
//
// 追踪处理选项：
//
//   - WithEnsureTrace() / WithGRPCEnsureTrace():
//     自动生成缺失的追踪字段（TraceID/SpanID/RequestID），
//     使当前服务成为分布式链路追踪的起点
//
//   - 默认行为：仅传播上游已有的追踪字段，不自动生成
//
// 使用示例：
//
//	// 网关服务：自动生成追踪信息
//	xtenant.HTTPMiddlewareWithOptions(
//	    xtenant.WithEnsureTrace(),
//	)
//
//	// 下游服务：只传播追踪信息（默认行为）
//	xtenant.HTTPMiddleware()
//
//	// 要求租户信息
//	xtenant.HTTPMiddlewareWithOptions(
//	    xtenant.WithRequireTenantID(),
//	    xtenant.WithEnsureTrace(),
//	)
//
// # 线程安全
//
// 所有导出函数都是线程安全的：
//
//   - Context 操作函数可并发调用
//   - HTTP/gRPC 中间件可并发处理请求
package xtenant
