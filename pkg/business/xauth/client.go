package xauth

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync/atomic"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"
)

// =============================================================================
// client 实现
// =============================================================================

// client 实现 Client 接口。
type client struct {
	config      *Config
	options     *Options
	httpClient  *HTTPClient
	tokenMgr    *TokenManager
	platformMgr *PlatformManager
	tokenCache  *TokenCache
	logger      *slog.Logger
	observer    xmetrics.Observer
	closed      atomic.Bool
}

// NewClient 创建新的认证服务客户端。
//
// 示例：
//
//	client, err := xauth.NewClient(&xauth.Config{
//	    Host: "https://auth.example.com",
//	}, xauth.WithCache(redisCache))
func NewClient(cfg *Config, opts ...Option) (Client, error) {
	// 验证并准备配置
	cfg, err := prepareConfig(cfg)
	if err != nil {
		return nil, err
	}

	// 应用选项
	options := applyOptions(opts)

	// 创建 HTTP 客户端
	httpClient, err := createHTTPClient(cfg, options)
	if err != nil {
		return nil, err
	}

	// 构建并返回客户端
	return buildClient(cfg, options, httpClient), nil
}

// prepareConfig 验证并准备配置。
func prepareConfig(cfg *Config) (*Config, error) {
	if cfg == nil {
		return nil, ErrNilConfig
	}

	// 克隆配置，避免外部修改
	cfg = cfg.Clone()

	// 应用默认值
	cfg.ApplyDefaults()

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("xauth: invalid config: %w", err)
	}

	return cfg, nil
}

// createHTTPClient 创建 HTTP 客户端。
func createHTTPClient(cfg *Config, options *Options) (*HTTPClient, error) {
	// 获取 observer
	observer := options.Observer
	if observer == nil {
		observer = xmetrics.NoopObserver{}
	}

	if options.HTTPClient != nil {
		return &HTTPClient{
			client:   options.HTTPClient,
			baseURL:  cfg.Host,
			timeout:  cfg.Timeout,
			observer: observer,
		}, nil
	}

	// 构建 TLS 配置
	tlsConfig := defaultTLSConfig()
	if cfg.TLS != nil {
		var err error
		tlsConfig, err = cfg.TLS.BuildTLSConfig()
		if err != nil {
			return nil, fmt.Errorf("xauth: build tls config failed: %w", err)
		}
	}

	return NewHTTPClient(HTTPClientConfig{
		BaseURL:   cfg.Host,
		Timeout:   cfg.Timeout,
		TLSConfig: tlsConfig,
		Observer:  observer,
	}), nil
}

// resolvedDefaults 保存从 Options/Config 解析后的运行时默认值。
type resolvedDefaults struct {
	logger           *slog.Logger
	observer         xmetrics.Observer
	cache            CacheStore
	refreshThreshold time.Duration
	platformDataTTL  time.Duration
	localCacheTTL    time.Duration
}

// resolveDefaults 从 Options 和 Config 解析运行时默认值。
func resolveDefaults(cfg *Config, options *Options) resolvedDefaults {
	d := resolvedDefaults{
		logger:           options.Logger,
		observer:         options.Observer,
		cache:            options.Cache,
		refreshThreshold: options.TokenRefreshThreshold,
		platformDataTTL:  options.PlatformDataCacheTTL,
		localCacheTTL:    options.LocalCacheTTL,
	}
	if d.logger == nil {
		d.logger = slog.Default()
	}
	if d.observer == nil {
		d.observer = xmetrics.NoopObserver{}
	}
	if d.cache == nil {
		d.cache = NoopCacheStore{}
	}
	if d.refreshThreshold <= 0 {
		d.refreshThreshold = cfg.TokenRefreshThreshold
	}
	if d.platformDataTTL <= 0 {
		d.platformDataTTL = cfg.PlatformDataCacheTTL
	}
	if d.localCacheTTL <= 0 {
		d.localCacheTTL = d.platformDataTTL
	}
	return d
}

// buildClient 构建完整的客户端实例。
func buildClient(cfg *Config, options *Options, httpClient *HTTPClient) *client {
	d := resolveDefaults(cfg, options)

	// 设计决策: ClientSecret == ClientID 是认证服务的内部约定（见 Config.ApplyDefaults），
	// 但如果这是非预期的配置遗漏，日志警告有助于排查。
	if cfg.ClientSecret == cfg.ClientID {
		d.logger.Warn("xauth: client_secret equals client_id; if this is unintended, set Config.ClientSecret explicitly")
	}

	tokenCache := NewTokenCache(TokenCacheConfig{
		Remote:             d.cache,
		EnableLocal:        options.EnableLocalCache,
		MaxLocalSize:       options.LocalCacheMaxSize,
		RefreshThreshold:   d.refreshThreshold,
		EnableSingleflight: options.EnableSingleflight,
	})

	tokenMgr := NewTokenManager(TokenManagerConfig{
		Config:                  cfg,
		HTTP:                    httpClient,
		Cache:                   tokenCache,
		Logger:                  d.logger,
		Observer:                d.observer,
		RefreshThreshold:        d.refreshThreshold,
		EnableBackgroundRefresh: options.EnableBackgroundRefresh,
	})

	enableLocal := options.EnableLocalCache
	platformMgr := NewPlatformManager(PlatformManagerConfig{
		HTTP:           httpClient,
		Cache:          d.cache,
		TokenMgr:       tokenMgr,
		Logger:         d.logger,
		Observer:       d.observer,
		CacheTTL:       d.platformDataTTL,
		EnableLocal:    &enableLocal,
		LocalCacheSize: options.LocalCacheMaxSize,
		LocalCacheTTL:  d.localCacheTTL,
	})

	return &client{
		config:      cfg,
		options:     options,
		httpClient:  httpClient,
		tokenMgr:    tokenMgr,
		platformMgr: platformMgr,
		tokenCache:  tokenCache,
		logger:      d.logger,
		observer:    d.observer,
	}
}

// defaultTLSConfig 返回默认 TLS 配置。
// 默认启用证书验证（安全优先），开发环境可通过 Config.TLS 配置跳过验证。
func defaultTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
}

// GetToken 获取指定租户的访问 Token。
func (c *client) GetToken(ctx context.Context, tenantID string) (string, error) {
	if c.closed.Load() {
		return "", ErrClientClosed
	}

	tenantID = c.resolveTenantID(tenantID)
	if tenantID == "" {
		return "", ErrMissingTenantID
	}

	return c.tokenMgr.GetToken(ctx, tenantID)
}

// VerifyToken 验证 Token 有效性。
func (c *client) VerifyToken(ctx context.Context, token string) (*TokenInfo, error) {
	if c.closed.Load() {
		return nil, ErrClientClosed
	}

	return c.tokenMgr.VerifyToken(ctx, token)
}

// GetPlatformID 获取指定租户的平台 ID。
func (c *client) GetPlatformID(ctx context.Context, tenantID string) (string, error) {
	if c.closed.Load() {
		return "", ErrClientClosed
	}

	tenantID = c.resolveTenantID(tenantID)
	if tenantID == "" {
		return "", ErrMissingTenantID
	}

	return c.platformMgr.GetPlatformID(ctx, tenantID)
}

// HasParentPlatform 判断指定租户是否有父平台。
func (c *client) HasParentPlatform(ctx context.Context, tenantID string) (bool, error) {
	if c.closed.Load() {
		return false, ErrClientClosed
	}

	tenantID = c.resolveTenantID(tenantID)
	if tenantID == "" {
		return false, ErrMissingTenantID
	}

	return c.platformMgr.HasParentPlatform(ctx, tenantID)
}

// GetUnclassRegionID 获取指定租户的未归类组 Region ID。
func (c *client) GetUnclassRegionID(ctx context.Context, tenantID string) (string, error) {
	if c.closed.Load() {
		return "", ErrClientClosed
	}

	tenantID = c.resolveTenantID(tenantID)
	if tenantID == "" {
		return "", ErrMissingTenantID
	}

	return c.platformMgr.GetUnclassRegionID(ctx, tenantID)
}

// Request 发送带认证的 HTTP 请求。
// 注意：响应体会被自动解析到 req.Response 中，此方法不返回 *http.Response。
//
// 如果启用了 EnableAutoRetryOn401 选项，遇到 401 错误时会自动清除 Token 缓存并重试一次。
func (c *client) Request(ctx context.Context, req *AuthRequest) error {
	if c.closed.Load() {
		return ErrClientClosed
	}

	if req == nil {
		return fmt.Errorf("xauth: nil request")
	}

	tenantID := c.resolveTenantID(req.TenantID)
	if tenantID == "" {
		return ErrMissingTenantID
	}

	err := c.doAuthRequest(ctx, tenantID, req)

	// 401 自动重试：清除缓存后重试一次
	if c.options.EnableAutoRetryOn401 && isUnauthorizedError(err) {
		c.logger.Debug("401 received, clearing token cache and retrying",
			slog.String("tenant_id", tenantID),
		)
		_ = c.tokenCache.Delete(ctx, tenantID) //nolint:errcheck // 缓存删除失败不影响重试逻辑
		return c.doAuthRequest(ctx, tenantID, req)
	}

	return err
}

// doAuthRequest 执行带认证的 HTTP 请求。
func (c *client) doAuthRequest(ctx context.Context, tenantID string, req *AuthRequest) error {
	// 设计决策: 绝对 URL 必须使用 HTTPS（除非 AllowInsecure=true），
	// 防止 Bearer Token 通过明文 HTTP 泄露到非安全通道。
	if !c.config.AllowInsecure && strings.HasPrefix(req.URL, "http://") {
		return fmt.Errorf("%w: request URL must use https:// when carrying Bearer token", ErrInsecureHost)
	}

	// 获取 Token
	token, err := c.tokenMgr.GetToken(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("xauth: get token failed: %w", err)
	}

	// 克隆 Headers，避免修改调用方的原始 map
	headers := make(map[string]string, len(req.Headers)+1)
	maps.Copy(headers, req.Headers)
	headers["Authorization"] = "Bearer " + token

	// 发送请求
	return c.httpClient.request(ctx, req.Method, req.URL, headers, req.Body, req.Response)
}

// isUnauthorizedError 检查是否是 401 未授权错误。
func isUnauthorizedError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 401
	}
	return errors.Is(err, ErrUnauthorized)
}

// InvalidateToken 主动使指定租户的 Token 缓存失效。
func (c *client) InvalidateToken(ctx context.Context, tenantID string) error {
	if c.closed.Load() {
		return ErrClientClosed
	}
	tenantID = c.resolveTenantID(tenantID)
	if tenantID == "" {
		return ErrMissingTenantID
	}
	return c.tokenMgr.InvalidateToken(ctx, tenantID)
}

// InvalidatePlatformCache 主动使指定租户的平台数据缓存失效。
// 用于平台信息变更后强制重新获取，避免等待 TTL 过期。
func (c *client) InvalidatePlatformCache(ctx context.Context, tenantID string) error {
	if c.closed.Load() {
		return ErrClientClosed
	}
	tenantID = c.resolveTenantID(tenantID)
	if tenantID == "" {
		return ErrMissingTenantID
	}
	return c.platformMgr.InvalidateCache(ctx, tenantID)
}

// Close 关闭客户端。
// 这会停止后台刷新任务并清理所有本地缓存。
func (c *client) Close() error {
	if c.closed.Swap(true) {
		return nil // 已关闭
	}

	// 停止后台刷新任务
	c.tokenMgr.Stop()

	// 清理本地缓存
	c.tokenCache.Clear()
	c.platformMgr.ClearLocalCache()

	c.logger.Debug("xauth client closed")

	return nil
}

// resolveTenantID 解析租户 ID。
// 如果传入为空，尝试从环境变量获取。
func (c *client) resolveTenantID(tenantID string) string {
	if tenantID != "" {
		return tenantID
	}
	return GetTenantIDFromEnv()
}

// =============================================================================
// 便捷函数
// =============================================================================

// MustNewClient 创建客户端，失败时 panic。
//
// Deprecated: 项目约定构造器应返回 error 而非 panic。请使用 NewClient 并处理错误。
// 此函数保留仅为向后兼容。
func MustNewClient(cfg *Config, opts ...Option) Client {
	c, err := NewClient(cfg, opts...)
	if err != nil {
		panic(fmt.Sprintf("xauth: create client failed: %v", err))
	}
	return c
}
