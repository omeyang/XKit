package xtenant

import "errors"

// =============================================================================
// 租户信息相关错误
// 设计决策: 命名使用 "empty" 而非 "missing"，与 xctx 的 "missing" 语义区分：
// - ErrEmpty*: 传输层校验——Header/Metadata 中的值为空
// - xctx.ErrMissing*: context 层读取——context 中未设置该字段
// =============================================================================

var (
	// ErrEmptyTenantID 租户 ID 为空
	ErrEmptyTenantID = errors.New("xtenant: empty tenant_id")

	// ErrEmptyTenantName 租户名称为空
	ErrEmptyTenantName = errors.New("xtenant: empty tenant_name")
)
