package storageopt

import (
	"testing"
	"time"
)

func BenchmarkMeasureOperation(b *testing.B) {
	start := time.Now()
	for b.Loop() {
		MeasureOperation(start)
	}
}

func BenchmarkHealthCounter(b *testing.B) {
	var h HealthCounter
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			h.IncPing()
			h.PingCount()
		}
	})
}

func BenchmarkValidatePagination(b *testing.B) {
	for b.Loop() {
		_, _ = ValidatePagination(100, 20) //nolint:errcheck // benchmark 中忽略返回值
	}
}

func BenchmarkCalculateTotalPages(b *testing.B) {
	for b.Loop() {
		CalculateTotalPages(9999, 20)
	}
}
