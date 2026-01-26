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
func NewRedisCacheStore(client redis.UniversalClient, opts ...RedisCacheOption) *RedisCacheStore {
	s := &RedisCacheStore{
		client:    client,
		keyPrefix: "xauth:",
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// tokenKey 生成 Token 缓存 key。
func (s *RedisCacheStore) tokenKey(tenantID string) string {
	return fmt.Sprintf("%stoken:%s", s.keyPrefix, tenantID)
}

// platformKey 生成平台数据缓存 key（Hash key）。
func (s *RedisCacheStore) platformKey(tenantID string) string {
	return fmt.Sprintf("%splatform:%s", s.keyPrefix, tenantID)
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
		// 兼容旧数据：无 ObtainedAtUnix 时使用当前时间
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

	// 序列化前设置 Unix 时间戳，以便反序列化时恢复真实获取时间
	if !token.ObtainedAt.IsZero() {
		token.ObtainedAtUnix = token.ObtainedAt.Unix()
	}

	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("xauth: marshal token failed: %w", err)
	}

	key := s.tokenKey(tenantID)
	if err := s.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("xauth: redis set failed: %w", err)
	}

	return nil
}

// GetPlatformData 从 Redis Hash 获取平台数据。
func (s *RedisCacheStore) GetPlatformData(ctx context.Context, tenantID string, field string) (string, error) {
	key := s.platformKey(tenantID)
	value, err := s.client.HGet(ctx, key, field).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrCacheMiss
		}
		return "", fmt.Errorf("xauth: redis hget failed: %w", err)
	}

	return value, nil
}

// SetPlatformData 将平台数据写入 Redis Hash。
func (s *RedisCacheStore) SetPlatformData(ctx context.Context, tenantID string, field, value string, ttl time.Duration) error {
	key := s.platformKey(tenantID)

	pipe := s.client.Pipeline()
	pipe.HSet(ctx, key, field, value)
	if ttl > 0 {
		pipe.Expire(ctx, key, ttl)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("xauth: redis hset failed: %w", err)
	}

	return nil
}

// Delete 删除租户相关的所有缓存。
func (s *RedisCacheStore) Delete(ctx context.Context, tenantID string) error {
	keys := []string{
		s.tokenKey(tenantID),
		s.platformKey(tenantID),
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

// Delete 空操作。
func (NoopCacheStore) Delete(_ context.Context, _ string) error {
	return nil
}
