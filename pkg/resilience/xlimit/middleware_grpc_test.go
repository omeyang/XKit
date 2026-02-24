package xlimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func setupGRPCTestLimiter(t *testing.T, limit int) Limiter {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	limiter, err := New(client,
		WithRules(TenantRule("tenant-limit", limit, time.Minute)),
		WithFallback(""),
	)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	t.Cleanup(func() {
		_ = limiter.Close() //nolint:errcheck // cleanup
		_ = client.Close()  //nolint:errcheck // cleanup
		mr.Close()
	})

	return limiter
}

func TestUnaryServerInterceptor_Basic(t *testing.T) {
	limiter := setupGRPCTestLimiter(t, 10)
	interceptor := UnaryServerInterceptor(limiter)

	// 创建带有 tenant ID 的上下文
	md := metadata.Pairs("x-tenant-id", "test-tenant")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	// 模拟 handler
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "response", nil
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	// 第一个请求应该通过
	resp, err := interceptor(ctx, "request", info, handler)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp != "response" {
		t.Errorf("expected 'response', got %v", resp)
	}
}

func TestUnaryServerInterceptor_RateLimited(t *testing.T) {
	limiter := setupGRPCTestLimiter(t, 2)
	interceptor := UnaryServerInterceptor(limiter)

	md := metadata.Pairs("x-tenant-id", "limited-tenant")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "response", nil
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	// 消耗配额
	for i := 0; i < 2; i++ {
		_, err := interceptor(ctx, "request", info, handler)
		if err != nil {
			t.Fatalf("request %d should pass, got %v", i+1, err)
		}
	}

	// 第三个请求应该被限流
	_, err := interceptor(ctx, "request", info, handler)
	if err == nil {
		t.Fatal("expected rate limit error")
	}

	// 验证返回 ResourceExhausted
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.ResourceExhausted {
		t.Errorf("expected ResourceExhausted, got %v", st.Code())
	}
}

func TestUnaryServerInterceptor_CustomKeyExtractor(t *testing.T) {
	limiter := setupGRPCTestLimiter(t, 10)

	// 自定义键提取器
	extractor := NewGRPCKeyExtractor(
		WithGRPCTenantHeader("x-custom-tenant"),
	)

	interceptor := UnaryServerInterceptor(limiter,
		WithGRPCKeyExtractor(extractor),
	)

	// 使用自定义 header
	md := metadata.Pairs("x-custom-tenant", "custom-tenant")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "response", nil
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	resp, err := interceptor(ctx, "request", info, handler)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp != "response" {
		t.Errorf("expected 'response', got %v", resp)
	}
}

func TestUnaryServerInterceptor_SkipFunc(t *testing.T) {
	limiter := setupGRPCTestLimiter(t, 1)

	// 跳过健康检查
	skipFunc := func(ctx context.Context, info *grpc.UnaryServerInfo) bool {
		return info.FullMethod == "/grpc.health.v1.Health/Check"
	}

	interceptor := UnaryServerInterceptor(limiter,
		WithGRPCSkipFunc(skipFunc),
	)

	md := metadata.Pairs("x-tenant-id", "skip-tenant")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "response", nil
	}

	// 健康检查应该不受限流影响
	healthInfo := &grpc.UnaryServerInfo{
		FullMethod: "/grpc.health.v1.Health/Check",
	}

	for i := 0; i < 10; i++ {
		_, err := interceptor(ctx, "request", healthInfo, handler)
		if err != nil {
			t.Fatalf("health check %d should pass, got %v", i+1, err)
		}
	}
}

func TestUnaryServerInterceptor_DifferentTenants(t *testing.T) {
	limiter := setupGRPCTestLimiter(t, 2)
	interceptor := UnaryServerInterceptor(limiter)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "response", nil
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	// 租户 A 消耗配额
	mdA := metadata.Pairs("x-tenant-id", "tenant-a")
	ctxA := metadata.NewIncomingContext(context.Background(), mdA)

	for i := 0; i < 2; i++ {
		_, err := interceptor(ctxA, "request", info, handler)
		if err != nil {
			t.Fatalf("tenant-a request %d should pass, got %v", i+1, err)
		}
	}

	// 租户 A 被限流
	_, err := interceptor(ctxA, "request", info, handler)
	if err == nil {
		t.Fatal("tenant-a should be limited")
	}

	// 租户 B 应该不受影响
	mdB := metadata.Pairs("x-tenant-id", "tenant-b")
	ctxB := metadata.NewIncomingContext(context.Background(), mdB)

	_, err = interceptor(ctxB, "request", info, handler)
	if err != nil {
		t.Fatalf("tenant-b should pass, got %v", err)
	}
}

func TestGRPCKeyExtractor_Default(t *testing.T) {
	extractor := DefaultGRPCKeyExtractor()

	md := metadata.Pairs(
		"x-tenant-id", "tenant-001",
		"x-caller-id", "order-service",
	)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{
		FullMethod: "/api.v1.UserService/GetUser",
	}

	key := extractor.Extract(ctx, info)

	if key.Tenant != "tenant-001" {
		t.Errorf("expected tenant 'tenant-001', got %q", key.Tenant)
	}
	if key.Caller != "order-service" {
		t.Errorf("expected caller 'order-service', got %q", key.Caller)
	}
	if key.Method != "/api.v1.UserService/GetUser" {
		t.Errorf("expected method '/api.v1.UserService/GetUser', got %q", key.Method)
	}
}

func TestGRPCKeyExtractor_NoMetadata(t *testing.T) {
	extractor := DefaultGRPCKeyExtractor()

	ctx := context.Background()
	info := &grpc.UnaryServerInfo{
		FullMethod: "/api.v1.UserService/GetUser",
	}

	key := extractor.Extract(ctx, info)

	// 没有 metadata 时，Tenant 和 Caller 应该为空
	if key.Tenant != "" {
		t.Errorf("expected empty tenant, got %q", key.Tenant)
	}
	if key.Caller != "" {
		t.Errorf("expected empty caller, got %q", key.Caller)
	}
	// Method 应该从 info 中获取
	if key.Method != "/api.v1.UserService/GetUser" {
		t.Errorf("expected method '/api.v1.UserService/GetUser', got %q", key.Method)
	}
}

func TestStreamServerInterceptor_Basic(t *testing.T) {
	limiter := setupGRPCTestLimiter(t, 10)
	interceptor := StreamServerInterceptor(limiter)

	md := metadata.Pairs("x-tenant-id", "stream-tenant")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return nil
	}

	info := &grpc.StreamServerInfo{
		FullMethod: "/test.Service/StreamMethod",
	}

	// 模拟 ServerStream
	mockStream := &mockServerStream{ctx: ctx}

	// 第一个请求应该通过
	err := interceptor(nil, mockStream, info, handler)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStreamServerInterceptor_RateLimited(t *testing.T) {
	limiter := setupGRPCTestLimiter(t, 2)
	interceptor := StreamServerInterceptor(limiter)

	md := metadata.Pairs("x-tenant-id", "stream-limited-tenant")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return nil
	}

	info := &grpc.StreamServerInfo{
		FullMethod: "/test.Service/StreamMethod",
	}

	mockStream := &mockServerStream{ctx: ctx}

	// 消耗配额
	for i := 0; i < 2; i++ {
		err := interceptor(nil, mockStream, info, handler)
		if err != nil {
			t.Fatalf("stream %d should pass, got %v", i+1, err)
		}
	}

	// 第三个流应该被限流
	err := interceptor(nil, mockStream, info, handler)
	if err == nil {
		t.Fatal("expected rate limit error")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.ResourceExhausted {
		t.Errorf("expected ResourceExhausted, got %v", st.Code())
	}
}

// mockServerStream 用于测试的 mock ServerStream
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}

func TestUnaryServerInterceptor_NilLimiterPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil limiter")
		}
		msg, ok := r.(string)
		if !ok || msg != "xlimit: UnaryServerInterceptor requires a non-nil Limiter" {
			t.Errorf("unexpected panic message: %v", r)
		}
	}()
	UnaryServerInterceptor(nil)
}

func TestStreamServerInterceptor_NilLimiterPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil limiter")
		}
		msg, ok := r.(string)
		if !ok || msg != "xlimit: StreamServerInterceptor requires a non-nil Limiter" {
			t.Errorf("unexpected panic message: %v", r)
		}
	}()
	StreamServerInterceptor(nil)
}

func BenchmarkUnaryServerInterceptor(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	limiter, err := New(client,
		WithRules(TenantRule("tenant-limit", 1000000, time.Minute)),
		WithFallback(""),
	)
	if err != nil {
		b.Fatalf("failed to create limiter: %v", err)
	}
	defer func() { _ = limiter.Close() }() //nolint:errcheck // defer cleanup

	interceptor := UnaryServerInterceptor(limiter)

	md := metadata.Pairs("x-tenant-id", "bench-tenant")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "response", nil
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = interceptor(ctx, "request", info, handler) //nolint:errcheck // benchmark
		}
	})
}
