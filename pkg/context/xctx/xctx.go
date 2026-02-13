package xctx

import "errors"

// =============================================================================
// Context Key 类型定义
// =============================================================================

// 设计决策: contextKey 使用 string 而非 int+iota，理由如下：
//   - 作为包私有类型，不会与其他包的 context key 冲突（Go context 比较包含类型信息）
//   - 字符串值在调试/日志中可读性高，便于排查 context 传播问题
//   - 性能差异可忽略（WithPlatformID ~36ns/op），不构成瓶颈
type contextKey string

// =============================================================================
// 通用错误
// =============================================================================

var (
	// ErrNilContext 表示传入的 context 为 nil。
	ErrNilContext = errors.New("xctx: nil context")
)

// =============================================================================
// Identity 相关错误
// =============================================================================

var (
	// ErrMissingPlatformID platform_id 缺失
	ErrMissingPlatformID = errors.New("xctx: missing platform_id")

	// ErrMissingTenantID tenant_id 缺失
	ErrMissingTenantID = errors.New("xctx: missing tenant_id")

	// ErrMissingTenantName tenant_name 缺失
	ErrMissingTenantName = errors.New("xctx: missing tenant_name")
)

// =============================================================================
// DeploymentType 相关错误
// =============================================================================

var (
	// ErrMissingDeploymentType deployment_type 缺失
	ErrMissingDeploymentType = errors.New("xctx: missing deployment_type")

	// ErrMissingDeploymentTypeValue deployment_type 值为空（用于 ParseDeploymentType）
	ErrMissingDeploymentTypeValue = errors.New("xctx: empty deployment_type value")

	// ErrInvalidDeploymentType deployment_type 非法（不是 LOCAL/SAAS）
	ErrInvalidDeploymentType = errors.New("xctx: invalid deployment_type")

	// ErrMissingDeploymentTypeEnv 环境变量 DEPLOYMENT_TYPE 缺失/为空
	ErrMissingDeploymentTypeEnv = errors.New("xctx: missing DEPLOYMENT_TYPE env var")
)

// =============================================================================
// Platform 相关错误
// =============================================================================

var (
	// ErrMissingHasParent has_parent 缺失
	ErrMissingHasParent = errors.New("xctx: missing has_parent")
)
