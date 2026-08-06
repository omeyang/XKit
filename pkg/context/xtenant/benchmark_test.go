package xtenant_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/context/xtenant"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// =============================================================================
// Context 操作 Benchmark
// =============================================================================

func BenchmarkTenantID(b *testing.B) {
	ctx := context.Background()
	ctx = mustCtxTenantID(b, ctx, "benchmark-tenant-id")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = xtenant.TenantID(ctx)
	}
}

func BenchmarkTenantName(b *testing.B) {
	ctx := context.Background()
	ctx = mustCtxTenantName(b, ctx, "benchmark-tenant-name")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = xtenant.TenantName(ctx)
	}
}

func BenchmarkWithTenantID(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := xtenant.WithTenantID(ctx, "benchmark-tenant"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWithTenantInfo(b *testing.B) {
	ctx := context.Background()
	info := xtenant.TenantInfo{
		TenantID:   "bench-id",
		TenantName: "bench-name",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := xtenant.WithTenantInfo(ctx, info); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetTenantInfo(b *testing.B) {
	ctx := context.Background()
	ctx = mustCtxTenantID(b, ctx, "bench-id")
	ctx = mustCtxTenantName(b, ctx, "bench-name")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = xtenant.GetTenantInfo(ctx)
	}
}

// =============================================================================
// TenantInfo 操作 Benchmark
// =============================================================================

func BenchmarkTenantInfo_IsEmpty(b *testing.B) {
	info := xtenant.TenantInfo{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = info.IsEmpty()
	}
}

func BenchmarkTenantInfo_IsEmpty_NonEmpty(b *testing.B) {
	info := xtenant.TenantInfo{TenantID: "t1", TenantName: "n1"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = info.IsEmpty()
	}
}

func BenchmarkTenantInfo_Validate(b *testing.B) {
	info := xtenant.TenantInfo{TenantID: "t1"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := info.Validate(); err == nil {
			b.Fatal("expected validation error")
		}
	}
}

// =============================================================================
// HTTP 操作 Benchmark
// =============================================================================

func BenchmarkExtractFromHTTPHeader(b *testing.B) {
	h := http.Header{}
	h.Set(xtenant.HeaderTenantID, "tenant-123")
	h.Set(xtenant.HeaderTenantName, "TestTenant")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = xtenant.ExtractFromHTTPHeader(h)
	}
}

func BenchmarkInjectToRequest(b *testing.B) {
	ctx := context.Background()
	ctx = mustCtxTenantID(b, ctx, "tenant-123")
	ctx = mustCtxTenantName(b, ctx, "TestTenant")
	req := httptest.NewRequest("GET", "/test", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		xtenant.InjectToRequest(ctx, req)
	}
}

func BenchmarkHTTPMiddleware(b *testing.B) {
	handler := xtenant.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = xtenant.TenantID(r.Context())
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(xtenant.HeaderTenantID, "tenant-123")
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.ServeHTTP(w, req)
	}
}

func BenchmarkHTTPMiddleware_Parallel(b *testing.B) {
	handler := xtenant.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = xtenant.TenantID(r.Context())
	}))

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set(xtenant.HeaderTenantID, "tenant-123")
		w := httptest.NewRecorder()

		for pb.Next() {
			handler.ServeHTTP(w, req)
		}
	})
}

// =============================================================================
// gRPC 操作 Benchmark
// =============================================================================

func BenchmarkExtractFromMetadata(b *testing.B) {
	md := metadata.Pairs(
		xtenant.MetaTenantID, "tenant-123",
		xtenant.MetaTenantName, "TestTenant",
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = xtenant.ExtractFromMetadata(md)
	}
}

func BenchmarkInjectToOutgoingContext(b *testing.B) {
	ctx := context.Background()
	ctx = mustCtxTenantID(b, ctx, "tenant-123")
	ctx = mustCtxTenantName(b, ctx, "TestTenant")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = xtenant.InjectToOutgoingContext(ctx)
	}
}

func BenchmarkGRPCUnaryServerInterceptor(b *testing.B) {
	interceptor := xtenant.GRPCUnaryServerInterceptor()

	md := metadata.Pairs(
		xtenant.MetaTenantID, "tenant-123",
		xtenant.MetaTenantName, "TestTenant",
	)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(ctx context.Context, req any) (any, error) {
		return nil, nil
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGRPCUnaryClientInterceptor(b *testing.B) {
	interceptor := xtenant.GRPCUnaryClientInterceptor()

	ctx := context.Background()
	ctx = mustCtxTenantID(b, ctx, "tenant-123")

	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		return nil
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := interceptor(ctx, "/test.Service/Method", nil, nil, nil, invoker); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInjectTenantToMetadata(b *testing.B) {
	info := xtenant.TenantInfo{
		TenantID:   "tenant-123",
		TenantName: "TestTenant",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		md := metadata.MD{}
		xtenant.InjectTenantToMetadata(md, info)
	}
}

func mustCtxTenantID(tb testing.TB, ctx context.Context, tenantID string) context.Context {
	tb.Helper()
	newCtx, err := xctx.WithTenantID(ctx, tenantID)
	if err != nil {
		tb.Fatalf("WithTenantID() error = %v", err)
	}
	return newCtx
}

func mustCtxTenantName(tb testing.TB, ctx context.Context, tenantName string) context.Context {
	tb.Helper()
	newCtx, err := xctx.WithTenantName(ctx, tenantName)
	if err != nil {
		tb.Fatalf("WithTenantName() error = %v", err)
	}
	return newCtx
}
