package xconf_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/omeyang/xkit/pkg/config/xconf"
)

// ExampleNew 演示从文件加载配置。
func ExampleNew() {
	// 创建临时配置文件
	tmpDir, err := os.MkdirTemp("", "xconf-example")
	if err != nil {
		fmt.Printf("failed to create temp dir: %v\n", err)
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }() //nolint:errcheck // cleanup temp dir, error is irrelevant

	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
app:
  name: my-app
  version: "1.0.0"
server:
  port: 8080
`
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		fmt.Printf("failed to write config file: %v\n", err)
		return
	}

	// 加载配置
	cfg, err := xconf.New(configPath)
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		return
	}

	// 使用底层 koanf 客户端读取值
	fmt.Printf("app.name: %s\n", cfg.Client().String("app.name"))
	fmt.Printf("app.version: %s\n", cfg.Client().String("app.version"))
	fmt.Printf("server.port: %d\n", cfg.Client().Int("server.port"))

	// Output:
	// app.name: my-app
	// app.version: 1.0.0
	// server.port: 8080
}

// ExampleNewFromBytes 演示从字节数据加载配置（适用于 K8s ConfigMap）。
func ExampleNewFromBytes() {
	// 模拟从 K8s ConfigMap 读取的配置数据
	configData := []byte(`
app:
  name: k8s-app
  environment: production
`)

	cfg, err := xconf.NewFromBytes(configData, xconf.FormatYAML)
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		return
	}

	fmt.Printf("app.name: %s\n", cfg.Client().String("app.name"))
	fmt.Printf("app.environment: %s\n", cfg.Client().String("app.environment"))

	// Output:
	// app.name: k8s-app
	// app.environment: production
}

// ExampleConfig_Unmarshal 演示类型安全的配置反序列化。
func ExampleConfig_Unmarshal() {
	configData := []byte(`
database:
  host: localhost
  port: 5432
  name: mydb
  maxConns: 10
`)

	cfg, err := xconf.NewFromBytes(configData, xconf.FormatYAML)
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		return
	}

	// 定义配置结构体
	type DatabaseConfig struct {
		Host     string `koanf:"host"`
		Port     int    `koanf:"port"`
		Name     string `koanf:"name"`
		MaxConns int    `koanf:"maxConns"`
	}

	var dbConfig DatabaseConfig
	if err := cfg.Unmarshal("database", &dbConfig); err != nil {
		fmt.Printf("failed to unmarshal config: %v\n", err)
		return
	}

	fmt.Printf("host: %s\n", dbConfig.Host)
	fmt.Printf("port: %d\n", dbConfig.Port)
	fmt.Printf("name: %s\n", dbConfig.Name)
	fmt.Printf("maxConns: %d\n", dbConfig.MaxConns)

	// Output:
	// host: localhost
	// port: 5432
	// name: mydb
	// maxConns: 10
}

// ExampleMustUnmarshal 演示程序启动时的必要配置加载。
func ExampleMustUnmarshal() {
	configData := []byte(`
app:
  name: critical-app
  required: true
`)

	cfg, err := xconf.NewFromBytes(configData, xconf.FormatYAML)
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		return
	}

	type AppConfig struct {
		Name     string `koanf:"name"`
		Required bool   `koanf:"required"`
	}

	var appConfig AppConfig
	xconf.MustUnmarshal(cfg, "app", &appConfig) // 失败时 panic

	fmt.Printf("name: %s\n", appConfig.Name)
	fmt.Printf("required: %t\n", appConfig.Required)

	// Output:
	// name: critical-app
	// required: true
}

// ExampleNew_withOptions 演示使用选项自定义配置行为。
func ExampleNew_withOptions() {
	// 创建临时配置文件
	tmpDir, err := os.MkdirTemp("", "xconf-example")
	if err != nil {
		fmt.Printf("failed to create temp dir: %v\n", err)
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }() //nolint:errcheck // cleanup temp dir, error is irrelevant

	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
app:
  name: custom-app
`
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		fmt.Printf("failed to write config file: %v\n", err)
		return
	}

	// 使用自定义分隔符
	cfg, err := xconf.New(configPath, xconf.WithDelim("_"))
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		return
	}

	// 使用下划线分隔符访问
	fmt.Printf("app_name: %s\n", cfg.Client().String("app_name"))

	// Output:
	// app_name: custom-app
}

// ExampleConfig_Reload 演示配置热重载。
func ExampleConfig_Reload() {
	// 创建临时配置文件
	tmpDir, err := os.MkdirTemp("", "xconf-example")
	if err != nil {
		fmt.Printf("failed to create temp dir: %v\n", err)
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }() //nolint:errcheck // cleanup temp dir, error is irrelevant

	configPath := filepath.Join(tmpDir, "config.yaml")
	initialContent := `
feature:
  enabled: false
`
	if err := os.WriteFile(configPath, []byte(initialContent), 0600); err != nil {
		fmt.Printf("failed to write config file: %v\n", err)
		return
	}

	cfg, err := xconf.New(configPath)
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		return
	}

	fmt.Printf("before reload - feature.enabled: %t\n", cfg.Client().Bool("feature.enabled"))

	// 模拟配置文件被外部更新
	updatedContent := `
feature:
  enabled: true
`
	if err := os.WriteFile(configPath, []byte(updatedContent), 0600); err != nil {
		fmt.Printf("failed to write config file: %v\n", err)
		return
	}

	// 重载配置
	if err := cfg.Reload(); err != nil {
		fmt.Printf("failed to reload config: %v\n", err)
		return
	}

	fmt.Printf("after reload - feature.enabled: %t\n", cfg.Client().Bool("feature.enabled"))

	// Output:
	// before reload - feature.enabled: false
	// after reload - feature.enabled: true
}

// ExampleNewFromBytes_json 演示加载 JSON 格式配置。
func ExampleNewFromBytes_json() {
	configData := []byte(`{
  "api": {
    "endpoint": "https://api.example.com",
    "timeout": 30
  }
}`)

	cfg, err := xconf.NewFromBytes(configData, xconf.FormatJSON)
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		return
	}

	fmt.Printf("api.endpoint: %s\n", cfg.Client().String("api.endpoint"))
	fmt.Printf("api.timeout: %d\n", cfg.Client().Int("api.timeout"))

	// Output:
	// api.endpoint: https://api.example.com
	// api.timeout: 30
}
