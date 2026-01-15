package xenv

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

// =============================================================================
// DeployType 类型定义
// =============================================================================

// DeployType 表示部署类型
//
// 用于区分本地/私有化部署（LOCAL）和 SaaS 云部署（SAAS）。
// 通常从 ConfigMap 环境变量 DEPLOYMENT_TYPE 获取。
type DeployType string

const (
	// DeployLocal 本地/私有化部署
	DeployLocal DeployType = "LOCAL"

	// DeploySaaS SaaS 云部署
	DeploySaaS DeployType = "SAAS"
)

// String 返回部署类型的字符串表示
func (d DeployType) String() string {
	return string(d)
}

// IsLocal 判断是否为本地/私有化部署
func (d DeployType) IsLocal() bool {
	return d == DeployLocal
}

// IsSaaS 判断是否为 SaaS 云部署
func (d DeployType) IsSaaS() bool {
	return d == DeploySaaS
}

// IsValid 判断部署类型是否有效（为已知类型）
func (d DeployType) IsValid() bool {
	return d == DeployLocal || d == DeploySaaS
}

// =============================================================================
// 错误定义
// =============================================================================

var (
	// ErrNotInitialized xenv 未初始化
	ErrNotInitialized = errors.New("xenv: not initialized, call Init() first")

	// ErrAlreadyInitialized 重复初始化
	ErrAlreadyInitialized = errors.New("xenv: already initialized")

	// ErrMissingEnv 环境变量 DEPLOYMENT_TYPE 缺失/为空
	ErrMissingEnv = errors.New("xenv: missing DEPLOYMENT_TYPE env var")

	// ErrInvalidType 部署类型非法（不是 LOCAL/SAAS）
	ErrInvalidType = errors.New("xenv: invalid deployment type")
)

// =============================================================================
// 常量
// =============================================================================

const (
	// EnvDeployType 环境变量名
	EnvDeployType = "DEPLOYMENT_TYPE"
)

// =============================================================================
// 全局状态
// =============================================================================

var (
	globalType   DeployType
	globalMu     sync.RWMutex
	initialized  atomic.Bool
)

// =============================================================================
// 初始化函数
// =============================================================================

// Init 从环境变量初始化部署类型
//
// 读取 DEPLOYMENT_TYPE 环境变量，支持大小写不敏感匹配：
//   - "LOCAL", "local", "Local" -> DeployLocal
//   - "SAAS", "saas", "SaaS" -> DeploySaaS
//
// 如果环境变量未设置或值非法，返回错误。
// 此函数应在 main() 中服务启动时调用一次。
//
// 如果已经初始化过，返回 ErrAlreadyInitialized。
// 如需重新初始化（如测试场景），请先调用 Reset()。
func Init() error {
	v, ok := os.LookupEnv(EnvDeployType)
	if !ok || strings.TrimSpace(v) == "" {
		return ErrMissingEnv
	}

	dt, err := Parse(v)
	if err != nil {
		return err
	}

	globalMu.Lock()
	defer globalMu.Unlock()

	// 在锁内检查，避免并发读取到空配置
	if initialized.Load() {
		return ErrAlreadyInitialized
	}

	// 先写配置，再设置 initialized 标志
	globalType = dt
	initialized.Store(true)

	return nil
}

// MustInit 从环境变量初始化部署类型，失败时 panic
//
// 适用于初始化失败应该终止程序的场景。
func MustInit() {
	if err := Init(); err != nil {
		panic(err)
	}
}

// InitWith 使用指定的部署类型初始化
//
// 用于测试场景或不依赖环境变量的初始化。
// 如果部署类型非法，返回错误。
//
// 如果已经初始化过，返回 ErrAlreadyInitialized。
// 如需重新初始化（如测试场景），请先调用 Reset()。
func InitWith(dt DeployType) error {
	if !dt.IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidType, dt)
	}

	globalMu.Lock()
	defer globalMu.Unlock()

	// 在锁内检查，避免并发覆盖
	if initialized.Load() {
		return ErrAlreadyInitialized
	}

	globalType = dt
	initialized.Store(true)

	return nil
}

// Reset 重置全局状态（仅用于测试）
func Reset() {
	globalMu.Lock()
	globalType = ""
	initialized.Store(false)
	globalMu.Unlock()
}

// =============================================================================
// 全局访问函数
// =============================================================================

// Type 返回当前部署类型
//
// 需要先调用 Init/MustInit 初始化。
// 如果未初始化，返回空字符串。
func Type() DeployType {
	if !initialized.Load() {
		return ""
	}
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalType
}

// IsLocal 判断是否为本地/私有化部署
//
// 需要先调用 Init/MustInit 初始化。
// 如果未初始化，返回 false。
func IsLocal() bool {
	return Type() == DeployLocal
}

// IsSaaS 判断是否为 SaaS 云部署
//
// 需要先调用 Init/MustInit 初始化。
// 如果未初始化，返回 false。
func IsSaaS() bool {
	return Type() == DeploySaaS
}

// IsInitialized 返回是否已初始化
func IsInitialized() bool {
	return initialized.Load()
}

// RequireType 返回当前部署类型，未初始化时返回错误
//
// 适用于必须明确知道部署类型的业务场景。
func RequireType() (DeployType, error) {
	if !initialized.Load() {
		return "", ErrNotInitialized
	}
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalType, nil
}

// =============================================================================
// 解析函数
// =============================================================================

// Parse 解析字符串为 DeployType
//
// 支持大小写不敏感匹配：
//   - "LOCAL", "local", "Local" -> DeployLocal
//   - "SAAS", "saas", "SaaS" -> DeploySaaS
func Parse(s string) (DeployType, error) {
	normalized := strings.ToUpper(strings.TrimSpace(s))
	switch normalized {
	case "LOCAL":
		return DeployLocal, nil
	case "SAAS":
		return DeploySaaS, nil
	case "":
		return "", ErrMissingEnv
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidType, s)
	}
}
