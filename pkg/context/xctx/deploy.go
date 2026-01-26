package xctx

import (
	"context"
	"fmt"

	"github.com/omeyang/xkit/internal/deploy"
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
//
// 命名说明：本函数命名为 DeploymentTypeRaw 而非 DeploymentType，原因如下：
//   - 包内其他字段使用 Xxx(ctx) 命名模式（如 TenantID, TraceID），但这些字段无需验证
//   - DeploymentType 需要区分"原始读取"和"验证读取"两种语义
//   - DeploymentType 与类型别名 DeploymentType 重名，使用 Raw 后缀避免混淆
//   - Raw 后缀明确表示"返回未验证的原始值"，与 GetDeploymentType 的验证语义形成对比
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

