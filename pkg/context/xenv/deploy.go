package xenv

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/omeyang/xkit/internal/deploy"
)

// =============================================================================
// DeployType 类型定义
// =============================================================================

// DeployType 表示部署类型
//
// 用于区分本地/私有化部署（LOCAL）和 SaaS 云部署（SAAS）。
// 通常从 ConfigMap 环境变量 DEPLOYMENT_TYPE 获取。
//
// DeployType 与 xctx.DeploymentType 是同一底层类型，可互换使用：
//   - xenv.DeployType: 进程级环境配置（全局单例）
//   - xctx.DeploymentType: 请求级 context 传播
type DeployType = deploy.Type

const (
	// DeployLocal 本地/私有化部署
	DeployLocal = deploy.Local

	// DeploySaaS SaaS 云部署
	DeploySaaS = deploy.SaaS
)

// =============================================================================
// 错误定义
// =============================================================================

var (
	// ErrNotInitialized xenv 未初始化
	ErrNotInitialized = errors.New("xenv: not initialized, call Init() first")

	// ErrAlreadyInitialized 重复初始化
	ErrAlreadyInitialized = errors.New("xenv: already initialized")

	// ErrMissingEnv 环境变量 DEPLOYMENT_TYPE 未设置
	ErrMissingEnv = errors.New("xenv: DEPLOYMENT_TYPE env var not set")

	// ErrEmptyEnv 环境变量 DEPLOYMENT_TYPE 值为空
	ErrEmptyEnv = errors.New("xenv: DEPLOYMENT_TYPE env var is empty")

	// ErrInvalidType 部署类型非法（不是 LOCAL/SAAS）
	//
	// 设计决策: 使用 xenv 自有错误哨兵（而非 deploy.ErrInvalidType 别名），
	// 避免公共 API 错误文案泄漏 internal/deploy 命名空间。
	// xctx 同理使用 ErrInvalidDeploymentType 作为自有哨兵。
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
	// 设计决策: globalType 使用 atomic.Value 实现无锁读取。
	// 初始化后值不变（Reset 仅测试可用），读路径无需 sync.RWMutex，
	// 消除了高并发下 RWMutex 的缓存行竞争（35ns → <1ns）。
	globalType atomic.Value // 存储 DeployType

	// globalMu 仅保护写路径（Init/InitWith/Reset）的并发序列化
	globalMu    sync.Mutex
	initialized atomic.Bool
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
// 错误场景：
//   - 环境变量未设置: ErrMissingEnv
//   - 环境变量为空/纯空白: ErrEmptyEnv
//   - 值非法: ErrInvalidType
//   - 已初始化: ErrAlreadyInitialized
//
// 此函数应在 main() 中服务启动时调用一次。
func Init() error {
	v, ok := os.LookupEnv(EnvDeployType)
	if !ok {
		return ErrMissingEnv
	}
	if strings.TrimSpace(v) == "" {
		return ErrEmptyEnv
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
	globalType.Store(dt)
	initialized.Store(true)

	return nil
}

// MustInit 从环境变量初始化部署类型，失败时 panic
//
// 设计决策: MustInit 遵循 Go 标准库惯例（如 regexp.MustCompile），
// 仅用于 main() 启动阶段，初始化失败意味着配置错误应立即终止进程。
// 业务代码应使用 Init() 的错误返回路径。
func MustInit() {
	if err := Init(); err != nil {
		panic(err)
	}
}

// InitWith 使用指定的部署类型初始化
//
// 设计决策: InitWith 作为公开 API 保留，支持不依赖环境变量的初始化场景
// （如集成测试、嵌入式部署）。通过 ErrAlreadyInitialized 保证单次初始化语义，
// 与 Init() 具有相同的不可变性保障。
//
// 如果部署类型非法，返回 ErrInvalidType。
// 如果已经初始化过，返回 ErrAlreadyInitialized。
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

	globalType.Store(dt)
	initialized.Store(true)

	return nil
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
	// 设计决策: 初始化后 globalType 不变，atomic.Value.Load 提供无锁读取。
	// globalType 仅通过 Init/InitWith/Reset 写入 DeployType 值，
	// 类型断言必然成功。保留 comma-ok 形式以满足 golangci-lint check-type-assertions 规则。
	dt, ok := globalType.Load().(DeployType)
	if !ok {
		return ""
	}
	return dt
}

// IsLocal 判断是否为本地/私有化部署
//
// 设计决策: 未初始化时返回 false（而非 panic 或 error），与 Go 零值语义一致。
// 需要区分"未初始化"和"非 Local"的场景应使用 RequireType()。
func IsLocal() bool {
	return Type() == DeployLocal
}

// IsSaaS 判断是否为 SaaS 云部署
//
// 设计决策: 未初始化时返回 false（而非 panic 或 error），与 Go 零值语义一致。
// 需要区分"未初始化"和"非 SaaS"的场景应使用 RequireType()。
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
	// 设计决策: globalType 仅存储 DeployType 值，类型断言必然成功。
	// 保留 comma-ok 形式以满足 golangci-lint check-type-assertions 规则。
	dt, ok := globalType.Load().(DeployType)
	if !ok {
		return "", ErrNotInitialized
	}
	return dt, nil
}

// =============================================================================
// 解析函数
// =============================================================================

// Parse 解析字符串为 DeployType
//
// 支持大小写不敏感匹配：
//   - "LOCAL", "local", "Local" -> DeployLocal
//   - "SAAS", "saas", "SaaS" -> DeploySaaS
//
// 设计决策: 空字符串和非法值统一返回 ErrInvalidType（而非区分 ErrMissingValue），
// 因为 Parse 是纯解析函数，不涉及环境变量语义。"空 vs 非法"的区分由调用方
// （如 Init）在更高层级处理。
func Parse(s string) (DeployType, error) {
	// deploy.Parse 内部已做 TrimSpace + ToUpper，直接委托
	dt, err := deploy.Parse(s)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrInvalidType, s)
	}
	return dt, nil
}
