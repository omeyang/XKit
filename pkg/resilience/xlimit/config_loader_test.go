package xlimit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/omeyang/xkit/pkg/config/xconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// NewXConfProvider
// =============================================================================

func TestNewXConfProvider(t *testing.T) {
	cfg, err := xconf.NewFromBytes([]byte(`ratelimit: {}`), xconf.FormatYAML)
	require.NoError(t, err)

	provider := NewXConfProvider(cfg, "ratelimit")
	require.NotNil(t, provider)
	assert.Equal(t, "ratelimit", provider.path)
}

// =============================================================================
// Load
// =============================================================================

func TestXConfProvider_Load_Success(t *testing.T) {
	data := []byte(`
ratelimit:
  key_prefix: "test:"
  rules:
    - name: "tenant-limit"
      key_template: "tenant:${tenant_id}"
      limit: 100
      window: 60s
`)
	cfg, err := xconf.NewFromBytes(data, xconf.FormatYAML)
	require.NoError(t, err)

	provider := NewXConfProvider(cfg, "ratelimit")
	config, err := provider.Load()
	require.NoError(t, err)
	assert.Equal(t, "test:", config.KeyPrefix)
	require.Len(t, config.Rules, 1)
	assert.Equal(t, "tenant-limit", config.Rules[0].Name)
	assert.Equal(t, 100, config.Rules[0].Limit)
	assert.Equal(t, 60*time.Second, config.Rules[0].Window)
}

func TestXConfProvider_Load_EmptyConfig(t *testing.T) {
	cfg, err := xconf.NewFromBytes([]byte(`ratelimit: {}`), xconf.FormatYAML)
	require.NoError(t, err)

	provider := NewXConfProvider(cfg, "ratelimit")
	config, err := provider.Load()
	require.NoError(t, err)
	assert.Empty(t, config.Rules)
}

func TestXConfProvider_Load_ValidationError(t *testing.T) {
	// 规则缺少必须字段 → Validate 失败
	data := []byte(`
ratelimit:
  rules:
    - name: ""
      limit: 0
`)
	cfg, err := xconf.NewFromBytes(data, xconf.FormatYAML)
	require.NoError(t, err)

	provider := NewXConfProvider(cfg, "ratelimit")
	_, err = provider.Load()
	assert.Error(t, err)
}

func TestXConfProvider_Load_WrongPath(t *testing.T) {
	// 路径不存在时 Unmarshal 返回零值 Config（koanf 的行为）
	data := []byte(`other_section: {key: value}`)
	cfg, err := xconf.NewFromBytes(data, xconf.FormatYAML)
	require.NoError(t, err)

	provider := NewXConfProvider(cfg, "ratelimit")
	config, err := provider.Load()
	require.NoError(t, err)
	assert.Empty(t, config.Rules)
}

// =============================================================================
// Watch
// =============================================================================

func TestXConfProvider_Watch_Lifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	initialData := []byte(`
ratelimit:
  key_prefix: "v1:"
  rules:
    - name: "r1"
      key_template: "global"
      limit: 10
      window: 60s
`)
	require.NoError(t, os.WriteFile(configPath, initialData, 0o600))

	cfg, err := xconf.New(configPath)
	require.NoError(t, err)

	provider := NewXConfProvider(cfg, "ratelimit")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := provider.Watch(ctx)
	require.NoError(t, err)
	require.NotNil(t, ch)

	// 等待 watcher 启动
	time.Sleep(100 * time.Millisecond)

	// 修改配置文件
	updatedData := []byte(`
ratelimit:
  key_prefix: "v2:"
  rules:
    - name: "r1"
      key_template: "global"
      limit: 200
      window: 60s
`)
	require.NoError(t, os.WriteFile(configPath, updatedData, 0o600))

	// 读取变更
	select {
	case change := <-ch:
		require.NoError(t, change.Err)
		assert.Equal(t, "v2:", change.NewConfig.KeyPrefix)
		require.Len(t, change.NewConfig.Rules, 1)
		assert.Equal(t, 200, change.NewConfig.Rules[0].Limit)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for config change")
	}

	// 取消 context 关闭 watcher
	cancel()

	// channel 应被关闭
	for range ch {
		// drain remaining events
	}
}

func TestXConfProvider_Watch_ContextCancel(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	require.NoError(t, os.WriteFile(configPath, []byte(`ratelimit: {}`), 0o600))

	cfg, err := xconf.New(configPath)
	require.NoError(t, err)

	provider := NewXConfProvider(cfg, "ratelimit")

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := provider.Watch(ctx)
	require.NoError(t, err)

	// 立即取消
	cancel()

	// channel 最终应被关闭（不应阻塞）
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // 成功：channel 已关闭
			}
		case <-timer.C:
			t.Fatal("timeout: channel was not closed after context cancel")
		}
	}
}

func TestXConfProvider_Watch_BytesConfigError(t *testing.T) {
	// bytes-based config 不支持 Watch
	cfg, err := xconf.NewFromBytes([]byte(`ratelimit: {}`), xconf.FormatYAML)
	require.NoError(t, err)

	provider := NewXConfProvider(cfg, "ratelimit")
	ctx := context.Background()

	_, err = provider.Watch(ctx)
	assert.Error(t, err, "Watch should fail for bytes-based config")
}
