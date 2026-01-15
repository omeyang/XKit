package xctx

import (
	"context"
	"fmt"
	"strings"

	"github.com/omeyang/xkit/pkg/context/xenv"
)

// =============================================================================
// DeploymentType 类型定义
// =============================================================================

// DeploymentType 表示部署类型
//
// 用于区分本地/私有化部署（LOCAL）和 SaaS 云部署（SAAS）。
// 通常从 ConfigMap 环境变量 DEPLOYMENT_TYPE 获取。
type DeploymentType string

const (
	// DeploymentLocal 本地/私有化部署
	DeploymentLocal DeploymentType = "LOCAL"

	// DeploymentSaaS SaaS 云部署
	DeploymentSaaS DeploymentType = "SAAS"
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
// DeploymentType 类型方法
// =============================================================================

// String 返回部署类型的字符串表示
func (d DeploymentType) String() string {
	return string(d)
}

// IsLocal 判断是否为本地/私有化部署
func (d DeploymentType) IsLocal() bool {
	return d == DeploymentLocal
}

// IsSaaS 判断是否为 SaaS 云部署
func (d DeploymentType) IsSaaS() bool {
	return d == DeploymentSaaS
}

// IsValid 判断部署类型是否有效（为已知类型）
func (d DeploymentType) IsValid() bool {
	return d == DeploymentLocal || d == DeploymentSaaS
}

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
	normalized := strings.ToUpper(strings.TrimSpace(s))
	switch normalized {
	case "LOCAL":
		return DeploymentLocal, nil
	case "SAAS":
		return DeploymentSaaS, nil
	case "":
		return "", ErrMissingDeploymentTypeValue
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidDeploymentType, s)
	}
}

// =============================================================================
// 类型转换（xctx.DeploymentType <-> xenv.DeployType）
// =============================================================================
//
// xctx.DeploymentType 和 xenv.DeployType 是两个功能相同但用途不同的类型：
//   - xctx.DeploymentType: 用于请求级 context 传播，随每个请求携带
//   - xenv.DeployType: 用于进程级环境配置，从环境变量初始化后全局共享
//
// 两者底层值相同（LOCAL/SAAS），以下函数提供安全的类型转换。

// ToEnvDeployType 将 xctx.DeploymentType 转换为 xenv.DeployType
//
// 用于在请求级 context 和进程级环境配置之间进行类型转换。
// 如果需要将 context 中的部署类型与全局配置进行比较，可以使用此方法。
//
// 示例：
//
//	dt, _ := xctx.GetDeploymentType(ctx)
//	if dt.ToEnvDeployType() == xenv.Type() {
//	    // 请求的部署类型与全局配置一致
//	}
func (d DeploymentType) ToEnvDeployType() xenv.DeployType {
	return xenv.DeployType(d)
}

// FromEnvDeployType 从 xenv.DeployType 创建 xctx.DeploymentType
//
// 用于将进程级配置转换为可注入 context 的类型。
// 典型场景：在请求入口处将全局配置注入 context。
//
// 示例：
//
//	dt := xctx.FromEnvDeployType(xenv.Type())
//	ctx, _ := xctx.WithDeploymentType(ctx, dt)
func FromEnvDeployType(dt xenv.DeployType) DeploymentType {
	return DeploymentType(dt)
}
