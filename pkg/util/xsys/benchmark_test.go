//go:build unix

package xsys

import "testing"

func BenchmarkSetFileLimit(b *testing.B) {
	// 1 alloc/op (16 B) 来自 unix.Rlimit 逃逸（传指针给系统调用），此为预期行为。
	// 使用相对于当前 hard limit 的值，确保基准测试走成功路径。
	soft, hard, err := GetFileLimit()
	if err != nil {
		b.Fatalf("GetFileLimit: %v", err)
	}
	b.Cleanup(func() {
		if restoreErr := SetFileLimit(soft); restoreErr != nil {
			b.Errorf("restore rlimit: %v", restoreErr)
		}
	})

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
	// 1 alloc/op (16 B) 来自 unix.Rlimit 逃逸（传指针给系统调用），此为预期行为。
	for b.Loop() {
		if _, _, err := GetFileLimit(); err != nil {
			b.Fatalf("GetFileLimit: %v", err)
		}
	}
}
