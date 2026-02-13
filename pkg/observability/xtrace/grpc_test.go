package xtrace_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/observability/xtrace"

	"google.golang.org/grpc/metadata"
)

// assertMetaValue 断言 metadata 中指定 key 的第一个值等于 expected。
func assertMetaValue(t *testing.T, md metadata.MD, key, expected string) {
	t.Helper()
	vals := md.Get(key)
	require.NotEmpty(t, vals, "metadata key %q should have a value", key)
	assert.Equal(t, expected, vals[0], "metadata key %q", key)
}

// assertMetaEmpty 断言 metadata 中指定 key 没有值。
func assertMetaEmpty(t *testing.T, md metadata.MD, key string) {
	t.Helper()
	vals := md.Get(key)
	assert.Empty(t, vals, "metadata key %q should be empty", key)
}

// =============================================================================
// gRPC Metadata 提取测试
// =============================================================================

func TestExtractFromMetadata(t *testing.T) {
	tests := []struct {
		name string
		md   metadata.MD
		want xtrace.TraceInfo
	}{
		{
			name: "nil Metadata",
			md:   nil,
			want: xtrace.TraceInfo{},
		},
		{
			name: "空 Metadata",
			md:   metadata.MD{},
			want: xtrace.TraceInfo{},
		},
		{
			name: "完整 Metadata",
			md: metadata.Pairs(
				xtrace.MetaTraceID, "0af7651916cd43dd8448eb211c80319c",
				xtrace.MetaSpanID, "b7ad6b7169203331",
				xtrace.MetaRequestID, "req-123",
			),
			want: xtrace.TraceInfo{
				TraceID:   "0af7651916cd43dd8448eb211c80319c",
				SpanID:    "b7ad6b7169203331",
				RequestID: "req-123",
			},
		},
		{
			name: "只有 TraceID",
			md: metadata.Pairs(
				xtrace.MetaTraceID, "trace123",
			),
			want: xtrace.TraceInfo{
				TraceID: "trace123",
			},
		},
		{
			name: "带空白的值会被 trim",
			md: metadata.Pairs(
				xtrace.MetaTraceID, "  trace123  ",
				xtrace.MetaRequestID, "  req456  ",
			),
			want: xtrace.TraceInfo{
				TraceID:   "trace123",
				RequestID: "req456",
			},
		},
		{
			name: "W3C traceparent 解析",
			md: metadata.Pairs(
				xtrace.MetaTraceparent, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			),
			want: xtrace.TraceInfo{
				Traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
				TraceID:     "0af7651916cd43dd8448eb211c80319c",
				SpanID:      "b7ad6b7169203331",
				TraceFlags:  "01",
			},
		},
		{
			name: "W3C traceparent 优先于自定义 Key",
			md: metadata.Pairs(
				xtrace.MetaTraceID, "custom-trace-id",
				xtrace.MetaTraceparent, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			),
			want: xtrace.TraceInfo{
				TraceID:     "0af7651916cd43dd8448eb211c80319c", // W3C traceparent 覆盖自定义值
				SpanID:      "b7ad6b7169203331",
				TraceFlags:  "01",
				Traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			},
		},
		{
			name: "无效 traceparent 被忽略",
			md: metadata.Pairs(
				xtrace.MetaTraceparent, "invalid-format",
			),
			want: xtrace.TraceInfo{
				Traceparent: "invalid-format",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := xtrace.ExtractFromMetadata(tt.md)
			if got.TraceID != tt.want.TraceID {
				t.Errorf("TraceID = %q, want %q", got.TraceID, tt.want.TraceID)
			}
			if got.SpanID != tt.want.SpanID {
				t.Errorf("SpanID = %q, want %q", got.SpanID, tt.want.SpanID)
			}
			if got.RequestID != tt.want.RequestID {
				t.Errorf("RequestID = %q, want %q", got.RequestID, tt.want.RequestID)
			}
			if got.TraceFlags != tt.want.TraceFlags {
				t.Errorf("TraceFlags = %q, want %q", got.TraceFlags, tt.want.TraceFlags)
			}
			if got.Traceparent != tt.want.Traceparent {
				t.Errorf("Traceparent = %q, want %q", got.Traceparent, tt.want.Traceparent)
			}
		})
	}
}

func TestExtractFromIncomingContext(t *testing.T) {
	t.Run("无 Metadata", func(t *testing.T) {
		ctx := context.Background()
		got := xtrace.ExtractFromIncomingContext(ctx)
		if !got.IsEmpty() {
			t.Errorf("ExtractFromIncomingContext() should be empty, got %+v", got)
		}
	})

	t.Run("有 Metadata", func(t *testing.T) {
		md := metadata.Pairs(
			xtrace.MetaTraceID, "t1",
			xtrace.MetaSpanID, "s1",
			xtrace.MetaRequestID, "r1",
		)
		ctx := metadata.NewIncomingContext(context.Background(), md)

		got := xtrace.ExtractFromIncomingContext(ctx)
		if got.TraceID != "t1" {
			t.Errorf("TraceID = %q, want %q", got.TraceID, "t1")
		}
		if got.SpanID != "s1" {
			t.Errorf("SpanID = %q, want %q", got.SpanID, "s1")
		}
		if got.RequestID != "r1" {
			t.Errorf("RequestID = %q, want %q", got.RequestID, "r1")
		}
	})
}

// =============================================================================
// gRPC Metadata 注入测试
// =============================================================================

func TestInjectToOutgoingContext(t *testing.T) {
	t.Run("注入追踪信息", func(t *testing.T) {
		ctx := context.Background()
		ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
		ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")
		ctx, _ = xctx.WithRequestID(ctx, "req-123")

		ctx = xtrace.InjectToOutgoingContext(ctx)

		md, ok := metadata.FromOutgoingContext(ctx)
		require.True(t, ok, "metadata not found in outgoing context")

		assertMetaValue(t, md, xtrace.MetaTraceID, "0af7651916cd43dd8448eb211c80319c")
		assertMetaValue(t, md, xtrace.MetaSpanID, "b7ad6b7169203331")
		assertMetaValue(t, md, xtrace.MetaRequestID, "req-123")

		// 验证 traceparent（-00 表示未采样，因为无法确定实际采样决策）
		assertMetaValue(t, md, xtrace.MetaTraceparent,
			"00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00")
	})

	t.Run("空 context 不添加 metadata", func(t *testing.T) {
		ctx := context.Background()
		ctx = xtrace.InjectToOutgoingContext(ctx)

		md, ok := metadata.FromOutgoingContext(ctx)
		assert.False(t, ok && len(md) > 0,
			"should not add metadata when no trace info, got %v", md)
	})
}

func TestInjectToOutgoingContext_WithExistingMetadata(t *testing.T) {
	// 覆盖 InjectToOutgoingContext: 已有 outgoing metadata 时复制并追加
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")

	// 预设 outgoing metadata
	existing := metadata.Pairs("custom-key", "custom-value")
	ctx = metadata.NewOutgoingContext(ctx, existing)

	ctx = xtrace.InjectToOutgoingContext(ctx)

	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok)

	// 原有 metadata 保留
	assertMetaValue(t, md, "custom-key", "custom-value")
	// 新追踪信息也注入了
	assertMetaValue(t, md, xtrace.MetaTraceID, "0af7651916cd43dd8448eb211c80319c")
}

func TestInjectToOutgoingContext_TraceFlagsPropagation(t *testing.T) {
	// 覆盖 InjectToOutgoingContext: trace-flags 传播到 traceparent
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "0af7651916cd43dd8448eb211c80319c")
	ctx, _ = xctx.WithSpanID(ctx, "b7ad6b7169203331")
	ctx, _ = xctx.WithTraceFlags(ctx, "01")

	ctx = xtrace.InjectToOutgoingContext(ctx)

	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok)

	expected := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	assertMetaValue(t, md, xtrace.MetaTraceparent, expected)
}

func TestInjectTraceToMetadata(t *testing.T) {
	t.Run("nil Metadata 不 panic", func(t *testing.T) {
		info := xtrace.TraceInfo{TraceID: "t1"}
		xtrace.InjectTraceToMetadata(nil, info) // 不应该 panic
	})

	t.Run("注入非空字段（有效 traceparent）", func(t *testing.T) {
		md := metadata.MD{}
		// 使用有效的 W3C traceparent 格式
		validTraceparent := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
		info := xtrace.TraceInfo{
			TraceID:     "0af7651916cd43dd8448eb211c80319c",
			SpanID:      "b7ad6b7169203331",
			RequestID:   "r1",
			Traceparent: validTraceparent,
			Tracestate:  "vendor=value",
		}
		xtrace.InjectTraceToMetadata(md, info)

		assertMetaValue(t, md, xtrace.MetaTraceID, "0af7651916cd43dd8448eb211c80319c")
		assertMetaValue(t, md, xtrace.MetaSpanID, "b7ad6b7169203331")
		assertMetaValue(t, md, xtrace.MetaRequestID, "r1")
		assertMetaValue(t, md, xtrace.MetaTraceparent, validTraceparent)
		assertMetaValue(t, md, xtrace.MetaTracestate, "vendor=value")
	})

	t.Run("空字段不注入", func(t *testing.T) {
		md := metadata.MD{}
		info := xtrace.TraceInfo{TraceID: "t1"} // 只有 TraceID
		xtrace.InjectTraceToMetadata(md, info)

		assertMetaEmpty(t, md, xtrace.MetaSpanID)
	})

	t.Run("无效 traceparent 被拒绝，回退生成", func(t *testing.T) {
		md := metadata.MD{}
		// 无效的 traceparent 格式（太短、格式错误）
		info := xtrace.TraceInfo{
			TraceID:     "0af7651916cd43dd8448eb211c80319c",
			SpanID:      "b7ad6b7169203331",
			Traceparent: "invalid-format", // 无效格式
		}
		xtrace.InjectTraceToMetadata(md, info)

		// 无效 traceparent 应该被拒绝，从 TraceID/SpanID 回退生成
		expected := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00"
		assertMetaValue(t, md, xtrace.MetaTraceparent, expected)
	})

	t.Run("version 00 带额外字段的 traceparent 被拒绝", func(t *testing.T) {
		md := metadata.MD{}
		// version 00 不允许额外字段（长度超过 55）
		info := xtrace.TraceInfo{
			TraceID:     "0af7651916cd43dd8448eb211c80319c",
			SpanID:      "b7ad6b7169203331",
			Traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01-extra",
		}
		xtrace.InjectTraceToMetadata(md, info)

		// 带额外字段的 version 00 应该被拒绝
		gotTraceparent := md.Get(xtrace.MetaTraceparent)
		assert.NotEmpty(t, gotTraceparent, "traceparent should exist")
		assert.NotEqual(t, info.Traceparent, gotTraceparent[0],
			"version 00 traceparent with extra fields should be rejected")
	})
}

// =============================================================================
// Metadata 常量测试
// =============================================================================

func TestMetadataConstants(t *testing.T) {
	// 验证常量值符合 gRPC metadata 的小写连字符约定
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"MetaTraceID", xtrace.MetaTraceID, "x-trace-id"},
		{"MetaSpanID", xtrace.MetaSpanID, "x-span-id"},
		{"MetaRequestID", xtrace.MetaRequestID, "x-request-id"},
		{"MetaTraceparent", xtrace.MetaTraceparent, "traceparent"},
		{"MetaTracestate", xtrace.MetaTracestate, "tracestate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

// =============================================================================
// Context 辅助函数测试
// =============================================================================

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()
	ctx, _ = xctx.WithTraceID(ctx, "trace123")
	ctx, _ = xctx.WithSpanID(ctx, "span456")
	ctx, _ = xctx.WithRequestID(ctx, "req789")
	ctx, _ = xctx.WithTraceFlags(ctx, "01")

	if got := xtrace.TraceID(ctx); got != "trace123" {
		t.Errorf("TraceID() = %q, want %q", got, "trace123")
	}
	if got := xtrace.SpanID(ctx); got != "span456" {
		t.Errorf("SpanID() = %q, want %q", got, "span456")
	}
	if got := xtrace.RequestID(ctx); got != "req789" {
		t.Errorf("RequestID() = %q, want %q", got, "req789")
	}
	if got := xtrace.TraceFlags(ctx); got != "01" {
		t.Errorf("TraceFlags() = %q, want %q", got, "01")
	}
}
