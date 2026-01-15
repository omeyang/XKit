package xauth

import (
	"context"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

// =============================================================================
// Context 集成
// =============================================================================

// GetTokenFromContext 从 context 获取租户 ID 并获取 Token。
// 租户 ID 从 xctx.TenantID(ctx) 获取。
func (c *client) GetTokenFromContext(ctx context.Context) (string, error) {
	tenantID := xctx.TenantID(ctx)
	return c.GetToken(ctx, tenantID)
}

// GetPlatformIDFromContext 从 context 获取租户 ID 并获取平台 ID。
// 租户 ID 从 xctx.TenantID(ctx) 获取。
func (c *client) GetPlatformIDFromContext(ctx context.Context) (string, error) {
	tenantID := xctx.TenantID(ctx)
	return c.GetPlatformID(ctx, tenantID)
}

// HasParentPlatformFromContext 从 context 获取租户 ID 并判断是否有父平台。
// 租户 ID 从 xctx.TenantID(ctx) 获取。
func (c *client) HasParentPlatformFromContext(ctx context.Context) (bool, error) {
	tenantID := xctx.TenantID(ctx)
	return c.HasParentPlatform(ctx, tenantID)
}

// GetUnclassRegionIDFromContext 从 context 获取租户 ID 并获取未归类组 Region ID。
// 租户 ID 从 xctx.TenantID(ctx) 获取。
func (c *client) GetUnclassRegionIDFromContext(ctx context.Context) (string, error) {
	tenantID := xctx.TenantID(ctx)
	return c.GetUnclassRegionID(ctx, tenantID)
}

// =============================================================================
// Context 扩展接口
// =============================================================================

// ContextClient 扩展 Client 接口，提供基于 context 的便捷方法。
type ContextClient interface {
	Client

	// GetTokenFromContext 从 context 获取租户 ID 并获取 Token。
	GetTokenFromContext(ctx context.Context) (string, error)

	// GetPlatformIDFromContext 从 context 获取租户 ID 并获取平台 ID。
	GetPlatformIDFromContext(ctx context.Context) (string, error)

	// HasParentPlatformFromContext 从 context 获取租户 ID 并判断是否有父平台。
	HasParentPlatformFromContext(ctx context.Context) (bool, error)

	// GetUnclassRegionIDFromContext 从 context 获取租户 ID 并获取未归类组 Region ID。
	GetUnclassRegionIDFromContext(ctx context.Context) (string, error)
}

// AsContextClient 将 Client 转换为 ContextClient。
// 如果 client 已经是 ContextClient，直接返回。
func AsContextClient(c Client) ContextClient {
	if cc, ok := c.(ContextClient); ok {
		return cc
	}
	return nil
}

// =============================================================================
// 便捷函数
// =============================================================================

// TenantIDFromContext 从 context 获取租户 ID。
// 如果 context 中没有，尝试从环境变量获取。
func TenantIDFromContext(ctx context.Context) string {
	if tenantID := xctx.TenantID(ctx); tenantID != "" {
		return tenantID
	}
	return GetTenantIDFromEnv()
}

// WithPlatformInfo 使用 xauth 客户端获取平台信息并注入 context。
// 这是一个便捷函数，用于在请求处理前预加载平台信息。
//
// tenantID 为空时，会尝试从 context 或环境变量获取。
//
// 示例：
//
//	ctx, err := xauth.WithPlatformInfo(ctx, authClient, tenantID)
//	if err != nil {
//	    return err
//	}
//	// 后续可以直接从 context 获取平台信息
//	platformID := xctx.PlatformID(ctx)
func WithPlatformInfo(ctx context.Context, c Client, tenantID string) (context.Context, error) {
	// 先解析 tenantID（支持从 context 或环境变量获取）
	if tenantID == "" {
		tenantID = TenantIDFromContext(ctx)
	}
	if tenantID == "" {
		return ctx, ErrMissingTenantID
	}

	// 获取平台 ID
	platformID, err := c.GetPlatformID(ctx, tenantID)
	if err != nil {
		return ctx, err
	}

	// 注入 context
	ctx, err = xctx.WithPlatformID(ctx, platformID)
	if err != nil {
		return ctx, err
	}

	// 注入租户 ID（使用解析后的真实 tenantID）
	ctx, err = xctx.WithTenantID(ctx, tenantID)
	if err != nil {
		return ctx, err
	}

	return ctx, nil
}
