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
//   - trace_flags  : 追踪标志（W3C 规范，采样决策；可选字段，不参与 IsComplete 检查）
//
// 部署类型（Deployment）- 运行环境：
//   - LOCAL : 本地/私有化部署
//   - SAAS  : SaaS 云部署
//
// # 命名约定
//
//	WithXxx(ctx, value)    - 注入：将 value 写入 context
//	Xxx(ctx)               - 读取：从 context 读取值，缺失时返回零值
//	RequireXxx(ctx)        - 强制读取：值必须存在，缺失时返回错误
//	MustXxx(ctx)           - 简化读取：值缺失时返回零值（不返回 ok/error）
//	EnsureXxx(ctx)         - 确保存在：若已存在则返回，否则自动生成
//	GetXxx(ctx)            - 批量读取：返回结构体
//
// 设计决策: 不同字段族根据语义需要提供不同的 API 子集，并非所有字段都具备完整的六件套：
//   - Identity (string 字段): With/Xxx/Require/GetIdentity/Validate — 无需 Must（零值即空字符串）
//   - HasParent (bool 字段): With/HasParent(value,ok)/Must/Require — 需要 ok 区分"未设置"
//   - Trace (string 字段): With/Xxx/Require/Ensure/Validate — 支持强制获取、自动生成和批量校验
//   - DeploymentType: With/DeploymentTypeRaw/GetDeploymentType — 需要验证，命名见各函数注释
//
// 推荐使用顺序：
//   - 读取字段：优先 Xxx(ctx)（零值安全）→ RequireXxx(ctx)（强制存在）→ MustXxx(ctx)（谨慎使用）
//   - 批量操作：优先 GetXxx(ctx) → .Validate()（错误链）或 .IsComplete()（条件判断）
//
// # 哨兵错误
//
//	ErrNilContext                - context 为 nil
//	ErrMissingPlatformID         - platform_id 缺失
//	ErrMissingTenantID           - tenant_id 缺失
//	ErrMissingTenantName         - tenant_name 缺失
//	ErrMissingTraceID            - trace_id 缺失
//	ErrMissingSpanID             - span_id 缺失
//	ErrMissingRequestID          - request_id 缺失
//	ErrMissingHasParent          - has_parent 缺失
//	ErrMissingDeploymentType     - deployment_type 缺失
//	ErrMissingDeploymentTypeValue - deployment_type 值为空（ParseDeploymentType 用）
//	ErrMissingDeploymentTypeEnv  - 环境变量 DEPLOYMENT_TYPE 缺失
//	ErrInvalidDeploymentType     - deployment_type 非法
//
// # 校验策略
//
// xctx 是纯粹的存取层，不对字段值进行格式校验（如 trace_id 长度/hex 格式）。
// 这是有意的设计选择：
//
//   - 校验策略因业务场景而异（严格校验 vs 宽松传播）
//   - 减少热路径上不必要的运行时开销
//   - 保持 API 简洁性，关注点分离
//
// EnsureXxx 系列函数的语义是"确保非空"，对已存在的值不做验证/不纠正。
// 如需格式校验，请在业务层或网关层自行实现。
//
// # 推荐的校验位置
//
// 建议在网关/入口层（而非 xctx 包内）进行格式校验。
// 这样可以在入口处拒绝非法请求，同时允许内部服务在必要时传播非标准值。
package xctx
