package xctx_test

import (
	"context"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

func BenchmarkIdentityAttrs(b *testing.B) {
	ctx, _ := xctx.WithPlatformID(context.Background(), "platform-123")
	ctx, _ = xctx.WithTenantID(ctx, "tenant-456")
	ctx, _ = xctx.WithTenantName(ctx, "租户名称")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = xctx.IdentityAttrs(ctx)
	}
}

func BenchmarkTraceAttrs(b *testing.B) {
	ctx, _ := xctx.WithTraceID(context.Background(), "trace-123")
	ctx, _ = xctx.WithSpanID(ctx, "span-456")
	ctx, _ = xctx.WithRequestID(ctx, "req-789")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = xctx.TraceAttrs(ctx)
	}
}

func BenchmarkLogAttrs(b *testing.B) {
	ctx, _ := xctx.WithPlatformID(context.Background(), "p1")
	ctx, _ = xctx.WithTenantID(ctx, "t1")
	ctx, _ = xctx.WithTenantName(ctx, "n1")
	ctx, _ = xctx.WithTraceID(ctx, "trace1")
	ctx, _ = xctx.WithSpanID(ctx, "span1")
	ctx, _ = xctx.WithRequestID(ctx, "req1")
	ctx, err := xctx.WithDeploymentType(ctx, xctx.DeploymentSaaS)
	if err != nil {
		b.Fatalf("setup WithDeploymentType failed: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = xctx.LogAttrs(ctx)
	}
	_ = err
}

func BenchmarkLogAttrs_Empty(b *testing.B) {
	ctx := context.Background()
	var err error
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = xctx.LogAttrs(ctx)
	}
	_ = err
}
