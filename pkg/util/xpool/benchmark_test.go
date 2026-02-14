package xpool

import (
	"sync/atomic"
	"testing"
)

func BenchmarkSubmit(b *testing.B) {
	pool, err := NewWorkerPool(4, 10000, func(_ int) {})
	if err != nil {
		b.Fatal(err)
	}
	defer pool.Stop()

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		pool.Submit(i) //nolint:errcheck // 基准测试中忽略提交错误
	}
}

func BenchmarkSubmit_Parallel(b *testing.B) {
	pool, err := NewWorkerPool(4, 10000, func(_ int) {})
	if err != nil {
		b.Fatal(err)
	}
	defer pool.Stop()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			pool.Submit(i) //nolint:errcheck // 基准测试中忽略提交错误
			i++
		}
	})
}

func BenchmarkSubmitAndProcess(b *testing.B) {
	var processed atomic.Int64
	pool, err := NewWorkerPool(4, 1000, func(n int) {
		processed.Add(1)
	})
	if err != nil {
		b.Fatal(err)
	}
	defer pool.Stop()

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		pool.Submit(i) //nolint:errcheck // 基准测试中忽略提交错误
	}
}
