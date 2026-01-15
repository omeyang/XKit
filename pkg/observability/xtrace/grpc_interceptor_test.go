package xtrace_test

import (
	"context"
	"errors"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/observability/xtrace"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// =============================================================================
// gRPC 服务端一元拦截器测试
// =============================================================================

func TestGRPCUnaryServerInterceptor(t *testing.T) {
	t.Run("提取追踪信息", func(t *testing.T) {
		var capturedTraceID, capturedSpanID string

		interceptor := xtrace.GRPCUnaryServerInterceptor()

		// 使用有效的 trace ID (32位十六进制) 和 span ID (16位十六进制)
		validTraceID := "0af7651916cd43dd8448eb211c80319c"
		validSpanID := "b7ad6b7169203331"

		// 模拟带追踪信息的 incoming context
		md := metadata.Pairs(
			xtrace.MetaTraceID, validTraceID,
			xtrace.MetaSpanID, validSpanID,
		)
		ctx := metadata.NewIncomingContext(context.Background(), md)

		handler := func(ctx context.Context, req any) (any, error) {
			capturedTraceID = xtrace.TraceID(ctx)
			capturedSpanID = xtrace.SpanID(ctx)
			return "response", nil
		}

		resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)

		if err != nil {
			t.Fatalf("interceptor error = %v", err)
		}
		if resp != "response" {
			t.Errorf("response = %v, want 'response'", resp)
		}
		if capturedTraceID != validTraceID {
			t.Errorf("TraceID = %q, want %q", capturedTraceID, validTraceID)
		}
		if capturedSpanID != validSpanID {
			t.Errorf("SpanID = %q, want %q", capturedSpanID, validSpanID)
		}
	})

	t.Run("无追踪信息时自动生成", func(t *testing.T) {
		var capturedTraceID string

		interceptor := xtrace.GRPCUnaryServerInterceptor()

		ctx := context.Background()
		handler := func(ctx context.Context, req any) (any, error) {
			capturedTraceID = xtrace.TraceID(ctx)
			return "ok", nil
		}

		_, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)

		if err != nil {
			t.Fatalf("interceptor error = %v", err)
		}
		// 默认自动生成
		if capturedTraceID == "" {
			t.Error("TraceID should be auto-generated")
		}
	})

	t.Run("Handler错误传递", func(t *testing.T) {
		interceptor := xtrace.GRPCUnaryServerInterceptor()

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
	t.Run("WithGRPCAutoGenerate(false)不自动生成", func(t *testing.T) {
		var capturedTraceID string

		interceptor := xtrace.GRPCUnaryServerInterceptorWithOptions(
			xtrace.WithGRPCAutoGenerate(false),
		)

		ctx := context.Background()
		handler := func(ctx context.Context, req any) (any, error) {
			capturedTraceID = xtrace.TraceID(ctx)
			return "ok", nil
		}

		_, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)

		if err != nil {
			t.Fatalf("interceptor error = %v", err)
		}
		// 禁用自动生成
		if capturedTraceID != "" {
			t.Errorf("TraceID should be empty when auto-generate is disabled, got %q", capturedTraceID)
		}
	})

	t.Run("WithGRPCAutoGenerate(true)自动生成", func(t *testing.T) {
		var capturedTraceID string

		interceptor := xtrace.GRPCUnaryServerInterceptorWithOptions(
			xtrace.WithGRPCAutoGenerate(true),
		)

		ctx := context.Background()
		handler := func(ctx context.Context, req any) (any, error) {
			capturedTraceID = xtrace.TraceID(ctx)
			return "ok", nil
		}

		_, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{}, handler)

		if err != nil {
			t.Fatalf("interceptor error = %v", err)
		}
		if capturedTraceID == "" {
			t.Error("TraceID should be auto-generated")
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
	t.Run("提取追踪信息", func(t *testing.T) {
		var capturedTraceID string

		interceptor := xtrace.GRPCStreamServerInterceptor()

		// 使用有效的 trace ID (32位十六进制)
		validTraceID := "1234567890abcdef1234567890abcdef"
		md := metadata.Pairs(xtrace.MetaTraceID, validTraceID)
		ctx := metadata.NewIncomingContext(context.Background(), md)

		stream := &mockServerStream{ctx: ctx}
		handler := func(srv any, stream grpc.ServerStream) error {
			capturedTraceID = xtrace.TraceID(stream.Context())
			return nil
		}

		err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)

		if err != nil {
			t.Fatalf("interceptor error = %v", err)
		}
		if capturedTraceID != validTraceID {
			t.Errorf("TraceID = %q, want %q", capturedTraceID, validTraceID)
		}
	})

	t.Run("Handler错误传递", func(t *testing.T) {
		interceptor := xtrace.GRPCStreamServerInterceptor()

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
	t.Run("WithGRPCAutoGenerate(false)不自动生成", func(t *testing.T) {
		var capturedTraceID string

		interceptor := xtrace.GRPCStreamServerInterceptorWithOptions(
			xtrace.WithGRPCAutoGenerate(false),
		)

		ctx := context.Background()
		stream := &mockServerStream{ctx: ctx}
		handler := func(srv any, stream grpc.ServerStream) error {
			capturedTraceID = xtrace.TraceID(stream.Context())
			return nil
		}

		err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)

		if err != nil {
			t.Fatalf("interceptor error = %v", err)
		}
		if capturedTraceID != "" {
			t.Errorf("TraceID should be empty when auto-generate is disabled, got %q", capturedTraceID)
		}
	})
}

// =============================================================================
// gRPC 客户端拦截器测试
// =============================================================================

func TestGRPCUnaryClientInterceptor(t *testing.T) {
	t.Run("注入追踪信息到outgoing context", func(t *testing.T) {
		var capturedMD metadata.MD

		interceptor := xtrace.GRPCUnaryClientInterceptor()

		// 创建带追踪信息的 context
		ctx := context.Background()
		ctx, _ = xctx.WithTraceID(ctx, "client-trace-123")
		ctx, _ = xctx.WithSpanID(ctx, "client-span-456")

		invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			var ok bool
			capturedMD, ok = metadata.FromOutgoingContext(ctx)
			if !ok {
				t.Fatal("outgoing metadata not found")
			}
			return nil
		}

		err := interceptor(ctx, "/test.Service/Method", nil, nil, nil, invoker)

		if err != nil {
			t.Fatalf("interceptor error = %v", err)
		}

		if got := capturedMD.Get(xtrace.MetaTraceID); len(got) == 0 || got[0] != "client-trace-123" {
			t.Errorf("MetaTraceID = %v, want [client-trace-123]", got)
		}
		if got := capturedMD.Get(xtrace.MetaSpanID); len(got) == 0 || got[0] != "client-span-456" {
			t.Errorf("MetaSpanID = %v, want [client-span-456]", got)
		}
	})

	t.Run("Invoker错误传递", func(t *testing.T) {
		interceptor := xtrace.GRPCUnaryClientInterceptor()

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
	t.Run("注入追踪信息到outgoing context", func(t *testing.T) {
		var capturedMD metadata.MD

		interceptor := xtrace.GRPCStreamClientInterceptor()

		// 创建带追踪信息的 context
		ctx := context.Background()
		ctx, _ = xctx.WithTraceID(ctx, "stream-trace-789")

		streamer := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			var ok bool
			capturedMD, ok = metadata.FromOutgoingContext(ctx)
			if !ok {
				t.Fatal("outgoing metadata not found")
			}
			return nil, nil
		}

		_, err := interceptor(ctx, &grpc.StreamDesc{}, nil, "/test.Service/StreamMethod", streamer)

		if err != nil {
			t.Fatalf("interceptor error = %v", err)
		}

		if got := capturedMD.Get(xtrace.MetaTraceID); len(got) == 0 || got[0] != "stream-trace-789" {
			t.Errorf("MetaTraceID = %v, want [stream-trace-789]", got)
		}
	})

	t.Run("Streamer错误传递", func(t *testing.T) {
		interceptor := xtrace.GRPCStreamClientInterceptor()

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
// wrappedServerStream 测试
// =============================================================================

func TestWrappedServerStream_Context(t *testing.T) {
	// 这个测试通过 StreamServerInterceptor 间接测试
	// wrappedServerStream 的 Context() 方法返回包装后的 context
	interceptor := xtrace.GRPCStreamServerInterceptor()

	// 使用有效的 trace ID (32位十六进制)
	validTraceID := "abcdef1234567890abcdef1234567890"
	md := metadata.Pairs(xtrace.MetaTraceID, validTraceID)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	stream := &mockServerStream{ctx: ctx}
	handler := func(srv any, stream grpc.ServerStream) error {
		// 验证 wrapped stream 返回正确的 context
		if traceID := xtrace.TraceID(stream.Context()); traceID != validTraceID {
			t.Errorf("wrapped stream TraceID = %q, want %q", traceID, validTraceID)
		}
		return nil
	}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
	if err != nil {
		t.Fatalf("interceptor error = %v", err)
	}
}
