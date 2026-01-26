package xctx_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

// =============================================================================
// Identity 操作测试
// =============================================================================

func TestPlatformID(t *testing.T) {
	t.Run("空context返回空字符串", func(t *testing.T) {
		if got := xctx.PlatformID(context.Background()); got != "" {
			t.Errorf("PlatformID(empty) = %q, want empty", got)
		}
	})

	t.Run("正常注入和提取", func(t *testing.T) {
		ctx, err := xctx.WithPlatformID(context.Background(), "platform-001")
		if err != nil {
			t.Fatalf("WithPlatformID() error = %v", err)
		}
		if got := xctx.PlatformID(ctx); got != "platform-001" {
			t.Errorf("PlatformID() = %q, want %q", got, "platform-001")
		}
	})

	t.Run("覆盖写入返回新值", func(t *testing.T) {
		ctx, _ := xctx.WithPlatformID(context.Background(), "old-platform")
		ctx, _ = xctx.WithPlatformID(ctx, "new-platform")
		if got := xctx.PlatformID(ctx); got != "new-platform" {
			t.Errorf("PlatformID(overwrite) = %q, want %q", got, "new-platform")
		}
	})

	t.Run("nil context返回空字符串", func(t *testing.T) {
		var nilCtx context.Context
		if got := xctx.PlatformID(nilCtx); got != "" {
			t.Errorf("PlatformID(nil) = %q, want empty", got)
		}
	})

	t.Run("nil context注入返回ErrNilContext", func(t *testing.T) {
		var nilCtx context.Context
		_, err := xctx.WithPlatformID(nilCtx, "platform-001")
		if !errors.Is(err, xctx.ErrNilContext) {
			t.Errorf("WithPlatformID(nil) error = %v, want %v", err, xctx.ErrNilContext)
		}
	})
}

func TestTenantFields(t *testing.T) {
	testCases := []struct {
		name      string
		setter    func(context.Context, string) (context.Context, error)
		getter    func(context.Context) string
		fieldName string
		testValue string
	}{
		{"TenantID", xctx.WithTenantID, xctx.TenantID, "TenantID", "tenant-123"},
		{"TenantName", xctx.WithTenantName, xctx.TenantName, "TenantName", "TestCompany"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("空context返回空字符串", func(t *testing.T) {
				if got := tc.getter(context.Background()); got != "" {
					t.Errorf("%s(empty) = %q, want empty", tc.fieldName, got)
				}
			})

			t.Run("正常注入和提取", func(t *testing.T) {
				ctx, err := tc.setter(context.Background(), tc.testValue)
				if err != nil {
					t.Fatalf("%s() error = %v", tc.fieldName, err)
				}
				if got := tc.getter(ctx); got != tc.testValue {
					t.Errorf("%s() = %q, want %q", tc.fieldName, got, tc.testValue)
				}
			})

			t.Run("nil context返回空字符串", func(t *testing.T) {
				var nilCtx context.Context
				if got := tc.getter(nilCtx); got != "" {
					t.Errorf("%s(nil) = %q, want empty", tc.fieldName, got)
				}
			})

			t.Run("nil context注入返回ErrNilContext", func(t *testing.T) {
				var nilCtx context.Context
				_, err := tc.setter(nilCtx, tc.testValue)
				if !errors.Is(err, xctx.ErrNilContext) {
					t.Errorf("With%s(nil) error = %v, want %v", tc.fieldName, err, xctx.ErrNilContext)
				}
			})
		})
	}
}

// =============================================================================
// Identity 结构体测试
// =============================================================================

func TestGetIdentity(t *testing.T) {
	t.Run("空context返回空结构体", func(t *testing.T) {
		id := xctx.GetIdentity(context.Background())
		if id.PlatformID != "" || id.TenantID != "" || id.TenantName != "" {
			t.Errorf("GetIdentity(empty) = %+v, want empty fields", id)
		}
	})

	t.Run("正常获取", func(t *testing.T) {
		ctx, _ := xctx.WithPlatformID(context.Background(), "p1")
		ctx, _ = xctx.WithTenantID(ctx, "t1")
		ctx, _ = xctx.WithTenantName(ctx, "n1")

		id := xctx.GetIdentity(ctx)
		if id.PlatformID != "p1" {
			t.Errorf("PlatformID = %q, want %q", id.PlatformID, "p1")
		}
		if id.TenantID != "t1" {
			t.Errorf("TenantID = %q, want %q", id.TenantID, "t1")
		}
		if id.TenantName != "n1" {
			t.Errorf("TenantName = %q, want %q", id.TenantName, "n1")
		}
	})

	t.Run("部分字段", func(t *testing.T) {
		ctx, _ := xctx.WithPlatformID(context.Background(), "p1")
		id := xctx.GetIdentity(ctx)
		if id.PlatformID != "p1" {
			t.Errorf("PlatformID = %q, want %q", id.PlatformID, "p1")
		}
		if id.TenantID != "" {
			t.Errorf("TenantID = %q, want empty", id.TenantID)
		}
	})
}

func TestIdentity_Validate(t *testing.T) {
	t.Run("全部存在", func(t *testing.T) {
		id := xctx.Identity{PlatformID: "p1", TenantID: "t1", TenantName: "n1"}
		if err := id.Validate(); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("缺少PlatformID", func(t *testing.T) {
		id := xctx.Identity{TenantID: "t1", TenantName: "n1"}
		if err := id.Validate(); !errors.Is(err, xctx.ErrMissingPlatformID) {
			t.Errorf("Validate() error = %v, want %v", err, xctx.ErrMissingPlatformID)
		}
	})

	t.Run("缺少TenantID", func(t *testing.T) {
		id := xctx.Identity{PlatformID: "p1", TenantName: "n1"}
		if err := id.Validate(); !errors.Is(err, xctx.ErrMissingTenantID) {
			t.Errorf("Validate() error = %v, want %v", err, xctx.ErrMissingTenantID)
		}
	})

	t.Run("缺少TenantName", func(t *testing.T) {
		id := xctx.Identity{PlatformID: "p1", TenantID: "t1"}
		if err := id.Validate(); !errors.Is(err, xctx.ErrMissingTenantName) {
			t.Errorf("Validate() error = %v, want %v", err, xctx.ErrMissingTenantName)
		}
	})
}

func TestIdentity_IsComplete(t *testing.T) {
	tests := []struct {
		name string
		id   xctx.Identity
		want bool
	}{
		{"全部存在", xctx.Identity{PlatformID: "p1", TenantID: "t1", TenantName: "n1"}, true},
		{"全部为空", xctx.Identity{}, false},
		{"缺少一个", xctx.Identity{PlatformID: "p1", TenantID: "t1"}, false},
		{"只有一个", xctx.Identity{PlatformID: "p1"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.IsComplete(); got != tt.want {
				t.Errorf("IsComplete() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// Require 函数测试（强制获取模式）
// =============================================================================

func TestRequireFunctions(t *testing.T) {
	tests := []struct {
		name      string
		testValue string
		wantErr   error
		setter    func(context.Context, string) (context.Context, error)
		require   func(context.Context) (string, error)
	}{
		{
			name:      "PlatformID",
			testValue: "platform-123",
			wantErr:   xctx.ErrMissingPlatformID,
			setter:    xctx.WithPlatformID,
			require:   xctx.RequirePlatformID,
		},
		{
			name:      "TenantID",
			testValue: "tenant-456",
			wantErr:   xctx.ErrMissingTenantID,
			setter:    xctx.WithTenantID,
			require:   xctx.RequireTenantID,
		},
		{
			name:      "TenantName",
			testValue: "TestCompany",
			wantErr:   xctx.ErrMissingTenantName,
			setter:    xctx.WithTenantName,
			require:   xctx.RequireTenantName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("存在则返回", func(t *testing.T) {
				ctx, err := tt.setter(context.Background(), tt.testValue)
				if err != nil {
					t.Fatalf("setter() error = %v", err)
				}
				got, err := tt.require(ctx)
				if err != nil {
					t.Errorf("Require%s() error = %v", tt.name, err)
				}
				if got != tt.testValue {
					t.Errorf("Require%s() = %q, want %q", tt.name, got, tt.testValue)
				}
			})

			t.Run("不存在则返回错误", func(t *testing.T) {
				_, err := tt.require(context.Background())
				if err == nil {
					t.Errorf("Require%s() should return error for empty context", tt.name)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
			})
		})
	}
}

// =============================================================================
// 示例测试
// =============================================================================

func ExampleGetIdentity() {
	ctx, _ := xctx.WithPlatformID(context.Background(), "platform-001")
	ctx, _ = xctx.WithTenantID(ctx, "tenant-002")
	ctx, _ = xctx.WithTenantName(ctx, "TestCompany")

	id := xctx.GetIdentity(ctx)
	fmt.Println("PlatformID:", id.PlatformID)
	fmt.Println("TenantID:", id.TenantID)
	fmt.Println("TenantName:", id.TenantName)
	fmt.Println("IsComplete:", id.IsComplete())
	// Output:
	// PlatformID: platform-001
	// TenantID: tenant-002
	// TenantName: TestCompany
	// IsComplete: true
}

func ExampleIdentity_Validate() {
	// 验证身份信息
	id := xctx.Identity{PlatformID: "p1", TenantID: "t1"}
	if err := id.Validate(); err != nil {
		fmt.Println("Error:", err)
	}
	// Output:
	// Error: xctx: missing tenant_name
}

// =============================================================================
// WithIdentity 批量注入测试
// =============================================================================

func TestWithIdentity(t *testing.T) {
	t.Run("全部字段非空", func(t *testing.T) {
		id := xctx.Identity{
			PlatformID: "platform-001",
			TenantID:   "tenant-002",
			TenantName: "TestCompany",
		}
		ctx, err := xctx.WithIdentity(context.Background(), id)
		if err != nil {
			t.Fatalf("WithIdentity() error = %v", err)
		}

		got := xctx.GetIdentity(ctx)
		if got.PlatformID != id.PlatformID {
			t.Errorf("PlatformID = %q, want %q", got.PlatformID, id.PlatformID)
		}
		if got.TenantID != id.TenantID {
			t.Errorf("TenantID = %q, want %q", got.TenantID, id.TenantID)
		}
		if got.TenantName != id.TenantName {
			t.Errorf("TenantName = %q, want %q", got.TenantName, id.TenantName)
		}
	})

	t.Run("部分字段为空", func(t *testing.T) {
		id := xctx.Identity{
			PlatformID: "platform-001",
			// TenantID 和 TenantName 为空
		}
		ctx, err := xctx.WithIdentity(context.Background(), id)
		if err != nil {
			t.Fatalf("WithIdentity() error = %v", err)
		}

		got := xctx.GetIdentity(ctx)
		if got.PlatformID != id.PlatformID {
			t.Errorf("PlatformID = %q, want %q", got.PlatformID, id.PlatformID)
		}
		// 空字段应被跳过，保持为空
		if got.TenantID != "" {
			t.Errorf("TenantID = %q, want empty", got.TenantID)
		}
		if got.TenantName != "" {
			t.Errorf("TenantName = %q, want empty", got.TenantName)
		}
	})

	t.Run("全部字段为空", func(t *testing.T) {
		id := xctx.Identity{}
		ctx, err := xctx.WithIdentity(context.Background(), id)
		if err != nil {
			t.Fatalf("WithIdentity() error = %v", err)
		}

		got := xctx.GetIdentity(ctx)
		if got.PlatformID != "" || got.TenantID != "" || got.TenantName != "" {
			t.Errorf("WithIdentity(empty) should not inject any fields, got %+v", got)
		}
	})

	t.Run("nil context返回ErrNilContext", func(t *testing.T) {
		var nilCtx context.Context
		id := xctx.Identity{PlatformID: "p1"}
		_, err := xctx.WithIdentity(nilCtx, id)
		if !errors.Is(err, xctx.ErrNilContext) {
			t.Errorf("WithIdentity(nil) error = %v, want %v", err, xctx.ErrNilContext)
		}
	})
}

func ExampleWithIdentity() {
	// 从请求头解析身份信息后批量注入
	id := xctx.Identity{
		PlatformID: "platform-001",
		TenantID:   "tenant-002",
		TenantName: "TestCompany",
	}
	ctx, _ := xctx.WithIdentity(context.Background(), id)

	// 验证注入结果
	got := xctx.GetIdentity(ctx)
	fmt.Println("PlatformID:", got.PlatformID)
	fmt.Println("IsComplete:", got.IsComplete())
	// Output:
	// PlatformID: platform-001
	// IsComplete: true
}
