package xauth

import (
	"context"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/omeyang/xkit/pkg/util/xlru"
)

// =============================================================================
// TokenCache 双层缓存
// =============================================================================

// TokenCache 提供双层 Token 缓存。
//   - L1: 本地内存缓存（xlru.Cache），内置 LRU 淘汰和 TTL 过期
//   - L2: Redis 缓存（可选），支持多实例共享
type TokenCache struct {
	// L1 本地缓存
	local *xlru.Cache[string, *TokenInfo]

	// L2 远程缓存
	remote CacheStore

	// singleflight 防止并发获取
	sf singleflight.Group

	// 配置
	enableLocal        bool
	refreshThreshold   time.Duration
	enableSingleflight bool
}

// TokenCacheConfig TokenCache 配置。
type TokenCacheConfig struct {
	// Remote 远程缓存存储。
	Remote CacheStore

	// EnableLocal 是否启用本地缓存。
	EnableLocal bool

	// MaxLocalSize 本地缓存最大条目数。
	MaxLocalSize int

	// LocalCacheTTL 本地缓存 TTL。
	// 默认使用 RefreshThreshold * 2，确保本地缓存在 Token 需要刷新前不会自动过期。
	LocalCacheTTL time.Duration

	// RefreshThreshold Token 刷新阈值。
	RefreshThreshold time.Duration

	// EnableSingleflight 是否启用 singleflight。
	EnableSingleflight bool
}

// NewTokenCache 创建 TokenCache。
func NewTokenCache(cfg TokenCacheConfig) *TokenCache {
	if cfg.MaxLocalSize <= 0 {
		cfg.MaxLocalSize = 1000
	}
	if cfg.RefreshThreshold <= 0 {
		cfg.RefreshThreshold = DefaultTokenRefreshThreshold
	}
	if cfg.LocalCacheTTL <= 0 {
		// 默认使用刷新阈值的 2 倍作为本地缓存 TTL
		// 这确保本地缓存不会在 Token 需要刷新前自动过期
		cfg.LocalCacheTTL = cfg.RefreshThreshold * 2
		if cfg.LocalCacheTTL < time.Minute {
			cfg.LocalCacheTTL = time.Minute
		}
	}

	remote := cfg.Remote
	if remote == nil {
		remote = NoopCacheStore{}
	}

	tc := &TokenCache{
		remote:             remote,
		enableLocal:        cfg.EnableLocal,
		refreshThreshold:   cfg.RefreshThreshold,
		enableSingleflight: cfg.EnableSingleflight,
	}

	// 创建 L1 本地缓存
	if cfg.EnableLocal {
		// xlru 内置 LRU 淘汰和 TTL 过期，无需手动管理
		local, err := xlru.New[string, *TokenInfo](xlru.Config{
			Size: cfg.MaxLocalSize,
			TTL:  cfg.LocalCacheTTL,
		})
		if err != nil {
			// MaxLocalSize > 0，不会返回错误
			// 但为安全起见，禁用本地缓存
			tc.enableLocal = false
		} else {
			tc.local = local
		}
	}

	return tc
}

// Get 获取 Token。
// 优先从 L1 获取，未命中时从 L2 获取。
// 返回 (token, needsRefresh, error)
// needsRefresh 为 true 表示 Token 即将过期，建议后台刷新。
func (c *TokenCache) Get(ctx context.Context, tenantID string) (*TokenInfo, bool, error) {
	// L1: 尝试本地缓存
	// xlru 自动处理 TTL 过期，Get 返回 false 表示未命中或已过期
	if c.enableLocal && c.local != nil {
		if token, ok := c.local.Get(tenantID); ok {
			// 检查 Token 是否过期（双重检查：xlru TTL + Token 自身过期时间）
			if token != nil && !token.IsExpired() {
				needsRefresh := token.IsExpiringSoon(c.refreshThreshold)
				return token, needsRefresh, nil
			}
			// Token 已过期，从本地缓存删除
			c.local.Delete(tenantID)
		}
	}

	// L2: 尝试远程缓存
	token, err := c.remote.GetToken(ctx, tenantID)
	if err != nil {
		return nil, false, err
	}

	// 处理 (nil, nil) 情况，视为 cache miss
	if token == nil {
		return nil, false, ErrCacheMiss
	}

	// 回填 L1
	if c.enableLocal {
		c.setLocal(tenantID, token)
	}

	needsRefresh := token.IsExpiringSoon(c.refreshThreshold)
	return token, needsRefresh, nil
}

// Set 设置 Token。
// 同时写入 L1 和 L2。
// ttl 参数为默认 TTL，实际使用的 TTL 会根据 token 的有效期动态计算。
func (c *TokenCache) Set(ctx context.Context, tenantID string, token *TokenInfo, ttl time.Duration) error {
	if token == nil {
		return nil
	}

	// 计算过期时间
	if token.ExpiresAt.IsZero() && token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}
	if token.ObtainedAt.IsZero() {
		token.ObtainedAt = time.Now()
	}

	// L1: 本地缓存
	if c.enableLocal {
		c.setLocal(tenantID, token)
	}

	// 计算实际 TTL：优先使用 token 的有效期，否则使用默认值
	// TTL = token 有效期 - 刷新阈值（确保在 token 过期前会触发刷新）
	actualTTL := ttl
	if token.ExpiresIn > 0 {
		tokenTTL := time.Duration(token.ExpiresIn)*time.Second - c.refreshThreshold
		if tokenTTL > 0 {
			actualTTL = tokenTTL
		} else {
			// 短期 Token：使用实际有效期减去安全边际（最小 10 秒）
			const safetyMargin = 10 * time.Second
			actualTTL = time.Duration(token.ExpiresIn)*time.Second - safetyMargin
			if actualTTL < time.Second {
				// 极短期 Token（< 11秒）：不缓存到远程存储
				return nil
			}
		}
	}

	// L2: 远程缓存
	if err := c.remote.SetToken(ctx, tenantID, token, actualTTL); err != nil {
		// 远程缓存失败不影响返回
		return err
	}

	return nil
}

// setLocal 设置本地缓存。
// xlru 自动处理 LRU 淘汰，无需手动管理容量。
func (c *TokenCache) setLocal(tenantID string, token *TokenInfo) {
	if c.local == nil || token == nil {
		return
	}
	// xlru.Set 自动处理容量限制和 LRU 淘汰
	c.local.Set(tenantID, token)
}

// Delete 删除 Token。
func (c *TokenCache) Delete(ctx context.Context, tenantID string) error {
	// L1
	if c.enableLocal && c.local != nil {
		c.local.Delete(tenantID)
	}

	// L2
	return c.remote.Delete(ctx, tenantID)
}

// GetOrLoad 获取 Token，未命中时调用 loader 加载。
// 使用 singleflight 防止并发加载。
func (c *TokenCache) GetOrLoad(
	ctx context.Context,
	tenantID string,
	loader func(ctx context.Context) (*TokenInfo, error),
	ttl time.Duration,
) (*TokenInfo, error) {
	// 尝试从缓存获取
	// needsRefresh 由调用方（TokenManager）处理，这里仅做缓存查询
	token, _, err := c.Get(ctx, tenantID)
	if err == nil && token != nil && !token.IsExpired() {
		return token, nil
	}

	// 缓存未命中或已过期，需要加载
	if !c.enableSingleflight {
		return c.loadAndSet(ctx, tenantID, loader, ttl)
	}

	// 使用 singleflight
	result, err, _ := c.sf.Do(tenantID, func() (any, error) {
		// double-check: 再次检查缓存
		if t, _, e := c.Get(ctx, tenantID); e == nil && t != nil && !t.IsExpired() {
			return t, nil
		}
		return c.loadAndSet(ctx, tenantID, loader, ttl)
	})

	if err != nil {
		return nil, err
	}
	token, ok := result.(*TokenInfo)
	if !ok {
		return nil, ErrTokenNotFound
	}
	return token, nil
}

// loadAndSet 加载并设置 Token。
func (c *TokenCache) loadAndSet(
	ctx context.Context,
	tenantID string,
	loader func(ctx context.Context) (*TokenInfo, error),
	ttl time.Duration,
) (*TokenInfo, error) {
	token, err := loader(ctx)
	if err != nil {
		return nil, err
	}

	// 设置缓存（失败不影响返回，Token 已加载成功）
	_ = c.Set(ctx, tenantID, token, ttl) //nolint:errcheck // 缓存失败不影响返回

	return token, nil
}

// Clear 清空所有本地缓存。
func (c *TokenCache) Clear() {
	if c.local != nil {
		c.local.Clear()
	}
}

// LocalSize 返回本地缓存条目数。
func (c *TokenCache) LocalSize() int {
	if c.local == nil {
		return 0
	}
	return c.local.Len()
}
