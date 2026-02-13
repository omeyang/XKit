package xtenant_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/context/xplatform"
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

func TestInjectTenantToMetadata_OnlyTenantID(t *testing.T) {
	md := metadata.MD{}
	info := xtenant.TenantInfo{TenantID: "t1"} // TenantName 为空
	xtenant.InjectTenantToMetadata(md, info)

	if got := md.Get(xtenant.MetaTenantID); len(got) == 0 || got[0] != "t1" {
		t.Errorf("MetaTenantID = %v, want [t1]", got)
	}
	if got := md.Get(xtenant.MetaTenantName); len(got) != 0 {
		t.Errorf("MetaTenantName should be empty, got %v", got)
	}
}

func TestInjectTenantToMetadata_OnlyTenantName(t *testing.T) {
	md := metadata.MD{}
	info := xtenant.TenantInfo{TenantName: "n1"} // TenantID 为空
	xtenant.InjectTenantToMetadata(md, info)

	if got := md.Get(xtenant.MetaTenantID); len(got) != 0 {
		t.Errorf("MetaTenantID should be empty, got %v", got)
	}
	if got := md.Get(xtenant.MetaTenantName); len(got) == 0 || got[0] != "n1" {
		t.Errorf("MetaTenantName = %v, want [n1]", got)
	}
}

func TestExtractFromIncomingContext_OnlyTenantID(t *testing.T) {
	md := metadata.Pairs(
		xtenant.MetaTenantID, "t1",
	)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	got := xtenant.ExtractFromIncomingContext(ctx)
	if got.TenantID != "t1" {
		t.Errorf("TenantID = %q, want %q", got.TenantID, "t1")
	}
	if got.TenantName != "" {
		t.Errorf("TenantName = %q, want empty", got.TenantName)
	}
}

func TestExtractFromIncomingContext_OnlyTenantName(t *testing.T) {
	md := metadata.Pairs(
		xtenant.MetaTenantName, "n1",
	)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	got := xtenant.ExtractFromIncomingContext(ctx)
	if got.TenantID != "" {
		t.Errorf("TenantID = %q, want empty", got.TenantID)
	}
	if got.TenantName != "n1" {
		t.Errorf("TenantName = %q, want %q", got.TenantName, "n1")
	}
}

// =============================================================================
// InjectToOutgoingContext trace 信息传播测试
// =============================================================================

func TestInjectToOutgoingContext_WithTraceInfo(t *testing.T) {
	xplatform.Reset()

	ctx := context.Background()
	var err error
	ctx, err = xctx.WithTraceID(ctx, "trace-001")
	require.NoError(t, err)
	ctx, err = xctx.WithSpanID(ctx, "span-001")
	require.NoError(t, err)
	ctx, err = xctx.WithRequestID(ctx, "req-001")
	require.NoError(t, err)
	ctx, err = xctx.WithTraceFlags(ctx, "01")
	require.NoError(t, err)

	ctx = xtenant.InjectToOutgoingContext(ctx)

	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok, "metadata not found in outgoing context")

	assertMetadataValue(t, md, xtenant.MetaTraceID, "trace-001")
	assertMetadataValue(t, md, xtenant.MetaSpanID, "span-001")
	assertMetadataValue(t, md, xtenant.MetaRequestID, "req-001")
	assertMetadataValue(t, md, xtenant.MetaTraceFlags, "01")
}

func TestInjectToOutgoingContext_PartialTraceInfo(t *testing.T) {
	xplatform.Reset()

	ctx := context.Background()
	var err error
	ctx, err = xctx.WithTraceID(ctx, "trace-only")
	require.NoError(t, err)

	ctx = xtenant.InjectToOutgoingContext(ctx)

	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok)

	assertMetadataValue(t, md, xtenant.MetaTraceID, "trace-only")
	// 未设置的字段不应出现
	assert.Empty(t, md.Get(xtenant.MetaSpanID))
	assert.Empty(t, md.Get(xtenant.MetaRequestID))
	assert.Empty(t, md.Get(xtenant.MetaTraceFlags))
}

func TestInjectToOutgoingContext_NilContext(t *testing.T) {
	xplatform.Reset()

	// nil context 应该 panic（与 Go 标准库行为一致）
	assert.Panics(t, func() {
		var nilCtx context.Context
		xtenant.InjectToOutgoingContext(nilCtx)
	})
}

func TestInjectToOutgoingContext_PreservesExistingMetadata(t *testing.T) {
	xplatform.Reset()

	ctx := context.Background()
	// 预设 outgoing metadata
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("existing-key", "existing-value"))

	var err error
	ctx, err = xctx.WithTenantID(ctx, "tenant-123")
	require.NoError(t, err)

	ctx = xtenant.InjectToOutgoingContext(ctx)

	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok)

	// 应保留已有 metadata
	assertMetadataValue(t, md, "existing-key", "existing-value")
	// 并添加租户信息
	assertMetadataValue(t, md, xtenant.MetaTenantID, "tenant-123")
}

// =============================================================================
// xplatform 集成测试（覆盖平台信息注入分支）
// =============================================================================

func TestInjectToOutgoingContext_WithPlatformInitialized(t *testing.T) {
	// 初始化 xplatform
	xplatform.Reset()
	err := xplatform.Init(xplatform.Config{
		PlatformID:      "test-platform-001",
		HasParent:       true,
		UnclassRegionID: "region-001",
	})
	require.NoError(t, err, "xplatform.Init()")
	t.Cleanup(xplatform.Reset)

	// 设置租户信息
	ctx := context.Background()
	ctx, err = xctx.WithTenantID(ctx, "tenant-123")
	require.NoError(t, err, "xctx.WithTenantID()")
	ctx, err = xctx.WithTenantName(ctx, "TestTenant")
	require.NoError(t, err, "xctx.WithTenantName()")

	ctx = xtenant.InjectToOutgoingContext(ctx)

	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok, "metadata not found in outgoing context")

	// 验证平台信息
	assertMetadataValue(t, md, xtenant.MetaPlatformID, "test-platform-001")
	assertMetadataValue(t, md, xtenant.MetaHasParent, "true")
	assertMetadataValue(t, md, xtenant.MetaUnclassRegionID, "region-001")

	// 验证租户信息
	assertMetadataValue(t, md, xtenant.MetaTenantID, "tenant-123")
	assertMetadataValue(t, md, xtenant.MetaTenantName, "TestTenant")
}

func TestInjectToOutgoingContext_WithPlatformNoParent(t *testing.T) {
	// 初始化 xplatform，HasParent = false
	xplatform.Reset()
	err := xplatform.Init(xplatform.Config{
		PlatformID: "test-platform-002",
		HasParent:  false,
	})
	if err != nil {
		t.Fatalf("xplatform.Init() error = %v", err)
	}
	t.Cleanup(xplatform.Reset)

	ctx := context.Background()
	ctx = xtenant.InjectToOutgoingContext(ctx)

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("metadata not found in outgoing context")
	}

	// 验证 HasParent = false
	if got := md.Get(xtenant.MetaHasParent); len(got) == 0 || got[0] != "false" {
		t.Errorf("MetaHasParent = %v, want [false]", got)
	}
	// UnclassRegionID 为空时不应设置
	if got := md.Get(xtenant.MetaUnclassRegionID); len(got) != 0 {
		t.Errorf("MetaUnclassRegionID should be empty, got %v", got)
	}
}

func TestInjectToOutgoingContext_WithPlatformNotInitialized(t *testing.T) {
	// 确保 xplatform 未初始化
	xplatform.Reset()

	ctx := context.Background()
	ctx, err := xctx.WithTenantID(ctx, "tenant-123")
	if err != nil {
		t.Fatalf("xctx.WithTenantID() error = %v", err)
	}

	ctx = xtenant.InjectToOutgoingContext(ctx)

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("metadata not found in outgoing context")
	}

	// xplatform 未初始化时不应设置平台相关 metadata
	if got := md.Get(xtenant.MetaPlatformID); len(got) != 0 {
		t.Errorf("MetaPlatformID should be empty when xplatform not initialized, got %v", got)
	}
	if got := md.Get(xtenant.MetaHasParent); len(got) != 0 {
		t.Errorf("MetaHasParent should be empty when xplatform not initialized, got %v", got)
	}
	if got := md.Get(xtenant.MetaUnclassRegionID); len(got) != 0 {
		t.Errorf("MetaUnclassRegionID should be empty when xplatform not initialized, got %v", got)
	}
	// 但租户信息应该正常设置
	if got := md.Get(xtenant.MetaTenantID); len(got) == 0 || got[0] != "tenant-123" {
		t.Errorf("MetaTenantID = %v, want [tenant-123]", got)
	}
}

// =============================================================================
// FG-S2: 出站传播清理旧租户键测试
// =============================================================================

func TestInjectToOutgoingContext_ClearsStalePlatformMetadata(t *testing.T) {
	// FG-S1 回归测试：xplatform 未初始化时清除旧平台 Metadata
	xplatform.Reset()
	err := xplatform.Init(xplatform.Config{
		PlatformID:      "plat-001",
		HasParent:       true,
		UnclassRegionID: "region-001",
	})
	require.NoError(t, err, "xplatform.Init()")

	// 先注入平台信息到 outgoing metadata
	ctx := context.Background()
	ctx = xtenant.InjectToOutgoingContext(ctx)

	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok)
	assertMetadataValue(t, md, xtenant.MetaPlatformID, "plat-001")
	assertMetadataValue(t, md, xtenant.MetaHasParent, "true")
	assertMetadataValue(t, md, xtenant.MetaUnclassRegionID, "region-001")

	// Reset xplatform，模拟 metadata 复用但平台未初始化的场景
	xplatform.Reset()
	ctx = xtenant.InjectToOutgoingContext(ctx)

	md, ok = metadata.FromOutgoingContext(ctx)
	require.True(t, ok)
	assert.Empty(t, md.Get(xtenant.MetaPlatformID), "stale PlatformID should be cleared")
	assert.Empty(t, md.Get(xtenant.MetaHasParent), "stale HasParent should be cleared")
	assert.Empty(t, md.Get(xtenant.MetaUnclassRegionID), "stale UnclassRegionID should be cleared")
}

func TestInjectToOutgoingContext_ClearsStaleMetadata(t *testing.T) {
	xplatform.Reset()

	t.Run("清除旧租户metadata", func(t *testing.T) {
		// 预设 outgoing metadata 带有旧租户值
		ctx := context.Background()
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
			xtenant.MetaTenantID, "old-tenant",
			xtenant.MetaTenantName, "OldTenant",
		))

		// 调用 InjectToOutgoingContext，context 无租户信息
		ctx = xtenant.InjectToOutgoingContext(ctx)

		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			t.Fatal("metadata not found")
		}

		// 旧值应被清除
		assert.Empty(t, md.Get(xtenant.MetaTenantID), "stale TenantID should be cleared")
		assert.Empty(t, md.Get(xtenant.MetaTenantName), "stale TenantName should be cleared")
	})

	t.Run("清除旧trace metadata", func(t *testing.T) {
		ctx := context.Background()
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
			xtenant.MetaTraceID, "old-trace",
			xtenant.MetaSpanID, "old-span",
		))

		ctx = xtenant.InjectToOutgoingContext(ctx)

		md, ok := metadata.FromOutgoingContext(ctx)
		// 没有任何信息时，可能不创建 metadata
		if ok {
			assert.Empty(t, md.Get(xtenant.MetaTraceID), "stale TraceID should be cleared")
			assert.Empty(t, md.Get(xtenant.MetaSpanID), "stale SpanID should be cleared")
		}
	})
}

// =============================================================================
// FG-M9: Header/Metadata 常量对应关系测试
// =============================================================================

func TestHeaderMetadataCorrespondence(t *testing.T) {
	// 验证每个 HTTP Header 常量都有对应的 gRPC Metadata 常量
	// 且 strings.ToLower(headerName) == metaName
	pairs := []struct {
		header string
		meta   string
	}{
		{xtenant.HeaderPlatformID, xtenant.MetaPlatformID},
		{xtenant.HeaderTenantID, xtenant.MetaTenantID},
		{xtenant.HeaderTenantName, xtenant.MetaTenantName},
		{xtenant.HeaderHasParent, xtenant.MetaHasParent},
		{xtenant.HeaderUnclassRegionID, xtenant.MetaUnclassRegionID},
		{xtenant.HeaderTraceID, xtenant.MetaTraceID},
		{xtenant.HeaderSpanID, xtenant.MetaSpanID},
		{xtenant.HeaderRequestID, xtenant.MetaRequestID},
		{xtenant.HeaderTraceFlags, xtenant.MetaTraceFlags},
	}

	for _, p := range pairs {
		t.Run(p.header, func(t *testing.T) {
			want := strings.ToLower(p.header)
			assert.Equal(t, want, p.meta,
				"Header %q should map to Metadata %q, got %q", p.header, want, p.meta)
		})
	}
}
