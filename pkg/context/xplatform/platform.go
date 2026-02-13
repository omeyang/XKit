package xplatform

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
)

// =============================================================================
// 错误定义
// =============================================================================

var (
	// ErrNotInitialized xplatform 未初始化
	ErrNotInitialized = errors.New("xplatform: not initialized, call Init() first")

	// ErrMissingPlatformID 缺少 PlatformID
	ErrMissingPlatformID = errors.New("xplatform: missing platform_id")

	// ErrInvalidPlatformID PlatformID 格式非法（包含空白字符或超过最大长度）
	ErrInvalidPlatformID = errors.New("xplatform: invalid platform_id format")

	// ErrInvalidUnclassRegionID UnclassRegionID 格式非法（包含空白字符或超过最大长度）
	ErrInvalidUnclassRegionID = errors.New("xplatform: invalid unclass_region_id format")

	// ErrAlreadyInitialized 重复初始化
	ErrAlreadyInitialized = errors.New("xplatform: already initialized")
)

// =============================================================================
// 配置结构
// =============================================================================

const (
	// maxPlatformIDLen PlatformID 的最大长度（字节）
	maxPlatformIDLen = 128

	// maxUnclassRegionIDLen UnclassRegionID 的最大长度（字节）
	maxUnclassRegionIDLen = 128
)

// Config 平台初始化配置
//
// 注意：此结构体与 xctx.Platform 有部分字段重叠（HasParent, UnclassRegionID），
// 但用途不同：
//   - Config: 进程级全局配置，包含 PlatformID
//   - xctx.Platform: 请求级 context，用于批量获取平台信息
type Config struct {
	// PlatformID 平台 ID（必填，来自 AUTH 服务）
	//
	// 校验规则：
	//   - 不能为空或纯空白字符
	//   - 不能包含空白字符（空格、制表符等）
	//   - 最大长度 128 字节（len 计算，非 UTF-8 字符数）
	PlatformID string

	// HasParent 是否有上级平台（可选，默认 false）
	HasParent bool

	// UnclassRegionID 未分类区域 ID（可选）
	//
	// 校验规则（仅非空时校验）：
	//   - 不能包含空白字符（空格、制表符等）
	//   - 最大长度 128 字节（len 计算，非 UTF-8 字符数）
	UnclassRegionID string
}

// Validate 验证配置有效性
//
// 校验规则：
//   - PlatformID 不能为空或纯空白字符 → ErrMissingPlatformID
//   - PlatformID 不能包含空白字符 → ErrInvalidPlatformID
//   - PlatformID 长度不超过 128 字节 → ErrInvalidPlatformID
//   - UnclassRegionID（非空时）不能包含空白字符 → ErrInvalidUnclassRegionID
//   - UnclassRegionID（非空时）长度不超过 128 字节 → ErrInvalidUnclassRegionID
func (c Config) Validate() error {
	if strings.TrimSpace(c.PlatformID) == "" {
		return ErrMissingPlatformID
	}
	if strings.ContainsFunc(c.PlatformID, unicode.IsSpace) {
		return fmt.Errorf("%w: contains whitespace", ErrInvalidPlatformID)
	}
	if len(c.PlatformID) > maxPlatformIDLen {
		return fmt.Errorf("%w: exceeds max length %d", ErrInvalidPlatformID, maxPlatformIDLen)
	}

	// 设计决策: UnclassRegionID 用于 HTTP Header / gRPC Metadata 传播（xtenant 包），
	// 必须校验空白字符和长度，防止 header 注入和溢出。允许空值（可选字段）。
	if c.UnclassRegionID != "" {
		if strings.ContainsFunc(c.UnclassRegionID, unicode.IsSpace) {
			return fmt.Errorf("%w: contains whitespace", ErrInvalidUnclassRegionID)
		}
		if len(c.UnclassRegionID) > maxUnclassRegionIDLen {
			return fmt.Errorf("%w: exceeds max length %d", ErrInvalidUnclassRegionID, maxUnclassRegionIDLen)
		}
	}

	return nil
}

// =============================================================================
// 全局状态
// =============================================================================

var (
	// 设计决策: globalConfig 使用 atomic.Pointer 实现无锁读取。
	// 初始化后指针不变（Reset 仅测试可用），读路径无需 sync.RWMutex，
	// 消除了高并发下 RWMutex 的缓存行竞争。
	globalConfig atomic.Pointer[Config]

	// globalMu 仅保护写路径（Init/Reset）的并发序列化
	globalMu sync.Mutex
)

// =============================================================================
// 初始化函数
// =============================================================================

// Init 初始化平台信息
//
// 使用从 AUTH 服务或配置获取的平台信息初始化。
// PlatformID 是必填字段，不能包含空白字符，最大长度 128 字节。
// 此函数应在 main() 中服务启动时调用一次。
//
// 错误优先级：ErrAlreadyInitialized > 配置校验错误。
//
// 设计决策: Init 接受 Config 结构体（直接传值），而非从环境变量读取。
// 这与 xenv.Init()（从环境变量读取）的设计不同，原因是平台信息通常
// 来自 AUTH 服务或配置文件，而非单一环境变量。
func Init(cfg Config) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	// 设计决策: 先检查初始化状态，再校验配置。
	// 确保"已初始化 + 非法配置"场景始终返回 ErrAlreadyInitialized，
	// 避免调用方将状态错误误判为参数错误。
	if globalConfig.Load() != nil {
		return ErrAlreadyInitialized
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	globalConfig.Store(&cfg)

	return nil
}

// MustInit 初始化平台信息，失败时 panic
//
// 适用于初始化失败应该终止程序的场景。
func MustInit(cfg Config) {
	if err := Init(cfg); err != nil {
		panic(err)
	}
}

// =============================================================================
// 全局访问函数
// =============================================================================

// PlatformID 返回当前平台 ID
//
// 需要先调用 Init/MustInit 初始化。
//
// 设计决策: 未初始化时返回空字符串（而非 panic 或 error），与 Go 零值语义一致。
// 需要区分"未初始化"和"初始化为空"的场景应使用 RequirePlatformID()。
func PlatformID() string {
	cfg := globalConfig.Load()
	if cfg == nil {
		return ""
	}
	return cfg.PlatformID
}

// HasParent 返回是否有上级平台
//
// 需要先调用 Init/MustInit 初始化。
//
// 设计决策: 未初始化时返回 false，与 Go 零值语义一致。
// 需要明确判断初始化状态的场景应使用 GetConfig()。
func HasParent() bool {
	cfg := globalConfig.Load()
	if cfg == nil {
		return false
	}
	return cfg.HasParent
}

// UnclassRegionID 返回未分类区域 ID
//
// 需要先调用 Init/MustInit 初始化。
//
// 设计决策: 未初始化时返回空字符串，与 Go 零值语义一致。
// 需要明确判断初始化状态的场景应使用 GetConfig()。
func UnclassRegionID() string {
	cfg := globalConfig.Load()
	if cfg == nil {
		return ""
	}
	return cfg.UnclassRegionID
}

// IsInitialized 返回是否已初始化
func IsInitialized() bool {
	return globalConfig.Load() != nil
}

// =============================================================================
// 带错误检查的访问函数
// =============================================================================

// RequirePlatformID 返回平台 ID，未初始化时返回错误
//
// 适用于必须明确知道平台 ID 的业务场景。
func RequirePlatformID() (string, error) {
	cfg := globalConfig.Load()
	if cfg == nil {
		return "", ErrNotInitialized
	}
	return cfg.PlatformID, nil
}

// GetConfig 返回当前配置的副本
//
// 适用于需要批量获取所有平台信息的场景。
// 如果未初始化，返回空配置和错误。
func GetConfig() (Config, error) {
	cfg := globalConfig.Load()
	if cfg == nil {
		return Config{}, ErrNotInitialized
	}
	return *cfg, nil
}

// =============================================================================
// 测试辅助
// =============================================================================

// Reset 重置全局状态（仅用于测试）
//
// 设计决策: Reset 保留为公开 API 而非限制在 export_test.go，
// 因为 xtenant 等外部包的测试代码也需要重置 xplatform 全局状态。
// 生产代码不应调用此函数，Init 后全局配置应保持不变。
//
// 线程安全，可在并发读取时调用。重置后所有读函数返回零值，
// IsInitialized() 返回 false。
func Reset() {
	globalMu.Lock()
	globalConfig.Store(nil)
	globalMu.Unlock()
}
