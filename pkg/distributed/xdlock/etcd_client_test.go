package xdlock

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/omeyang/xkit/pkg/storage/xetcd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/client/v3/concurrency"
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
	var nilCtx context.Context
	originalCtx := context.Background()
	opts := &etcdClientOptions{Context: originalCtx}

	WithEtcdClientContext(nilCtx)(opts)

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
// convertXetcdError 测试
// =============================================================================

func TestConvertXetcdError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantErr      error
		wantOriginal error // 验证原始错误保留在链中
		wantNil      bool
	}{
		{"nil error", nil, nil, nil, true},
		{"ErrNilConfig", xetcd.ErrNilConfig, ErrNilConfig, xetcd.ErrNilConfig, false},
		{"ErrNoEndpoints", xetcd.ErrNoEndpoints, ErrNoEndpoints, xetcd.ErrNoEndpoints, false},
		{"other error", errors.New("some error"), nil, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertXetcdError(tt.err)
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			if tt.wantErr != nil {
				// 使用 ErrorIs 而非 Equal，因为错误现在是包装过的
				assert.ErrorIs(t, result, tt.wantErr)
			} else {
				assert.Contains(t, result.Error(), "xdlock:")
				assert.ErrorIs(t, result, tt.err)
			}
			// 验证原始错误保留在链中
			if tt.wantOriginal != nil {
				assert.ErrorIs(t, result, tt.wantOriginal)
			}
		})
	}
}

// =============================================================================
// wrapEtcdError 测试
// =============================================================================

func TestWrapEtcdError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantErr      error
		wantOriginal error // 验证原始错误也在链中
		wantNil      bool
	}{
		{"nil", nil, nil, nil, true},
		{"ErrLocked", concurrency.ErrLocked, ErrLockHeld, concurrency.ErrLocked, false},
		{"ErrSessionExpired", concurrency.ErrSessionExpired, ErrSessionExpired, concurrency.ErrSessionExpired, false},
		{"ErrLockReleased", concurrency.ErrLockReleased, ErrNotLocked, concurrency.ErrLockReleased, false},
		{"context.Canceled", context.Canceled, context.Canceled, nil, false},
		{"context.DeadlineExceeded", context.DeadlineExceeded, context.DeadlineExceeded, nil, false},
		{"other error", errors.New("other"), nil, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapEtcdError(tt.err)
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			if tt.wantErr != nil {
				assert.ErrorIs(t, result, tt.wantErr)
			}
			// 验证原始错误保留在链中
			if tt.wantOriginal != nil {
				assert.ErrorIs(t, result, tt.wantOriginal)
			}
		})
	}
}

// =============================================================================
// etcd 工厂选项测试
// =============================================================================

func TestDefaultEtcdFactoryOptions(t *testing.T) {
	opts := defaultEtcdFactoryOptions()
	assert.Equal(t, 60, opts.TTL)
	assert.NotNil(t, opts.Context)
}

func TestWithEtcdTTL_Internal(t *testing.T) {
	tests := []struct {
		name    string
		ttl     int
		wantTTL int
	}{
		{"positive", 30, 30},
		{"zero keeps default", 0, 60},
		{"negative keeps default", -1, 60},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultEtcdFactoryOptions()
			WithEtcdTTL(tt.ttl)(opts)
			assert.Equal(t, tt.wantTTL, opts.TTL)
		})
	}
}

func TestWithEtcdContext_Internal(t *testing.T) {
	t.Run("valid context", func(t *testing.T) {
		opts := defaultEtcdFactoryOptions()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		WithEtcdContext(ctx)(opts)
		assert.Equal(t, ctx, opts.Context)
	})

	t.Run("nil context keeps default", func(t *testing.T) {
		opts := defaultEtcdFactoryOptions()
		original := opts.Context
		WithEtcdContext(nil)(opts)
		assert.Equal(t, original, opts.Context)
	})
}

// =============================================================================
// Mutex 选项内部测试
// =============================================================================

func TestMutexOptions_Internal(t *testing.T) {
	t.Run("WithRetryDelayFunc sets func", func(t *testing.T) {
		opts := defaultMutexOptions()
		fn := func(_ int) time.Duration { return time.Second }
		WithRetryDelayFunc(fn)(opts)
		assert.NotNil(t, opts.RetryDelayFunc)
	})

	t.Run("WithRetryDelayFunc nil keeps nil", func(t *testing.T) {
		opts := defaultMutexOptions()
		WithRetryDelayFunc(nil)(opts)
		assert.Nil(t, opts.RetryDelayFunc)
	})

	t.Run("WithGenValueFunc sets func", func(t *testing.T) {
		opts := defaultMutexOptions()
		fn := func() (string, error) { return "v", nil }
		WithGenValueFunc(fn)(opts)
		assert.NotNil(t, opts.GenValueFunc)
	})

	t.Run("WithGenValueFunc nil keeps nil", func(t *testing.T) {
		opts := defaultMutexOptions()
		WithGenValueFunc(nil)(opts)
		assert.Nil(t, opts.GenValueFunc)
	})

	t.Run("WithSetNXOnExtend true", func(t *testing.T) {
		opts := defaultMutexOptions()
		WithSetNXOnExtend(true)(opts)
		assert.True(t, opts.SetNXOnExtend)
	})

	t.Run("WithSetNXOnExtend false", func(t *testing.T) {
		opts := defaultMutexOptions()
		opts.SetNXOnExtend = true
		WithSetNXOnExtend(false)(opts)
		assert.False(t, opts.SetNXOnExtend)
	})

	t.Run("WithDriftFactor zero keeps default", func(t *testing.T) {
		opts := defaultMutexOptions()
		WithDriftFactor(0.0)(opts)
		assert.Equal(t, 0.01, opts.DriftFactor)
	})

	t.Run("WithDriftFactor positive sets value", func(t *testing.T) {
		opts := defaultMutexOptions()
		WithDriftFactor(0.05)(opts)
		assert.Equal(t, 0.05, opts.DriftFactor)
	})

	t.Run("WithDriftFactor negative keeps default", func(t *testing.T) {
		opts := defaultMutexOptions()
		WithDriftFactor(-0.01)(opts)
		assert.Equal(t, 0.01, opts.DriftFactor)
	})

	t.Run("WithTimeoutFactor zero keeps default", func(t *testing.T) {
		opts := defaultMutexOptions()
		WithTimeoutFactor(0.0)(opts)
		assert.Equal(t, 0.05, opts.TimeoutFactor)
	})

	t.Run("WithTimeoutFactor positive sets value", func(t *testing.T) {
		opts := defaultMutexOptions()
		WithTimeoutFactor(0.1)(opts)
		assert.Equal(t, 0.1, opts.TimeoutFactor)
	})

	t.Run("WithTimeoutFactor negative keeps default", func(t *testing.T) {
		opts := defaultMutexOptions()
		WithTimeoutFactor(-0.05)(opts)
		assert.Equal(t, 0.05, opts.TimeoutFactor)
	})
}

// =============================================================================
// Key 验证内部测试
// =============================================================================

func TestValidateKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr error
	}{
		{"valid key", "my-lock", nil},
		{"valid with dots", "resource.lock", nil},
		{"valid unicode", "中文锁名", nil},
		{"at max length", strings.Repeat("x", maxKeyLength), nil},
		{"empty string", "", ErrEmptyKey},
		{"space only", " ", ErrEmptyKey},
		{"tabs only", "\t\t", ErrEmptyKey},
		{"mixed whitespace", " \t\n ", ErrEmptyKey},
		{"over max length", strings.Repeat("x", maxKeyLength+1), ErrKeyTooLong},
		{"way over max length", strings.Repeat("x", maxKeyLength*2), ErrKeyTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateKey(tt.key)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// wrapRedisError 测试
// =============================================================================

func TestWrapRedisError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantErr      error
		wantOriginal error // 验证原始错误也在链中
		wantNil      bool
	}{
		{"nil", nil, nil, nil, true},
		{"context.Canceled", context.Canceled, context.Canceled, nil, false},
		{"context.DeadlineExceeded", context.DeadlineExceeded, context.DeadlineExceeded, nil, false},
		{"ErrTaken", &redsync.ErrTaken{}, ErrLockHeld, nil, false},
		{"ErrFailed", redsync.ErrFailed, ErrLockFailed, redsync.ErrFailed, false},
		{"ErrExtendFailed", redsync.ErrExtendFailed, ErrExtendFailed, redsync.ErrExtendFailed, false},
		{"ErrLockAlreadyExpired", redsync.ErrLockAlreadyExpired, errLockExpired, redsync.ErrLockAlreadyExpired, false},
		{"other error", errors.New("other"), nil, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapRedisError(tt.err)
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			if tt.wantErr != nil {
				assert.ErrorIs(t, result, tt.wantErr)
			} else {
				// 未知错误应原样返回
				assert.Equal(t, tt.err, result)
			}
			// 验证原始错误保留在链中
			if tt.wantOriginal != nil {
				assert.ErrorIs(t, result, tt.wantOriginal)
			}
		})
	}
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
	defer factory.Close(t.Context())
	defer client.Close()

	// 测试锁操作
	ctx := context.Background()
	handle, err := factory.Lock(ctx, "test-lock")
	require.NoError(t, err)
	require.NotNil(t, handle)

	err = handle.Unlock(ctx)
	require.NoError(t, err)
}
