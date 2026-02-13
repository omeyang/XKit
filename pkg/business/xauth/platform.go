package xauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"time"

	"github.com/omeyang/xkit/pkg/observability/xmetrics"
	"github.com/omeyang/xkit/pkg/util/xlru"
	"golang.org/x/sync/singleflight"
)

// =============================================================================
// PlatformManager 平台信息管理器
// =============================================================================

// PlatformManager 负责平台信息的获取和缓存。
type PlatformManager struct {
	http     *HTTPClient
	cache    CacheStore
	tokenMgr *TokenManager
	logger   *slog.Logger
	observer xmetrics.Observer

	// 配置
	cacheTTL time.Duration

	// 本地缓存（带 TTL 的 LRU 缓存）
	localCache *xlru.Cache[string, string] // key: "tenantID:field"
	sf         singleflight.Group
}

// PlatformManagerConfig PlatformManager 配置。
type PlatformManagerConfig struct {
	HTTP     *HTTPClient
	Cache    CacheStore
	TokenMgr *TokenManager
	Logger   *slog.Logger
	Observer xmetrics.Observer
	CacheTTL time.Duration

	// LocalCacheSize 本地缓存最大条目数。
	// 默认 1000。
	LocalCacheSize int

	// LocalCacheTTL 本地缓存 TTL。
	// 默认与 CacheTTL 相同。
	LocalCacheTTL time.Duration
}

// NewPlatformManager 创建 PlatformManager。
func NewPlatformManager(cfg PlatformManagerConfig) *PlatformManager {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Observer == nil {
		cfg.Observer = xmetrics.NoopObserver{}
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = DefaultPlatformDataCacheTTL
	}
	if cfg.Cache == nil {
		cfg.Cache = NoopCacheStore{}
	}
	if cfg.LocalCacheSize <= 0 {
		cfg.LocalCacheSize = 1000
	}
	if cfg.LocalCacheTTL <= 0 {
		cfg.LocalCacheTTL = cfg.CacheTTL
	}

	// 创建本地 LRU 缓存
	localCache, err := xlru.New[string, string](xlru.Config{
		Size: cfg.LocalCacheSize,
		TTL:  cfg.LocalCacheTTL,
	})
	if err != nil {
		// 这不应该发生，因为我们已经验证了 LocalCacheSize > 0
		cfg.Logger.Error("failed to create local cache, using fallback",
			slog.String("error", err.Error()))
		// 创建一个最小的缓存作为 fallback，Size=1 不会失败
		//nolint:errcheck // Size=1 保证不会返回错误
		localCache, _ = xlru.New[string, string](xlru.Config{Size: 1, TTL: cfg.LocalCacheTTL})
	}

	return &PlatformManager{
		http:       cfg.HTTP,
		cache:      cfg.Cache,
		tokenMgr:   cfg.TokenMgr,
		logger:     cfg.Logger,
		observer:   cfg.Observer,
		cacheTTL:   cfg.CacheTTL,
		localCache: localCache,
	}
}

// GetPlatformID 获取平台 ID。
func (m *PlatformManager) GetPlatformID(ctx context.Context, tenantID string) (string, error) {
	return m.getField(ctx, tenantID, CacheFieldPlatformID, m.fetchPlatformID)
}

// HasParentPlatform 判断是否有父平台。
func (m *PlatformManager) HasParentPlatform(ctx context.Context, tenantID string) (bool, error) {
	value, err := m.getField(ctx, tenantID, CacheFieldHasParent, m.fetchHasParent)
	if err != nil {
		return false, err
	}
	return strconv.ParseBool(value)
}

// GetUnclassRegionID 获取未归类组 Region ID。
func (m *PlatformManager) GetUnclassRegionID(ctx context.Context, tenantID string) (string, error) {
	return m.getField(ctx, tenantID, CacheFieldUnclassRegionID, m.fetchUnclassRegionID)
}

// getField 获取平台数据字段。
// 使用三级缓存：本地缓存 -> Redis 缓存 -> 远程 API。
func (m *PlatformManager) getField(
	ctx context.Context,
	tenantID, field string,
	fetcher func(ctx context.Context, tenantID string) (string, error),
) (string, error) {
	// 开始观测
	ctx, span := xmetrics.Start(ctx, m.observer, xmetrics.SpanOptions{
		Component: MetricsComponent,
		Operation: MetricsOpGetPlatformData,
		Kind:      xmetrics.KindClient,
		Attrs: []xmetrics.Attr{
			{Key: MetricsAttrTenantID, Value: tenantID},
			{Key: "field", Value: field},
		},
	})
	var fetchErr error
	defer func() {
		span.End(xmetrics.Result{Err: fetchErr})
	}()

	// 1. 尝试本地缓存
	if value := m.getLocalCache(tenantID, field); value != "" {
		return value, nil
	}

	// 2. 尝试 Redis 缓存
	if value := m.getFromRemoteCache(ctx, tenantID, field); value != "" {
		return value, nil
	}

	// 3. 从 API 获取（使用 singleflight）
	result, fetchErr := m.fetchWithSingleflight(ctx, tenantID, field, fetcher)
	return result, fetchErr
}

// getFromRemoteCache 从 Redis 缓存获取并回填本地缓存。
func (m *PlatformManager) getFromRemoteCache(ctx context.Context, tenantID, field string) string {
	value, err := m.cache.GetPlatformData(ctx, tenantID, field)
	if err == nil && value != "" {
		m.setLocalCache(tenantID, field, value)
		return value
	}
	if err != nil && !errors.Is(err, ErrCacheMiss) {
		m.logger.Warn("get platform data from cache failed",
			slog.String("tenant_id", tenantID),
			slog.String("field", field),
			slog.String("error", err.Error()),
		)
	}
	return ""
}

// fetchWithSingleflight 使用 singleflight 从 API 获取数据。
// 设计决策: singleflight.Do 使用第一个调用方的 ctx 发起请求，后续共享结果。
// 若首个 ctx 被取消，所有等待者均收到错误。这是 singleflight 的已知限制，
// 但对平台数据查询场景可接受——请求超时一致，取消概率低。
func (m *PlatformManager) fetchWithSingleflight(
	ctx context.Context,
	tenantID, field string,
	fetcher func(ctx context.Context, tenantID string) (string, error),
) (string, error) {
	sfKey := fmt.Sprintf("%s:%s", tenantID, field)
	result, err, _ := m.sf.Do(sfKey, func() (any, error) {
		return m.fetchAndCache(ctx, tenantID, field, fetcher)
	})

	if err != nil {
		return "", err
	}

	// 类型断言：singleflight 回调函数返回 string，断言应始终成功
	s, ok := result.(string)
	if !ok {
		return "", fmt.Errorf("xauth: unexpected result type from singleflight")
	}
	return s, nil
}

// fetchAndCache 从 API 获取并写入缓存。
func (m *PlatformManager) fetchAndCache(
	ctx context.Context,
	tenantID, field string,
	fetcher func(ctx context.Context, tenantID string) (string, error),
) (string, error) {
	// double-check 缓存
	if v := m.getLocalCache(tenantID, field); v != "" {
		return v, nil
	}

	// 从 API 获取
	v, err := fetcher(ctx, tenantID)
	if err != nil {
		return "", err
	}

	// 写入缓存
	m.setLocalCache(tenantID, field, v)
	if cacheErr := m.cache.SetPlatformData(ctx, tenantID, field, v, m.cacheTTL); cacheErr != nil {
		m.logger.Warn("set platform data to cache failed",
			slog.String("tenant_id", tenantID),
			slog.String("field", field),
			slog.String("error", cacheErr.Error()),
		)
	}

	return v, nil
}

// idResponse 用于提取 ID 类型响应的通用接口。
type idResponse interface {
	getID() string
}

func (r *PlatformSelfResponse) getID() string  { return r.Data.ID }
func (r *UnclassRegionResponse) getID() string { return r.Data.ID }

// fetchIDField 从 API 获取 ID 类型字段的通用方法。
// 用于 fetchPlatformID 和 fetchUnclassRegionID，消除代码重复。
func (m *PlatformManager) fetchIDField(
	ctx context.Context,
	tenantID string,
	apiPath string,
	errNotFound error,
	logField string,
	newResp func() idResponse,
) (string, error) {
	// 获取 Token
	token, err := m.tokenMgr.GetToken(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("get token: %w", err)
	}

	path := fmt.Sprintf("%s?projectId=%s", apiPath, url.QueryEscape(tenantID))
	resp := newResp()

	if err := m.http.RequestWithAuth(ctx, "GET", path, token, nil, nil, resp); err != nil {
		return "", fmt.Errorf("fetch %s: %w", logField, err)
	}

	id := resp.getID()
	if id == "" {
		return "", errNotFound
	}

	m.logger.Debug("fetched "+logField,
		slog.String("tenant_id", tenantID),
		slog.String(logField, id),
	)

	return id, nil
}

// fetchPlatformID 从 API 获取平台 ID。
func (m *PlatformManager) fetchPlatformID(ctx context.Context, tenantID string) (string, error) {
	return m.fetchIDField(ctx, tenantID, PathPlatformSelf, ErrPlatformIDNotFound, "platform_id",
		func() idResponse { return &PlatformSelfResponse{} })
}

// fetchHasParent 从 API 判断是否有父平台。
func (m *PlatformManager) fetchHasParent(ctx context.Context, tenantID string) (string, error) {
	// 获取 Token
	token, err := m.tokenMgr.GetToken(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("get token: %w", err)
	}

	path := fmt.Sprintf("%s?projectId=%s", PathHasParent, url.QueryEscape(tenantID))

	var resp HasParentResponse
	if err := m.http.RequestWithAuth(ctx, "GET", path, token, nil, nil, &resp); err != nil {
		return "", fmt.Errorf("get has parent: %w", err)
	}

	value := strconv.FormatBool(resp.Data)

	m.logger.Debug("fetched has_parent",
		slog.String("tenant_id", tenantID),
		slog.String("has_parent", value),
	)

	return value, nil
}

// fetchUnclassRegionID 从 API 获取未归类组 Region ID。
func (m *PlatformManager) fetchUnclassRegionID(ctx context.Context, tenantID string) (string, error) {
	return m.fetchIDField(ctx, tenantID, PathUnclassRegion, ErrUnclassRegionIDNotFound, "unclass_region_id",
		func() idResponse { return &UnclassRegionResponse{} })
}

// localCacheKey 构建本地缓存键。
func localCacheKey(tenantID, field string) string {
	return tenantID + ":" + field
}

// getLocalCache 从本地缓存获取。
func (m *PlatformManager) getLocalCache(tenantID, field string) string {
	value, ok := m.localCache.Get(localCacheKey(tenantID, field))
	if !ok {
		return ""
	}
	return value
}

// setLocalCache 设置本地缓存。
func (m *PlatformManager) setLocalCache(tenantID, field, value string) {
	m.localCache.Set(localCacheKey(tenantID, field), value)
}

// ClearLocalCache 清空本地缓存。
func (m *PlatformManager) ClearLocalCache() {
	m.localCache.Clear()
}

// InvalidateCache 使指定租户的缓存失效。
// 注意：本地缓存使用 "tenantID:field" 作为键，需要删除所有相关字段。
func (m *PlatformManager) InvalidateCache(ctx context.Context, tenantID string) error {
	// 删除该租户的所有本地缓存字段
	for _, field := range []string{CacheFieldPlatformID, CacheFieldHasParent, CacheFieldUnclassRegionID} {
		m.localCache.Delete(localCacheKey(tenantID, field))
	}
	return m.cache.Delete(ctx, tenantID)
}
