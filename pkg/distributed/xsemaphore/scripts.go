package xsemaphore

import (
	"context"
	_ "embed"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
)

// =============================================================================
// 脚本状态码常量
// =============================================================================

const (
	// scriptStatusOK 操作成功
	scriptStatusOK = 0
	// scriptStatusCapacityFull 全局容量已满
	scriptStatusCapacityFull = 1
	// scriptStatusTenantQuotaExceeded 租户配额已满
	scriptStatusTenantQuotaExceeded = 2
	// scriptStatusNotHeld 许可未持有
	scriptStatusNotHeld = 3
)

// =============================================================================
// Lua 脚本嵌入
// =============================================================================

var (
	//go:embed lua/acquire.lua
	acquireLuaSource string

	//go:embed lua/release.lua
	releaseLuaSource string

	//go:embed lua/extend.lua
	extendLuaSource string

	//go:embed lua/query.lua
	queryLuaSource string
)

// =============================================================================
// 脚本管理器 - 单例模式确保脚本只创建一次
// =============================================================================

// scripts 持有所有 Redis 脚本实例
type scripts struct {
	acquire *redis.Script
	release *redis.Script
	extend  *redis.Script
	query   *redis.Script
}

var (
	globalScripts     *scripts
	globalScriptsOnce sync.Once
)

// getScripts 获取脚本实例（线程安全的单例）
func getScripts() *scripts {
	globalScriptsOnce.Do(func() {
		globalScripts = &scripts{
			acquire: redis.NewScript(acquireLuaSource),
			release: redis.NewScript(releaseLuaSource),
			extend:  redis.NewScript(extendLuaSource),
			query:   redis.NewScript(queryLuaSource),
		}
	})
	return globalScripts
}

// =============================================================================
// 脚本预热
// =============================================================================

// WarmupScripts 预热脚本，将脚本加载到 Redis 缓存中
//
// 建议在应用启动时调用，避免首次执行时的编译开销。
// 如果 Redis 不可用，返回错误但不影响后续使用（会在首次执行时重试）。
// 如果 ctx 为 nil，返回 [ErrNilContext]；如果 client 为 nil，返回 [ErrNilClient]。
func WarmupScripts(ctx context.Context, client redis.UniversalClient) error {
	if ctx == nil {
		return ErrNilContext
	}
	if client == nil {
		return ErrNilClient
	}

	s := getScripts()

	// 使用 SCRIPT LOAD 预加载脚本
	// redis.Script.Load 会执行 SCRIPT LOAD 并缓存 SHA
	// 设计决策: 顺序加载而非 Pipeline 批量加载。启动时一次性操作，额外 3 个 RTT（~3ms）
	// 不影响服务启动时间，且顺序加载更易于定位失败的脚本。
	if err := s.acquire.Load(ctx, client).Err(); err != nil {
		return fmt.Errorf("load acquire script: %w", err)
	}
	if err := s.release.Load(ctx, client).Err(); err != nil {
		return fmt.Errorf("load release script: %w", err)
	}
	if err := s.extend.Load(ctx, client).Err(); err != nil {
		return fmt.Errorf("load extend script: %w", err)
	}
	if err := s.query.Load(ctx, client).Err(); err != nil {
		return fmt.Errorf("load query script: %w", err)
	}

	return nil
}
