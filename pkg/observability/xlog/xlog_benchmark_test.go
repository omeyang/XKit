package xlog_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/omeyang/xkit/pkg/observability/xlog"
)

// =============================================================================
// 性能测试
// =============================================================================

func BenchmarkLogger_Info(b *testing.B) {
	logger, cleanup, err := xlog.New().
		SetOutput(io.Discard). // 避免 benchmark 输出污染和 I/O 开销
		SetLevel(xlog.LevelInfo).
		Build()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := cleanup(); err != nil {
			b.Errorf("cleanup error: %v", err)
		}
	})

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info(ctx, "benchmark message")
	}
}

func BenchmarkLogger_Info_Disabled(b *testing.B) {
	logger, cleanup, err := xlog.New().
		SetOutput(io.Discard). // 避免 benchmark 输出污染和 I/O 开销
		SetLevel(xlog.LevelError).
		Build()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := cleanup(); err != nil {
			b.Errorf("cleanup error: %v", err)
		}
	})

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info(ctx, "should be skipped")
	}
}

func BenchmarkLogger_With(b *testing.B) {
	logger, cleanup, err := xlog.New().Build()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := cleanup(); err != nil {
			b.Errorf("cleanup error: %v", err)
		}
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = logger.With(slog.String("key", "value"))
	}
}
