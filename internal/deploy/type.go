package deploy

import (
	"errors"
	"fmt"
	"strings"
)

// EnvName 环境变量名（单一事实来源）
//
// xenv.EnvDeploymentType 和 xctx.EnvDeploymentType 均引用此常量，
// 确保环境变量名变更时只需修改一处。
const EnvName = "DEPLOYMENT_TYPE"

// Type 表示部署类型
//
// 用于区分本地/私有化部署（LOCAL）和 SaaS 云部署（SAAS）。
// 通常从 ConfigMap 环境变量 DEPLOYMENT_TYPE 获取。
type Type string

const (
	// Local 本地/私有化部署
	Local Type = "LOCAL"

	// SaaS SaaS 云部署
	SaaS Type = "SAAS"
)

// 错误定义
var (
	// ErrMissingValue 部署类型值缺失/为空
	ErrMissingValue = errors.New("deploy: missing deployment type value")

	// ErrInvalidType 部署类型非法（不是 LOCAL/SAAS）
	ErrInvalidType = errors.New("deploy: invalid deployment type")
)

// String 返回部署类型的字符串表示
func (d Type) String() string {
	return string(d)
}

// IsLocal 判断是否为本地/私有化部署
func (d Type) IsLocal() bool {
	return d == Local
}

// IsSaaS 判断是否为 SaaS 云部署
func (d Type) IsSaaS() bool {
	return d == SaaS
}

// IsValid 判断部署类型是否有效（为已知类型）
func (d Type) IsValid() bool {
	return d == Local || d == SaaS
}

// Parse 解析字符串为 Type
//
// 支持大小写不敏感匹配：
//   - "LOCAL", "local", "Local" -> Local
//   - "SAAS", "saas", "SaaS" -> SaaS
func Parse(s string) (Type, error) {
	normalized := strings.ToUpper(strings.TrimSpace(s))
	switch normalized {
	case "LOCAL":
		return Local, nil
	case "SAAS":
		return SaaS, nil
	case "":
		return "", ErrMissingValue
	default:
		return "", fmt.Errorf("%w: %q (expected LOCAL or SAAS)", ErrInvalidType, s)
	}
}
