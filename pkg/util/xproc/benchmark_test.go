package xproc

import "testing"

func BenchmarkProcessID(b *testing.B) {
	for b.Loop() {
		_ = ProcessID()
	}
}

func BenchmarkProcessName(b *testing.B) {
	for b.Loop() {
		_ = ProcessName()
	}
}
