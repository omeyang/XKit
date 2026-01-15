package xtenant_test

import (
	"context"
	"errors"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/context/xtenant"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// =============================================================================
// gRPC 服务端一元拦截器测试
// =============================================================================

func TestGRPCUnaryServerInterceptor(t *testing.T) {
	t.Run("提取租户信息", func(t *testing.T) {
		var capturedTenantID, capturedTenantName string

		interceptor := xtenant.GRPCUnaryServerInterceptor()

		// 模拟带租户信息的 incoming context
		md := metadata.Pairs(
			xtenant.MetaTenantID, "tenant-123",
			xtenant.MetaTenantName, "TestTenant",
		)
		ctx := metadata.NewIncomingContext(context.Background(), md)

		handler := func(ctx context.Context, req any) (any, error) {
			capturedTenantID = xtenant.TenantID(ctx)
			capturedTenantName = xtenant.TenantName(ctx)
			return "response", nil
		}

		resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)

		if err != nil {
			t.Fatalf("interceptor error = %v", err)
		}
		if resp != "response" {
			t.Errorf("response = %v, want 'response'", resp)
		}
		if capturedTenantID != "tenant-123" {
			t.Errorf("TenantID = %q, want %q", capturedTenantID, "tenant-123")
		}
		if capturedTenantName != "TestTenant" {
			t.Errorf("TenantName = %q, want %q", capturedTenantName, "TestTenant")
		}
	})

	t.Run("无租户信息正常通过", func(t *testing.T) {
		interceptor := xtenant.GRPCUnaryServerInterceptor()

		ctx := context.Background()
		handler := func(ctx context.Context, req any) (any, error) {
			return "ok", nil
		}

		resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)

		if err != nil {
			t.Fatalf("interceptor error = %v", err)
		}
		if resp != "ok" {
			t.Errorf("response = %v, want 'ok'", resp)
		}
	})

	t.Run("Handler错误传递", func(t *testing.T) {
		interceptor := xtenant.GRPCUnaryServerInterceptor()

		ctx := context.Background()
		expectedErr := errors.New("handler error")
		handler := func(ctx context.Context, req any) (any, error) {
			return nil, expectedErr
		}

		_, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)

		if err != expectedErr {
			t.Errorf("error = %v, want %v", err, expectedErr)
		}
	})
}

func TestGRPCUnaryServerInterceptorWithOptions(t *testing.T) {
	t.Run("RequireTenant 有租户信息通过", func(t *testing.T) {
		interceptor := xtenant.GRPCUnaryServerInterceptorWithOptions(
			xtenant.WithGRPCRequireTenant(),
		)

		// Validate 要求 TenantID 和 TenantName 都不为空
		md := metadata.Pairs(
			xtenant.MetaTenantID, "tenant-123",
			xtenant.MetaTenantName, "TestTenant",
		)
		ctx := metadata.NewIncomingContext(context.Background(), md)

		handler := func(ctx context.Context, req any) (any, error) {
			return "ok", nil
		}

		resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)

		if err != nil {
			t.Fatalf("interceptor error = %v", err)
		}
		if resp != "ok" {
			t.Errorf("response = %v, want 'ok'", resp)
		}
	})

	t.Run("RequireTenant 无租户信息返回错误", func(t *testing.T) {
		interceptor := xtenant.GRPCUnaryServerInterceptorWithOptions(
			xtenant.WithGRPCRequireTenant(),
		)

		ctx := context.Background()
		handler := func(ctx context.Context, req any) (any, error) {
			return "ok", nil
		}

		_, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)

		if err == nil {
			t.Fatal("expected error when tenant info is missing")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatalf("expected gRPC status error, got %v", err)
		}
		if st.Code() != codes.InvalidArgument {
			t.Errorf("status code = %v, want %v", st.Code(), codes.InvalidArgument)
		}
	})
}

// =============================================================================
// gRPC 服务端流式拦截器测试
// =============================================================================

// mockServerStream 模拟 ServerStream
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}

func TestGRPCStreamServerInterceptor(t *testing.T) {
	t.Run("提取租户信息", func(t *testing.T) {
		var capturedTenantID string

		interceptor := xtenant.GRPCStreamServerInterceptor()

		md := metadata.Pairs(xtenant.MetaTenantID, "tenant-456")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		stream := &mockServerStream{ctx: ctx}
		handler := func(srv any, stream grpc.ServerStream) error {
			capturedTenantID = xtenant.TenantID(stream.Context())
			return nil
		}

		err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)

		if err != nil {
			t.Fatalf("interceptor error = %v", err)
		}
		if capturedTenantID != "tenant-456" {
			t.Errorf("TenantID = %q, want %q", capturedTenantID, "tenant-456")
		}
	})

	t.Run("Handler错误传递", func(t *testing.T) {
		interceptor := xtenant.GRPCStreamServerInterceptor()

		ctx := context.Background()
		stream := &mockServerStream{ctx: ctx}
		expectedErr := errors.New("stream handler error")
		handler := func(srv any, stream grpc.ServerStream) error {
			return expectedErr
		}

		err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)

		if err != expectedErr {
			t.Errorf("error = %v, want %v", err, expectedErr)
		}
	})
}

func TestGRPCStreamServerInterceptorWithOptions(t *testing.T) {
	t.Run("RequireTenant 无租户信息返回错误", func(t *testing.T) {
		interceptor := xtenant.GRPCStreamServerInterceptorWithOptions(
			xtenant.WithGRPCRequireTenant(),
		)

		ctx := context.Background()
		stream := &mockServerStream{ctx: ctx}
		handler := func(srv any, stream grpc.ServerStream) error {
			return nil
		}

		err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)

		if err == nil {
			t.Fatal("expected error when tenant info is missing")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatalf("expected gRPC status error, got %v", err)
		}
		if st.Code() != codes.InvalidArgument {
			t.Errorf("status code = %v, want %v", st.Code(), codes.InvalidArgument)
		}
	})
}

// =============================================================================
// gRPC 客户端拦截器测试
// =============================================================================

func TestGRPCUnaryClientInterceptor(t *testing.T) {
	t.Run("注入租户信息到 outgoing context", func(t *testing.T) {
		var capturedMD metadata.MD

		interceptor := xtenant.GRPCUnaryClientInterceptor()

		// 创建带租户信息的 context
		ctx := context.Background()
		var err error
		ctx, err = xctx.WithTenantID(ctx, "client-tenant-123")
		if err != nil {
			t.Fatalf("WithTenantID() error = %v", err)
		}
		ctx, err = xctx.WithTenantName(ctx, "客户端租户")
		if err != nil {
			t.Fatalf("WithTenantName() error = %v", err)
		}

		invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			var ok bool
			capturedMD, ok = metadata.FromOutgoingContext(ctx)
			if !ok {
				t.Fatal("outgoing metadata not found")
			}
			return nil
		}

		err = interceptor(ctx, "/test.Service/Method", nil, nil, nil, invoker)

		if err != nil {
			t.Fatalf("interceptor error = %v", err)
		}

		if got := capturedMD.Get(xtenant.MetaTenantID); len(got) == 0 || got[0] != "client-tenant-123" {
			t.Errorf("MetaTenantID = %v, want [client-tenant-123]", got)
		}
		if got := capturedMD.Get(xtenant.MetaTenantName); len(got) == 0 || got[0] != "客户端租户" {
			t.Errorf("MetaTenantName = %v, want [客户端租户]", got)
		}
	})

	t.Run("Invoker错误传递", func(t *testing.T) {
		interceptor := xtenant.GRPCUnaryClientInterceptor()

		ctx := context.Background()
		expectedErr := errors.New("invoker error")
		invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			return expectedErr
		}

		err := interceptor(ctx, "/test.Service/Method", nil, nil, nil, invoker)

		if err != expectedErr {
			t.Errorf("error = %v, want %v", err, expectedErr)
		}
	})
}

func TestGRPCStreamClientInterceptor(t *testing.T) {
	t.Run("注入租户信息到 outgoing context", func(t *testing.T) {
		var capturedMD metadata.MD

		interceptor := xtenant.GRPCStreamClientInterceptor()

		// 创建带租户信息的 context
		ctx := context.Background()
		var err error
		ctx, err = xctx.WithTenantID(ctx, "stream-tenant-789")
		if err != nil {
			t.Fatalf("WithTenantID() error = %v", err)
		}

		streamer := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			var ok bool
			capturedMD, ok = metadata.FromOutgoingContext(ctx)
			if !ok {
				t.Fatal("outgoing metadata not found")
			}
			return nil, nil
		}

		_, err = interceptor(ctx, &grpc.StreamDesc{}, nil, "/test.Service/StreamMethod", streamer)

		if err != nil {
			t.Fatalf("interceptor error = %v", err)
		}

		if got := capturedMD.Get(xtenant.MetaTenantID); len(got) == 0 || got[0] != "stream-tenant-789" {
			t.Errorf("MetaTenantID = %v, want [stream-tenant-789]", got)
		}
	})

	t.Run("Streamer错误传递", func(t *testing.T) {
		interceptor := xtenant.GRPCStreamClientInterceptor()

		ctx := context.Background()
		expectedErr := errors.New("streamer error")
		streamer := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			return nil, expectedErr
		}

		_, err := interceptor(ctx, &grpc.StreamDesc{}, nil, "/test.Service/StreamMethod", streamer)

		if err != expectedErr {
			t.Errorf("error = %v, want %v", err, expectedErr)
		}
	})
}

// =============================================================================
// 辅助函数测试
// =============================================================================

func TestWithGRPCRequireTenant(t *testing.T) {
	// 验证 WithGRPCRequireTenant 选项能正确设置
	interceptor := xtenant.GRPCUnaryServerInterceptorWithOptions(
		xtenant.WithGRPCRequireTenant(),
	)

	// 无租户信息应该返回错误
	ctx := context.Background()
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		return nil, nil
	})

	if err == nil {
		t.Error("expected error with WithGRPCRequireTenant when no tenant info")
	}
}
