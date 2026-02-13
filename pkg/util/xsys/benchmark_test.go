//go:build unix

package xsys

import "testing"

func BenchmarkSetFileLimit(b *testing.B) {
	// 使用相对于当前 hard limit 的值，确保基准测试走成功路径。
	soft, hard, err := GetFileLimit()
	if err != nil {
		b.Fatalf("GetFileLimit: %v", err)
	}
	defer func() {
		if restoreErr := SetFileLimit(soft); restoreErr != nil {
			b.Logf("restore rlimit: %v", restoreErr)
		}
	}()

	target := hard / 2
	if target == 0 {
		target = 1
	}

	for b.Loop() {
		if err := SetFileLimit(target); err != nil {
			b.Fatalf("SetFileLimit(%d): %v", target, err)
		}
	}
}

func BenchmarkGetFileLimit(b *testing.B) {
	for b.Loop() {
		if _, _, err := GetFileLimit(); err != nil {
			b.Fatalf("GetFileLimit: %v", err)
		}
	}
}
