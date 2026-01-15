package xconf

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AppConfig 测试用配置结构体
type AppConfig struct {
	App    App    `koanf:"app"`
	Server Server `koanf:"server"`
}

type App struct {
	Name    string `koanf:"name"`
	Version string `koanf:"version"`
	Debug   bool   `koanf:"debug"`
}

type Server struct {
	Host string `koanf:"host"`
	Port int    `koanf:"port"`
}

// =============================================================================
// 测试数据
// =============================================================================

const testYAMLContent = `
app:
  name: test-app
  version: "1.0.0"
  debug: true
server:
  host: localhost
  port: 8080
`

const testJSONContent = `{
  "app": {
    "name": "test-app",
    "version": "1.0.0",
    "debug": true
  },
  "server": {
    "host": "localhost",
    "port": 8080
  }
}`

// =============================================================================
// 辅助函数
// =============================================================================

func createTempFile(t *testing.T, name, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, name)
	err := os.WriteFile(path, []byte(content), 0600)
	require.NoError(t, err)
	return path
}

// =============================================================================
// New 函数测试
// =============================================================================

func TestNew_YAML(t *testing.T) {
	path := createTempFile(t, "config.yaml", testYAMLContent)

	cfg, err := New(path)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, path, cfg.Path())
	assert.Equal(t, FormatYAML, cfg.Format())

	// 验证可以读取配置值
	assert.Equal(t, "test-app", cfg.Client().String("app.name"))
	assert.Equal(t, "1.0.0", cfg.Client().String("app.version"))
	assert.True(t, cfg.Client().Bool("app.debug"))
	assert.Equal(t, "localhost", cfg.Client().String("server.host"))
	assert.Equal(t, 8080, cfg.Client().Int("server.port"))
}

func TestNew_YML(t *testing.T) {
	path := createTempFile(t, "config.yml", testYAMLContent)

	cfg, err := New(path)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, FormatYAML, cfg.Format())
	assert.Equal(t, "test-app", cfg.Client().String("app.name"))
}

func TestNew_JSON(t *testing.T) {
	path := createTempFile(t, "config.json", testJSONContent)

	cfg, err := New(path)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, path, cfg.Path())
	assert.Equal(t, FormatJSON, cfg.Format())

	assert.Equal(t, "test-app", cfg.Client().String("app.name"))
	assert.Equal(t, 8080, cfg.Client().Int("server.port"))
}

func TestNew_EmptyPath(t *testing.T) {
	cfg, err := New("")
	assert.Nil(t, cfg)
	assert.ErrorIs(t, err, ErrEmptyPath)
}

func TestNew_FileNotExist(t *testing.T) {
	cfg, err := New("/nonexistent/path/config.yaml")
	assert.Nil(t, cfg)
	assert.ErrorIs(t, err, ErrLoadFailed)
}

func TestNew_UnsupportedFormat(t *testing.T) {
	path := createTempFile(t, "config.toml", "key = \"value\"")

	cfg, err := New(path)
	assert.Nil(t, cfg)
	assert.ErrorIs(t, err, ErrUnsupportedFormat)
}

func TestNew_InvalidYAML(t *testing.T) {
	path := createTempFile(t, "config.yaml", "invalid: yaml: content: ::::")

	cfg, err := New(path)
	assert.Nil(t, cfg)
	assert.ErrorIs(t, err, ErrParseFailed)
}

func TestNew_InvalidJSON(t *testing.T) {
	path := createTempFile(t, "config.json", "{invalid json}")

	cfg, err := New(path)
	assert.Nil(t, cfg)
	assert.ErrorIs(t, err, ErrParseFailed)
}

func TestNew_WithOptions(t *testing.T) {
	content := `
app:
  name: test-app
`
	path := createTempFile(t, "config.yaml", content)

	cfg, err := New(path, WithDelim("_"), WithTag("json"))
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// 使用自定义分隔符
	assert.Equal(t, "test-app", cfg.Client().String("app_name"))
}

// =============================================================================
// NewFromBytes 函数测试
// =============================================================================

func TestNewFromBytes_YAML(t *testing.T) {
	cfg, err := NewFromBytes([]byte(testYAMLContent), FormatYAML)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Empty(t, cfg.Path())
	assert.Equal(t, FormatYAML, cfg.Format())

	assert.Equal(t, "test-app", cfg.Client().String("app.name"))
	assert.Equal(t, 8080, cfg.Client().Int("server.port"))
}

func TestNewFromBytes_JSON(t *testing.T) {
	cfg, err := NewFromBytes([]byte(testJSONContent), FormatJSON)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Empty(t, cfg.Path())
	assert.Equal(t, FormatJSON, cfg.Format())

	assert.Equal(t, "test-app", cfg.Client().String("app.name"))
}

func TestNewFromBytes_EmptyData(t *testing.T) {
	// 空数据应该可以创建空配置（与 New 行为一致）
	cfg, err := NewFromBytes([]byte{}, FormatYAML)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Empty(t, cfg.Path())
	assert.Equal(t, FormatYAML, cfg.Format())

	// nil 也应该可以创建空配置
	cfg, err = NewFromBytes(nil, FormatJSON)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Empty(t, cfg.Client().String("any.key"))
}

func TestNewFromBytes_EmptyDataConsistentWithNew(t *testing.T) {
	// 验证 NewFromBytes 和 New 对空数据的行为一致
	// 1. New 允许空文件
	emptyPath := createTempFile(t, "empty.yaml", "")
	cfgFromFile, err := New(emptyPath)
	require.NoError(t, err)
	require.NotNil(t, cfgFromFile)

	// 2. NewFromBytes 也允许空数据
	cfgFromBytes, err := NewFromBytes([]byte{}, FormatYAML)
	require.NoError(t, err)
	require.NotNil(t, cfgFromBytes)

	// 3. 两者都返回空配置
	assert.Empty(t, cfgFromFile.Client().String("any.key"))
	assert.Empty(t, cfgFromBytes.Client().String("any.key"))

	// 4. 空配置也可以正常 Unmarshal（返回零值）
	type TestConfig struct {
		Name string `koanf:"name"`
		Port int    `koanf:"port"`
	}

	var cfgA, cfgB TestConfig
	err = cfgFromFile.Unmarshal("", &cfgA)
	require.NoError(t, err)
	err = cfgFromBytes.Unmarshal("", &cfgB)
	require.NoError(t, err)

	// 零值
	assert.Empty(t, cfgA.Name)
	assert.Zero(t, cfgA.Port)
	assert.Empty(t, cfgB.Name)
	assert.Zero(t, cfgB.Port)
}

func TestNewFromBytes_UnsupportedFormat(t *testing.T) {
	cfg, err := NewFromBytes([]byte("data"), Format("toml"))
	assert.Nil(t, cfg)
	assert.ErrorIs(t, err, ErrUnsupportedFormat)
}

// =============================================================================
// Unmarshal 测试
// =============================================================================

func TestUnmarshal_Full(t *testing.T) {
	path := createTempFile(t, "config.yaml", testYAMLContent)

	cfg, err := New(path)
	require.NoError(t, err)

	var appCfg AppConfig
	err = cfg.Unmarshal("", &appCfg)
	require.NoError(t, err)

	assert.Equal(t, "test-app", appCfg.App.Name)
	assert.Equal(t, "1.0.0", appCfg.App.Version)
	assert.True(t, appCfg.App.Debug)
	assert.Equal(t, "localhost", appCfg.Server.Host)
	assert.Equal(t, 8080, appCfg.Server.Port)
}

func TestUnmarshal_Partial(t *testing.T) {
	path := createTempFile(t, "config.yaml", testYAMLContent)

	cfg, err := New(path)
	require.NoError(t, err)

	var app App
	err = cfg.Unmarshal("app", &app)
	require.NoError(t, err)

	assert.Equal(t, "test-app", app.Name)
	assert.Equal(t, "1.0.0", app.Version)
	assert.True(t, app.Debug)
}

func TestUnmarshal_NonexistentPath(t *testing.T) {
	path := createTempFile(t, "config.yaml", testYAMLContent)

	cfg, err := New(path)
	require.NoError(t, err)

	var app App
	// 不存在的路径不会报错，只是值为零值
	err = cfg.Unmarshal("nonexistent", &app)
	require.NoError(t, err)
	assert.Empty(t, app.Name)
}

func TestMustUnmarshal_Success(t *testing.T) {
	path := createTempFile(t, "config.yaml", testYAMLContent)

	cfg, err := New(path)
	require.NoError(t, err)

	var appCfg AppConfig
	assert.NotPanics(t, func() {
		cfg.MustUnmarshal("", &appCfg)
	})

	assert.Equal(t, "test-app", appCfg.App.Name)
}

func TestMustUnmarshal_Panic(t *testing.T) {
	path := createTempFile(t, "config.yaml", testYAMLContent)

	cfg, err := New(path)
	require.NoError(t, err)

	// 传入非指针会导致反序列化失败
	var appCfg AppConfig
	assert.Panics(t, func() {
		cfg.MustUnmarshal("", appCfg) // 注意：没有 &
	})
}

// =============================================================================
// Reload 测试
// =============================================================================

func TestReload_Success(t *testing.T) {
	// 初始配置
	initialContent := `
app:
  name: initial-name
  version: "1.0.0"
`
	path := createTempFile(t, "config.yaml", initialContent)

	cfg, err := New(path)
	require.NoError(t, err)

	assert.Equal(t, "initial-name", cfg.Client().String("app.name"))

	// 修改配置文件
	updatedContent := `
app:
  name: updated-name
  version: "2.0.0"
`
	err = os.WriteFile(path, []byte(updatedContent), 0600)
	require.NoError(t, err)

	// 重载配置
	err = cfg.Reload()
	require.NoError(t, err)

	// 验证新值
	assert.Equal(t, "updated-name", cfg.Client().String("app.name"))
	assert.Equal(t, "2.0.0", cfg.Client().String("app.version"))
}

func TestReload_FromBytes_Error(t *testing.T) {
	cfg, err := NewFromBytes([]byte(testYAMLContent), FormatYAML)
	require.NoError(t, err)

	err = cfg.Reload()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot reload config created from bytes")
}

func TestReload_FileDeleted(t *testing.T) {
	path := createTempFile(t, "config.yaml", testYAMLContent)

	cfg, err := New(path)
	require.NoError(t, err)

	// 删除配置文件
	err = os.Remove(path)
	require.NoError(t, err)

	// 重载应该失败
	err = cfg.Reload()
	assert.ErrorIs(t, err, ErrLoadFailed)
}

func TestReload_Concurrent(t *testing.T) {
	path := createTempFile(t, "config.yaml", testYAMLContent)

	cfg, err := New(path)
	require.NoError(t, err)

	var wg sync.WaitGroup
	const goroutines = 10

	// 并发读取和重载
	for i := 0; i < goroutines; i++ {
		wg.Add(2)

		// 读取 goroutine
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = cfg.Client().String("app.name")
			}
		}()

		// 重载 goroutine
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				// 忽略重载错误，仅测试并发安全性
				_ = cfg.Reload() //nolint:errcheck
			}
		}()
	}

	wg.Wait()
}

// =============================================================================
// 内部函数测试
// =============================================================================

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		path     string
		expected Format
		hasError bool
	}{
		{"/path/to/config.yaml", FormatYAML, false},
		{"/path/to/config.yml", FormatYAML, false},
		{"/path/to/config.YAML", FormatYAML, false},
		{"/path/to/config.YML", FormatYAML, false},
		{"/path/to/config.json", FormatJSON, false},
		{"/path/to/config.JSON", FormatJSON, false},
		{"/path/to/config.toml", "", true},
		{"/path/to/config.xml", "", true},
		{"/path/to/config", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			format, err := detectFormat(tt.path)
			if tt.hasError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrUnsupportedFormat)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, format)
			}
		})
	}
}

func TestIsValidFormat(t *testing.T) {
	assert.True(t, isValidFormat(FormatYAML))
	assert.True(t, isValidFormat(FormatJSON))
	assert.False(t, isValidFormat(Format("toml")))
	assert.False(t, isValidFormat(Format("")))
}

// =============================================================================
// 边界情况测试
// =============================================================================

func TestEmptyConfigFile(t *testing.T) {
	path := createTempFile(t, "config.yaml", "")

	// 空文件应该可以加载（但没有任何配置值）
	cfg, err := New(path)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Empty(t, cfg.Client().String("any.key"))
}

func TestNestedConfig(t *testing.T) {
	content := `
level1:
  level2:
    level3:
      value: deep-value
`
	path := createTempFile(t, "config.yaml", content)

	cfg, err := New(path)
	require.NoError(t, err)

	assert.Equal(t, "deep-value", cfg.Client().String("level1.level2.level3.value"))
}

func TestArrayConfig(t *testing.T) {
	content := `
servers:
  - host: server1
    port: 8080
  - host: server2
    port: 8081
`
	path := createTempFile(t, "config.yaml", content)

	cfg, err := New(path)
	require.NoError(t, err)

	type ServerList struct {
		Servers []Server `koanf:"servers"`
	}

	var servers ServerList
	err = cfg.Unmarshal("", &servers)
	require.NoError(t, err)

	assert.Len(t, servers.Servers, 2)
	assert.Equal(t, "server1", servers.Servers[0].Host)
	assert.Equal(t, 8080, servers.Servers[0].Port)
	assert.Equal(t, "server2", servers.Servers[1].Host)
	assert.Equal(t, 8081, servers.Servers[1].Port)
}

func TestMapConfig(t *testing.T) {
	content := `
features:
  feature1: true
  feature2: false
  feature3: true
`
	path := createTempFile(t, "config.yaml", content)

	cfg, err := New(path)
	require.NoError(t, err)

	type Features struct {
		Features map[string]bool `koanf:"features"`
	}

	var features Features
	err = cfg.Unmarshal("", &features)
	require.NoError(t, err)

	assert.Len(t, features.Features, 3)
	assert.True(t, features.Features["feature1"])
	assert.False(t, features.Features["feature2"])
	assert.True(t, features.Features["feature3"])
}
