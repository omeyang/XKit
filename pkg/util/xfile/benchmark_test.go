package xfile

import (
	"path/filepath"
	"testing"
)

// =============================================================================
// 性能测试（Benchmark）
// =============================================================================

// BenchmarkSanitizePath 测试路径规范化性能
func BenchmarkSanitizePath(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = SanitizePath("/var/log/app.log")
	}
}

// BenchmarkSanitizePathLong 测试长路径规范化性能
func BenchmarkSanitizePathLong(b *testing.B) {
	longPath := "/var/log/application/service/component/subcomponent/module/app.log"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = SanitizePath(longPath)
	}
}

// BenchmarkSanitizePathWithDots 测试带点路径规范化性能
func BenchmarkSanitizePathWithDots(b *testing.B) {
	pathWithDots := "/var/./log/./app/./service/./app.log"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = SanitizePath(pathWithDots)
	}
}

// BenchmarkSanitizePathRelative 测试相对路径规范化性能
func BenchmarkSanitizePathRelative(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = SanitizePath("logs/app.log")
	}
}

// BenchmarkSanitizePathParallel 测试并发路径规范化性能
func BenchmarkSanitizePathParallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = SanitizePath("/var/log/app.log")
		}
	})
}

// BenchmarkEnsureDir 测试目录创建性能（目录已存在）
func BenchmarkEnsureDir(b *testing.B) {
	tmpDir := b.TempDir()
	filename := filepath.Join(tmpDir, "app.log")

	// 先创建一次
	_ = EnsureDir(filename)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = EnsureDir(filename)
	}
}

// BenchmarkEnsureDirNew 测试创建新目录性能
func BenchmarkEnsureDirNew(b *testing.B) {
	tmpDir := b.TempDir()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		filename := filepath.Join(tmpDir, "bench", string(rune('a'+i%26)), "app.log")
		_ = EnsureDir(filename)
	}
}

// BenchmarkEnsureDirWithPerm 测试带权限目录创建性能
func BenchmarkEnsureDirWithPerm(b *testing.B) {
	tmpDir := b.TempDir()
	filename := filepath.Join(tmpDir, "app.log")

	// 先创建一次
	_ = EnsureDirWithPerm(filename, 0750)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = EnsureDirWithPerm(filename, 0750)
	}
}

// BenchmarkEnsureDirDeep 测试深层目录创建性能
func BenchmarkEnsureDirDeep(b *testing.B) {
	tmpDir := b.TempDir()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		filename := filepath.Join(tmpDir, "a", "b", "c", "d", "e", "f", string(rune('a'+i%26)), "app.log")
		_ = EnsureDir(filename)
	}
}

// BenchmarkEnsureDirParallel 测试并发目录创建性能
func BenchmarkEnsureDirParallel(b *testing.B) {
	tmpDir := b.TempDir()
	filename := filepath.Join(tmpDir, "app.log")

	// 先创建一次
	_ = EnsureDir(filename)

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = EnsureDir(filename)
		}
	})
}

// BenchmarkEnsureDirCurrentDir 测试当前目录文件性能（快速路径）
func BenchmarkEnsureDirCurrentDir(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = EnsureDir("app.log")
	}
}
