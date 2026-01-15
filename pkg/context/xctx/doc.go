// Package xctx 提供轻量级的请求上下文管理。
//
// 整合身份信息（identity）、追踪信息（trace）和部署类型（deployment）的 context 存取能力，
// 并为日志系统提供属性提取功能。
//
// # 核心功能
//
// 身份信息（Identity）- 标识请求来源：
//   - platform_id  : 平台标识
//   - tenant_id    : 租户标识
//   - tenant_name  : 租户名称
//
// 平台信息（Platform）- 平台层级关系：
//   - has_parent       : 是否有上级平台（SaaS 多级部署场景）
//   - unclass_region_id: 未分类区域 ID
//
// 追踪信息（Trace）- 分布式追踪：
//   - trace_id     : 追踪标识（W3C 规范，128-bit）
//   - span_id      : 跨度标识（W3C 规范，64-bit）
//   - request_id   : 请求标识
//   - trace_flags  : 追踪标志（W3C 规范，采样决策）
//
// 部署类型（Deployment）- 运行环境：
//   - LOCAL : 本地/私有化部署
//   - SAAS  : SaaS 云部署
//
// # 快速开始
//
//	// 注入身份和追踪信息
//	ctx, _ := xctx.WithPlatformID(ctx, "platform-001")
//	ctx, _ = xctx.WithTenantID(ctx, "tenant-002")
//	ctx, _ = xctx.EnsureTrace(ctx) // 自动生成 TraceID、SpanID、RequestID
//
//	// 提取日志属性
//	attrs, _ := xctx.LogAttrs(ctx)
//	slog.LogAttrs(ctx, slog.LevelInfo, "request started", attrs...)
//
// # 命名约定
//
//	WithXxx(ctx, value)    - 注入：将 value 写入 context
//	Xxx(ctx)               - 读取：从 context 读取值，缺失时返回零值
//	RequireXxx(ctx)        - 强制读取：值必须存在，缺失时返回错误
//	EnsureXxx(ctx)         - 确保存在：若已存在则返回，否则自动生成
//	GetXxx(ctx)            - 批量读取：返回结构体
//
// # 哨兵错误
//
//	ErrNilContext            - context 为 nil
//	ErrMissingPlatformID     - platform_id 缺失
//	ErrMissingTenantID       - tenant_id 缺失
//	ErrMissingTenantName     - tenant_name 缺失
//	ErrMissingHasParent      - has_parent 缺失
//	ErrMissingDeploymentType - deployment_type 缺失
//	ErrInvalidDeploymentType - deployment_type 非法
package xctx
