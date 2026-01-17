package xlimit

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"
)

// PodCountProvider 动态 Pod 数量提供器接口
// 用于在本地降级时计算配额分摊
type PodCountProvider interface {
	// GetPodCount 获取当前 Pod 数量
	// 返回的数量用于计算本地配额：本地配额 = 分布式配额 / PodCount
	GetPodCount(ctx context.Context) (int, error)
}

// StaticPodCount 静态 Pod 数量提供器
// 返回固定的 Pod 数量，适用于已知 Pod 数量的场景
type StaticPodCount int

// GetPodCount 返回静态配置的 Pod 数量
func (s StaticPodCount) GetPodCount(_ context.Context) (int, error) {
	if s <= 0 {
		return 1, nil
	}
	return int(s), nil
}

// EnvPodCount 从环境变量获取 Pod 数量
// 支持缓存和默认值
type EnvPodCount struct {
	// EnvVar 环境变量名称
	EnvVar string
	// DefaultCount 默认 Pod 数量（当环境变量未设置或无效时使用）
	DefaultCount int
	// CacheDuration 缓存时长，0 表示每次都读取环境变量
	CacheDuration time.Duration

	mu          sync.RWMutex
	cachedCount int
	cachedAt    time.Time
}

// NewEnvPodCount 创建环境变量 Pod 数量提供器
// envVar: 环境变量名称
// defaultCount: 默认 Pod 数量
func NewEnvPodCount(envVar string, defaultCount int) *EnvPodCount {
	if defaultCount <= 0 {
		defaultCount = 1
	}
	return &EnvPodCount{
		EnvVar:       envVar,
		DefaultCount: defaultCount,
	}
}

// WithCacheDuration 设置缓存时长
func (e *EnvPodCount) WithCacheDuration(d time.Duration) *EnvPodCount {
	e.CacheDuration = d
	return e
}

// GetPodCount 从环境变量获取 Pod 数量
func (e *EnvPodCount) GetPodCount(_ context.Context) (int, error) {
	// 检查缓存
	if e.CacheDuration > 0 {
		e.mu.RLock()
		if e.cachedCount > 0 && time.Since(e.cachedAt) < e.CacheDuration {
			count := e.cachedCount
			e.mu.RUnlock()
			return count, nil
		}
		e.mu.RUnlock()
	}

	// 读取环境变量
	value := os.Getenv(e.EnvVar)
	if value == "" {
		return e.DefaultCount, nil
	}

	count, err := strconv.Atoi(value)
	if err != nil || count <= 0 {
		return e.DefaultCount, nil
	}

	// 更新缓存
	if e.CacheDuration > 0 {
		e.mu.Lock()
		e.cachedCount = count
		e.cachedAt = time.Now()
		e.mu.Unlock()
	}

	return count, nil
}
