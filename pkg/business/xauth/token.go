package xauth

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"
)

// =============================================================================
// TokenManager Token 管理器
// =============================================================================

// TokenManager 负责 Token 的获取、刷新和验证。
type TokenManager struct {
	config   *Config
	http     *HTTPClient
	cache    *TokenCache
	logger   *slog.Logger
	observer xmetrics.Observer

	// 配置
	refreshThreshold        time.Duration
	enableBackgroundRefresh bool

	// 后台刷新去重：防止同一租户重复刷新
	refreshing sync.Map // map[tenantID]struct{}

	// 用于 graceful shutdown，取消后台刷新
	ctx    context.Context
	cancel context.CancelFunc
}

// TokenManagerConfig TokenManager 配置。
type TokenManagerConfig struct {
	Config                  *Config
	HTTP                    *HTTPClient
	Cache                   *TokenCache
	Logger                  *slog.Logger
	Observer                xmetrics.Observer
	RefreshThreshold        time.Duration
	EnableBackgroundRefresh bool
}

// NewTokenManager 创建 TokenManager。
func NewTokenManager(cfg TokenManagerConfig) *TokenManager {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Observer == nil {
		cfg.Observer = xmetrics.NoopObserver{}
	}
	if cfg.RefreshThreshold <= 0 {
		cfg.RefreshThreshold = cfg.Config.TokenRefreshThreshold
	}
	if cfg.RefreshThreshold <= 0 {
		cfg.RefreshThreshold = DefaultTokenRefreshThreshold
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TokenManager{
		config:                  cfg.Config,
		http:                    cfg.HTTP,
		cache:                   cfg.Cache,
		logger:                  cfg.Logger,
		observer:                cfg.Observer,
		refreshThreshold:        cfg.RefreshThreshold,
		enableBackgroundRefresh: cfg.EnableBackgroundRefresh,
		ctx:                     ctx,
		cancel:                  cancel,
	}
}

// GetToken 获取指定租户的 Token。
// 优先从缓存获取，缓存未命中或即将过期时获取新 Token。
func (m *TokenManager) GetToken(ctx context.Context, tenantID string) (string, error) {
	// 开始观测
	ctx, span := xmetrics.Start(ctx, m.observer, xmetrics.SpanOptions{
		Component: "xauth",
		Operation: "GetToken",
		Kind:      xmetrics.KindClient,
		Attrs:     []xmetrics.Attr{{Key: "tenant_id", Value: tenantID}},
	})
	var err error
	defer func() {
		span.End(xmetrics.Result{Err: err})
	}()

	// 使用缓存的 GetOrLoad
	token, err := m.cache.GetOrLoad(ctx, tenantID, func(ctx context.Context) (*TokenInfo, error) {
		return m.obtainToken(ctx, tenantID)
	}, m.calculateTokenTTL(nil))

	if err != nil {
		return "", err
	}

	// 检查是否需要后台刷新（去重：防止同一租户重复刷新）
	if m.enableBackgroundRefresh && token.IsExpiringSoon(m.refreshThreshold) {
		if _, loaded := m.refreshing.LoadOrStore(tenantID, struct{}{}); !loaded {
			go m.backgroundRefresh(tenantID)
		}
	}

	return token.AccessToken, nil
}

// obtainToken 获取新 Token。
// 优先使用 API Key，失败后使用 client_credentials。
func (m *TokenManager) obtainToken(ctx context.Context, tenantID string) (*TokenInfo, error) {
	// 优先使用 API Key
	if m.config.APIKey != "" {
		token, err := m.obtainAPIKeyToken(ctx, tenantID)
		if err == nil {
			return token, nil
		}
		m.logger.Debug("API key token failed, trying client credentials",
			slog.String("tenant_id", tenantID),
			slog.String("error", err.Error()),
		)
	}

	// 使用 client_credentials
	return m.obtainClientToken(ctx, tenantID)
}

// obtainClientToken 使用 client_credentials 获取 Token。
func (m *TokenManager) obtainClientToken(ctx context.Context, tenantID string) (*TokenInfo, error) {
	// 构建 URL
	params := url.Values{
		"client_id":     {m.config.ClientID},
		"client_secret": {m.config.ClientSecret},
		"grant_type":    {"client_credentials"},
	}
	if tenantID != "" {
		params.Set("project_id", tenantID)
	}

	path := PathTokenObtain + "?" + params.Encode()

	var token TokenInfo
	if err := m.http.Post(ctx, path, nil, nil, &token); err != nil {
		return nil, fmt.Errorf("obtain client token: %w", err)
	}

	if token.AccessToken == "" {
		return nil, ErrTokenNotFound
	}

	// 设置时间信息
	token.ObtainedAt = time.Now()
	if token.ExpiresIn > 0 {
		token.ExpiresAt = token.ObtainedAt.Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	m.logger.Debug("obtained client token",
		slog.String("tenant_id", tenantID),
		slog.Int64("expires_in", token.ExpiresIn),
	)

	return &token, nil
}

// obtainAPIKeyToken 使用 API Key 获取 Token。
func (m *TokenManager) obtainAPIKeyToken(ctx context.Context, tenantID string) (*TokenInfo, error) {
	if m.config.APIKey == "" {
		return nil, ErrMissingAPIKey
	}

	body := map[string]string{"apiKey": m.config.APIKey}

	var resp APIAccessTokenResponse
	if err := m.http.Post(ctx, PathAPIAccessToken, nil, body, &resp); err != nil {
		return nil, fmt.Errorf("obtain api key token: %w", err)
	}

	if resp.Data.AccessToken == "" {
		return nil, ErrTokenNotFound
	}

	token := &TokenInfo{
		AccessToken: resp.Data.AccessToken,
		ObtainedAt:  time.Now(),
		// API Key Token 通常有较长的有效期，默认设置为 6 小时
		ExpiresIn: int64(DefaultTokenCacheTTL.Seconds()),
	}
	token.ExpiresAt = token.ObtainedAt.Add(DefaultTokenCacheTTL)

	m.logger.Debug("obtained api key token",
		slog.String("tenant_id", tenantID),
	)

	return token, nil
}

// RefreshToken 刷新 Token。
func (m *TokenManager) RefreshToken(ctx context.Context, tenantID string, currentToken *TokenInfo) (*TokenInfo, error) {
	// 如果有 refresh_token，尝试使用它
	if currentToken != nil && currentToken.RefreshToken != "" {
		token, err := m.refreshWithRefreshToken(ctx, tenantID, currentToken)
		if err == nil {
			return token, nil
		}
		m.logger.Debug("refresh token failed, obtaining new token",
			slog.String("tenant_id", tenantID),
			slog.String("error", err.Error()),
		)
	}

	// 获取新 Token
	return m.obtainToken(ctx, tenantID)
}

// refreshWithRefreshToken 使用 refresh_token 刷新 Token。
func (m *TokenManager) refreshWithRefreshToken(ctx context.Context, tenantID string, currentToken *TokenInfo) (*TokenInfo, error) {
	if currentToken == nil || currentToken.RefreshToken == "" {
		return nil, ErrRefreshTokenNotFound
	}

	params := url.Values{
		"client_id":     {m.config.ClientID},
		"client_secret": {m.config.ClientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {currentToken.RefreshToken},
	}

	path := PathTokenObtain + "?" + params.Encode()

	headers := map[string]string{
		"Authorization": "Bearer " + currentToken.AccessToken,
	}

	var token TokenInfo
	if err := m.http.Post(ctx, path, headers, nil, &token); err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}

	if token.AccessToken == "" {
		return nil, ErrTokenNotFound
	}

	// 设置时间信息
	token.ObtainedAt = time.Now()
	if token.ExpiresIn > 0 {
		token.ExpiresAt = token.ObtainedAt.Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	m.logger.Info("refreshed token",
		slog.String("tenant_id", tenantID),
		slog.Int64("expires_in", token.ExpiresIn),
	)

	return &token, nil
}

// VerifyToken 验证 Token。
func (m *TokenManager) VerifyToken(ctx context.Context, token string) (*TokenInfo, error) {
	if token == "" {
		return nil, ErrMissingToken
	}

	// 开始观测
	ctx, span := xmetrics.Start(ctx, m.observer, xmetrics.SpanOptions{
		Component: "xauth",
		Operation: "VerifyToken",
		Kind:      xmetrics.KindClient,
	})
	var verifyErr error
	defer func() {
		span.End(xmetrics.Result{Err: verifyErr})
	}()

	params := url.Values{
		"token": {token},
	}
	path := PathTokenVerify + "?" + params.Encode()

	var resp VerifyResponse
	if err := m.http.Post(ctx, path, nil, nil, &resp); err != nil {
		verifyErr = fmt.Errorf("verify token: %w", err)
		return nil, verifyErr
	}

	if !resp.Data.Active {
		verifyErr = ErrTokenInvalid
		return nil, verifyErr
	}

	// 构建 TokenInfo
	tokenInfo := &TokenInfo{
		AccessToken: token,
		ExpiresAt:   time.Unix(resp.Data.Exp, 0),
	}

	return tokenInfo, nil
}

// backgroundRefresh 后台刷新 Token。
func (m *TokenManager) backgroundRefresh(tenantID string) {
	// 完成后从去重 map 中删除
	defer m.refreshing.Delete(tenantID)

	// 使用带超时的子 context，继承父级取消信号
	// 这样在 Stop() 调用时，后台刷新会被取消
	ctx, cancel := context.WithTimeout(m.ctx, m.config.Timeout)
	defer cancel()

	// 检查是否已经被取消
	select {
	case <-ctx.Done():
		m.logger.Debug("background refresh: canceled",
			slog.String("tenant_id", tenantID),
		)
		return
	default:
	}

	// 获取当前 Token
	currentToken, _, err := m.cache.Get(ctx, tenantID)
	if err != nil {
		m.logger.Debug("background refresh: get current token failed",
			slog.String("tenant_id", tenantID),
			slog.String("error", err.Error()),
		)
		return
	}

	// 刷新 Token
	newToken, err := m.RefreshToken(ctx, tenantID, currentToken)
	if err != nil {
		// 检查是否因取消导致的错误
		if m.ctx.Err() != nil {
			m.logger.Debug("background refresh: canceled during refresh",
				slog.String("tenant_id", tenantID),
			)
			return
		}
		m.logger.Warn("background refresh: refresh token failed",
			slog.String("tenant_id", tenantID),
			slog.String("error", err.Error()),
		)
		return
	}

	// 更新缓存
	if err := m.cache.Set(ctx, tenantID, newToken, m.calculateTokenTTL(newToken)); err != nil {
		m.logger.Warn("background refresh: set cache failed",
			slog.String("tenant_id", tenantID),
			slog.String("error", err.Error()),
		)
	}

	m.logger.Debug("background refresh: token refreshed",
		slog.String("tenant_id", tenantID),
	)
}

// Stop 停止 TokenManager，取消所有后台刷新任务。
// 这是 graceful shutdown 的一部分，应在 client.Close() 时调用。
func (m *TokenManager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// calculateTokenTTL 计算 Token 缓存 TTL。
func (m *TokenManager) calculateTokenTTL(token *TokenInfo) time.Duration {
	if token == nil || token.ExpiresIn <= 0 {
		return DefaultTokenCacheTTL
	}

	// Token 缓存 TTL = Token 有效期 - 刷新阈值
	// 这样在 Token 即将过期前会触发刷新
	ttl := time.Duration(token.ExpiresIn)*time.Second - m.refreshThreshold
	if ttl <= 0 {
		ttl = time.Duration(token.ExpiresIn) * time.Second / 2
	}
	return ttl
}

// InvalidateToken 使 Token 失效（从缓存删除）。
func (m *TokenManager) InvalidateToken(ctx context.Context, tenantID string) error {
	return m.cache.Delete(ctx, tenantID)
}
