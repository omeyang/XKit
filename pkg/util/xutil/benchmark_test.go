package xutil

import "testing"

func BenchmarkIf_True(b *testing.B) {
	for b.Loop() {
		_ = If(true, 1, 2)
	}
}

func BenchmarkIf_False(b *testing.B) {
	for b.Loop() {
		_ = If(false, 1, 2)
	}
}

func BenchmarkIfString_True(b *testing.B) {
	for b.Loop() {
		_ = If(true, "hello", "world")
	}
}

func BenchmarkIfString_False(b *testing.B) {
	for b.Loop() {
		_ = If(false, "hello", "world")
	}
}
