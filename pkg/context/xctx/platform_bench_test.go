package xctx_test

import (
	"context"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

func BenchmarkWithHasParent(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = xctx.WithHasParent(ctx, true)
	}
}

func BenchmarkHasParent(b *testing.B) {
	ctx, _ := xctx.WithHasParent(context.Background(), true)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = xctx.HasParent(ctx)
	}
}

func BenchmarkHasParentOrDefault(b *testing.B) {
	ctx, _ := xctx.WithHasParent(context.Background(), true)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = xctx.HasParentOrDefault(ctx)
	}
}

func BenchmarkRequireHasParent(b *testing.B) {
	ctx, _ := xctx.WithHasParent(context.Background(), true)
	var err error
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = xctx.RequireHasParent(ctx)
	}
	_ = err
}

func BenchmarkWithUnclassRegionID(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = xctx.WithUnclassRegionID(ctx, "region-001")
	}
}

func BenchmarkUnclassRegionID(b *testing.B) {
	ctx, _ := xctx.WithUnclassRegionID(context.Background(), "region-001")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = xctx.UnclassRegionID(ctx)
	}
}

func BenchmarkGetPlatform(b *testing.B) {
	ctx, _ := xctx.WithHasParent(context.Background(), true)
	ctx, _ = xctx.WithUnclassRegionID(ctx, "region-001")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = xctx.GetPlatform(ctx)
	}
}

func BenchmarkWithPlatform(b *testing.B) {
	ctx := context.Background()
	p := xctx.Platform{
		HasParent:       true,
		UnclassRegionID: "region-001",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = xctx.WithPlatform(ctx, p)
	}
}
