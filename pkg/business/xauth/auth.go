package xauth

import (
	"context"
	"time"
)

// =============================================================================
// Client 接口
// =============================================================================

// Client 定义认证服务客户端接口。
type Client interface {
	// GetToken 获取指定租户的访问 Token。
	// 优先从缓存获取，缓存未命中或 Token 即将过期时自动刷新。
	//
	// tenantID 为空时，尝试从环境变量 TENANT_PROJECT_ID 获取。
	GetToken(ctx context.Context, tenantID string) (string, error)

	// VerifyToken 验证 Token 有效性。
	// 返回 Token 信息，验证失败返回错误。
	VerifyToken(ctx context.Context, token string) (*TokenInfo, error)

	// GetPlatformID 获取指定租户的平台 ID。
	// 结果会被缓存。
	//
	// tenantID 为空时，尝试从环境变量 TENANT_PROJECT_ID 获取。
	GetPlatformID(ctx context.Context, tenantID string) (string, error)

	// HasParentPlatform 判断指定租户是否有父平台。
	// 结果会被缓存。
	//
	// tenantID 为空时，尝试从环境变量 TENANT_PROJECT_ID 获取。
	HasParentPlatform(ctx context.Context, tenantID string) (bool, error)

	// GetUnclassRegionID 获取指定租户的未归类组 Region ID。
	// 结果会被缓存。
	//
	// tenantID 为空时，尝试从环境变量 TENANT_PROJECT_ID 获取。
	GetUnclassRegionID(ctx context.Context, tenantID string) (string, error)

	// Request 发送带认证的 HTTP 请求。
	// 自动添加 Authorization 头。
	// 响应体会被自动解析到 req.Response 中。
	Request(ctx context.Context, req *AuthRequest) error

	// InvalidateToken 主动使指定租户的 Token 缓存失效。
	// 用于 Token 被服务端撤销或权限变更后强制重新获取。
	InvalidateToken(ctx context.Context, tenantID string) error

	// InvalidatePlatformCache 主动使指定租户的平台数据缓存失效。
	// 用于平台信息变更后强制重新获取，避免等待 TTL 过期。
	InvalidatePlatformCache(ctx context.Context, tenantID string) error

	// Close 关闭客户端，释放资源。
	// ctx 当前未使用，保留参数是为了与项目统一生命周期接口约定（D-02）一致，
	// 并为将来带超时的优雅关闭预留扩展空间。
	Close(ctx context.Context) error
}

// =============================================================================
// Token 相关类型
// =============================================================================

// TokenInfo Token 信息。
type TokenInfo struct {
	// AccessToken 访问令牌。
	AccessToken string `json:"access_token"`

	// TokenType Token 类型（通常为 "bearer"）。
	TokenType string `json:"token_type"`

	// RefreshToken 刷新令牌（可选）。
	RefreshToken string `json:"refresh_token,omitempty"`

	// ExpiresIn Token 有效期（秒）。
	ExpiresIn int64 `json:"expires_in"`

	// Scope Token 权限范围。
	Scope string `json:"scope,omitempty"`

	// ExpiresAt Token 过期时间（计算得出）。
	ExpiresAt time.Time `json:"-"`

	// ObtainedAt Token 获取时间。
	ObtainedAt time.Time `json:"-"`

	// ObtainedAtUnix 用于 Redis 序列化的获取时间戳（秒）。
	// 因为 ObtainedAt 有 json:"-" 标签，反序列化后会丢失，
	// 此字段用于在 Redis 中保存并恢复真实的获取时间。
	ObtainedAtUnix int64 `json:"obtained_at_unix,omitempty"`
}

// IsExpired 判断 Token 是否已过期。
func (t *TokenInfo) IsExpired() bool {
	if t == nil || t.AccessToken == "" {
		return true
	}
	return time.Now().After(t.ExpiresAt)
}

// IsExpiringSoon 判断 Token 是否即将过期。
// threshold 为过期阈值，Token 剩余有效期小于此值时返回 true。
func (t *TokenInfo) IsExpiringSoon(threshold time.Duration) bool {
	if t == nil || t.AccessToken == "" {
		return true
	}
	return time.Now().Add(threshold).After(t.ExpiresAt)
}

// TTL 返回 Token 剩余有效期。
func (t *TokenInfo) TTL() time.Duration {
	if t == nil {
		return 0
	}
	ttl := time.Until(t.ExpiresAt)
	if ttl < 0 {
		return 0
	}
	return ttl
}

// VerifyResponse Token 验证响应。
type VerifyResponse struct {
	// Code 响应码。
	Code int `json:"code"`

	// Message 响应消息。
	Message string `json:"message"`

	// Data Token 详细信息。
	Data VerifyData `json:"data"`
}

// VerifyData Token 验证详细信息。
// 设计决策: JSON tag 与认证服务 API 响应字段名对齐（混用 camelCase 和 snake_case），
// 并非自定义命名，不可随意修改。
type VerifyData struct {
	// Active Token 是否有效。
	Active bool `json:"active"`

	// IdentityType 身份类型（"client_credentials" 或 "user"）。
	IdentityType string `json:"identity_type"`

	// Exp Token 过期时间戳。
	Exp int64 `json:"exp"`

	// Scope 权限范围。
	Scope []string `json:"scope"`

	// Regions 可访问的 Region 列表。
	Regions []string `json:"regions"`

	// Authorities 权限列表。
	Authorities []string `json:"authorities"`

	// ClientID 客户端 ID（client_credentials 类型）。
	ClientID string `json:"client_id,omitempty"`

	// UserID 用户 ID（user 类型）。
	UserID string `json:"user_id,omitempty"`

	// TenantID 租户 ID。
	TenantID string `json:"tenant_id,omitempty"`

	// TimeZone 时区（user 类型）。
	TimeZone string `json:"timeZone,omitempty"`

	// Language 语言（user 类型）。
	Language string `json:"language,omitempty"`

	// Name 用户名（user 类型）。
	Name string `json:"name,omitempty"`

	// NickName 昵称（user 类型）。
	NickName string `json:"nickName,omitempty"`
}

// =============================================================================
// 请求相关类型
// =============================================================================

// AuthRequest 带认证的 HTTP 请求。
type AuthRequest struct {
	// TenantID 租户 ID。
	// 为空时尝试从环境变量获取。
	TenantID string

	// URL 请求 URL。
	URL string

	// Method HTTP 方法（GET, POST, PUT, DELETE 等）。
	Method string

	// Headers 自定义请求头。
	Headers map[string]string

	// Body 请求体（将被 JSON 序列化）。
	Body any

	// Response 响应体指针（将被 JSON 反序列化）。
	Response any
}

// =============================================================================
// 平台信息相关类型
// =============================================================================

// PlatformInfo 平台信息。
type PlatformInfo struct {
	// ID 平台 ID。
	ID string `json:"id"`

	// HasParent 是否有父平台。
	HasParent bool `json:"has_parent"`

	// UnclassRegionID 未归类组 Region ID。
	UnclassRegionID string `json:"unclass_region_id"`
}

// =============================================================================
// CacheStore 接口
// =============================================================================

// CacheStore 定义缓存存储接口。
// 支持 Token 和平台数据的缓存。
type CacheStore interface {
	// GetToken 从缓存获取 Token。
	// 返回 ErrCacheMiss 表示缓存未命中。
	// 注意：实现者不应返回 (nil, nil)，应返回 (nil, ErrCacheMiss)。
	GetToken(ctx context.Context, tenantID string) (*TokenInfo, error)

	// SetToken 将 Token 写入缓存。
	// ttl 为缓存有效期。
	SetToken(ctx context.Context, tenantID string, token *TokenInfo, ttl time.Duration) error

	// GetPlatformData 从缓存获取平台数据。
	// field 为字段名（platform_id, unclass_region_id, has_parent）。
	// 返回 ErrCacheMiss 表示缓存未命中。
	GetPlatformData(ctx context.Context, tenantID string, field string) (string, error)

	// SetPlatformData 将平台数据写入缓存。
	SetPlatformData(ctx context.Context, tenantID string, field, value string, ttl time.Duration) error

	// DeleteToken 仅删除 Token 缓存。
	// 用于 Token 失效时不影响平台数据缓存。
	DeleteToken(ctx context.Context, tenantID string) error

	// DeletePlatformData 仅删除平台数据缓存。
	// 用于平台信息变更时不影响 Token 缓存。
	DeletePlatformData(ctx context.Context, tenantID string) error

	// Delete 删除租户的所有缓存（Token + 平台数据）。
	Delete(ctx context.Context, tenantID string) error
}

// =============================================================================
// API 响应类型（内部使用，映射服务端 JSON 响应）
// =============================================================================

// APIAccessTokenResponse API Key 获取 Token 的响应。
type APIAccessTokenResponse struct {
	// Code 响应码。
	Code int `json:"code"`

	// Message 响应消息。
	Message string `json:"message"`

	// Data Token 数据。
	Data struct {
		AccessToken string `json:"access_token"`
	} `json:"data"`
}

// PlatformSelfResponse 获取平台信息的响应。
type PlatformSelfResponse struct {
	// Data 平台数据。
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}

// UnclassRegionResponse 获取未归类组 Region 的响应。
type UnclassRegionResponse struct {
	// Data Region 数据。
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}

// HasParentResponse 判断是否有父平台的响应。
type HasParentResponse struct {
	// Data 是否有父平台。
	Data bool `json:"data"`
}
