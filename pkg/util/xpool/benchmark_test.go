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

// BenchmarkSubmit_ZeroReject 测量纯 Submit 热路径性能（无拒绝）。
// 使用足够大的队列确保零拒绝，与 BenchmarkSubmit 形成对比。
func BenchmarkSubmit_ZeroReject(b *testing.B) {
	pool, err := New(4, 1<<20, func(_ int) {})
	if err != nil {
		b.Fatal(err)
	}
	defer pool.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := pool.Submit(0); err != nil {
			b.Fatalf("unexpected reject: %v", err)
		}
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

// BenchmarkSubmit_LargeQueue_Parallel 测量并行 Submit 大队列场景性能。
// 使用 1<<20 队列降低拒绝率，与 BenchmarkSubmit_Parallel（小队列）形成对照。
//
// 设计决策: 使用容错计数而非 b.Fatalf——并行场景下 4 个 worker 的消费速率
// 无法保证匹配任意 GOMAXPROCS 的提交速率，大队列减少但不保证零拒绝。
func BenchmarkSubmit_LargeQueue_Parallel(b *testing.B) {
	pool, err := New(4, 1<<20, func(_ int) {})
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
			if err := pool.Submit(j); err != nil {
				b.Fatal(err)
			}
		}
		if err := pool.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
