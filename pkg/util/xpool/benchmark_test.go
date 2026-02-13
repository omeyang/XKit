package xpool

import (
	"sync/atomic"
	"testing"
)

func BenchmarkSubmit(b *testing.B) {
	pool, err := New(4, 10000, func(_ int) {})
	if err != nil {
		b.Fatal(err)
	}
	defer pool.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		pool.Submit(0)
	}
}

func BenchmarkSubmit_Parallel(b *testing.B) {
	pool, err := New(4, 10000, func(_ int) {})
	if err != nil {
		b.Fatal(err)
	}
	defer pool.Close()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pool.Submit(0)
		}
	})
}

func BenchmarkSubmitAndProcess(b *testing.B) {
	var processed atomic.Int64
	pool, err := New(4, 1000, func(_ int) {
		processed.Add(1)
	})
	if err != nil {
		b.Fatal(err)
	}
	defer pool.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		pool.Submit(0)
	}
}

// BenchmarkLifecycle 测量 New→Submit(N)→Close 完整生命周期开销。
func BenchmarkLifecycle(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		pool, err := New(2, 64, func(_ int) {})
		if err != nil {
			b.Fatal(err)
		}
		for j := range 10 {
			pool.Submit(j)
		}
		pool.Close()
	}
}
