package xplatform

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// =============================================================================
// 错误定义
// =============================================================================

var (
	// ErrNotInitialized xplatform 未初始化
	ErrNotInitialized = errors.New("xplatform: not initialized, call Init() first")

	// ErrMissingPlatformID 缺少 PlatformID
	ErrMissingPlatformID = errors.New("xplatform: missing platform_id")

	// ErrAlreadyInitialized 重复初始化
	ErrAlreadyInitialized = errors.New("xplatform: already initialized")
)

// =============================================================================
// 配置结构
// =============================================================================

// Config 平台初始化配置
//
// 注意：此结构体与 xctx.Platform 有部分字段重叠（HasParent, UnclassRegionID），
// 但用途不同：
//   - Config: 进程级全局配置，包含 PlatformID
//   - xctx.Platform: 请求级 context，用于批量获取平台信息
type Config struct {
	// PlatformID 平台 ID（必填，来自 AUTH 服务）
	PlatformID string

	// HasParent 是否有上级平台（可选，默认 false）
	HasParent bool

	// UnclassRegionID 未分类区域 ID（可选）
	UnclassRegionID string
}

// Validate 验证配置有效性
func (c Config) Validate() error {
	if c.PlatformID == "" {
		return ErrMissingPlatformID
	}
	return nil
}

// =============================================================================
// 全局状态
// =============================================================================

var (
	globalConfig Config
	globalMu     sync.RWMutex
	initialized  atomic.Bool
)

// =============================================================================
// 初始化函数
// =============================================================================

// Init 初始化平台信息
//
// 使用从 AUTH 服务或配置获取的平台信息初始化。
// PlatformID 是必填字段。
// 此函数应在 main() 中服务启动时调用一次。
//
// 如果已经初始化过，返回 ErrAlreadyInitialized。
// 如需重新初始化（如测试场景），请先调用 Reset()。
func Init(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	globalMu.Lock()
	defer globalMu.Unlock()

	// 在锁内检查，避免并发读取到空配置
	if initialized.Load() {
		return ErrAlreadyInitialized
	}

	// 先写配置，再设置 initialized 标志
	// 确保并发读取时能看到完整的配置
	globalConfig = cfg
	initialized.Store(true)

	return nil
}

// MustInit 初始化平台信息，失败时 panic
//
// 适用于初始化失败应该终止程序的场景。
func MustInit(cfg Config) {
	if err := Init(cfg); err != nil {
		panic(fmt.Sprintf("xplatform.MustInit: %v", err))
	}
}

// Reset 重置全局状态（仅用于测试）
func Reset() {
	globalMu.Lock()
	globalConfig = Config{}
	initialized.Store(false)
	globalMu.Unlock()
}

// =============================================================================
// 全局访问函数
// =============================================================================

// PlatformID 返回当前平台 ID
//
// 需要先调用 Init/MustInit 初始化。
// 如果未初始化，返回空字符串。
func PlatformID() string {
	if !initialized.Load() {
		return ""
	}
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalConfig.PlatformID
}

// HasParent 返回是否有上级平台
//
// 需要先调用 Init/MustInit 初始化。
// 如果未初始化，返回 false。
func HasParent() bool {
	if !initialized.Load() {
		return false
	}
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalConfig.HasParent
}

// UnclassRegionID 返回未分类区域 ID
//
// 需要先调用 Init/MustInit 初始化。
// 如果未初始化或未设置，返回空字符串。
func UnclassRegionID() string {
	if !initialized.Load() {
		return ""
	}
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalConfig.UnclassRegionID
}

// IsInitialized 返回是否已初始化
func IsInitialized() bool {
	return initialized.Load()
}

// =============================================================================
// 带错误检查的访问函数
// =============================================================================

// RequirePlatformID 返回平台 ID，未初始化时返回错误
//
// 适用于必须明确知道平台 ID 的业务场景。
func RequirePlatformID() (string, error) {
	if !initialized.Load() {
		return "", ErrNotInitialized
	}
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalConfig.PlatformID, nil
}

// GetConfig 返回当前配置的副本
//
// 适用于需要批量获取所有平台信息的场景。
// 如果未初始化，返回空配置和错误。
func GetConfig() (Config, error) {
	if !initialized.Load() {
		return Config{}, ErrNotInitialized
	}
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalConfig, nil
}
