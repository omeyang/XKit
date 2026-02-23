package xtenant

import (
	"context"
	"strings"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

// =============================================================================
// TenantInfo 租户信息（请求级）
// =============================================================================

// TenantInfo 租户信息结构体
//
// 用于批量操作请求级的租户信息。
type TenantInfo struct {
	// TenantID 租户 ID
	TenantID string

	// TenantName 租户名称
	TenantName string
}

// IsEmpty 判断租户信息是否为空
func (t TenantInfo) IsEmpty() bool {
	return t.TenantID == "" && t.TenantName == ""
}

// Validate 验证必填字段
func (t TenantInfo) Validate() error {
	if t.TenantID == "" {
		return ErrEmptyTenantID
	}
	if t.TenantName == "" {
		return ErrEmptyTenantName
	}
	return nil
}

// =============================================================================
// Context 获取函数（请求级信息）
// 设计决策: 这些函数是 xctx 同名函数的 1:1 便捷包装，使调用方只需
// 导入 xtenant 即可完成所有租户操作，无需关心底层 xctx 包。
// =============================================================================

// TenantID 从 context 获取租户 ID
//
// 返回空字符串表示未设置。
// 底层使用 xctx.TenantID。
func TenantID(ctx context.Context) string {
	return xctx.TenantID(ctx)
}

// TenantName 从 context 获取租户名称
//
// 返回空字符串表示未设置。
// 底层使用 xctx.TenantName。
func TenantName(ctx context.Context) string {
	return xctx.TenantName(ctx)
}

// GetTenantInfo 从 context 批量获取租户信息
//
// 返回 TenantInfo 结构体，字段可能为空字符串。
func GetTenantInfo(ctx context.Context) TenantInfo {
	return TenantInfo{
		TenantID:   xctx.TenantID(ctx),
		TenantName: xctx.TenantName(ctx),
	}
}

// RequireTenantID 从 context 获取租户 ID，不存在则返回错误
//
// 适用于必须有租户信息的业务场景。
func RequireTenantID(ctx context.Context) (string, error) {
	return xctx.RequireTenantID(ctx)
}

// RequireTenantName 从 context 获取租户名称，不存在则返回错误
func RequireTenantName(ctx context.Context) (string, error) {
	return xctx.RequireTenantName(ctx)
}

// =============================================================================
// Context 注入函数
// =============================================================================

// WithTenantID 将租户 ID 注入 context
//
// 如果 ctx 为 nil，返回错误。
// 底层使用 xctx.WithTenantID。
func WithTenantID(ctx context.Context, tenantID string) (context.Context, error) {
	return xctx.WithTenantID(ctx, tenantID)
}

// WithTenantName 将租户名称注入 context
//
// 如果 ctx 为 nil，返回错误。
// 底层使用 xctx.WithTenantName。
func WithTenantName(ctx context.Context, tenantName string) (context.Context, error) {
	return xctx.WithTenantName(ctx, tenantName)
}

// WithTenantInfo 将 TenantInfo 批量注入 context
//
// 只注入非空字段（TrimSpace 后判断）。如果 info 为零值（IsEmpty() == true），
// 返回原始 ctx 且不做任何修改。
// 如果 ctx 为 nil，返回错误。
//
// 设计决策: 对 TenantID/TenantName 做 TrimSpace 后再判断是否为空，
// 与 Extract 函数的空白处理语义保持一致（ExtractFromHTTPHeader/ExtractFromMetadata
// 均使用 TrimSpace）。这确保纯空白值不会被注入 context。
func WithTenantInfo(ctx context.Context, info TenantInfo) (context.Context, error) {
	if ctx == nil {
		return nil, xctx.ErrNilContext
	}

	var err error

	if tid := strings.TrimSpace(info.TenantID); tid != "" {
		ctx, err = xctx.WithTenantID(ctx, tid)
		if err != nil { // 防御性处理：当前 xctx 实现下不可达
			return nil, err
		}
	}

	if tname := strings.TrimSpace(info.TenantName); tname != "" {
		ctx, err = xctx.WithTenantName(ctx, tname)
		if err != nil { // 防御性处理：当前 xctx 实现下不可达
			return nil, err
		}
	}

	return ctx, nil
}
