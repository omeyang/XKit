package xctx_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/omeyang/xkit/pkg/context/xctx"
)

// =============================================================================
// HasParent 操作测试
// =============================================================================

func TestWithHasParent(t *testing.T) {
	t.Run("注入true", func(t *testing.T) {
		ctx, err := xctx.WithHasParent(context.Background(), true)
		if err != nil {
			t.Fatalf("WithHasParent(true) error = %v", err)
		}
		v, ok := xctx.HasParent(ctx)
		if !ok {
			t.Error("HasParent() ok = false, want true")
		}
		if !v {
			t.Error("HasParent() = false, want true")
		}
	})

	t.Run("注入false", func(t *testing.T) {
		ctx, err := xctx.WithHasParent(context.Background(), false)
		if err != nil {
			t.Fatalf("WithHasParent(false) error = %v", err)
		}
		v, ok := xctx.HasParent(ctx)
		if !ok {
			t.Error("HasParent() ok = false, want true")
		}
		if v {
			t.Error("HasParent() = true, want false")
		}
	})

	t.Run("覆盖写入返回新值", func(t *testing.T) {
		ctx, _ := xctx.WithHasParent(context.Background(), true)
		ctx, _ = xctx.WithHasParent(ctx, false)
		v, ok := xctx.HasParent(ctx)
		if !ok {
			t.Error("HasParent() ok = false, want true")
		}
		if v {
			t.Error("HasParent(overwrite) = true, want false")
		}
	})

	t.Run("nil context返回ErrNilContext", func(t *testing.T) {
		var nilCtx context.Context
		_, err := xctx.WithHasParent(nilCtx, true)
		if !errors.Is(err, xctx.ErrNilContext) {
			t.Errorf("WithHasParent(nil) error = %v, want %v", err, xctx.ErrNilContext)
		}
	})
}

func TestHasParent(t *testing.T) {
	t.Run("未设置返回false和ok=false", func(t *testing.T) {
		v, ok := xctx.HasParent(context.Background())
		if ok {
			t.Error("HasParent(empty) ok = true, want false")
		}
		if v {
			t.Error("HasParent(empty) = true, want false")
		}
	})

	t.Run("nil context返回false和ok=false", func(t *testing.T) {
		var nilCtx context.Context
		v, ok := xctx.HasParent(nilCtx)
		if ok {
			t.Error("HasParent(nil) ok = true, want false")
		}
		if v {
			t.Error("HasParent(nil) = true, want false")
		}
	})

	t.Run("区分未设置和设置为false", func(t *testing.T) {
		// 未设置
		_, okEmpty := xctx.HasParent(context.Background())
		if okEmpty {
			t.Error("未设置时 ok 应为 false")
		}

		// 设置为 false
		ctx, _ := xctx.WithHasParent(context.Background(), false)
		v, okSet := xctx.HasParent(ctx)
		if !okSet {
			t.Error("设置为false后 ok 应为 true")
		}
		if v {
			t.Error("设置为false后值应为 false")
		}
	})
}

func TestMustHasParent(t *testing.T) {
	t.Run("未设置返回false", func(t *testing.T) {
		if xctx.MustHasParent(context.Background()) {
			t.Error("MustHasParent(empty) = true, want false")
		}
	})

	t.Run("nil context返回false", func(t *testing.T) {
		var nilCtx context.Context
		if xctx.MustHasParent(nilCtx) {
			t.Error("MustHasParent(nil) = true, want false")
		}
	})

	t.Run("设置为true返回true", func(t *testing.T) {
		ctx, _ := xctx.WithHasParent(context.Background(), true)
		if !xctx.MustHasParent(ctx) {
			t.Error("MustHasParent(true) = false, want true")
		}
	})

	t.Run("设置为false返回false", func(t *testing.T) {
		ctx, _ := xctx.WithHasParent(context.Background(), false)
		if xctx.MustHasParent(ctx) {
			t.Error("MustHasParent(false) = true, want false")
		}
	})
}

func TestRequireHasParent(t *testing.T) {
	t.Run("存在则返回值", func(t *testing.T) {
		ctx, _ := xctx.WithHasParent(context.Background(), true)
		v, err := xctx.RequireHasParent(ctx)
		if err != nil {
			t.Errorf("RequireHasParent() error = %v", err)
		}
		if !v {
			t.Error("RequireHasParent() = false, want true")
		}
	})

	t.Run("设置为false也能正确返回", func(t *testing.T) {
		ctx, _ := xctx.WithHasParent(context.Background(), false)
		v, err := xctx.RequireHasParent(ctx)
		if err != nil {
			t.Errorf("RequireHasParent() error = %v", err)
		}
		if v {
			t.Error("RequireHasParent() = true, want false")
		}
	})

	t.Run("不存在则返回错误", func(t *testing.T) {
		_, err := xctx.RequireHasParent(context.Background())
		if err == nil {
			t.Error("RequireHasParent() should return error for empty context")
		}
		if !errors.Is(err, xctx.ErrMissingHasParent) {
			t.Errorf("error = %v, want %v", err, xctx.ErrMissingHasParent)
		}
	})

	t.Run("nil context返回ErrNilContext", func(t *testing.T) {
		var nilCtx context.Context
		_, err := xctx.RequireHasParent(nilCtx)
		if err == nil {
			t.Error("RequireHasParent(nil) should return error")
		}
		if !errors.Is(err, xctx.ErrNilContext) {
			t.Errorf("error = %v, want %v", err, xctx.ErrNilContext)
		}
	})
}

// =============================================================================
// 示例测试
// =============================================================================

func ExampleHasParent() {
	// 场景：SaaS 多级部署中判断平台层级关系
	ctx, _ := xctx.WithHasParent(context.Background(), true)

	// 方式1：使用 HasParent 区分"未设置"和"设置为false"
	if hasParent, ok := xctx.HasParent(ctx); ok {
		fmt.Println("HasParent is set to:", hasParent)
	} else {
		fmt.Println("HasParent is not set")
	}

	// 方式2：使用 MustHasParent 简化获取
	fmt.Println("MustHasParent:", xctx.MustHasParent(ctx))

	// Output:
	// HasParent is set to: true
	// MustHasParent: true
}

func ExampleRequireHasParent() {
	// 必须明确知道平台层级关系的场景
	ctx, _ := xctx.WithHasParent(context.Background(), false)

	hasParent, err := xctx.RequireHasParent(ctx)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Println("HasParent:", hasParent)
	// Output:
	// HasParent: false
}

func ExampleRequireHasParent_error() {
	// 未设置时返回错误
	_, err := xctx.RequireHasParent(context.Background())
	if err != nil {
		fmt.Println("Error:", err)
	}
	// Output:
	// Error: xctx: missing has_parent
}

// =============================================================================
// UnclassRegionID 操作测试
// =============================================================================

func TestWithUnclassRegionID(t *testing.T) {
	t.Run("注入区域ID", func(t *testing.T) {
		ctx, err := xctx.WithUnclassRegionID(context.Background(), "region-001")
		if err != nil {
			t.Fatalf("WithUnclassRegionID() error = %v", err)
		}
		got := xctx.UnclassRegionID(ctx)
		if got != "region-001" {
			t.Errorf("UnclassRegionID() = %q, want %q", got, "region-001")
		}
	})

	t.Run("注入空字符串", func(t *testing.T) {
		ctx, err := xctx.WithUnclassRegionID(context.Background(), "")
		if err != nil {
			t.Fatalf("WithUnclassRegionID() error = %v", err)
		}
		got := xctx.UnclassRegionID(ctx)
		if got != "" {
			t.Errorf("UnclassRegionID() = %q, want empty", got)
		}
	})

	t.Run("覆盖写入返回新值", func(t *testing.T) {
		ctx, _ := xctx.WithUnclassRegionID(context.Background(), "region-001")
		ctx, _ = xctx.WithUnclassRegionID(ctx, "region-002")
		got := xctx.UnclassRegionID(ctx)
		if got != "region-002" {
			t.Errorf("UnclassRegionID() = %q, want %q", got, "region-002")
		}
	})

	t.Run("nil context返回ErrNilContext", func(t *testing.T) {
		var nilCtx context.Context
		_, err := xctx.WithUnclassRegionID(nilCtx, "region-001")
		if !errors.Is(err, xctx.ErrNilContext) {
			t.Errorf("WithUnclassRegionID(nil) error = %v, want %v", err, xctx.ErrNilContext)
		}
	})
}

func TestUnclassRegionID(t *testing.T) {
	t.Run("未设置返回空字符串", func(t *testing.T) {
		got := xctx.UnclassRegionID(context.Background())
		if got != "" {
			t.Errorf("UnclassRegionID(empty) = %q, want empty", got)
		}
	})

	t.Run("nil context返回空字符串", func(t *testing.T) {
		var nilCtx context.Context
		got := xctx.UnclassRegionID(nilCtx)
		if got != "" {
			t.Errorf("UnclassRegionID(nil) = %q, want empty", got)
		}
	})
}

// =============================================================================
// Platform 结构体操作测试
// =============================================================================

func TestGetPlatform(t *testing.T) {
	t.Run("空context返回零值", func(t *testing.T) {
		p := xctx.GetPlatform(context.Background())
		if p.HasParent {
			t.Error("GetPlatform(empty).HasParent = true, want false")
		}
		if p.UnclassRegionID != "" {
			t.Errorf("GetPlatform(empty).UnclassRegionID = %q, want empty", p.UnclassRegionID)
		}
	})

	t.Run("nil context返回零值", func(t *testing.T) {
		var nilCtx context.Context
		p := xctx.GetPlatform(nilCtx)
		if p.HasParent {
			t.Error("GetPlatform(nil).HasParent = true, want false")
		}
		if p.UnclassRegionID != "" {
			t.Errorf("GetPlatform(nil).UnclassRegionID = %q, want empty", p.UnclassRegionID)
		}
	})

	t.Run("获取完整平台信息", func(t *testing.T) {
		ctx, _ := xctx.WithHasParent(context.Background(), true)
		ctx, _ = xctx.WithUnclassRegionID(ctx, "region-001")

		p := xctx.GetPlatform(ctx)
		if !p.HasParent {
			t.Error("GetPlatform().HasParent = false, want true")
		}
		if p.UnclassRegionID != "region-001" {
			t.Errorf("GetPlatform().UnclassRegionID = %q, want %q", p.UnclassRegionID, "region-001")
		}
	})

	t.Run("部分设置", func(t *testing.T) {
		// 只设置 HasParent
		ctx, _ := xctx.WithHasParent(context.Background(), true)
		p := xctx.GetPlatform(ctx)
		if !p.HasParent {
			t.Error("GetPlatform().HasParent = false, want true")
		}
		if p.UnclassRegionID != "" {
			t.Errorf("GetPlatform().UnclassRegionID = %q, want empty", p.UnclassRegionID)
		}
	})
}

func TestWithPlatform(t *testing.T) {
	t.Run("注入完整Platform", func(t *testing.T) {
		p := xctx.Platform{
			HasParent:       true,
			UnclassRegionID: "region-001",
		}
		ctx, err := xctx.WithPlatform(context.Background(), p)
		if err != nil {
			t.Fatalf("WithPlatform() error = %v", err)
		}

		got := xctx.GetPlatform(ctx)
		if got.HasParent != p.HasParent {
			t.Errorf("HasParent = %v, want %v", got.HasParent, p.HasParent)
		}
		if got.UnclassRegionID != p.UnclassRegionID {
			t.Errorf("UnclassRegionID = %q, want %q", got.UnclassRegionID, p.UnclassRegionID)
		}
	})

	t.Run("HasParent为false也会注入", func(t *testing.T) {
		p := xctx.Platform{
			HasParent:       false,
			UnclassRegionID: "region-001",
		}
		ctx, err := xctx.WithPlatform(context.Background(), p)
		if err != nil {
			t.Fatalf("WithPlatform() error = %v", err)
		}

		// 验证 HasParent 被正确设置为 false
		v, ok := xctx.HasParent(ctx)
		if !ok {
			t.Error("HasParent should be set (ok = true)")
		}
		if v {
			t.Error("HasParent = true, want false")
		}
	})

	t.Run("空UnclassRegionID不注入", func(t *testing.T) {
		// 先设置一个值
		ctx, _ := xctx.WithUnclassRegionID(context.Background(), "existing")

		// 用空的 Platform 覆盖
		p := xctx.Platform{
			HasParent:       true,
			UnclassRegionID: "", // 空字符串不注入
		}
		ctx, err := xctx.WithPlatform(ctx, p)
		if err != nil {
			t.Fatalf("WithPlatform() error = %v", err)
		}

		// UnclassRegionID 应该保持原值（因为空字符串不注入）
		got := xctx.UnclassRegionID(ctx)
		if got != "existing" {
			t.Errorf("UnclassRegionID = %q, want %q (should keep existing)", got, "existing")
		}
	})

	t.Run("nil context返回ErrNilContext", func(t *testing.T) {
		var nilCtx context.Context
		p := xctx.Platform{HasParent: true}
		_, err := xctx.WithPlatform(nilCtx, p)
		if !errors.Is(err, xctx.ErrNilContext) {
			t.Errorf("WithPlatform(nil) error = %v, want %v", err, xctx.ErrNilContext)
		}
	})
}

// =============================================================================
// Platform 常量测试
// =============================================================================

func TestPlatformKeyConstants(t *testing.T) {
	if xctx.KeyHasParent != "has_parent" {
		t.Errorf("KeyHasParent = %q, want %q", xctx.KeyHasParent, "has_parent")
	}
	if xctx.KeyUnclassRegionID != "unclass_region_id" {
		t.Errorf("KeyUnclassRegionID = %q, want %q", xctx.KeyUnclassRegionID, "unclass_region_id")
	}
}

// =============================================================================
// Platform 示例测试
// =============================================================================

func ExampleWithUnclassRegionID() {
	ctx, _ := xctx.WithUnclassRegionID(context.Background(), "region-001")
	fmt.Println("UnclassRegionID:", xctx.UnclassRegionID(ctx))
	// Output:
	// UnclassRegionID: region-001
}

func ExampleGetPlatform() {
	ctx, _ := xctx.WithHasParent(context.Background(), true)
	ctx, _ = xctx.WithUnclassRegionID(ctx, "region-001")

	p := xctx.GetPlatform(ctx)
	fmt.Println("HasParent:", p.HasParent)
	fmt.Println("UnclassRegionID:", p.UnclassRegionID)
	// Output:
	// HasParent: true
	// UnclassRegionID: region-001
}

func ExampleWithPlatform() {
	p := xctx.Platform{
		HasParent:       true,
		UnclassRegionID: "region-001",
	}
	ctx, _ := xctx.WithPlatform(context.Background(), p)

	got := xctx.GetPlatform(ctx)
	fmt.Println("HasParent:", got.HasParent)
	fmt.Println("UnclassRegionID:", got.UnclassRegionID)
	// Output:
	// HasParent: true
	// UnclassRegionID: region-001
}
