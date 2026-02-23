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
	var rejected int64
	for b.Loop() {
		if err := pool.Submit(0); err != nil {
			rejected++
		}
	}
	if rejected > 0 {
		b.ReportMetric(float64(rejected)/float64(b.N)*100, "reject-%")
	}
}

func BenchmarkSubmit_Parallel(b *testing.B) {
	pool, err := New(4, 10000, func(_ int) {})
	if err != nil {
		b.Fatal(err)
	}
	defer pool.Close()

	var rejected atomic.Int64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := pool.Submit(0); err != nil {
				rejected.Add(1)
			}
		}
	})
	if r := rejected.Load(); r > 0 {
		b.ReportMetric(float64(r)/float64(b.N)*100, "reject-%")
	}
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
	var rejected int64
	for b.Loop() {
		if err := pool.Submit(0); err != nil {
			rejected++
		}
	}
	if rejected > 0 {
		b.ReportMetric(float64(rejected)/float64(b.N)*100, "reject-%")
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
