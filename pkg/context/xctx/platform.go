package xctx

import "context"

// =============================================================================
// Platform 日志属性 Key 常量
// =============================================================================

// Platform Key 常量，遵循下划线分隔的命名约定
const (
	// KeyHasParent 表示当前平台是否有上级平台
	KeyHasParent = "has_parent"

	// KeyUnclassRegionID 未分类区域 ID
	KeyUnclassRegionID = "unclass_region_id"

	// PlatformFieldCount 平台字段数量（用于预分配切片容量）
	PlatformFieldCount = 2
)

// =============================================================================
// Platform Context Key 定义
// =============================================================================

const (
	keyHasParent       = contextKey("xctx:has_parent")
	keyUnclassRegionID = contextKey("xctx:unclass_region_id")
)

// =============================================================================
// HasParent 操作
// =============================================================================

// WithHasParent 将 has_parent 标志注入 context
//
// 语义：标识当前平台是否有上级平台（用于 SaaS 多级部署场景）。
// 如果 ctx 为 nil，返回 ErrNilContext。
func WithHasParent(ctx context.Context, hasParent bool) (context.Context, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	return context.WithValue(ctx, keyHasParent, hasParent), nil
}

// HasParent 从 context 提取 has_parent 标志。
// 返回 (value, ok)，ok 表示该字段是否存在（用于区分"未设置"和"设置为false"）。
func HasParent(ctx context.Context) (value bool, ok bool) {
	if ctx == nil {
		return false, false
	}
	v, ok := ctx.Value(keyHasParent).(bool)
	return v, ok
}

// MustHasParent 从 context 提取 has_parent 标志，不存在则返回 false
//
// 语义：简化版获取函数，适用于只关心"是否有上级"而不关心"是否设置"的场景。
// 如果需要区分"未设置"和"设置为false"，请使用 HasParent。
func MustHasParent(ctx context.Context) bool {
	v, _ := HasParent(ctx)
	return v
}

// RequireHasParent 从 context 获取 has_parent 标志，不存在则返回错误
//
// 语义：值必须存在，缺失时返回 ErrMissingHasParent。
// 适用于必须明确知道平台层级关系的业务场景。
// 如果 ctx 为 nil，返回 ErrNilContext。
func RequireHasParent(ctx context.Context) (bool, error) {
	if ctx == nil {
		return false, ErrNilContext
	}
	v, ok := HasParent(ctx)
	if !ok {
		return false, ErrMissingHasParent
	}
	return v, nil
}

// =============================================================================
// UnclassRegionID 操作
// =============================================================================

// WithUnclassRegionID 将未分类区域 ID 注入 context
//
// 如果 ctx 为 nil，返回 ErrNilContext。
func WithUnclassRegionID(ctx context.Context, regionID string) (context.Context, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	return context.WithValue(ctx, keyUnclassRegionID, regionID), nil
}

// UnclassRegionID 从 context 提取未分类区域 ID，不存在返回空字符串
func UnclassRegionID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(keyUnclassRegionID).(string); ok {
		return v
	}
	return ""
}

// =============================================================================
// Platform 结构体（批量获取模式）
// =============================================================================

// Platform 平台信息结构体
//
// 用于批量获取平台信息，替代多参数函数。
type Platform struct {
	HasParent       bool
	UnclassRegionID string
}

// GetPlatform 从 context 批量获取平台信息
//
// 返回 Platform 结构体。
// 注意：HasParent 默认为 false，无法区分"未设置"和"设置为false"。
// 如需区分，请使用 HasParent(ctx) 函数。
func GetPlatform(ctx context.Context) Platform {
	hasParent, _ := HasParent(ctx)
	return Platform{
		HasParent:       hasParent,
		UnclassRegionID: UnclassRegionID(ctx),
	}
}

// WithPlatform 将 Platform 结构体中的字段批量注入 context
//
// 注意：HasParent 总是会被注入（即使为 false），UnclassRegionID 仅在非空时注入。
// 如果 ctx 为 nil，返回 ErrNilContext。
func WithPlatform(ctx context.Context, p Platform) (context.Context, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	var err error
	ctx, err = WithHasParent(ctx, p.HasParent)
	if err != nil {
		return nil, err
	}
	if p.UnclassRegionID != "" {
		ctx, err = WithUnclassRegionID(ctx, p.UnclassRegionID)
		if err != nil {
			return nil, err
		}
	}
	return ctx, nil
}
