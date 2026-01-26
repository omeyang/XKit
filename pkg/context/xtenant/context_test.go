package xtenant_test

import (
	"context"
	"errors"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
	"github.com/omeyang/xkit/pkg/context/xtenant"
)

// =============================================================================
// TenantInfo 测试
// =============================================================================

func TestTenantInfo_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		info xtenant.TenantInfo
		want bool
	}{
		{
			name: "全部为空",
			info: xtenant.TenantInfo{},
			want: true,
		},
		{
			name: "只有TenantID",
			info: xtenant.TenantInfo{TenantID: "t1"},
			want: false,
		},
		{
			name: "只有TenantName",
			info: xtenant.TenantInfo{TenantName: "n1"},
			want: false,
		},
		{
			name: "全部非空",
			info: xtenant.TenantInfo{TenantID: "t1", TenantName: "n1"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTenantInfo_Validate(t *testing.T) {
	tests := []struct {
		name    string
		info    xtenant.TenantInfo
		wantErr error
	}{
		{
			name:    "全部存在",
			info:    xtenant.TenantInfo{TenantID: "t1", TenantName: "n1"},
			wantErr: nil,
		},
		{
			name:    "缺少TenantID",
			info:    xtenant.TenantInfo{TenantName: "n1"},
			wantErr: xtenant.ErrEmptyTenantID,
		},
		{
			name:    "缺少TenantName",
			info:    xtenant.TenantInfo{TenantID: "t1"},
			wantErr: xtenant.ErrEmptyTenantName,
		},
		{
			name:    "全部缺少",
			info:    xtenant.TenantInfo{},
			wantErr: xtenant.ErrEmptyTenantID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.info.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("Validate() error = %v, want nil", err)
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
				}
			}
		})
	}
}

// =============================================================================
// Context 操作测试
// =============================================================================

func TestTenantID(t *testing.T) {
	t.Run("未设置返回空字符串", func(t *testing.T) {
		ctx := context.Background()
		if got := xtenant.TenantID(ctx); got != "" {
			t.Errorf("TenantID() = %q, want empty", got)
		}
	})

	t.Run("已设置返回值", func(t *testing.T) {
		ctx := context.Background()
		ctx, err := xctx.WithTenantID(ctx, "tenant-123")
		if err != nil {
			t.Fatalf("WithTenantID() error = %v", err)
		}
		if got := xtenant.TenantID(ctx); got != "tenant-123" {
			t.Errorf("TenantID() = %q, want %q", got, "tenant-123")
		}
	})
}

func TestTenantName(t *testing.T) {
	t.Run("未设置返回空字符串", func(t *testing.T) {
		ctx := context.Background()
		if got := xtenant.TenantName(ctx); got != "" {
			t.Errorf("TenantName() = %q, want empty", got)
		}
	})

	t.Run("已设置返回值", func(t *testing.T) {
		ctx := context.Background()
		ctx, err := xctx.WithTenantName(ctx, "TestTenant")
		if err != nil {
			t.Fatalf("WithTenantName() error = %v", err)
		}
		if got := xtenant.TenantName(ctx); got != "TestTenant" {
			t.Errorf("TenantName() = %q, want %q", got, "TestTenant")
		}
	})
}

func TestGetTenantInfo(t *testing.T) {
	t.Run("未设置返回空结构体", func(t *testing.T) {
		ctx := context.Background()
		info := xtenant.GetTenantInfo(ctx)
		if !info.IsEmpty() {
			t.Errorf("GetTenantInfo() should be empty, got %+v", info)
		}
	})

	t.Run("已设置返回完整信息", func(t *testing.T) {
		ctx := context.Background()
		ctx, err := xctx.WithTenantID(ctx, "t1")
		if err != nil {
			t.Fatalf("WithTenantID() error = %v", err)
		}
		ctx, err = xctx.WithTenantName(ctx, "n1")
		if err != nil {
			t.Fatalf("WithTenantName() error = %v", err)
		}

		info := xtenant.GetTenantInfo(ctx)
		if info.TenantID != "t1" || info.TenantName != "n1" {
			t.Errorf("GetTenantInfo() = %+v, want TenantID=t1, TenantName=n1", info)
		}
	})
}

func TestWithTenantID(t *testing.T) {
	t.Run("正常注入", func(t *testing.T) {
		ctx := context.Background()
		ctx, err := xtenant.WithTenantID(ctx, "tenant-123")
		if err != nil {
			t.Fatalf("WithTenantID() error = %v", err)
		}
		if got := xtenant.TenantID(ctx); got != "tenant-123" {
			t.Errorf("TenantID() = %q, want %q", got, "tenant-123")
		}
	})

	t.Run("nil context 返回错误", func(t *testing.T) {
		var nilCtx context.Context
		_, err := xtenant.WithTenantID(nilCtx, "t1")
		if err == nil {
			t.Error("WithTenantID(nil) should return error")
		}
	})
}

func TestWithTenantName(t *testing.T) {
	t.Run("正常注入", func(t *testing.T) {
		ctx := context.Background()
		ctx, err := xtenant.WithTenantName(ctx, "TestTenant")
		if err != nil {
			t.Fatalf("WithTenantName() error = %v", err)
		}
		if got := xtenant.TenantName(ctx); got != "TestTenant" {
			t.Errorf("TenantName() = %q, want %q", got, "TestTenant")
		}
	})

	t.Run("nil context 返回错误", func(t *testing.T) {
		var nilCtx context.Context
		_, err := xtenant.WithTenantName(nilCtx, "n1")
		if err == nil {
			t.Error("WithTenantName(nil) should return error")
		}
	})
}

func TestWithTenantInfo(t *testing.T) {
	t.Run("注入完整信息", func(t *testing.T) {
		ctx := context.Background()
		info := xtenant.TenantInfo{
			TenantID:   "t1",
			TenantName: "n1",
		}
		ctx, err := xtenant.WithTenantInfo(ctx, info)
		if err != nil {
			t.Fatalf("WithTenantInfo() error = %v", err)
		}

		got := xtenant.GetTenantInfo(ctx)
		if got.TenantID != "t1" || got.TenantName != "n1" {
			t.Errorf("GetTenantInfo() = %+v, want %+v", got, info)
		}
	})

	t.Run("只注入非空字段", func(t *testing.T) {
		ctx := context.Background()
		ctx, err := xctx.WithTenantName(ctx, "original")
		if err != nil {
			t.Fatalf("xctx.WithTenantName() error = %v", err)
		}

		info := xtenant.TenantInfo{TenantID: "t1"} // TenantName 为空
		ctx, err = xtenant.WithTenantInfo(ctx, info)
		if err != nil {
			t.Fatalf("WithTenantInfo() error = %v", err)
		}

		// TenantID 被设置，TenantName 保持原值
		if got := xtenant.TenantID(ctx); got != "t1" {
			t.Errorf("TenantID() = %q, want %q", got, "t1")
		}
		if got := xtenant.TenantName(ctx); got != "original" {
			t.Errorf("TenantName() = %q, want %q", got, "original")
		}
	})

	t.Run("nil context 返回错误", func(t *testing.T) {
		var nilCtx context.Context
		_, err := xtenant.WithTenantInfo(nilCtx, xtenant.TenantInfo{TenantID: "t1"})
		if err == nil {
			t.Error("WithTenantInfo(nil) should return error")
		}
	})
}

func TestRequireTenantID(t *testing.T) {
	t.Run("存在时返回值", func(t *testing.T) {
		ctx := context.Background()
		ctx, err := xctx.WithTenantID(ctx, "t1")
		if err != nil {
			t.Fatalf("xctx.WithTenantID() error = %v", err)
		}

		got, err := xtenant.RequireTenantID(ctx)
		if err != nil {
			t.Fatalf("RequireTenantID() error = %v", err)
		}
		if got != "t1" {
			t.Errorf("RequireTenantID() = %q, want %q", got, "t1")
		}
	})

	t.Run("不存在时返回错误", func(t *testing.T) {
		ctx := context.Background()
		_, err := xtenant.RequireTenantID(ctx)
		if err == nil {
			t.Error("RequireTenantID() should return error when not set")
		}
	})
}

func TestRequireTenantName(t *testing.T) {
	t.Run("存在时返回值", func(t *testing.T) {
		ctx := context.Background()
		ctx, err := xctx.WithTenantName(ctx, "n1")
		if err != nil {
			t.Fatalf("xctx.WithTenantName() error = %v", err)
		}

		got, err := xtenant.RequireTenantName(ctx)
		if err != nil {
			t.Fatalf("RequireTenantName() error = %v", err)
		}
		if got != "n1" {
			t.Errorf("RequireTenantName() = %q, want %q", got, "n1")
		}
	})

	t.Run("不存在时返回错误", func(t *testing.T) {
		ctx := context.Background()
		_, err := xtenant.RequireTenantName(ctx)
		if err == nil {
			t.Error("RequireTenantName() should return error when not set")
		}
	})
}
