package xctx_test

import (
	"context"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

func BenchmarkWithDeploymentType(b *testing.B) {
	ctx := context.Background()
	var err error
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, err = xctx.WithDeploymentType(ctx, xctx.DeploymentSaaS)
	}
	_ = err
}

func BenchmarkGetDeploymentType(b *testing.B) {
	ctx, err := xctx.WithDeploymentType(context.Background(), xctx.DeploymentSaaS)
	if err != nil {
		b.Fatalf("setup WithDeploymentType failed: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, err = xctx.GetDeploymentType(ctx)
	}
	_ = err
}

func BenchmarkIsLocal(b *testing.B) {
	ctx, err := xctx.WithDeploymentType(context.Background(), xctx.DeploymentLocal)
	if err != nil {
		b.Fatalf("setup WithDeploymentType failed: %v", err)
	}
	var ok bool
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ok, err = xctx.IsLocal(ctx)
	}
	_, _ = ok, err
}

func BenchmarkParseDeploymentType(b *testing.B) {
	b.ReportAllocs()
	var err error
	for b.Loop() {
		_, err = xctx.ParseDeploymentType("SAAS")
	}
	_ = err
}
