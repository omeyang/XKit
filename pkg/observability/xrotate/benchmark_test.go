package xrotate

import (
	"bytes"
	"path/filepath"
	"testing"
)

// =============================================================================
// 性能测试（Benchmark）
// =============================================================================

// BenchmarkWrite 测试写入性能
//
// 测量单次写入操作的性能，包括：
//   - 写入延迟
//   - 内存分配
func BenchmarkWrite(b *testing.B) {
	tmpDir := b.TempDir()
	filename := filepath.Join(tmpDir, "bench.log")

	r, err := NewLumberjack(filename)
	if err != nil {
		b.Fatal(err)
	}
	defer r.Close()

	data := []byte("benchmark log line with some content\n")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r.Write(data)
	}
}

// BenchmarkWriteParallel 测试并发写入性能
//
// 测量多个 goroutine 并发写入时的性能，验证并发安全实现的开销
func BenchmarkWriteParallel(b *testing.B) {
	tmpDir := b.TempDir()
	filename := filepath.Join(tmpDir, "bench_parallel.log")

	r, err := NewLumberjack(filename)
	if err != nil {
		b.Fatal(err)
	}
	defer r.Close()

	data := []byte("benchmark log line with some content\n")

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r.Write(data)
		}
	})
}

// BenchmarkWriteLarge 测试大数据块写入性能
//
// 测量写入 64KB 数据块的吞吐量
func BenchmarkWriteLarge(b *testing.B) {
	tmpDir := b.TempDir()
	filename := filepath.Join(tmpDir, "bench_large.log")

	r, err := NewLumberjack(filename)
	if err != nil {
		b.Fatal(err)
	}
	defer r.Close()

	// 64KB 数据块
	data := bytes.Repeat([]byte("x"), 64*1024)

	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		r.Write(data)
	}
}

// BenchmarkWriteSmall 测试小数据块写入性能
//
// 测量写入短日志行（模拟真实场景）的性能
func BenchmarkWriteSmall(b *testing.B) {
	tmpDir := b.TempDir()
	filename := filepath.Join(tmpDir, "bench_small.log")

	r, err := NewLumberjack(filename)
	if err != nil {
		b.Fatal(err)
	}
	defer r.Close()

	// 模拟真实日志行（约 100 字节）
	data := []byte("2024-01-15T10:30:45.123Z INFO  [main] Processing request from client 192.168.1.100\n")

	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		r.Write(data)
	}
}

// BenchmarkWriteWithOptions 测试使用自定义配置的写入性能
//
// 验证 Option 模式对运行时性能无影响
func BenchmarkWriteWithOptions(b *testing.B) {
	tmpDir := b.TempDir()
	filename := filepath.Join(tmpDir, "bench_options.log")

	r, err := NewLumberjack(filename,
		WithMaxSize(100),
		WithMaxBackups(5),
		WithMaxAge(7),
		WithCompress(false),
		WithLocalTime(true),
	)
	if err != nil {
		b.Fatal(err)
	}
	defer r.Close()

	data := []byte("benchmark log line with some content\n")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r.Write(data)
	}
}

// BenchmarkNewLumberjack 测试创建 Rotator 的性能
//
// 测量初始化开销，包括配置解析、路径检查、目录创建
func BenchmarkNewLumberjack(b *testing.B) {
	tmpDir := b.TempDir()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		filename := filepath.Join(tmpDir, "bench_new.log")
		r, err := NewLumberjack(filename)
		if err != nil {
			b.Fatal(err)
		}
		r.Close()
	}
}

// BenchmarkNewLumberjackWithOptions 测试使用 Option 创建 Rotator 的性能
func BenchmarkNewLumberjackWithOptions(b *testing.B) {
	tmpDir := b.TempDir()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		filename := filepath.Join(tmpDir, "bench_new_opts.log")
		r, err := NewLumberjack(filename,
			WithMaxSize(50),
			WithMaxBackups(10),
			WithMaxAge(7),
			WithCompress(true),
			WithLocalTime(true),
		)
		if err != nil {
			b.Fatal(err)
		}
		r.Close()
	}
}

// BenchmarkRotate 测试手动轮转性能
//
// 测量轮转操作的开销（文件关闭、重命名、创建新文件）
func BenchmarkRotate(b *testing.B) {
	tmpDir := b.TempDir()
	filename := filepath.Join(tmpDir, "bench_rotate.log")

	r, err := NewLumberjack(filename,
		WithCompress(false), // 禁用压缩避免影响测量
		WithMaxBackups(100), // 足够多的备份位置
	)
	if err != nil {
		b.Fatal(err)
	}
	defer r.Close()

	// 先写入一些数据
	r.Write([]byte("initial data\n"))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r.Rotate()
	}
}
