package xsys

import "testing"

func BenchmarkSetFileLimit(b *testing.B) {
	// 保存原始限制，测试结束后恢复。
	soft, _, err := GetFileLimit()
	if err == nil {
		defer func() { _ = SetFileLimit(soft) }()
	}

	for b.Loop() {
		_ = SetFileLimit(1024)
	}
}

func BenchmarkGetFileLimit(b *testing.B) {
	for b.Loop() {
		_, _, _ = GetFileLimit()
	}
}
