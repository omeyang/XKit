//go:build !windows

package xdbg

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBreakerRegistry 测试用的 mock 熔断器注册表。
type mockBreakerRegistry struct {
	breakers map[string]*BreakerInfo
}

func newMockBreakerRegistry() *mockBreakerRegistry {
	return &mockBreakerRegistry{
		breakers: make(map[string]*BreakerInfo),
	}
}

func (r *mockBreakerRegistry) List() []string {
	names := make([]string, 0, len(r.breakers))
	for name := range r.breakers {
		names = append(names, name)
	}
	return names
}

func (r *mockBreakerRegistry) Get(name string) (*BreakerInfo, bool) {
	info, ok := r.breakers[name]
	return info, ok
}

func (r *mockBreakerRegistry) Reset(name string) error {
	if info, ok := r.breakers[name]; ok {
		info.State = "closed"
		info.ConsecutiveFailures = 0
		return nil
	}
	return ErrCommandNotFound
}

func (r *mockBreakerRegistry) addBreaker(info *BreakerInfo) {
	r.breakers[info.Name] = info
}

// mockLimiterRegistry 测试用的 mock 限流器注册表。
type mockLimiterRegistry struct {
	limiters map[string]*LimiterInfo
}

func newMockLimiterRegistry() *mockLimiterRegistry {
	return &mockLimiterRegistry{
		limiters: make(map[string]*LimiterInfo),
	}
}

func (r *mockLimiterRegistry) List() []string {
	names := make([]string, 0, len(r.limiters))
	for name := range r.limiters {
		names = append(names, name)
	}
	return names
}

func (r *mockLimiterRegistry) Get(name string) (*LimiterInfo, bool) {
	info, ok := r.limiters[name]
	return info, ok
}

func (r *mockLimiterRegistry) addLimiter(info *LimiterInfo) {
	r.limiters[info.Name] = info
}

// mockCacheRegistry 测试用的 mock 缓存注册表。
type mockCacheRegistry struct {
	caches map[string]*CacheStats
}

func newMockCacheRegistry() *mockCacheRegistry {
	return &mockCacheRegistry{
		caches: make(map[string]*CacheStats),
	}
}

func (r *mockCacheRegistry) List() []string {
	names := make([]string, 0, len(r.caches))
	for name := range r.caches {
		names = append(names, name)
	}
	return names
}

func (r *mockCacheRegistry) Get(name string) (*CacheStats, bool) {
	stats, ok := r.caches[name]
	return stats, ok
}

func (r *mockCacheRegistry) addCache(stats *CacheStats) {
	r.caches[stats.Name] = stats
}

// mockConfigProvider 测试用的 mock 配置提供者。
type mockConfigProvider struct {
	config map[string]any
}

func newMockConfigProvider() *mockConfigProvider {
	return &mockConfigProvider{
		config: map[string]any{
			"app":     "test-app",
			"version": "1.0.0",
		},
	}
}

func (p *mockConfigProvider) Dump() map[string]any {
	return p.config
}

func TestBreakerCommand_List(t *testing.T) {
	registry := newMockBreakerRegistry()
	registry.addBreaker(&BreakerInfo{
		Name:           "api-breaker",
		State:          "closed",
		Requests:       100,
		TotalSuccesses: 95,
		TotalFailures:  5,
	})

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithBreakerRegistry(registry),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("breaker")
	if cmd == nil {
		t.Fatal("breaker command not registered")
	}

	output, err := cmd.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "熔断器列表") {
		t.Error("output should contain '熔断器列表'")
	}

	if !strings.Contains(output, "api-breaker") {
		t.Error("output should contain 'api-breaker'")
	}

	if !strings.Contains(output, "closed") {
		t.Error("output should contain 'closed'")
	}
}

func TestBreakerCommand_Show(t *testing.T) {
	registry := newMockBreakerRegistry()
	registry.addBreaker(&BreakerInfo{
		Name:                 "api-breaker",
		State:                "open",
		Requests:             1000,
		TotalSuccesses:       900,
		TotalFailures:        100,
		ConsecutiveSuccesses: 0,
		ConsecutiveFailures:  10,
	})

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithBreakerRegistry(registry),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("breaker")

	output, err := cmd.Execute(context.Background(), []string{"api-breaker"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "熔断器: api-breaker") {
		t.Error("output should contain breaker name")
	}

	if !strings.Contains(output, "状态:     open") {
		t.Error("output should contain state")
	}

	if !strings.Contains(output, "连续失败: 10") {
		t.Error("output should contain consecutive failures")
	}
}

func TestBreakerCommand_Reset(t *testing.T) {
	registry := newMockBreakerRegistry()
	registry.addBreaker(&BreakerInfo{
		Name:                "api-breaker",
		State:               "open",
		ConsecutiveFailures: 10,
	})

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithBreakerRegistry(registry),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("breaker")

	output, err := cmd.Execute(context.Background(), []string{"api-breaker", "reset"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "已重置") {
		t.Error("output should confirm reset")
	}

	// 验证状态已重置
	info, _ := registry.Get("api-breaker")
	if info.State != "closed" {
		t.Errorf("breaker state = %q, want %q", info.State, "closed")
	}
}

func TestBreakerCommand_NotFound(t *testing.T) {
	registry := newMockBreakerRegistry()

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithBreakerRegistry(registry),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("breaker")

	_, err = cmd.Execute(context.Background(), []string{"unknown"})
	if err == nil {
		t.Error("expected error for unknown breaker")
	}
}

func TestBreakerCommand_NoRegistry(t *testing.T) {
	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// 没有配置 BreakerRegistry，breaker 命令不应该注册
	cmd := srv.registry.Get("breaker")
	if cmd != nil {
		t.Error("breaker command should not be registered without BreakerRegistry")
	}
}

func TestLimitCommand_List(t *testing.T) {
	registry := newMockLimiterRegistry()
	registry.addLimiter(&LimiterInfo{
		Name:      "api-limiter",
		Type:      "token_bucket",
		Limit:     100,
		Remaining: 50,
		Reset:     1000,
	})

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithLimiterRegistry(registry),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("limit")
	if cmd == nil {
		t.Fatal("limit command not registered")
	}

	output, err := cmd.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "限流器列表") {
		t.Error("output should contain '限流器列表'")
	}

	if !strings.Contains(output, "api-limiter") {
		t.Error("output should contain 'api-limiter'")
	}

	if !strings.Contains(output, "token_bucket") {
		t.Error("output should contain 'token_bucket'")
	}
}

func TestLimitCommand_Show(t *testing.T) {
	registry := newMockLimiterRegistry()
	registry.addLimiter(&LimiterInfo{
		Name:      "api-limiter",
		Type:      "sliding_window",
		Limit:     1000,
		Remaining: 500,
		Reset:     60,
	})

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithLimiterRegistry(registry),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("limit")

	output, err := cmd.Execute(context.Background(), []string{"api-limiter"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "限流器: api-limiter") {
		t.Error("output should contain limiter name")
	}

	if !strings.Contains(output, "类型:   sliding_window") {
		t.Error("output should contain type")
	}

	if !strings.Contains(output, "配额:   1000") {
		t.Error("output should contain limit")
	}
}

func TestLimitCommand_NotFound(t *testing.T) {
	registry := newMockLimiterRegistry()

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithLimiterRegistry(registry),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("limit")

	_, err = cmd.Execute(context.Background(), []string{"unknown"})
	if err == nil {
		t.Error("expected error for unknown limiter")
	}
}

func TestCacheCommand_List(t *testing.T) {
	registry := newMockCacheRegistry()
	registry.addCache(&CacheStats{
		Name:    "user-cache",
		Type:    "lru",
		Hits:    900,
		Misses:  100,
		Size:    500,
		MaxSize: 1000,
	})

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithCacheRegistry(registry),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("cache")
	if cmd == nil {
		t.Fatal("cache command not registered")
	}

	output, err := cmd.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "缓存列表") {
		t.Error("output should contain '缓存列表'")
	}

	if !strings.Contains(output, "user-cache") {
		t.Error("output should contain 'user-cache'")
	}

	if !strings.Contains(output, "90.0%") {
		t.Error("output should contain hit rate")
	}
}

func TestCacheCommand_Show(t *testing.T) {
	registry := newMockCacheRegistry()
	registry.addCache(&CacheStats{
		Name:    "user-cache",
		Type:    "lru",
		Hits:    800,
		Misses:  200,
		Size:    300,
		MaxSize: 1000,
	})

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithCacheRegistry(registry),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("cache")

	output, err := cmd.Execute(context.Background(), []string{"user-cache"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "缓存: user-cache") {
		t.Error("output should contain cache name")
	}

	if !strings.Contains(output, "命中率:   80.0%") {
		t.Error("output should contain hit rate")
	}

	if !strings.Contains(output, "当前大小: 300") {
		t.Error("output should contain current size")
	}
}

func TestCacheCommand_NotFound(t *testing.T) {
	registry := newMockCacheRegistry()

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithCacheRegistry(registry),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("cache")

	_, err = cmd.Execute(context.Background(), []string{"unknown"})
	if err == nil {
		t.Error("expected error for unknown cache")
	}
}

func TestConfigCommand(t *testing.T) {
	provider := newMockConfigProvider()

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithConfigProvider(provider),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("config")
	if cmd == nil {
		t.Fatal("config command not registered")
	}

	output, err := cmd.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(output, "test-app") {
		t.Error("output should contain app name")
	}

	if !strings.Contains(output, "1.0.0") {
		t.Error("output should contain version")
	}
}

func TestConfigCommand_EmptyConfig(t *testing.T) {
	provider := &mockConfigProvider{config: nil}

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithConfigProvider(provider),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("config")

	output, err := cmd.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if output != "配置为空" {
		t.Errorf("output = %q, want %q", output, "配置为空")
	}
}

func TestConfigCommand_NoProvider(t *testing.T) {
	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// 没有配置 ConfigProvider，config 命令不应该注册
	cmd := srv.registry.Get("config")
	if cmd != nil {
		t.Error("config command should not be registered without ConfigProvider")
	}
}

func TestXkitCommandsConditionalRegistration(t *testing.T) {
	a := assert.New(t)

	// 不配置任何 xkit 组件
	srv1, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
	)
	require.NoError(t, err, "New() without xkit components")

	// 验证 xkit 命令未注册
	a.False(srv1.registry.Has("breaker"), "breaker should not be registered")
	a.False(srv1.registry.Has("limit"), "limit should not be registered")
	a.False(srv1.registry.Has("cache"), "cache should not be registered")
	a.False(srv1.registry.Has("config"), "config should not be registered")

	// 配置所有 xkit 组件
	srv2, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithBreakerRegistry(newMockBreakerRegistry()),
		WithLimiterRegistry(newMockLimiterRegistry()),
		WithCacheRegistry(newMockCacheRegistry()),
		WithConfigProvider(newMockConfigProvider()),
	)
	require.NoError(t, err, "New() with all xkit components")

	// 验证 xkit 命令已注册
	a.True(srv2.registry.Has("breaker"), "breaker should be registered")
	a.True(srv2.registry.Has("limit"), "limit should be registered")
	a.True(srv2.registry.Has("cache"), "cache should be registered")
	a.True(srv2.registry.Has("config"), "config should be registered")
}

func TestBreakerCommand_EmptyList(t *testing.T) {
	registry := newMockBreakerRegistry()

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithBreakerRegistry(registry),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("breaker")

	output, err := cmd.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if output != "没有注册的熔断器" {
		t.Errorf("output = %q, want %q", output, "没有注册的熔断器")
	}
}

func TestLimitCommand_EmptyList(t *testing.T) {
	registry := newMockLimiterRegistry()

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithLimiterRegistry(registry),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("limit")

	output, err := cmd.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if output != "没有注册的限流器" {
		t.Errorf("output = %q, want %q", output, "没有注册的限流器")
	}
}

func TestCacheCommand_EmptyList(t *testing.T) {
	registry := newMockCacheRegistry()

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithCacheRegistry(registry),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cmd := srv.registry.Get("cache")

	output, err := cmd.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if output != "没有注册的缓存" {
		t.Errorf("output = %q, want %q", output, "没有注册的缓存")
	}
}

func TestXkitCommand_HelpStrings(t *testing.T) {
	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithBreakerRegistry(newMockBreakerRegistry()),
		WithLimiterRegistry(newMockLimiterRegistry()),
		WithCacheRegistry(newMockCacheRegistry()),
		WithConfigProvider(newMockConfigProvider()),
	)
	require.NoError(t, err)

	tests := []struct {
		name     string
		wantHelp string
	}{
		{"breaker", "查看/重置熔断器状态"},
		{"limit", "查看限流器状态"},
		{"cache", "查看缓存统计"},
		{"config", "查看运行时配置"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := srv.registry.Get(tt.name)
			require.NotNil(t, cmd, "command %q should be registered", tt.name)
			assert.Contains(t, cmd.Help(), tt.wantHelp)
		})
	}
}

func TestBreakerCommand_ResetNotFound(t *testing.T) {
	registry := newMockBreakerRegistry()

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithBreakerRegistry(registry),
	)
	require.NoError(t, err)

	cmd := srv.registry.Get("breaker")
	_, err = cmd.Execute(context.Background(), []string{"nonexistent", "reset"})
	assert.Error(t, err, "reset on nonexistent breaker should fail")
}

func TestCacheCommand_ZeroHitsAndMisses(t *testing.T) {
	registry := newMockCacheRegistry()
	registry.addCache(&CacheStats{
		Name:    "empty-cache",
		Type:    "lru",
		Hits:    0,
		Misses:  0,
		Size:    0,
		MaxSize: 100,
	})

	srv, err := New(
		WithBackgroundMode(true),
		WithAuditLogger(NewNoopAuditLogger()),
		WithCacheRegistry(registry),
	)
	require.NoError(t, err)

	cmd := srv.registry.Get("cache")

	// List: zero hits/misses should show 0.0% hit rate
	output, err := cmd.Execute(context.Background(), nil)
	require.NoError(t, err)
	assert.Contains(t, output, "0.0%")

	// Show: zero hits/misses should show 0.0% hit rate
	output, err = cmd.Execute(context.Background(), []string{"empty-cache"})
	require.NoError(t, err)
	assert.Contains(t, output, "0.0%")
}
