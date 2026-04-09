package xfile

import (
	"path/filepath"
	"testing"
)

// =============================================================================
// 性能测试（Benchmark）
//
// 使用 b.N 循环，函数调用本身具有副作用（文件系统操作），无需额外防优化措施。
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

// BenchmarkSanitizePathTraversal 测试路径穿越拒绝路径性能
func BenchmarkSanitizePathTraversal(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = SanitizePath("../etc/passwd")
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

// =============================================================================
// SafeJoin 性能测试
// =============================================================================

// BenchmarkSafeJoin 测试安全路径拼接性能
func BenchmarkSafeJoin(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = SafeJoin("/var/log", "app.log")
	}
}

// BenchmarkSafeJoinSubdir 测试子目录路径拼接性能
func BenchmarkSafeJoinSubdir(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = SafeJoin("/var/log", "myapp/service/app.log")
	}
}

// BenchmarkSafeJoinReject 测试路径穿越拒绝路径性能
func BenchmarkSafeJoinReject(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = SafeJoin("/var/log", "../etc/passwd")
	}
}

// BenchmarkSafeJoinParallel 测试并发安全路径拼接性能
func BenchmarkSafeJoinParallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = SafeJoin("/var/log", "app.log")
		}
	})
}

// BenchmarkSafeJoinWithSymlinks 测试带符号链接解析的路径拼接性能
func BenchmarkSafeJoinWithSymlinks(b *testing.B) {
	tmpDir := b.TempDir()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = SafeJoinWithOptions(tmpDir, "app.log", SafeJoinOptions{ResolveSymlinks: true})
	}
}

// =============================================================================
// EnsureDir 性能测试
// =============================================================================

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

// BenchmarkEnsureDirMultiPath 测试多路径目录创建性能
// 注意：26 个路径在首轮创建后，后续迭代测量的是目录已存在时的 stat 性能。
func BenchmarkEnsureDirMultiPath(b *testing.B) {
	tmpDir := b.TempDir()
	// 预生成 26 个不同的路径，避免在循环中依赖索引
	paths := make([]string, 26)
	for i := range paths {
		paths[i] = filepath.Join(tmpDir, "bench", string(rune('a'+i)), "app.log")
	}

	idx := 0
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = EnsureDir(paths[idx%26])
		idx++
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
	paths := make([]string, 26)
	for i := range paths {
		paths[i] = filepath.Join(tmpDir, "a", "b", "c", "d", "e", "f", string(rune('a'+i)), "app.log")
	}

	idx := 0
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = EnsureDir(paths[idx%26])
		idx++
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
