//go:build integration

package xlimit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// =============================================================================
// etcd 测试环境设置
// =============================================================================

func setupEtcd(t *testing.T) (*clientv3.Client, func()) {
	t.Helper()

	if endpoints := os.Getenv("XKIT_ETCD_ENDPOINTS"); endpoints != "" {
		client, err := clientv3.New(clientv3.Config{
			Endpoints:   []string{endpoints},
			DialTimeout: 5 * time.Second,
		})
		if err != nil {
			t.Skipf("无法连接到 etcd %s: %v", endpoints, err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := client.Status(ctx, endpoints); err != nil {
			client.Close()
			t.Skipf("etcd 健康检查失败: %v", err)
		}

		return client, func() { client.Close() }
	}

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "quay.io/coreos/etcd:v3.5.17",
		ExposedPorts: []string{"2379/tcp"},
		Cmd: []string{
			"etcd",
			"--name=etcd0",
			"--advertise-client-urls=http://0.0.0.0:2379",
			"--listen-client-urls=http://0.0.0.0:2379",
			"--initial-advertise-peer-urls=http://0.0.0.0:2380",
			"--listen-peer-urls=http://0.0.0.0:2380",
			"--initial-cluster=etcd0=http://0.0.0.0:2380",
		},
		WaitingFor: wait.ForListeningPort("2379/tcp").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("etcd container not available: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("etcd host failed: %v", err)
	}
	port, err := container.MappedPort(ctx, "2379/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("etcd port failed: %v", err)
	}

	endpoint := host + ":" + port.Port()
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("etcd client failed: %v", err)
	}

	return client, func() {
		client.Close()
		_ = container.Terminate(ctx)
	}
}

// =============================================================================
// EtcdLoader 集成测试
// =============================================================================

func TestEtcdLoader_Load_Integration(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("加载 YAML 配置", func(t *testing.T) {
		key := "/test/xlimit/yaml"
		yamlConfig := `
key_prefix: "etcd-test:"
rules:
  - name: tenant
    key_template: "tenant:${tenant_id}"
    limit: 100
    window: "1m"
`
		_, err := client.Put(ctx, key, yamlConfig)
		require.NoError(t, err)
		defer client.Delete(ctx, key)

		loader := NewEtcdLoader(client, key)
		config, err := loader.Load(ctx)
		require.NoError(t, err)
		assert.Equal(t, "etcd-test:", config.KeyPrefix)
		assert.Len(t, config.Rules, 1)
		assert.Equal(t, "tenant", config.Rules[0].Name)
		assert.Equal(t, 100, config.Rules[0].Limit)
	})

	t.Run("加载 JSON 配置", func(t *testing.T) {
		key := "/test/xlimit/json"
		jsonConfig := `{
  "key_prefix": "json-test:",
  "rules": [
    {
      "name": "api",
      "key_template": "api:${method}:${path}",
      "limit": 500,
      "window": "30s"
    }
  ]
}`
		_, err := client.Put(ctx, key, jsonConfig)
		require.NoError(t, err)
		defer client.Delete(ctx, key)

		loader := NewEtcdLoader(client, key, WithEtcdFormat(FormatJSON))
		config, err := loader.Load(ctx)
		require.NoError(t, err)
		assert.Equal(t, "json-test:", config.KeyPrefix)
		assert.Len(t, config.Rules, 1)
		assert.Equal(t, "api", config.Rules[0].Name)
		assert.Equal(t, 500, config.Rules[0].Limit)
	})

	t.Run("配置不存在", func(t *testing.T) {
		key := "/test/xlimit/nonexistent"
		loader := NewEtcdLoader(client, key)
		_, err := loader.Load(ctx)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrConfigNotFound)
	})

	t.Run("无效 YAML 配置", func(t *testing.T) {
		key := "/test/xlimit/invalid-yaml"
		_, err := client.Put(ctx, key, "invalid: yaml: content: [")
		require.NoError(t, err)
		defer client.Delete(ctx, key)

		loader := NewEtcdLoader(client, key)
		_, err = loader.Load(ctx)
		assert.Error(t, err)
	})

	t.Run("无效 JSON 配置", func(t *testing.T) {
		key := "/test/xlimit/invalid-json"
		_, err := client.Put(ctx, key, "{invalid json}")
		require.NoError(t, err)
		defer client.Delete(ctx, key)

		loader := NewEtcdLoader(client, key, WithEtcdFormat(FormatJSON))
		_, err = loader.Load(ctx)
		assert.Error(t, err)
	})
}

func TestEtcdLoader_SaveConfig_Integration(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("保存并加载配置 - YAML", func(t *testing.T) {
		key := "/test/xlimit/save-yaml"
		defer client.Delete(ctx, key)

		loader := NewEtcdLoader(client, key)

		enabled := true
		config := &Config{
			KeyPrefix:     "save-test:",
			EnableMetrics: true,
			EnableHeaders: true,
			Fallback:      FallbackLocal,
			LocalPodCount: 5,
			Rules: []Rule{
				{
					Name:        "tenant",
					KeyTemplate: "tenant:${tenant_id}",
					Limit:       200,
					Window:      time.Minute,
					Burst:       250,
					Enabled:     &enabled,
					Overrides: []Override{
						{
							Match: "tenant:vip-*",
							Limit: 1000,
							Burst: 1200,
						},
					},
				},
			},
		}

		err := loader.SaveConfig(ctx, config)
		require.NoError(t, err)

		// 重新加载并验证
		loadedConfig, err := loader.Load(ctx)
		require.NoError(t, err)
		assert.Equal(t, config.KeyPrefix, loadedConfig.KeyPrefix)
		assert.Equal(t, config.EnableMetrics, loadedConfig.EnableMetrics)
		assert.Equal(t, config.Fallback, loadedConfig.Fallback)
		assert.Len(t, loadedConfig.Rules, 1)
		assert.Equal(t, config.Rules[0].Name, loadedConfig.Rules[0].Name)
		assert.Equal(t, config.Rules[0].Limit, loadedConfig.Rules[0].Limit)
	})

	t.Run("保存并加载配置 - JSON", func(t *testing.T) {
		key := "/test/xlimit/save-json"
		defer client.Delete(ctx, key)

		loader := NewEtcdLoader(client, key, WithEtcdFormat(FormatJSON))

		config := &Config{
			KeyPrefix: "json-save:",
			Fallback:  FallbackOpen,
			Rules: []Rule{
				{
					Name:        "api",
					KeyTemplate: "api:${path}",
					Limit:       100,
					Window:      30 * time.Second,
				},
			},
		}

		err := loader.SaveConfig(ctx, config)
		require.NoError(t, err)

		// 验证 JSON 格式存储
		resp, err := client.Get(ctx, key)
		require.NoError(t, err)
		assert.Contains(t, string(resp.Kvs[0].Value), "\"key_prefix\"")

		// 重新加载并验证
		loadedConfig, err := loader.Load(ctx)
		require.NoError(t, err)
		assert.Equal(t, config.KeyPrefix, loadedConfig.KeyPrefix)
	})
}

func TestEtcdLoader_Watch_Integration(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("监听配置变更", func(t *testing.T) {
		key := "/test/xlimit/watch"
		initialConfig := `
rules:
  - name: initial
    key_template: initial
    limit: 10
    window: "1s"
`
		_, err := client.Put(ctx, key, initialConfig)
		require.NoError(t, err)
		defer client.Delete(ctx, key)

		loader := NewEtcdLoader(client, key)
		defer loader.Stop()

		watchCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		configCh, err := loader.Watch(watchCtx)
		require.NoError(t, err)

		// 等待 watch 建立
		time.Sleep(100 * time.Millisecond)

		// 更新配置
		updatedConfig := `
rules:
  - name: updated
    key_template: updated
    limit: 20
    window: "2s"
`
		_, err = client.Put(ctx, key, updatedConfig)
		require.NoError(t, err)

		// 等待配置更新通知
		select {
		case config := <-configCh:
			assert.Len(t, config.Rules, 1)
			assert.Equal(t, "updated", config.Rules[0].Name)
			assert.Equal(t, 20, config.Rules[0].Limit)
		case <-time.After(5 * time.Second):
			t.Fatal("watch 超时")
		}
	})

	t.Run("停止后 Watch 返回错误", func(t *testing.T) {
		key := "/test/xlimit/watch-stop"
		loader := NewEtcdLoader(client, key)

		loader.Stop()

		_, err := loader.Watch(ctx)
		assert.Error(t, err)
	})

	t.Run("多次配置更新", func(t *testing.T) {
		key := "/test/xlimit/watch-multi"
		initialConfig := `
rules:
  - name: v1
    key_template: v1
    limit: 10
    window: "1s"
`
		_, err := client.Put(ctx, key, initialConfig)
		require.NoError(t, err)
		defer client.Delete(ctx, key)

		loader := NewEtcdLoader(client, key)
		defer loader.Stop()

		watchCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		configCh, err := loader.Watch(watchCtx)
		require.NoError(t, err)

		// 等待 watch 建立
		time.Sleep(100 * time.Millisecond)

		// 多次更新配置
		versions := []string{"v2", "v3"}
		for _, version := range versions {
			config := `
rules:
  - name: ` + version + `
    key_template: ` + version + `
    limit: 100
    window: "1m"
`
			_, err = client.Put(ctx, key, config)
			require.NoError(t, err)

			select {
			case config := <-configCh:
				assert.Equal(t, version, config.Rules[0].Name)
			case <-time.After(5 * time.Second):
				t.Fatalf("watch 超时等待版本 %s", version)
			}
		}
	})
}

func TestEtcdLoader_WithReloadableConfig_Integration(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	ctx := context.Background()
	key := "/test/xlimit/reloadable"

	initialConfig := `
rules:
  - name: test
    key_template: test
    limit: 10
    window: "1s"
`
	_, err := client.Put(ctx, key, initialConfig)
	require.NoError(t, err)
	defer client.Delete(ctx, key)

	loader := NewEtcdLoader(client, key)

	var configChanged bool
	reloadable := NewReloadableConfig(loader, func(_, newConfig *Config) {
		if newConfig.Rules[0].Limit == 50 {
			configChanged = true
		}
	})

	startCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	err = reloadable.Start(startCtx)
	require.NoError(t, err)
	defer reloadable.Stop()

	// 验证初始配置
	config := reloadable.GetConfig()
	require.NotNil(t, config)
	assert.Equal(t, 10, config.Rules[0].Limit)

	// 更新配置
	updatedConfig := `
rules:
  - name: test
    key_template: test
    limit: 50
    window: "1s"
`
	_, err = client.Put(ctx, key, updatedConfig)
	require.NoError(t, err)

	// 等待配置更新（使用轮询检查，最多等待5秒）
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		config = reloadable.GetConfig()
		if config != nil && config.Rules[0].Limit == 50 {
			configChanged = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 验证配置已更新
	config = reloadable.GetConfig()
	assert.Equal(t, 50, config.Rules[0].Limit)
	assert.True(t, configChanged, "onChange 回调应该被调用")
}

// =============================================================================
// 边界情况和错误处理测试
// =============================================================================

func TestEtcdLoader_ErrorCases_Integration(t *testing.T) {
	client, cleanup := setupEtcd(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("不支持的配置格式", func(t *testing.T) {
		key := "/test/xlimit/unsupported-format"
		_, err := client.Put(ctx, key, "test")
		require.NoError(t, err)
		defer client.Delete(ctx, key)

		loader := &EtcdLoader{
			client: client,
			key:    key,
			format: ConfigFormat("unsupported"),
		}

		_, err = loader.Load(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "不支持的配置格式")
	})

	t.Run("保存配置 - 不支持的格式", func(t *testing.T) {
		key := "/test/xlimit/save-unsupported"
		loader := &EtcdLoader{
			client: client,
			key:    key,
			format: ConfigFormat("unsupported"),
		}

		config := &Config{
			Rules: []Rule{{Name: "test", KeyTemplate: "test", Limit: 10, Window: time.Second}},
		}

		err := loader.SaveConfig(ctx, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "不支持的配置格式")
	})

	t.Run("Context 取消", func(t *testing.T) {
		key := "/test/xlimit/context-cancel"
		loader := NewEtcdLoader(client, key)

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel() // 立即取消

		_, err := loader.Load(cancelCtx)
		assert.Error(t, err)
	})

	t.Run("Stop 幂等性", func(t *testing.T) {
		key := "/test/xlimit/stop-idempotent"
		loader := NewEtcdLoader(client, key)

		// 多次调用 Stop 不应该 panic
		err := loader.Stop()
		assert.NoError(t, err)
		err = loader.Stop()
		assert.NoError(t, err)
	})
}

// =============================================================================
// 完整配置转换测试
// =============================================================================

func TestConfigToFileConfig_Integration(t *testing.T) {
	enabled := true
	config := &Config{
		KeyPrefix:     "test:",
		LocalPodCount: 5,
		EnableMetrics: true,
		EnableHeaders: true,
		Fallback:      FallbackClose,
		Rules: []Rule{
			{
				Name:        "tenant",
				KeyTemplate: "tenant:${tenant_id}",
				Limit:       100,
				Window:      time.Minute,
				Burst:       150,
				Enabled:     &enabled,
				Overrides: []Override{
					{
						Match:  "tenant:vip-*",
						Limit:  500,
						Window: 2 * time.Minute,
						Burst:  700,
					},
					{
						Match: "tenant:basic-*",
						Limit: 50,
					},
				},
			},
		},
	}

	fc := configToFileConfig(config)

	assert.Equal(t, "test:", fc.KeyPrefix)
	assert.Equal(t, 5, fc.LocalPodCount)
	assert.True(t, fc.EnableMetrics)
	assert.True(t, fc.EnableHeaders)
	assert.Equal(t, "fail-close", fc.FallbackStrategy)
	assert.Len(t, fc.Rules, 1)

	rule := fc.Rules[0]
	assert.Equal(t, "tenant", rule.Name)
	assert.Equal(t, 100, rule.Limit)
	assert.Equal(t, 150, rule.Burst)
	assert.NotNil(t, rule.Enabled)
	assert.True(t, *rule.Enabled)
	assert.Len(t, rule.Overrides, 2)

	// 验证 override 转换
	assert.Equal(t, "tenant:vip-*", rule.Overrides[0].Match)
	assert.Equal(t, 500, rule.Overrides[0].Limit)
	assert.Equal(t, "2m0s", rule.Overrides[0].Window)
	assert.Equal(t, 700, rule.Overrides[0].Burst)

	// Window 为 0 时不应该设置
	assert.Equal(t, "", rule.Overrides[1].Window)
}
