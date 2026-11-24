package xtenant_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/context/xtenant"

	"google.golang.org/grpc/metadata"
)

// assertMetadataValue 验证 gRPC metadata 中指定 key 的首个值等于 expected。
func assertMetadataValue(t *testing.T, md metadata.MD, key, expected string) {
	t.Helper()
	vals := md.Get(key)
	require.NotEmpty(t, vals, "metadata key %q should be present", key)
	assert.Equal(t, expected, vals[0], "metadata key %q", key)
}

// =============================================================================
// gRPC Metadata 提取测试
// =============================================================================

func TestExtractFromMetadata(t *testing.T) {
	tests := []struct {
		name string
		md   metadata.MD
		want xtenant.TenantInfo
	}{
		{
			name: "nil Metadata",
			md:   nil,
			want: xtenant.TenantInfo{},
		},
		{
			name: "空 Metadata",
			md:   metadata.MD{},
			want: xtenant.TenantInfo{},
		},
		{
			name: "完整 Metadata",
			md: metadata.Pairs(
				xtenant.MetaTenantID, "tenant-123",
				xtenant.MetaTenantName, "TestTenant",
			),
			want: xtenant.TenantInfo{
				TenantID:   "tenant-123",
				TenantName: "TestTenant",
			},
		},
		{
			name: "只有 TenantID",
			md: metadata.Pairs(
				xtenant.MetaTenantID, "tenant-123",
			),
			want: xtenant.TenantInfo{
				TenantID: "tenant-123",
			},
		},
		{
			name: "带空白的值会被 trim",
			md: metadata.Pairs(
				xtenant.MetaTenantID, "  tenant-123  ",
				xtenant.MetaTenantName, "  TestTenant  ",
			),
			want: xtenant.TenantInfo{
				TenantID:   "tenant-123",
				TenantName: "TestTenant",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := xtenant.ExtractFromMetadata(tt.md)
			if got.TenantID != tt.want.TenantID || got.TenantName != tt.want.TenantName {
				t.Errorf("ExtractFromMetadata() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestExtractFromMetadata_MultiValue 验证多值 Metadata 取首值的契约（FG-L1 回归测试）。
//
// doc.go:31 和 grpc.go:396-397 定义了"单值语义，多值取第一个"的行为。
func TestExtractFromMetadata_MultiValue(t *testing.T) {
	// metadata.Pairs 中同一 key 出现多次即构成多值
	md := metadata.Join(
		metadata.Pairs(xtenant.MetaTenantID, "first-tenant"),
		metadata.Pairs(xtenant.MetaTenantID, "second-tenant"),
		metadata.Pairs(xtenant.MetaTenantName, "first-name"),
		metadata.Pairs(xtenant.MetaTenantName, "second-name"),
	)

	info := xtenant.ExtractFromMetadata(md)

	// 应取第一个值
	assert.Equal(t, "first-tenant", info.TenantID, "should take first value for TenantID")
	assert.Equal(t, "first-name", info.TenantName, "should take first value for TenantName")
}

func TestExtractFromIncomingContext(t *testing.T) {
	t.Run("无 Metadata", func(t *testing.T) {
		ctx := context.Background()
		got := xtenant.ExtractFromIncomingContext(ctx)
		if !got.IsEmpty() {
			t.Errorf("ExtractFromIncomingContext() should be empty, got %+v", got)
		}
	})

	t.Run("有 Metadata", func(t *testing.T) {
		md := metadata.Pairs(
			xtenant.MetaTenantID, "t1",
			xtenant.MetaTenantName, "n1",
		)
		ctx := metadata.NewIncomingContext(context.Background(), md)

		got := xtenant.ExtractFromIncomingContext(ctx)
		if got.TenantID != "t1" || got.TenantName != "n1" {
			t.Errorf("ExtractFromIncomingContext() = %+v, want TenantID=t1, TenantName=n1", got)
		}
	})
}

// =============================================================================
// gRPC Metadata 注入测试
// =============================================================================

func TestInjectToOutgoingContext(t *testing.T) {
	t.Run("注入租户信息", func(t *testing.T) {
		ctx := context.Background()
		ctx, err := xctx.WithTenantID(ctx, "tenant-123")
		if err != nil {
			t.Fatalf("xctx.WithTenantID() error = %v", err)
		}
		ctx, err = xctx.WithTenantName(ctx, "TestTenant")
		if err != nil {
			t.Fatalf("xctx.WithTenantName() error = %v", err)
		}

		ctx = xtenant.InjectToOutgoingContext(ctx)

		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			t.Fatal("metadata not found in outgoing context")
		}

		if got := md.Get(xtenant.MetaTenantID); len(got) == 0 || got[0] != "tenant-123" {
			t.Errorf("MetaTenantID = %v, want [tenant-123]", got)
		}
		if got := md.Get(xtenant.MetaTenantName); len(got) == 0 || got[0] != "TestTenant" {
			t.Errorf("MetaTenantName = %v, want [TestTenant]", got)
		}
	})

	t.Run("空 context 不添加 metadata", func(t *testing.T) {
		ctx := context.Background()
		ctx = xtenant.InjectToOutgoingContext(ctx)

		// 当没有任何信息时，不应该添加 metadata
		md, ok := metadata.FromOutgoingContext(ctx)
		if ok && len(md) > 0 {
			t.Errorf("should not add metadata when no tenant info, got %v", md)
		}
	})
}

func TestInjectTenantToMetadata(t *testing.T) {
	t.Run("nil Metadata 不 panic", func(t *testing.T) {
		info := xtenant.TenantInfo{TenantID: "t1"}
		xtenant.InjectTenantToMetadata(nil, info) // 不应该 panic
	})

	t.Run("注入非空字段", func(t *testing.T) {
		md := metadata.MD{}
		info := xtenant.TenantInfo{
			TenantID:   "t1",
			TenantName: "n1",
		}
		xtenant.InjectTenantToMetadata(md, info)

		if got := md.Get(xtenant.MetaTenantID); len(got) == 0 || got[0] != "t1" {
			t.Errorf("MetaTenantID = %v, want [t1]", got)
		}
		if got := md.Get(xtenant.MetaTenantName); len(got) == 0 || got[0] != "n1" {
			t.Errorf("MetaTenantName = %v, want [n1]", got)
		}
	})

	t.Run("空字段不注入", func(t *testing.T) {
		md := metadata.MD{}
		info := xtenant.TenantInfo{TenantID: "t1"} // TenantName 为空
		xtenant.InjectTenantToMetadata(md, info)

		if got := md.Get(xtenant.MetaTenantName); len(got) != 0 {
			t.Errorf("MetaTenantName should be empty, got %v", got)
		}
	})

	t.Run("TrimSpace归一化（FG-M4回归测试）", func(t *testing.T) {
		md := metadata.MD{}
		info := xtenant.TenantInfo{
			TenantID:   "  t1  ",
			TenantName: "  n1  ",
		}
		xtenant.InjectTenantToMetadata(md, info)

		assertMetadataValue(t, md, xtenant.MetaTenantID, "t1")
		assertMetadataValue(t, md, xtenant.MetaTenantName, "n1")
	})

	t.Run("纯空白值不注入（FG-M4回归测试）", func(t *testing.T) {
		md := metadata.MD{}
		info := xtenant.TenantInfo{
			TenantID:   "   ",
			TenantName: "  \t",
		}
		xtenant.InjectTenantToMetadata(md, info)

		assert.Empty(t, md.Get(xtenant.MetaTenantID), "whitespace-only TenantID should not be injected")
		assert.Empty(t, md.Get(xtenant.MetaTenantName), "whitespace-only TenantName should not be injected")
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
		{"MetaPlatformID", xtenant.MetaPlatformID, "x-platform-id"},
		{"MetaTenantID", xtenant.MetaTenantID, "x-tenant-id"},
		{"MetaTenantName", xtenant.MetaTenantName, "x-tenant-name"},
		{"MetaHasParent", xtenant.MetaHasParent, "x-has-parent"},
		{"MetaUnclassRegionID", xtenant.MetaUnclassRegionID, "x-unclass-region-id"},
		{"MetaTraceID", xtenant.MetaTraceID, "x-trace-id"},
		{"MetaSpanID", xtenant.MetaSpanID, "x-span-id"},
		{"MetaRequestID", xtenant.MetaRequestID, "x-request-id"},
		{"MetaTraceFlags", xtenant.MetaTraceFlags, "x-trace-flags"},
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
// gRPC Trace Metadata 提取测试
// =============================================================================

func TestExtractTraceFromMetadata(t *testing.T) {
	tests := []struct {
		name string
		md   metadata.MD
		want xctx.Trace
	}{
		{
			name: "nil Metadata",
			md:   nil,
			want: xctx.Trace{},
		},
		{
			name: "空 Metadata",
			md:   metadata.MD{},
			want: xctx.Trace{},
		},
		{
			name: "完整 Trace Metadata",
			md: metadata.Pairs(
				xtenant.MetaTraceID, "trace-001",
				xtenant.MetaSpanID, "span-001",
				xtenant.MetaRequestID, "req-001",
				xtenant.MetaTraceFlags, "01",
			),
			want: xctx.Trace{
				TraceID:    "trace-001",
				SpanID:     "span-001",
				RequestID:  "req-001",
				TraceFlags: "01",
			},
		},
		{
			name: "部分 Trace Metadata",
			md: metadata.Pairs(
				xtenant.MetaTraceID, "trace-002",
			),
			want: xctx.Trace{
				TraceID: "trace-002",
			},
		},
		{
			name: "带空白的值会被 trim",
			md: metadata.Pairs(
				xtenant.MetaTraceID, "  trace-003  ",
				xtenant.MetaSpanID, "  span-003  ",
			),
			want: xctx.Trace{
				TraceID: "trace-003",
				SpanID:  "span-003",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := xtenant.ExtractTraceFromMetadata(tt.md)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractTraceFromIncomingContext(t *testing.T) {
	t.Run("nil context", func(t *testing.T) {
		var nilCtx context.Context
		got := xtenant.ExtractTraceFromIncomingContext(nilCtx)
		assert.Equal(t, xctx.Trace{}, got)
	})

	t.Run("无 Metadata", func(t *testing.T) {
		got := xtenant.ExtractTraceFromIncomingContext(context.Background())
		assert.Equal(t, xctx.Trace{}, got)
	})

	t.Run("有 Trace Metadata", func(t *testing.T) {
		md := metadata.Pairs(
			xtenant.MetaTraceID, "trace-001",
			xtenant.MetaSpanID, "span-001",
			xtenant.MetaRequestID, "req-001",
			xtenant.MetaTraceFlags, "01",
		)
		ctx := metadata.NewIncomingContext(context.Background(), md)

		got := xtenant.ExtractTraceFromIncomingContext(ctx)
		assert.Equal(t, "trace-001", got.TraceID)
		assert.Equal(t, "span-001", got.SpanID)
		assert.Equal(t, "req-001", got.RequestID)
		assert.Equal(t, "01", got.TraceFlags)
	})
}

func TestExtractFromIncomingContext_NilContext(t *testing.T) {
	var nilCtx context.Context
	got := xtenant.ExtractFromIncomingContext(nilCtx)
	assert.True(t, got.IsEmpty())
}

// =============================================================================
// 补充测试：覆盖更多边界情况
// =============================================================================

func TestInjectToOutgoingContext_OnlyTenantID(t *testing.T) {
	ctx := context.Background()
	ctx, err := xctx.WithTenantID(ctx, "tenant-only")
	if err != nil {
		t.Fatalf("xctx.WithTenantID() error = %v", err)
	}
	// 不设置 TenantName

	ctx = xtenant.InjectToOutgoingContext(ctx)

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("metadata not found in outgoing context")
	}

	if got := md.Get(xtenant.MetaTenantID); len(got) == 0 || got[0] != "tenant-only" {
		t.Errorf("MetaTenantID = %v, want [tenant-only]", got)
	}
	// TenantName 不应该在 metadata 中
	if got := md.Get(xtenant.MetaTenantName); len(got) != 0 {
		t.Errorf("MetaTenantName should not be set, got %v", got)
	}
}

func TestInjectToOutgoingContext_OnlyTenantName(t *testing.T) {
	ctx := context.Background()
	ctx, err := xctx.WithTenantName(ctx, "name-only")
	if err != nil {
		t.Fatalf("xctx.WithTenantName() error = %v", err)
	}
	// 不设置 TenantID

	ctx = xtenant.InjectToOutgoingContext(ctx)

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("metadata not found in outgoing context")
	}

	// TenantID 不应该在 metadata 中
	if got := md.Get(xtenant.MetaTenantID); len(got) != 0 {
		t.Errorf("MetaTenantID should not be set, got %v", got)
	}
	if got := md.Get(xtenant.MetaTenantName); len(got) == 0 || got[0] != "name-only" {
		t.Errorf("MetaTenantName = %v, want [name-only]", got)
	}
}
