package xtenant_test

import (
	"context"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/context/xplatform"
	"github.com/omeyang/xkit/pkg/context/xtenant"

	"google.golang.org/grpc/metadata"
)

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
	if err != nil {
		t.Fatalf("xplatform.Init() error = %v", err)
	}
	t.Cleanup(xplatform.Reset)

	// 设置租户信息
	ctx := context.Background()
	ctx, err = xctx.WithTenantID(ctx, "tenant-123")
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

	// 验证平台信息
	if got := md.Get(xtenant.MetaPlatformID); len(got) == 0 || got[0] != "test-platform-001" {
		t.Errorf("MetaPlatformID = %v, want [test-platform-001]", got)
	}
	if got := md.Get(xtenant.MetaHasParent); len(got) == 0 || got[0] != "true" {
		t.Errorf("MetaHasParent = %v, want [true]", got)
	}
	if got := md.Get(xtenant.MetaUnclassRegionID); len(got) == 0 || got[0] != "region-001" {
		t.Errorf("MetaUnclassRegionID = %v, want [region-001]", got)
	}

	// 验证租户信息
	if got := md.Get(xtenant.MetaTenantID); len(got) == 0 || got[0] != "tenant-123" {
		t.Errorf("MetaTenantID = %v, want [tenant-123]", got)
	}
	if got := md.Get(xtenant.MetaTenantName); len(got) == 0 || got[0] != "TestTenant" {
		t.Errorf("MetaTenantName = %v, want [TestTenant]", got)
	}
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
