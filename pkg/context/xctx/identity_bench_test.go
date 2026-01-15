package xctx_test

import (
	"context"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

func BenchmarkWithPlatformID(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = xctx.WithPlatformID(ctx, "platform-123")
	}
}

func BenchmarkPlatformID(b *testing.B) {
	ctx, _ := xctx.WithPlatformID(context.Background(), "platform-123")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = xctx.PlatformID(ctx)
	}
}

func BenchmarkGetIdentity(b *testing.B) {
	ctx, _ := xctx.WithPlatformID(context.Background(), "p1")
	ctx, _ = xctx.WithTenantID(ctx, "t1")
	ctx, _ = xctx.WithTenantName(ctx, "n1")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = xctx.GetIdentity(ctx)
	}
}

func BenchmarkIdentity_Validate(b *testing.B) {
	id := xctx.Identity{PlatformID: "p1", TenantID: "t1", TenantName: "n1"}
	if err := id.Validate(); err != nil {
		b.Fatalf("test data invalid: %v", err)
	}
	b.ResetTimer()
	var err error
	for i := 0; i < b.N; i++ {
		err = id.Validate()
	}
	_ = err
}
