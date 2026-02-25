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
// # 包命名说明
//
// 虽然本包也传播平台和追踪信息，但核心职责是租户信息的提取和传播，
// 平台信息来自 xplatform（进程级），追踪信息来自 xctx（底层存储），
// 它们只是在传播时一并携带。因此包名以核心职责 tenant 命名。
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
//   - X-Tenant-ID: 租户 ID（单值字段，多值时取第一个）
//   - X-Tenant-Name: 租户名称（单值字段，多值时取第一个）
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
// 出站传播使用"以 context 为准"的语义：有值则 Set，无值则删除已有的键。
// 这防止了请求对象或 metadata 复用时旧租户信息泄漏到下游。
//
// 平台信息的传播是单向的：出站时从 xplatform 注入到请求/metadata，
// 入站时不从请求/metadata 提取平台信息到 context。
// 这是因为平台信息是进程级配置（由 xplatform.Init 设置），
// 不应被上游传入的值覆盖。
//
// # 信任边界与中间件链顺序
//
// 中间件默认信任传输层（Header/Metadata）携带的租户信息。
// 推荐的中间件/拦截器链顺序：
//
//  1. 认证/鉴权中间件（验证请求来源、校验 Token）
//  2. xtenant 中间件（从已认证的请求中提取租户信息）
//  3. 业务 Handler
//
// 如果认证层需要设置可信租户，应在 xtenant 中间件之后通过
// WithTenantID/WithTenantName 覆盖传输层的值。
//
// 本包不内置"冲突拒绝"机制，租户来源的校验由认证层负责。
//
// 路由级跳过（如健康检查端点）可通过在中间件外层包装判断逻辑实现：
//
//	tenantMw := xtenant.HTTPMiddlewareWithOptions(xtenant.WithRequireTenantID())
//	handler := tenantMw(bizHandler)
//	mux.Handle("/api/", handler)          // 需要租户校验
//	mux.Handle("/healthz", bizHandler)    // 跳过租户校验
//
// # 与 xplatform、xctx 的关系
//
//   - xplatform: 管理进程级别的平台信息（PlatformID、HasParent、UnclassRegionID）
//   - xctx: 提供纯粹的 context 操作（底层实现）
//   - xtenant: 提供租户信息的提取和传播（本包）
//
// xtenant 在内部使用 xctx 进行 context 操作，在传播时从 xplatform 获取平台信息。
// 本包提供的 TenantID()、WithTenantID() 等函数是 xctx 同名函数的便捷包装，
// 使用方只需导入 xtenant 即可完成所有租户相关操作。
//
// # gRPC-Gateway 集成
//
// 使用 gRPC-Gateway 时，需配置自定义 HeaderMatcher 以转发租户相关的 X-* 头。
// 注意：key 参数是 HTTP 规范化后的形式（http.CanonicalHeaderKey），
// 例如 "X-Request-ID" 会被规范化为 "X-Request-Id"，
// 因此精确比较必须使用 strings.EqualFold 而非 ==。
//
//	gwmux := runtime.NewServeMux(
//	    runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
//	        if strings.HasPrefix(key, "X-Tenant-") || strings.HasPrefix(key, "X-Trace-") ||
//	            strings.HasPrefix(key, "X-Platform-") || strings.EqualFold(key, "X-Has-Parent") ||
//	            strings.EqualFold(key, "X-Unclass-Region-ID") || strings.EqualFold(key, "X-Request-ID") {
//	            return key, true
//	        }
//	        return runtime.DefaultHeaderMatcher(key)
//	    }),
//	)
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
// # 线程安全
//
// 所有导出函数都是线程安全的：
//
//   - Context 操作函数可并发调用
//   - HTTP/gRPC 中间件可并发处理请求
package xtenant
