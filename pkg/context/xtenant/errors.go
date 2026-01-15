package xtenant

import "errors"

// =============================================================================
// 租户信息相关错误
// =============================================================================

var (
	// ErrEmptyTenantID 租户 ID 为空
	ErrEmptyTenantID = errors.New("xtenant: empty tenant_id")

	// ErrEmptyTenantName 租户名称为空
	ErrEmptyTenantName = errors.New("xtenant: empty tenant_name")
)
