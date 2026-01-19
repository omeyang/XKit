package xdlock

import (
	"context"
	"crypto/tls"
	"errors"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/storage/xetcd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testContextKey 用于测试的 context key 类型
type testContextKey string

// =============================================================================
// EtcdConfig 测试
// =============================================================================

func TestEtcdConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *EtcdConfig
		wantErr error
	}{
		{
			name: "valid config",
			config: &EtcdConfig{
				Endpoints: []string{"localhost:2379"},
			},
			wantErr: nil,
		},
		{
			name: "valid config with multiple endpoints",
			config: &EtcdConfig{
				Endpoints: []string{"node1:2379", "node2:2379", "node3:2379"},
			},
			wantErr: nil,
		},
		{
			name:    "empty endpoints",
			config:  &EtcdConfig{},
			wantErr: xetcd.ErrNoEndpoints, // EtcdConfig 是 xetcd.Config 的别名
		},
		{
			name: "nil endpoints",
			config: &EtcdConfig{
				Endpoints: nil,
			},
			wantErr: xetcd.ErrNoEndpoints, // EtcdConfig 是 xetcd.Config 的别名
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultEtcdConfig(t *testing.T) {
	config := DefaultEtcdConfig()

	assert.Equal(t, 5*time.Second, config.DialTimeout)
	assert.Equal(t, 10*time.Second, config.DialKeepAliveTime)
	assert.Equal(t, 3*time.Second, config.DialKeepAliveTimeout)
	assert.True(t, config.RejectOldCluster)
	assert.True(t, config.PermitWithoutStream)
	assert.Empty(t, config.Endpoints)
	assert.Empty(t, config.Username)
	assert.Empty(t, config.Password)
}

// =============================================================================
// NewEtcdClient 测试
// =============================================================================

func TestNewEtcdClient_NilConfig(t *testing.T) {
	client, err := NewEtcdClient(nil)

	assert.Nil(t, client)
	assert.ErrorIs(t, err, ErrNilConfig)
}

func TestNewEtcdClient_NoEndpoints(t *testing.T) {
	config := &EtcdConfig{}

	client, err := NewEtcdClient(config)

	assert.Nil(t, client)
	assert.ErrorIs(t, err, ErrNoEndpoints)
}

func TestNewEtcdClient_EmptyEndpoints(t *testing.T) {
	config := &EtcdConfig{
		Endpoints: []string{},
	}

	client, err := NewEtcdClient(config)

	assert.Nil(t, client)
	assert.ErrorIs(t, err, ErrNoEndpoints)
}

// =============================================================================
// EtcdClientOption 测试
// =============================================================================

func TestEtcdClientOptions_Defaults(t *testing.T) {
	opts := defaultEtcdClientOptions()

	assert.NotNil(t, opts.Context)
	assert.False(t, opts.HealthCheck)
	assert.Equal(t, 10*time.Second, opts.HealthTimeout)
	assert.Nil(t, opts.TLSConfig)
}

func TestWithEtcdClientContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), testContextKey("key"), "value")
	opts := defaultEtcdClientOptions()

	WithEtcdClientContext(ctx)(opts)

	assert.Equal(t, ctx, opts.Context)
}

func TestWithEtcdClientContext_Nil(t *testing.T) {
	originalCtx := context.Background()
	opts := &etcdClientOptions{Context: originalCtx}

	WithEtcdClientContext(nil)(opts)

	// nil context 不应修改现有 context
	assert.Equal(t, originalCtx, opts.Context)
}

func TestWithEtcdHealthCheck(t *testing.T) {
	tests := []struct {
		name          string
		enabled       bool
		timeout       time.Duration
		expectEnabled bool
		expectTimeout time.Duration
	}{
		{
			name:          "enable with custom timeout",
			enabled:       true,
			timeout:       5 * time.Second,
			expectEnabled: true,
			expectTimeout: 5 * time.Second,
		},
		{
			name:          "enable with zero timeout keeps default",
			enabled:       true,
			timeout:       0,
			expectEnabled: true,
			expectTimeout: 10 * time.Second, // 默认值
		},
		{
			name:          "disable health check",
			enabled:       false,
			timeout:       5 * time.Second,
			expectEnabled: false,
			expectTimeout: 5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultEtcdClientOptions()

			WithEtcdHealthCheck(tt.enabled, tt.timeout)(opts)

			assert.Equal(t, tt.expectEnabled, opts.HealthCheck)
			assert.Equal(t, tt.expectTimeout, opts.HealthTimeout)
		})
	}
}

func TestWithEtcdTLS(t *testing.T) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	opts := defaultEtcdClientOptions()

	WithEtcdTLS(tlsConfig)(opts)

	assert.Equal(t, tlsConfig, opts.TLSConfig)
}

func TestWithEtcdTLS_Nil(t *testing.T) {
	opts := defaultEtcdClientOptions()

	WithEtcdTLS(nil)(opts)

	assert.Nil(t, opts.TLSConfig)
}

// =============================================================================
// NewEtcdFactoryFromConfig 测试
// =============================================================================

func TestNewEtcdFactoryFromConfig_NilConfig(t *testing.T) {
	factory, client, err := NewEtcdFactoryFromConfig(nil, nil)

	assert.Nil(t, factory)
	assert.Nil(t, client)
	assert.ErrorIs(t, err, ErrNilConfig)
}

func TestNewEtcdFactoryFromConfig_NoEndpoints(t *testing.T) {
	config := &EtcdConfig{}

	factory, client, err := NewEtcdFactoryFromConfig(config, nil)

	assert.Nil(t, factory)
	assert.Nil(t, client)
	assert.ErrorIs(t, err, ErrNoEndpoints)
}

// =============================================================================
// 错误定义测试
// =============================================================================

func TestErrorDefinitions(t *testing.T) {
	// 确保错误定义存在且可以正确匹配
	assert.True(t, errors.Is(ErrNilConfig, ErrNilConfig))
	assert.True(t, errors.Is(ErrNoEndpoints, ErrNoEndpoints))

	// 确保错误消息包含 xdlock 前缀
	assert.Contains(t, ErrNilConfig.Error(), "xdlock:")
	assert.Contains(t, ErrNoEndpoints.Error(), "xdlock:")
}

// =============================================================================
// Client 类型别名测试
// =============================================================================

func TestClientTypeAlias(t *testing.T) {
	// 测试 Client 类型别名可以正确使用
	// 这是一个编译时测试，确保类型别名正确定义
	var _ Client = nil // 可以赋值 nil
}

// =============================================================================
// convertXetcdError 测试
// =============================================================================

func TestConvertXetcdError(t *testing.T) {
	// 测试 nil 错误
	assert.Nil(t, convertXetcdError(nil))

	// 测试其他错误包装
	someErr := errors.New("some error")
	wrapped := convertXetcdError(someErr)
	assert.Contains(t, wrapped.Error(), "xdlock:")
	assert.ErrorIs(t, wrapped, someErr)
}

// =============================================================================
// 示例测试（文档用途）
// =============================================================================

func ExampleEtcdConfig() {
	// 创建 etcd 配置
	config := &EtcdConfig{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		// 处理错误
		return
	}

	// 使用配置创建客户端
	// client, err := NewEtcdClient(config)
	_ = config
}

func ExampleDefaultEtcdConfig() {
	// 获取默认配置
	config := DefaultEtcdConfig()

	// 设置必填字段
	config.Endpoints = []string{"localhost:2379"}

	// 可选：覆盖默认值
	config.DialTimeout = 10 * time.Second

	_ = config
}

func ExampleNewEtcdClient() {
	config := &EtcdConfig{
		Endpoints: []string{"localhost:2379"},
	}

	// 基本用法（不带选项）
	// client, err := NewEtcdClient(config)

	// 带健康检查
	// client, err := NewEtcdClient(config, WithEtcdHealthCheck(true, 5*time.Second))

	// 带 TLS
	// tlsConfig := &tls.Config{...}
	// client, err := NewEtcdClient(config, WithEtcdTLS(tlsConfig))

	_ = config
}

func ExampleNewEtcdFactoryFromConfig() {
	config := &EtcdConfig{
		Endpoints: []string{"localhost:2379"},
	}

	// 创建带健康检查的工厂
	// factory, client, err := NewEtcdFactoryFromConfig(
	//     config,
	//     []EtcdClientOption{WithEtcdHealthCheck(true, 5*time.Second)},
	//     WithEtcdTTL(30),
	// )
	// if err != nil {
	//     log.Fatal(err)
	// }
	// defer factory.Close()
	// defer client.Close()

	_ = config
}

// =============================================================================
// 基准测试
// =============================================================================

func BenchmarkEtcdConfig_Validate(b *testing.B) {
	config := &EtcdConfig{
		Endpoints: []string{"localhost:2379", "localhost:2380", "localhost:2381"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.Validate()
	}
}

func BenchmarkDefaultEtcdClientOptions(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = defaultEtcdClientOptions()
	}
}

// =============================================================================
// 集成测试占位（需要实际 etcd 服务）
// =============================================================================

// TestNewEtcdClient_Integration 集成测试
// 需要运行的 etcd 服务
func TestNewEtcdClient_Integration(t *testing.T) {
	t.Skip("跳过集成测试：需要实际 etcd 服务")

	config := &EtcdConfig{
		Endpoints: []string{"localhost:2379"},
	}

	client, err := NewEtcdClient(config, WithEtcdHealthCheck(true, 5*time.Second))
	require.NoError(t, err)
	defer client.Close()

	// 测试基本操作
	ctx := context.Background()
	_, err = client.Put(ctx, "test-key", "test-value")
	require.NoError(t, err)

	resp, err := client.Get(ctx, "test-key")
	require.NoError(t, err)
	require.Len(t, resp.Kvs, 1)
	assert.Equal(t, "test-value", string(resp.Kvs[0].Value))

	// 清理
	_, err = client.Delete(ctx, "test-key")
	require.NoError(t, err)
}

func TestNewEtcdFactoryFromConfig_Integration(t *testing.T) {
	t.Skip("跳过集成测试：需要实际 etcd 服务")

	config := &EtcdConfig{
		Endpoints: []string{"localhost:2379"},
	}

	factory, client, err := NewEtcdFactoryFromConfig(
		config,
		[]EtcdClientOption{WithEtcdHealthCheck(true, 5*time.Second)},
		WithEtcdTTL(30),
	)
	require.NoError(t, err)
	defer factory.Close()
	defer client.Close()

	// 测试锁操作
	ctx := context.Background()
	handle, err := factory.Lock(ctx, "test-lock")
	require.NoError(t, err)
	require.NotNil(t, handle)

	err = handle.Unlock(ctx)
	require.NoError(t, err)
}
