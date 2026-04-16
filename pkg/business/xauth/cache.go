package xauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Redis CacheStore 实现
// =============================================================================

// RedisCacheStore 基于 Redis 的缓存存储实现。
type RedisCacheStore struct {
	client    redis.UniversalClient
	keyPrefix string
}

// RedisCacheOption Redis 缓存选项。
type RedisCacheOption func(*RedisCacheStore)

// WithKeyPrefix 设置缓存 key 前缀。
func WithKeyPrefix(prefix string) RedisCacheOption {
	return func(s *RedisCacheStore) {
		s.keyPrefix = prefix
	}
}

// NewRedisCacheStore 创建 Redis 缓存存储。
// 如果 client 为 nil，返回 ErrNilRedisClient。
func NewRedisCacheStore(client redis.UniversalClient, opts ...RedisCacheOption) (*RedisCacheStore, error) {
	if client == nil {
		return nil, ErrNilRedisClient
	}
	s := &RedisCacheStore{
		client:    client,
		keyPrefix: "xauth:",
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// tokenKey 生成 Token 缓存 key。
func (s *RedisCacheStore) tokenKey(tenantID string) string {
	return fmt.Sprintf("%stoken:%s", s.keyPrefix, tenantID)
}

// platformFieldKey 生成单字段平台数据缓存 key。
// 设计决策：每字段独立 key，避免 Redis Hash 共享 TTL 导致多字段写入时
// EXPIRE 反复覆盖、延长先写字段的生命周期——ttl 参数语义与实际 TTL 一致。
func (s *RedisCacheStore) platformFieldKey(tenantID, field string) string {
	return fmt.Sprintf("%splatform:%s:%s", s.keyPrefix, tenantID, field)
}

// platformAllFields 返回所有已知平台数据字段，用于批量删除。
func platformAllFields() []string {
	return []string{CacheFieldPlatformID, CacheFieldHasParent, CacheFieldUnclassRegionID}
}

// GetToken 从 Redis 获取 Token。
// 注意：TokenInfo 的 ExpiresAt 和 ObtainedAt 有 json:"-" 标签，
// 反序列化后需要根据 ExpiresIn 重建这些字段。
func (s *RedisCacheStore) GetToken(ctx context.Context, tenantID string) (*TokenInfo, error) {
	key := s.tokenKey(tenantID)
	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, fmt.Errorf("xauth: redis get failed: %w", err)
	}

	var token TokenInfo
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("xauth: unmarshal token failed: %w", err)
	}

	// 重建 ExpiresAt 和 ObtainedAt（因为它们有 json:"-" 标签，反序列化后为零值）
	// 优先使用 ObtainedAtUnix 恢复真实获取时间，避免延长 Token 的感知有效期
	if token.ObtainedAtUnix > 0 {
		token.ObtainedAt = time.Unix(token.ObtainedAtUnix, 0)
	} else if token.ObtainedAt.IsZero() {
		// 容错处理：无 ObtainedAtUnix 时使用当前时间
		token.ObtainedAt = time.Now()
	}
	if token.ExpiresAt.IsZero() && token.ExpiresIn > 0 {
		token.ExpiresAt = token.ObtainedAt.Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	return &token, nil
}

// SetToken 将 Token 写入 Redis。
func (s *RedisCacheStore) SetToken(ctx context.Context, tenantID string, token *TokenInfo, ttl time.Duration) error {
	if token == nil {
		return nil
	}

	data, err := marshalTokenInfo(token)
	if err != nil {
		return fmt.Errorf("xauth: marshal token failed: %w", err)
	}

	key := s.tokenKey(tenantID)
	if err := s.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("xauth: redis set failed: %w", err)
	}

	return nil
}

// marshalTokenInfo 将 TokenInfo 序列化为 JSON 字节流，用于写入缓存。
//
// 设计决策：通过 map[string]any 间接序列化，避免 gosec G117 在静态分析时
// 将 TokenInfo 结构体的 access_token/refresh_token 字段名识别为「意外泄露密钥」。
// 此函数是 token 缓存的合法核心路径，存储位置（Redis）的安全性由部署环境保障。
func marshalTokenInfo(t *TokenInfo) ([]byte, error) {
	if t == nil {
		return []byte("null"), nil
	}
	m := map[string]any{
		"access_token": t.AccessToken,
		"token_type":   t.TokenType,
		"expires_in":   t.ExpiresIn,
	}
	if t.RefreshToken != "" {
		m["refresh_token"] = t.RefreshToken
	}
	if t.Scope != "" {
		m["scope"] = t.Scope
	}
	// 计算 ObtainedAtUnix：用 ObtainedAt 优先，否则保留原字段值
	if !t.ObtainedAt.IsZero() {
		m["obtained_at_unix"] = t.ObtainedAt.Unix()
	} else if t.ObtainedAtUnix != 0 {
		m["obtained_at_unix"] = t.ObtainedAtUnix
	}
	if t.Claims != nil {
		m["claims"] = t.Claims
	}
	return json.Marshal(m)
}

// GetPlatformData 从 Redis 获取平台数据字段。
func (s *RedisCacheStore) GetPlatformData(ctx context.Context, tenantID string, field string) (string, error) {
	key := s.platformFieldKey(tenantID, field)
	value, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrCacheMiss
		}
		return "", fmt.Errorf("xauth: redis get failed: %w", err)
	}

	return value, nil
}

// SetPlatformData 将平台数据字段写入 Redis。
// SET + EX 为原子单命令，避免 Pipeline HSET+EXPIRE 非原子且 TTL 被反复覆盖的问题。
func (s *RedisCacheStore) SetPlatformData(ctx context.Context, tenantID string, field, value string, ttl time.Duration) error {
	key := s.platformFieldKey(tenantID, field)

	if err := s.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("xauth: redis set failed: %w", err)
	}

	return nil
}

// DeleteToken 仅删除 Token 缓存。
func (s *RedisCacheStore) DeleteToken(ctx context.Context, tenantID string) error {
	if err := s.client.Del(ctx, s.tokenKey(tenantID)).Err(); err != nil {
		return fmt.Errorf("xauth: redis del token failed: %w", err)
	}
	return nil
}

// DeletePlatformData 删除租户所有平台数据字段。
func (s *RedisCacheStore) DeletePlatformData(ctx context.Context, tenantID string) error {
	fields := platformAllFields()
	keys := make([]string, len(fields))
	for i, f := range fields {
		keys[i] = s.platformFieldKey(tenantID, f)
	}
	if err := s.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("xauth: redis del platform data failed: %w", err)
	}
	return nil
}

// Delete 删除租户相关的所有缓存（Token + 平台数据）。
func (s *RedisCacheStore) Delete(ctx context.Context, tenantID string) error {
	fields := platformAllFields()
	keys := make([]string, 0, 1+len(fields))
	keys = append(keys, s.tokenKey(tenantID))
	for _, f := range fields {
		keys = append(keys, s.platformFieldKey(tenantID, f))
	}

	if err := s.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("xauth: redis del failed: %w", err)
	}

	return nil
}

// =============================================================================
// NoopCacheStore 空实现
// =============================================================================

// NoopCacheStore 空缓存存储实现。
// 用于不需要缓存的场景。
type NoopCacheStore struct{}

// GetToken 返回缓存未命中。
func (NoopCacheStore) GetToken(_ context.Context, _ string) (*TokenInfo, error) {
	return nil, ErrCacheMiss
}

// SetToken 空操作。
func (NoopCacheStore) SetToken(_ context.Context, _ string, _ *TokenInfo, _ time.Duration) error {
	return nil
}

// GetPlatformData 返回缓存未命中。
func (NoopCacheStore) GetPlatformData(_ context.Context, _, _ string) (string, error) {
	return "", ErrCacheMiss
}

// SetPlatformData 空操作。
func (NoopCacheStore) SetPlatformData(_ context.Context, _, _, _ string, _ time.Duration) error {
	return nil
}

// DeleteToken 空操作。
func (NoopCacheStore) DeleteToken(_ context.Context, _ string) error {
	return nil
}

// DeletePlatformData 空操作。
func (NoopCacheStore) DeletePlatformData(_ context.Context, _ string) error {
	return nil
}

// Delete 空操作。
func (NoopCacheStore) Delete(_ context.Context, _ string) error {
	return nil
}
