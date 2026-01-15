package xauth

import (
	"context"
	"sync"
	"time"
)

// =============================================================================
// Mock CacheStore
// =============================================================================

// mockCacheStore 用于测试的缓存存储 Mock。
type mockCacheStore struct {
	mu              sync.RWMutex
	tokens          map[string]*TokenInfo
	platformData    map[string]map[string]string
	getTokenErr     error
	setTokenErr     error
	getPlatformErr  error
	setPlatformErr  error
	deleteErr       error
	getTokenCalls   int
	setTokenCalls   int
	lastSetTokenTTL time.Duration // 记录最后一次 SetToken 使用的 TTL
}

func newMockCacheStore() *mockCacheStore {
	return &mockCacheStore{
		tokens:       make(map[string]*TokenInfo),
		platformData: make(map[string]map[string]string),
	}
}

func (m *mockCacheStore) GetToken(_ context.Context, tenantID string) (*TokenInfo, error) {
	m.mu.Lock()
	m.getTokenCalls++
	m.mu.Unlock()

	if m.getTokenErr != nil {
		return nil, m.getTokenErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if token, ok := m.tokens[tenantID]; ok {
		return token, nil
	}
	return nil, ErrCacheMiss
}

func (m *mockCacheStore) SetToken(_ context.Context, tenantID string, token *TokenInfo, ttl time.Duration) error {
	m.mu.Lock()
	m.setTokenCalls++
	m.lastSetTokenTTL = ttl
	m.mu.Unlock()

	if m.setTokenErr != nil {
		return m.setTokenErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[tenantID] = token
	return nil
}

func (m *mockCacheStore) GetPlatformData(_ context.Context, tenantID string, field string) (string, error) {
	if m.getPlatformErr != nil {
		return "", m.getPlatformErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if fields, ok := m.platformData[tenantID]; ok {
		if value, ok := fields[field]; ok {
			return value, nil
		}
	}
	return "", ErrCacheMiss
}

func (m *mockCacheStore) SetPlatformData(_ context.Context, tenantID string, field, value string, _ time.Duration) error {
	if m.setPlatformErr != nil {
		return m.setPlatformErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.platformData[tenantID] == nil {
		m.platformData[tenantID] = make(map[string]string)
	}
	m.platformData[tenantID][field] = value
	return nil
}

func (m *mockCacheStore) Delete(_ context.Context, tenantID string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tokens, tenantID)
	delete(m.platformData, tenantID)
	return nil
}

// =============================================================================
// Mock Client (用于单元测试)
// =============================================================================

// mockClient 实现 Client 接口的 Mock。
type mockClient struct {
	mu sync.RWMutex

	tokens         map[string]string
	platformIDs    map[string]string
	hasParent      map[string]bool
	unclassRegions map[string]string
	verifyData     map[string]*TokenInfo

	getTokenErr          error
	verifyTokenErr       error
	getPlatformIDErr     error
	hasParentPlatformErr error
	getUnclassRegionErr  error
	requestErr           error

	closed bool
}

func newMockClient() *mockClient {
	return &mockClient{
		tokens:         make(map[string]string),
		platformIDs:    make(map[string]string),
		hasParent:      make(map[string]bool),
		unclassRegions: make(map[string]string),
		verifyData:     make(map[string]*TokenInfo),
	}
}

func (m *mockClient) GetToken(_ context.Context, tenantID string) (string, error) {
	if m.getTokenErr != nil {
		return "", m.getTokenErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if token, ok := m.tokens[tenantID]; ok {
		return token, nil
	}
	return "mock-token-" + tenantID, nil
}

func (m *mockClient) VerifyToken(_ context.Context, token string) (*TokenInfo, error) {
	if m.verifyTokenErr != nil {
		return nil, m.verifyTokenErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if info, ok := m.verifyData[token]; ok {
		return info, nil
	}
	return &TokenInfo{
		AccessToken: token,
		ExpiresAt:   time.Now().Add(time.Hour),
	}, nil
}

func (m *mockClient) GetPlatformID(_ context.Context, tenantID string) (string, error) {
	if m.getPlatformIDErr != nil {
		return "", m.getPlatformIDErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if id, ok := m.platformIDs[tenantID]; ok {
		return id, nil
	}
	return "platform-" + tenantID, nil
}

func (m *mockClient) HasParentPlatform(_ context.Context, tenantID string) (bool, error) {
	if m.hasParentPlatformErr != nil {
		return false, m.hasParentPlatformErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if has, ok := m.hasParent[tenantID]; ok {
		return has, nil
	}
	return false, nil
}

func (m *mockClient) GetUnclassRegionID(_ context.Context, tenantID string) (string, error) {
	if m.getUnclassRegionErr != nil {
		return "", m.getUnclassRegionErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if id, ok := m.unclassRegions[tenantID]; ok {
		return id, nil
	}
	return "region-" + tenantID, nil
}

func (m *mockClient) Request(_ context.Context, _ *AuthRequest) error {
	return m.requestErr
}

func (m *mockClient) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// =============================================================================
// Test Helpers
// =============================================================================

// testToken 创建测试用的 TokenInfo。
func testToken(accessToken string, expiresIn int64) *TokenInfo {
	now := time.Now()
	return &TokenInfo{
		AccessToken:  accessToken,
		TokenType:    "bearer",
		RefreshToken: "refresh-" + accessToken,
		ExpiresIn:    expiresIn,
		ExpiresAt:    now.Add(time.Duration(expiresIn) * time.Second),
		ObtainedAt:   now,
	}
}

// testConfig 创建测试用的 Config。
func testConfig() *Config {
	return &Config{
		Host:                  "https://auth.test.com",
		ClientID:              "test-client",
		ClientSecret:          "test-secret",
		Timeout:               5 * time.Second,
		TokenRefreshThreshold: 1 * time.Minute,
		PlatformDataCacheTTL:  5 * time.Minute,
	}
}
