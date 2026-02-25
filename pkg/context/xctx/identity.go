package xctx

import "context"

// =============================================================================
// Identity 日志属性 Key 常量
// =============================================================================

// Identity Key 常量，遵循下划线分隔的命名约定
const (
	KeyPlatformID = "platform_id"
	KeyTenantID   = "tenant_id"
	KeyTenantName = "tenant_name"

	// identityFieldCount 身份字段数量（用于 slog 属性预分配，不导出以避免脆弱的 API 契约）
	identityFieldCount = 3
)

// =============================================================================
// Identity Context Key 定义
// =============================================================================

const (
	keyPlatformID = contextKey("xctx:platform_id")
	keyTenantID   = contextKey("xctx:tenant_id")
	keyTenantName = contextKey("xctx:tenant_name")
)

// =============================================================================
// PlatformID 操作
// =============================================================================

// WithPlatformID 将 platform ID 注入 context
//
// 设计决策: 返回 error 而非 panic（项目规范：构造函数统一返回 error），
// 虽然唯一错误条件是 nil ctx，但保持所有 WithXxx 签名一致，便于中间件链统一处理。
// 不校验 value 有效性（如空字符串），因为 xctx 是纯存取层（见 doc.go 校验策略）。
// 如需确认值存在，请使用 RequirePlatformID。
func WithPlatformID(ctx context.Context, platformID string) (context.Context, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	return context.WithValue(ctx, keyPlatformID, platformID), nil
}

// PlatformID 从 context 提取 platform ID，不存在返回空字符串
func PlatformID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(keyPlatformID).(string); ok {
		return v
	}
	return ""
}

// =============================================================================
// TenantID 操作
// =============================================================================

// WithTenantID 将 tenant ID 注入 context
//
// 如果 ctx 为 nil，返回 ErrNilContext。
func WithTenantID(ctx context.Context, tenantID string) (context.Context, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	return context.WithValue(ctx, keyTenantID, tenantID), nil
}

// TenantID 从 context 提取 tenant ID，不存在返回空字符串
func TenantID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(keyTenantID).(string); ok {
		return v
	}
	return ""
}

// =============================================================================
// TenantName 操作
// =============================================================================

// WithTenantName 将 tenant name 注入 context
//
// 如果 ctx 为 nil，返回 ErrNilContext。
func WithTenantName(ctx context.Context, tenantName string) (context.Context, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	return context.WithValue(ctx, keyTenantName, tenantName), nil
}

// TenantName 从 context 提取 tenant name，不存在返回空字符串
func TenantName(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(keyTenantName).(string); ok {
		return v
	}
	return ""
}

// =============================================================================
// Require 函数：强制获取模式
// 身份信息必须由业务层提供，缺失时返回错误由上层决策
// =============================================================================

// RequirePlatformID 从 context 获取 platform ID，不存在则返回错误。
//
// 语义：值必须存在，缺失时返回 ErrMissingPlatformID。
// 与 PlatformID() 不同，此函数在值缺失时返回错误而非空字符串，
// 适用于必须有身份信息的业务场景，由调用方决定如何处理错误。
// 如果 ctx 为 nil，返回 ErrNilContext。
func RequirePlatformID(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", ErrNilContext
	}
	v := PlatformID(ctx)
	if v == "" {
		return "", ErrMissingPlatformID
	}
	return v, nil
}

// RequireTenantID 从 context 获取 tenant ID，不存在则返回错误。
//
// 语义：值必须存在，缺失时返回 ErrMissingTenantID。
// 适用于多租户隔离场景，确保业务操作在正确的租户上下文中执行。
// 如果 ctx 为 nil，返回 ErrNilContext。
func RequireTenantID(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", ErrNilContext
	}
	v := TenantID(ctx)
	if v == "" {
		return "", ErrMissingTenantID
	}
	return v, nil
}

// RequireTenantName 从 context 获取 tenant name，不存在则返回错误。
//
// 语义：值必须存在，缺失时返回 ErrMissingTenantName。
// 如果 ctx 为 nil，返回 ErrNilContext。
func RequireTenantName(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", ErrNilContext
	}
	v := TenantName(ctx)
	if v == "" {
		return "", ErrMissingTenantName
	}
	return v, nil
}

// =============================================================================
// Identity 结构体（批量获取模式）
// =============================================================================

// Identity 身份信息结构体
// 用于批量获取身份信息，替代多返回值函数。
type Identity struct {
	PlatformID string
	TenantID   string
	TenantName string
}

// Validate 校验 Identity 必填字段是否完整，缺失时返回对应的哨兵错误。
//
// 采用 fail-fast 策略：仅返回第一个缺失字段的错误（按 PlatformID → TenantID → TenantName 顺序）。
// 如需一次性获取所有缺失字段，请逐字段调用 RequireXxx 或自行遍历检查。
//
// 与 IsComplete() 检查相同条件，区别在于返回类型：
//   - Validate() 返回 error，适用于中间件/业务层的错误处理链
//   - IsComplete() 返回 bool，适用于条件判断和日志记录
//
// 约束：
//   - PlatformID 必须存在
//   - TenantID 必须存在
//   - TenantName 必须存在
func (i Identity) Validate() error {
	if i.PlatformID == "" {
		return ErrMissingPlatformID
	}
	if i.TenantID == "" {
		return ErrMissingTenantID
	}
	if i.TenantName == "" {
		return ErrMissingTenantName
	}
	return nil
}

// IsComplete 判断 Identity 是否完整（所有字段均非空）
func (i Identity) IsComplete() bool {
	return i.PlatformID != "" && i.TenantID != "" && i.TenantName != ""
}

// GetIdentity 从 context 批量获取所有身份信息
// 返回 Identity 结构体，字段可能为空字符串。
// 使用 Validate() 检查必填字段，使用 IsComplete() 检查是否全部存在。
func GetIdentity(ctx context.Context) Identity {
	return Identity{
		PlatformID: PlatformID(ctx),
		TenantID:   TenantID(ctx),
		TenantName: TenantName(ctx),
	}
}

// WithIdentity 将 Identity 结构体中的非空字段批量注入 context。
//
// 仅注入非空字段，空字符串字段会被跳过。
// 适用于从上游请求（如 HTTP Header、gRPC Metadata）解析身份信息后一次性注入。
// 如果 ctx 为 nil，返回 ErrNilContext。
func WithIdentity(ctx context.Context, id Identity) (context.Context, error) {
	return applyOptionalFields(ctx, []contextFieldSetter{
		{value: id.PlatformID, set: WithPlatformID},
		{value: id.TenantID, set: WithTenantID},
		{value: id.TenantName, set: WithTenantName},
	})
}
