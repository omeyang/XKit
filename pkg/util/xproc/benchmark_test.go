package xproc

import "testing"

func BenchmarkProcessID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = ProcessID()
	}
}

func BenchmarkProcessName(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = ProcessName()
	}
}

// BenchmarkProcessName_ColdStart 测量进程名称首次解析（含 os.Executable + baseName）的开销。
// 与 BenchmarkProcessName（缓存命中）配合，覆盖冷/热双路径，
// 确保首次解析路径出现回归时能及时暴露。
func BenchmarkProcessName_ColdStart(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ResetProcessName()
		_ = ProcessName()
	}
}
