package xctx

import (
	"context"
	"fmt"

	"github.com/omeyang/xkit/internal/deploy"
	"github.com/omeyang/xkit/pkg/context/xenv"
)

// =============================================================================
// DeploymentType 类型定义
// =============================================================================

// DeploymentType 表示部署类型
//
// 用于区分本地/私有化部署（LOCAL）和 SaaS 云部署（SAAS）。
// 通常从 ConfigMap 环境变量 DEPLOYMENT_TYPE 获取。
//
// 这是 deploy.Type 的类型别名，用于请求级 context 传播。
// 如需进程级环境配置，请使用 xenv.DeployType。
type DeploymentType = deploy.Type

const (
	// DeploymentLocal 本地/私有化部署
	DeploymentLocal = deploy.Local

	// DeploymentSaaS SaaS 云部署
	DeploymentSaaS = deploy.SaaS
)

// =============================================================================
// DeploymentType Key 常量
// =============================================================================

const (
	// KeyDeploymentType 日志属性 key
	KeyDeploymentType = "deployment_type"

	// EnvDeploymentType 环境变量名
	EnvDeploymentType = "DEPLOYMENT_TYPE"

	// DeploymentFieldCount 部署字段数量（用于 slog 属性预分配）
	DeploymentFieldCount = 1
)

// =============================================================================
// DeploymentType Context Key 定义
// =============================================================================

const keyDeploymentType = contextKey("xctx:deployment_type")

// =============================================================================
// DeploymentType Context 操作
// =============================================================================

// WithDeploymentType 将部署类型注入 context（仅允许 LOCAL/SAAS）
func WithDeploymentType(ctx context.Context, dt DeploymentType) (context.Context, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if !dt.IsValid() {
		return nil, fmt.Errorf("%w: %q", ErrInvalidDeploymentType, dt)
	}
	return context.WithValue(ctx, keyDeploymentType, dt), nil
}

// DeploymentTypeRaw 从 context 提取部署类型，不存在返回空字符串
//
// 不进行验证，仅返回原始值。适用于只需读取值而不关心验证的场景。
// 如需验证部署类型有效性，请使用 GetDeploymentType。
func DeploymentTypeRaw(ctx context.Context) DeploymentType {
	if ctx == nil {
		return ""
	}
	switch v := ctx.Value(keyDeploymentType).(type) {
	case DeploymentType:
		return v
	case string:
		return DeploymentType(v)
	default:
		return ""
	}
}

// GetDeploymentType 从 context 提取并验证部署类型（仅允许 LOCAL/SAAS）
//
// 注意：此函数包含验证逻辑，缺失或无效值会返回错误。
// 命名为 GetXxx 而非 RequireXxx 是因为部署类型需要额外的格式验证。
// 如只需读取原始值，请使用 DeploymentTypeRaw。
func GetDeploymentType(ctx context.Context) (DeploymentType, error) {
	if ctx == nil {
		return "", ErrNilContext
	}

	v := ctx.Value(keyDeploymentType)
	if v == nil {
		return "", ErrMissingDeploymentType
	}

	switch raw := v.(type) {
	case DeploymentType:
		if !raw.IsValid() {
			return "", fmt.Errorf("%w: %q", ErrInvalidDeploymentType, raw)
		}
		return raw, nil
	case string:
		dt := DeploymentType(raw)
		if !dt.IsValid() {
			return "", fmt.Errorf("%w: %q", ErrInvalidDeploymentType, raw)
		}
		return dt, nil
	default:
		return "", fmt.Errorf("%w: %T", ErrInvalidDeploymentType, v)
	}
}

// =============================================================================
// 便捷判断函数
// =============================================================================

// IsLocal 判断 context 中的部署类型是否为本地/私有化部署
func IsLocal(ctx context.Context) (bool, error) {
	dt, err := GetDeploymentType(ctx)
	if err != nil {
		return false, err
	}
	return dt.IsLocal(), nil
}

// IsSaaS 判断 context 中的部署类型是否为 SaaS 云部署
func IsSaaS(ctx context.Context) (bool, error) {
	dt, err := GetDeploymentType(ctx)
	if err != nil {
		return false, err
	}
	return dt.IsSaaS(), nil
}

// =============================================================================
// 解析函数
// =============================================================================

// ParseDeploymentType 解析字符串为 DeploymentType
//
// 支持大小写不敏感匹配：
//   - "LOCAL", "local", "Local" -> DeploymentLocal
//   - "SAAS", "saas", "SaaS" -> DeploymentSaaS
func ParseDeploymentType(s string) (DeploymentType, error) {
	dt, err := deploy.Parse(s)
	if err != nil {
		if err == deploy.ErrMissingValue {
			return "", ErrMissingDeploymentTypeValue
		}
		return "", fmt.Errorf("%w: %q", ErrInvalidDeploymentType, s)
	}
	return dt, nil
}

// =============================================================================
// 类型转换（向后兼容）
// =============================================================================
//
// 由于 xctx.DeploymentType 和 xenv.DeployType 现在都是 deploy.Type 的类型别名，
// 它们实际上是同一类型，可以直接互换使用，无需转换。
//
// 以下函数保留用于向后兼容，但实际上只是简单返回传入的值。

// ToEnvDeployType 将 xctx.DeploymentType 转换为 xenv.DeployType
//
// Deprecated: 由于两者现在是同一类型的别名，可以直接使用，无需转换。
// 此函数保留用于向后兼容。
//
// 示例：
//
//	dt, _ := xctx.GetDeploymentType(ctx)
//	if dt == xenv.Type() {  // 直接比较即可
//	    // 请求的部署类型与全局配置一致
//	}
func ToEnvDeployType(d DeploymentType) xenv.DeployType {
	return d
}

// FromEnvDeployType 从 xenv.DeployType 创建 xctx.DeploymentType
//
// Deprecated: 由于两者现在是同一类型的别名，可以直接使用，无需转换。
// 此函数保留用于向后兼容。
//
// 示例：
//
//	dt := xenv.Type()           // 直接使用即可
//	ctx, _ := xctx.WithDeploymentType(ctx, dt)
func FromEnvDeployType(dt xenv.DeployType) DeploymentType {
	return dt
}
