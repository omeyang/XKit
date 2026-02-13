package xconf

import (
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// 基准测试数据
// =============================================================================

const benchmarkYAMLSmall = `
app:
  name: test-app
  version: "1.0.0"
server:
  port: 8080
`

const benchmarkYAMLMedium = `
app:
  name: test-app
  version: "1.0.0"
  description: "A test application for benchmarking"
  debug: true
  features:
    - feature1
    - feature2
    - feature3
server:
  host: localhost
  port: 8080
  timeout: 30s
  maxConnections: 100
database:
  host: localhost
  port: 5432
  name: testdb
  user: testuser
  sslMode: disable
  maxIdleConns: 10
  maxOpenConns: 100
logging:
  level: info
  format: json
  output: stdout
`

const benchmarkJSONSmall = `{
  "app": {"name": "test-app", "version": "1.0.0"},
  "server": {"port": 8080}
}`

// =============================================================================
// 辅助函数
// =============================================================================

func createBenchFile(b *testing.B, name, content string) string {
	b.Helper()
	tmpDir := b.TempDir()
	path := filepath.Join(tmpDir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		b.Fatal(err)
	}
	return path
}

// =============================================================================
// New 基准测试
// =============================================================================

func BenchmarkNew_YAML_Small(b *testing.B) {
	path := createBenchFile(b, "config.yaml", benchmarkYAMLSmall)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := New(path)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNew_YAML_Medium(b *testing.B) {
	path := createBenchFile(b, "config.yaml", benchmarkYAMLMedium)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := New(path)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNew_JSON_Small(b *testing.B) {
	path := createBenchFile(b, "config.json", benchmarkJSONSmall)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := New(path)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// NewFromBytes 基准测试
// =============================================================================

func BenchmarkNewFromBytes_YAML_Small(b *testing.B) {
	data := []byte(benchmarkYAMLSmall)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := NewFromBytes(data, FormatYAML)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNewFromBytes_YAML_Medium(b *testing.B) {
	data := []byte(benchmarkYAMLMedium)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := NewFromBytes(data, FormatYAML)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNewFromBytes_JSON_Small(b *testing.B) {
	data := []byte(benchmarkJSONSmall)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := NewFromBytes(data, FormatJSON)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// Client 操作基准测试
// =============================================================================

func BenchmarkClient_String(b *testing.B) {
	cfg, err := NewFromBytes([]byte(benchmarkYAMLMedium), FormatYAML)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.Client().String("app.name")
	}
}

func BenchmarkClient_Int(b *testing.B) {
	cfg, err := NewFromBytes([]byte(benchmarkYAMLMedium), FormatYAML)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.Client().Int("server.port")
	}
}

func BenchmarkClient_Bool(b *testing.B) {
	cfg, err := NewFromBytes([]byte(benchmarkYAMLMedium), FormatYAML)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.Client().Bool("app.debug")
	}
}

func BenchmarkClient_Strings(b *testing.B) {
	cfg, err := NewFromBytes([]byte(benchmarkYAMLMedium), FormatYAML)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.Client().Strings("app.features")
	}
}

// =============================================================================
// Unmarshal 基准测试
// =============================================================================

type BenchAppConfig struct {
	App struct {
		Name    string `koanf:"name"`
		Version string `koanf:"version"`
	} `koanf:"app"`
	Server struct {
		Port int `koanf:"port"`
	} `koanf:"server"`
}

type BenchFullConfig struct {
	App struct {
		Name        string   `koanf:"name"`
		Version     string   `koanf:"version"`
		Description string   `koanf:"description"`
		Debug       bool     `koanf:"debug"`
		Features    []string `koanf:"features"`
	} `koanf:"app"`
	Server struct {
		Host           string `koanf:"host"`
		Port           int    `koanf:"port"`
		Timeout        string `koanf:"timeout"`
		MaxConnections int    `koanf:"maxConnections"`
	} `koanf:"server"`
	Database struct {
		Host         string `koanf:"host"`
		Port         int    `koanf:"port"`
		Name         string `koanf:"name"`
		User         string `koanf:"user"`
		SSLMode      string `koanf:"sslMode"`
		MaxIdleConns int    `koanf:"maxIdleConns"`
		MaxOpenConns int    `koanf:"maxOpenConns"`
	} `koanf:"database"`
	Logging struct {
		Level  string `koanf:"level"`
		Format string `koanf:"format"`
		Output string `koanf:"output"`
	} `koanf:"logging"`
}

func BenchmarkUnmarshal_Small(b *testing.B) {
	cfg, err := NewFromBytes([]byte(benchmarkYAMLSmall), FormatYAML)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var config BenchAppConfig
		if err := cfg.Unmarshal("", &config); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_Medium(b *testing.B) {
	cfg, err := NewFromBytes([]byte(benchmarkYAMLMedium), FormatYAML)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var config BenchFullConfig
		if err := cfg.Unmarshal("", &config); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_Partial(b *testing.B) {
	cfg, err := NewFromBytes([]byte(benchmarkYAMLMedium), FormatYAML)
	if err != nil {
		b.Fatal(err)
	}

	type AppOnly struct {
		Name    string `koanf:"name"`
		Version string `koanf:"version"`
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var config AppOnly
		if err := cfg.Unmarshal("app", &config); err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// Reload 基准测试
// =============================================================================

func BenchmarkReload(b *testing.B) {
	path := createBenchFile(b, "config.yaml", benchmarkYAMLMedium)

	cfg, err := New(path)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.Reload(); err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// 并发基准测试
// =============================================================================

func BenchmarkClient_String_Parallel(b *testing.B) {
	cfg, err := NewFromBytes([]byte(benchmarkYAMLMedium), FormatYAML)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = cfg.Client().String("app.name")
		}
	})
}

func BenchmarkUnmarshal_Parallel(b *testing.B) {
	cfg, err := NewFromBytes([]byte(benchmarkYAMLMedium), FormatYAML)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var config BenchFullConfig
			if err := cfg.Unmarshal("", &config); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// =============================================================================
// 内存分配基准测试
// =============================================================================

func BenchmarkNewFromBytes_Allocs(b *testing.B) {
	data := []byte(benchmarkYAMLMedium)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := NewFromBytes(data, FormatYAML)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_Allocs(b *testing.B) {
	cfg, err := NewFromBytes([]byte(benchmarkYAMLMedium), FormatYAML)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var config BenchFullConfig
		if err := cfg.Unmarshal("", &config); err != nil {
			b.Fatal(err)
		}
	}
}
