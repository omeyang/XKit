package xauth

import (
	"context"
	"errors"
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

	// 用于 graceful shutdown，取消后台刷新并等待完成
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
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
// Config、HTTP 和 Cache 为必填依赖，缺失时返回错误。
func NewTokenManager(cfg TokenManagerConfig) (*TokenManager, error) {
	if cfg.Config == nil {
		return nil, ErrNilConfig
	}
	if cfg.HTTP == nil {
		return nil, ErrNilHTTPClient
	}
	if cfg.Cache == nil {
		return nil, ErrNilCache
	}

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
	}, nil
}

// GetToken 获取指定租户的 Token。
// 优先从缓存获取，缓存未命中或即将过期时获取新 Token。
func (m *TokenManager) GetToken(ctx context.Context, tenantID string) (string, error) {
	// 开始观测
	ctx, span := xmetrics.Start(ctx, m.observer, xmetrics.SpanOptions{
		Component: MetricsComponent,
		Operation: MetricsOpGetToken,
		Kind:      xmetrics.KindClient,
		Attrs:     []xmetrics.Attr{{Key: MetricsAttrTenantID, Value: tenantID}},
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
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				m.backgroundRefresh(tenantID)
			}()
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
	// 凭据通过 POST body 传递，避免在 URL 中暴露（RFC 6749 §2.3.1）
	form := url.Values{
		"client_id":     {m.config.ClientID},
		"client_secret": {m.config.ClientSecret},
		"grant_type":    {"client_credentials"},
	}
	if tenantID != "" {
		form.Set("project_id", tenantID)
	}

	headers := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	}

	var token TokenInfo
	if err := m.http.Post(ctx, PathTokenObtain, headers, form.Encode(), &token); err != nil {
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

	// 设计决策: API Key Token 响应中不包含 expires_in 字段（APIAccessTokenResponse.Data 仅有 access_token），
	// 因此使用 DefaultTokenCacheTTL（6 小时）作为默认有效期。如果服务端实际有效期短于此值，
	// 客户端会在 Token 过期后收到 401 并通过 EnableAutoRetryOn401 机制自动重试。
	// 如需调整此默认值，可通过 Config.APIKeyTokenTTL 配置（未来扩展）。
	token := &TokenInfo{
		AccessToken: resp.Data.AccessToken,
		ObtainedAt:  time.Now(),
		ExpiresIn:   int64(DefaultTokenCacheTTL.Seconds()),
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

	// 凭据通过 POST body 传递，避免在 URL 中暴露
	form := url.Values{
		"client_id":     {m.config.ClientID},
		"client_secret": {m.config.ClientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {currentToken.RefreshToken},
	}

	headers := map[string]string{
		"Authorization": "Bearer " + currentToken.AccessToken,
		"Content-Type":  "application/x-www-form-urlencoded",
	}

	var token TokenInfo
	if err := m.http.Post(ctx, PathTokenObtain, headers, form.Encode(), &token); err != nil {
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

	m.logger.Debug("refreshed token",
		slog.String("tenant_id", tenantID),
		slog.Int64("expires_in", token.ExpiresIn),
	)

	return &token, nil
}

// VerifyToken 验证 Token。
// 设计决策: 完全委托认证服务端校验 Token 有效性。客户端不做本地 exp/issuer/audience
// 校验，因为服务端是 Token 状态的唯一权威来源（可能有 grace period、吊销等机制），
// 客户端冗余校验反而可能导致与服务端判定不一致。
func (m *TokenManager) VerifyToken(ctx context.Context, token string) (*TokenInfo, error) {
	if token == "" {
		return nil, ErrMissingToken
	}

	// 开始观测
	ctx, span := xmetrics.Start(ctx, m.observer, xmetrics.SpanOptions{
		Component: MetricsComponent,
		Operation: MetricsOpVerifyToken,
		Kind:      xmetrics.KindClient,
	})
	var verifyErr error
	defer func() {
		span.End(xmetrics.Result{Err: verifyErr})
	}()

	// Token 通过 POST body 传递，避免在 URL 中暴露
	form := url.Values{
		"token": {token},
	}
	headers := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	}

	var resp VerifyResponse
	if err := m.http.Post(ctx, PathTokenVerify, headers, form.Encode(), &resp); err != nil {
		verifyErr = fmt.Errorf("verify token: %w", err)
		return nil, verifyErr
	}

	if !resp.Data.Active {
		verifyErr = ErrTokenInvalid
		return nil, verifyErr
	}

	// 构建 TokenInfo，保留服务端返回的完整声明供调用方授权决策
	claims := resp.Data
	tokenInfo := &TokenInfo{
		AccessToken: token,
		ExpiresAt:   time.Unix(resp.Data.Exp, 0),
		Claims:      &claims,
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
		if errors.Is(err, ErrCacheMiss) {
			m.logger.Debug("background refresh: cache miss, skipping",
				slog.String("tenant_id", tenantID),
			)
		} else {
			m.logger.Warn("background refresh: get current token failed",
				slog.String("tenant_id", tenantID),
				slog.String("error", err.Error()),
			)
		}
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

// Stop 停止 TokenManager，取消所有后台刷新任务并等待完成。
// 这是 graceful shutdown 的一部分，应在 client.Close() 时调用。
func (m *TokenManager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
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

// VerifyTokenForTenant 验证 Token 有效性并校验租户一致性。
// 如果 Token 有效但其声明中的 TenantID 与期望的 tenantID 不匹配，返回错误。
// 这是一个便捷方法，等价于 VerifyToken + 手动校验 Claims.TenantID。
func VerifyTokenForTenant(ctx context.Context, c Client, token, tenantID string) (*TokenInfo, error) {
	if c == nil {
		return nil, ErrNilClient
	}
	info, err := c.VerifyToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if info.Claims != nil && info.Claims.TenantID != "" && info.Claims.TenantID != tenantID {
		return nil, fmt.Errorf("%w: token tenant_id %q does not match expected %q",
			ErrTokenInvalid, info.Claims.TenantID, tenantID)
	}
	return info, nil
}
